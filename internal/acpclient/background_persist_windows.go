//go:build windows

package acpclient

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

const (
	backgroundMoveFileFlags      = windows.MOVEFILE_REPLACE_EXISTING | windows.MOVEFILE_WRITE_THROUGH
	backgroundPrivateInheritance = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
)

func backgroundWriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := backgroundEnsurePrivateDir(dir); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) //nolint:errcheck
	if err := backgroundSetPrivateACL(tmpPath); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := backgroundReplaceFile(tmpPath, path); err != nil {
		return err
	}
	if err := backgroundSetPrivateACL(path); err != nil {
		return newBackgroundPostCommitError(err)
	}
	return nil
}

func backgroundOpenPrivateAppend(path string) (*os.File, error) {
	if err := backgroundEnsurePrivateDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	if err := backgroundSetPrivateACL(path); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func backgroundEnsurePrivateDir(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("private directory path %s is not a directory", path)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	if parent != path {
		if err := backgroundEnsurePrivateDir(parent); err != nil {
			return err
		}
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			info, statErr := os.Stat(path)
			if statErr == nil && info.IsDir() {
				return nil
			}
		}
		return err
	}
	return backgroundSetPrivateACL(path)
}

func backgroundEnsureOwnedPrivateDir(path string) error {
	if err := backgroundValidateOwnedPrivateDir(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := backgroundEnsurePrivateDir(path); err != nil {
		return err
	}
	if err := backgroundValidateOwnedPrivateDir(path); err != nil {
		return err
	}
	return backgroundSetPrivateACL(path)
}

func backgroundValidateOwnedPrivateDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return storeLockUnsafePathError(path, errors.New("dedicated lock path is not a physical directory"))
	}
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attributes, err := windows.GetFileAttributes(pathPtr)
	if err != nil {
		return err
	}
	if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return storeLockUnsafePathError(path, errors.New("dedicated lock path is a reparse point"))
	}
	return nil
}

func backgroundReplaceFile(oldPath, newPath string) error {
	oldPtr, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	newPtr, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(oldPtr, newPtr, backgroundMoveFileFlags)
}

func backgroundRemoveFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func backgroundSyncParentDir(string) error {
	return nil
}

func backgroundSetPrivateACL(path string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       backgroundPrivateInheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid),
		},
	}}, nil)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	)
}
