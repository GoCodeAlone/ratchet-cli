//go:build !windows

package hooks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

var hookAuditEffectiveUID = os.Geteuid

func rotateHookAuditPath(source, destination string) error {
	return os.Rename(source, destination)
}

func openHookAuditFile(path string, create bool) (*os.File, bool, error) {
	parent := filepath.Dir(path)
	parentInfo, err := os.Lstat(parent)
	if errors.Is(err, os.ErrNotExist) && create {
		if err := ensureHookAuditPrivateDir(parent); err != nil {
			return nil, false, err
		}
		parentInfo, err = os.Lstat(parent)
	}
	if errors.Is(err, os.ErrNotExist) && !create {
		return nil, false, os.ErrNotExist
	}
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
		f, openErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR|os.O_APPEND, 0o600)
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
	f, err := os.OpenFile(path, flags, 0)
	if err != nil {
		return nil, false, fmt.Errorf("open managed hook audit: %w", err)
	}
	if err := validateHookAuditIdentity(path, f); err != nil {
		_ = f.Close()
		return nil, false, err
	}
	return f, false, nil
}

func ensureHookAuditPrivateDir(path string) error {
	missing := make([]string, 0, 2)
	current := path
	for {
		if _, err := os.Lstat(current); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect managed hook audit namespace: %w", err)
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			return errors.New("managed hook audit namespace has no existing ancestor")
		}
		current = parent
	}
	for i := len(missing) - 1; i >= 0; i-- {
		created := false
		if err := os.Mkdir(missing[i], 0o700); err == nil {
			created = true
		} else if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create managed hook audit namespace: %w", err)
		}
		if created {
			if err := os.Chmod(missing[i], 0o700); err != nil {
				return fmt.Errorf("secure managed hook audit namespace: %w", err)
			}
		}
		info, err := os.Lstat(missing[i])
		if err != nil {
			return fmt.Errorf("inspect managed hook audit namespace: %w", err)
		}
		if err := validateHookAuditParent(missing[i], info); err != nil {
			return err
		}
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
