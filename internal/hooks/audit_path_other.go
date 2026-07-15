//go:build !windows

package hooks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

var (
	hookAuditEffectiveUID = os.Geteuid
	hookAuditOpenFile     = os.OpenFile
)

func rotateHookAuditPath(source, destination string) error {
	return os.Rename(source, destination)
}

func openHookAuditFile(path string, create bool) (*os.File, bool, error) {
	ready, err := prepareHookAuditPrivateNamespace(path, create)
	if err != nil {
		return nil, false, err
	}
	if !ready {
		return nil, false, os.ErrNotExist
	}
	parent := filepath.Dir(path)
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return nil, false, fmt.Errorf("inspect managed hook audit namespace: %w", err)
	}
	if err := validateHookAuditParent(parent, parentInfo); err != nil {
		return nil, false, err
	}

	before, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if !create {
			return nil, false, os.ErrNotExist
		}
		f, openErr := hookAuditOpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR|os.O_APPEND, 0o600)
		if openErr != nil && !errors.Is(openErr, os.ErrExist) {
			return nil, false, fmt.Errorf("create managed hook audit: %w", openErr)
		}
		if openErr == nil {
			if chmodErr := f.Chmod(0o600); chmodErr != nil {
				_ = f.Close()
				return nil, false, fmt.Errorf("secure managed hook audit: %w", chmodErr)
			}
			if identityErr := validateHookAuditIdentity(path, f); identityErr != nil {
				_ = f.Close()
				return nil, false, identityErr
			}
			return f, true, nil
		}
		before, err = os.Lstat(path)
	}
	if err != nil {
		return nil, false, fmt.Errorf("inspect managed hook audit: %w", err)
	}
	if err := validateHookAuditFile(path, before); err != nil {
		return nil, false, err
	}
	flags := os.O_RDONLY
	if create {
		flags = os.O_RDWR | os.O_APPEND
	}
	f, err := hookAuditOpenFile(path, flags, 0)
	if err != nil {
		return nil, false, fmt.Errorf("open managed hook audit: %w", err)
	}
	if err := validateHookAuditIdentity(path, f); err != nil {
		_ = f.Close()
		return nil, false, err
	}
	return f, false, nil
}

func prepareHookAuditPrivateNamespace(path string, create bool) (bool, error) {
	_, directories, err := hookAuditNamespace(path)
	if err != nil {
		return false, err
	}
	for _, directory := range directories {
		_, statErr := os.Lstat(directory)
		if errors.Is(statErr, os.ErrNotExist) && !create {
			return false, nil
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return false, fmt.Errorf("inspect managed hook audit namespace: %w", statErr)
		}
		created := false
		if errors.Is(statErr, os.ErrNotExist) {
			if err := os.Mkdir(directory, 0o700); err == nil {
				created = true
			} else if !errors.Is(err, os.ErrExist) {
				return false, fmt.Errorf("create managed hook audit namespace: %w", err)
			}
		}
		if created {
			if err := os.Chmod(directory, 0o700); err != nil {
				return false, fmt.Errorf("secure managed hook audit namespace: %w", err)
			}
		}
		info, err := os.Lstat(directory)
		if err != nil {
			return false, fmt.Errorf("inspect managed hook audit namespace: %w", err)
		}
		if err := validateHookAuditParent(directory, info); err != nil {
			return false, err
		}
	}
	return true, nil
}

func acquireHookAuditTrustedAnchor(path string) (func() error, error) {
	anchor, _, err := hookAuditNamespace(path)
	if err != nil {
		return nil, err
	}
	if err := validateHookAuditTrustedAncestry(anchor); err != nil {
		return nil, err
	}
	file, err := os.Open(anchor)
	if err != nil {
		return nil, fmt.Errorf("open managed hook audit trusted anchor: %w", err)
	}
	if err := validateHookAuditTrustedAncestry(anchor); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if err := validateHookAuditAnchorIdentity(anchor, file); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return func() error {
		return errors.Join(
			validateHookAuditTrustedAncestry(anchor),
			validateHookAuditAnchorIdentity(anchor, file),
			file.Close(),
		)
	}, nil
}

func validateHookAuditTrustedAncestry(anchor string) error {
	anchorInfo, err := os.Lstat(anchor)
	if err != nil {
		return fmt.Errorf("inspect managed hook audit trusted anchor: %w", err)
	}
	if anchorInfo.Mode()&os.ModeSymlink != 0 || !anchorInfo.IsDir() {
		return errors.New("managed hook audit trusted anchor must be a regular directory")
	}
	canonical, err := filepath.EvalSymlinks(anchor)
	if err != nil {
		return fmt.Errorf("resolve managed hook audit trusted anchor: %w", err)
	}
	if err := validateHookAuditTrustedDirectoryChain(canonical, false); err != nil {
		return err
	}
	for current := anchor; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect managed hook audit trusted anchor ancestry: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if err := validateHookAuditTrustedOwner(current, info); err != nil {
				return err
			}
		} else if err := validateHookAuditTrustedDirectory(current, info, current == anchor); err != nil {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return nil
}

func validateHookAuditTrustedDirectoryChain(path string, anchor bool) error {
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect managed hook audit trusted anchor ancestry: %w", err)
		}
		if err := validateHookAuditTrustedDirectory(current, info, anchor && current == path); err != nil {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
	}
}

func validateHookAuditTrustedDirectory(path string, info os.FileInfo, anchor bool) error {
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("managed hook audit trusted anchor ancestry must contain only regular directories")
	}
	if err := validateHookAuditTrustedOwner(path, info); err != nil {
		return err
	}
	if err := validateHookAuditPlatformACL(path); err != nil {
		return err
	}
	if info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
		location := "ancestry"
		if anchor {
			location = "anchor"
		}
		return fmt.Errorf("managed hook audit trusted %s permits untrusted mutation: %s", location, path)
	}
	return nil
}

func validateHookAuditTrustedOwner(path string, info os.FileInfo) error {
	uid, ok := hookAuditMetadataUint(info, "Uid")
	if !ok {
		return fmt.Errorf("managed hook audit trusted anchor owner cannot be verified: %s", path)
	}
	currentUID := hookAuditEffectiveUID()
	if currentUID < 0 || uid != 0 && uid != uint64(currentUID) {
		return fmt.Errorf("managed hook audit trusted anchor has an untrusted owner: %s", path)
	}
	return nil
}

func validateHookAuditAnchorIdentity(path string, file *os.File) error {
	opened, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened managed hook audit trusted anchor: %w", err)
	}
	current, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect managed hook audit trusted anchor identity: %w", err)
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.IsDir() || !os.SameFile(opened, current) {
		return errors.New("managed hook audit trusted anchor changed during transaction")
	}
	return nil
}

func validateHookAuditParent(path string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("managed hook audit namespace must be a regular directory")
	}
	if info.Mode().Perm() != 0o700 {
		return fmt.Errorf("managed hook audit namespace must have mode 0700, got %04o", info.Mode().Perm())
	}
	return validateHookAuditOwner(path, info)
}

func validateHookAuditFile(path string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("managed hook audit must be a regular non-symlink file")
	}
	if info.Mode().Perm() != 0o600 {
		return fmt.Errorf("managed hook audit must have mode 0600, got %04o", info.Mode().Perm())
	}
	if links, ok := hookAuditMetadataUint(info, "Nlink"); ok && links != 1 {
		return errors.New("managed hook audit must have exactly one link")
	}
	return validateHookAuditOwner(path, info)
}

func validateHookAuditIdentity(path string, f *os.File) error {
	opened, err := f.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened managed hook audit: %w", err)
	}
	if err := validateHookAuditFile(path, opened); err != nil {
		return err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect managed hook audit identity: %w", err)
	}
	if err := validateHookAuditFile(path, current); err != nil {
		return err
	}
	if !os.SameFile(opened, current) {
		return errors.New("managed hook audit target changed during open")
	}
	return nil
}

func validateHookAuditOwner(path string, info os.FileInfo) error {
	uid, ok := hookAuditMetadataUint(info, "Uid")
	if !ok {
		return fmt.Errorf("managed hook audit owner cannot be verified: %s", path)
	}
	currentUID := hookAuditEffectiveUID()
	if currentUID < 0 {
		return fmt.Errorf("managed hook audit owner cannot be verified: %s", path)
	}
	if uid != uint64(currentUID) {
		return fmt.Errorf("managed hook audit is not owned by the current user: %s", path)
	}
	return nil
}

func hookAuditMetadataUint(info os.FileInfo, fieldName string) (uint64, bool) {
	value := reflect.ValueOf(info.Sys())
	if !value.IsValid() {
		return 0, false
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return 0, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return 0, false
	}
	field := value.FieldByName(fieldName)
	if !field.IsValid() {
		return 0, false
	}
	switch field.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return field.Uint(), true
	default:
		return 0, false
	}
}

func syncHookAuditDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close() //nolint:errcheck
	return dir.Sync()
}
