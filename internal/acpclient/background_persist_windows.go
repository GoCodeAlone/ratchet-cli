//go:build windows

package acpclient

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	backgroundMoveFileFlags      = windows.MOVEFILE_REPLACE_EXISTING | windows.MOVEFILE_WRITE_THROUGH
	backgroundPrivateInheritance = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
)

type backgroundWindowsFileIDInfo struct {
	VolumeSerialNumber uint64
	FileID             [16]byte
}

type backgroundWindowsAuditTransaction struct {
	path           string
	parentPath     string
	parent         *os.File
	parentIdentity backgroundWindowsFileIDInfo
	lock           *os.File
	lockOverlapped *windows.Overlapped
	file           *os.File
}

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
	f, _, err := backgroundOpenWindowsAuditFile(path, true)
	if err != nil {
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func backgroundOpenAuditTransaction(path string, create bool) (backgroundAuditTransaction, error) {
	parentPath := filepath.Dir(path)
	if err := backgroundEnsurePrivateDir(parentPath); err != nil {
		return nil, err
	}
	parent, parentIdentity, err := backgroundOpenWindowsAuditParent(parentPath)
	if err != nil {
		return nil, err
	}
	lockPath := filepath.Join(parentPath, backgroundAuditLockName)
	lock, _, err := backgroundOpenWindowsAuditFile(lockPath, true)
	if err != nil {
		return nil, errors.Join(err, parent.Close())
	}
	lockOverlapped := new(windows.Overlapped)
	if err := windows.LockFileEx(windows.Handle(lock.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, lockOverlapped); err != nil {
		return nil, errors.Join(err, lock.Close(), parent.Close())
	}
	lockIdentity, err := backgroundWindowsHandleIdentity(windows.Handle(lock.Fd()))
	if err == nil {
		err = backgroundValidateWindowsFileIdentity(lockPath, lockIdentity)
	}
	if err == nil {
		err = backgroundValidateWindowsParentIdentity(parentPath, parentIdentity)
	}
	if err != nil {
		return nil, errors.Join(err, backgroundReleaseWindowsAuditLock(lock, lockOverlapped), parent.Close())
	}
	f, _, err := backgroundOpenWindowsAuditFile(path, create)
	if errors.Is(err, os.ErrNotExist) && !create {
		return &backgroundWindowsAuditTransaction{
			path: path, parentPath: parentPath, parent: parent,
			parentIdentity: parentIdentity, lock: lock, lockOverlapped: lockOverlapped,
		}, nil
	}
	if err != nil {
		return nil, errors.Join(err, backgroundReleaseWindowsAuditLock(lock, lockOverlapped), parent.Close())
	}
	return &backgroundWindowsAuditTransaction{
		path: path, parentPath: parentPath, parent: parent,
		parentIdentity: parentIdentity, lock: lock, lockOverlapped: lockOverlapped, file: f,
	}, nil
}

func backgroundOpenWindowsAuditParent(path string) (*os.File, backgroundWindowsFileIDInfo, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, backgroundWindowsFileIDInfo{}, err
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		if backgroundWindowsPathNotExist(err) {
			return nil, backgroundWindowsFileIDInfo{}, os.ErrNotExist
		}
		return nil, backgroundWindowsFileIDInfo{}, storeLockUnsafePathError(path, err)
	}
	f := os.NewFile(uintptr(handle), path)
	if f == nil {
		_ = windows.CloseHandle(handle)
		return nil, backgroundWindowsFileIDInfo{}, errors.New("create audit parent handle")
	}
	if err := backgroundValidateWindowsPrivateHandle(handle, path, true); err != nil {
		_ = f.Close()
		return nil, backgroundWindowsFileIDInfo{}, err
	}
	identity, err := backgroundWindowsHandleIdentity(handle)
	if err != nil {
		_ = f.Close()
		return nil, backgroundWindowsFileIDInfo{}, err
	}
	return f, identity, nil
}

func backgroundOpenWindowsAuditFile(path string, create bool) (*os.File, bool, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, false, err
	}
	const existingAccess = windows.GENERIC_READ | windows.GENERIC_WRITE | windows.READ_CONTROL
	handle, err := windows.CreateFile(
		pathPtr,
		existingAccess,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	created := false
	if err != nil && backgroundWindowsPathNotExist(err) && create {
		handle, err = windows.CreateFile(
			pathPtr,
			existingAccess|windows.WRITE_DAC|windows.WRITE_OWNER,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
			nil,
			windows.CREATE_NEW,
			windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
			0,
		)
		created = err == nil
		if errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			handle, err = windows.CreateFile(
				pathPtr,
				existingAccess,
				windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
				nil,
				windows.OPEN_EXISTING,
				windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
				0,
			)
			created = false
		}
	}
	if err != nil {
		if backgroundWindowsPathNotExist(err) && !create {
			return nil, false, os.ErrNotExist
		}
		return nil, false, storeLockUnsafePathError(path, err)
	}
	f := os.NewFile(uintptr(handle), path)
	if f == nil {
		_ = windows.CloseHandle(handle)
		return nil, false, errors.New("create audit file handle")
	}
	if created {
		if err := backgroundSetPrivateHandleACL(handle); err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return nil, false, err
		}
	}
	if err := backgroundValidateWindowsPrivateHandle(handle, path, false); err != nil {
		_ = f.Close()
		if created {
			_ = os.Remove(path)
		}
		return nil, false, err
	}
	return f, created, nil
}

func backgroundWindowsPathNotExist(err error) bool {
	return errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)
}

func backgroundValidateWindowsPrivateHandle(handle windows.Handle, path string, directory bool) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return err
	}
	isDirectory := info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	if isDirectory != directory || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return storeLockUnsafePathError(path, errors.New("target type or reparse attributes are unsafe"))
	}
	if !directory && info.NumberOfLinks != 1 {
		return storeLockUnsafePathError(path, fmt.Errorf("target has %d links, want one", info.NumberOfLinks))
	}
	descriptor, err := windows.GetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return storeLockUnsafePathError(path, err)
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return storeLockUnsafePathError(path, err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	if owner == nil || !owner.Equals(user.User.Sid) {
		return storeLockUnsafePathError(path, errors.New("target owner is not the current user"))
	}
	control, _, err := descriptor.Control()
	if err != nil {
		return storeLockUnsafePathError(path, err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return storeLockUnsafePathError(path, errors.New("target DACL is not protected"))
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return storeLockUnsafePathError(path, err)
	}
	if dacl == nil || dacl.AceCount == 0 {
		return storeLockUnsafePathError(path, errors.New("target DACL is not owner-only"))
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return storeLockUnsafePathError(path, err)
		}
		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE ||
			ace.Mask != windows.GENERIC_ALL ||
			!aceSID.Equals(user.User.Sid) {
			return storeLockUnsafePathError(path, errors.New("target DACL is not owner-only full control"))
		}
	}
	return nil
}

func backgroundWindowsHandleIdentity(handle windows.Handle) (backgroundWindowsFileIDInfo, error) {
	var identity backgroundWindowsFileIDInfo
	err := windows.GetFileInformationByHandleEx(
		handle,
		windows.FileIdInfo,
		(*byte)(unsafe.Pointer(&identity)),
		uint32(unsafe.Sizeof(identity)),
	)
	return identity, err
}

func backgroundValidateWindowsParentIdentity(path string, want backgroundWindowsFileIDInfo) error {
	parent, got, err := backgroundOpenWindowsAuditParent(path)
	if err != nil {
		return err
	}
	defer parent.Close() //nolint:errcheck
	if got != want {
		return storeLockUnsafePathError(path, errors.New("audit parent identity changed"))
	}
	return nil
}

func backgroundValidateWindowsFileIdentity(path string, want backgroundWindowsFileIDInfo) error {
	current, _, err := backgroundOpenWindowsAuditFile(path, false)
	if err != nil {
		return err
	}
	defer current.Close() //nolint:errcheck
	got, err := backgroundWindowsHandleIdentity(windows.Handle(current.Fd()))
	if err != nil {
		return err
	}
	if got != want {
		return storeLockUnsafePathError(path, errors.New("target identity changed"))
	}
	return nil
}

func backgroundReleaseWindowsAuditLock(lock *os.File, overlapped *windows.Overlapped) error {
	if lock == nil {
		return nil
	}
	return errors.Join(
		windows.UnlockFileEx(windows.Handle(lock.Fd()), 0, 1, 0, overlapped),
		lock.Close(),
	)
}

func (t *backgroundWindowsAuditTransaction) File() *os.File { return t.file }

func (t *backgroundWindowsAuditTransaction) ValidateForMutation() error {
	if t.file == nil {
		return storeLockUnsafePathError(t.path, os.ErrNotExist)
	}
	if err := backgroundValidateWindowsPrivateHandle(windows.Handle(t.file.Fd()), t.path, false); err != nil {
		return err
	}
	if err := backgroundValidateWindowsParentIdentity(t.parentPath, t.parentIdentity); err != nil {
		return err
	}
	want, err := backgroundWindowsHandleIdentity(windows.Handle(t.file.Fd()))
	if err != nil {
		return err
	}
	return backgroundValidateWindowsFileIdentity(t.path, want)
}

func (t *backgroundWindowsAuditTransaction) SyncParent() error { return nil }

func (t *backgroundWindowsAuditTransaction) Close() error {
	return errors.Join(backgroundReleaseWindowsAuditLock(t.lock, t.lockOverlapped), t.parent.Close())
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
	owner, acl, err := backgroundPrivateSecurity()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		owner,
		nil,
		acl,
		nil,
	)
}

func backgroundSetPrivateHandleACL(handle windows.Handle) error {
	owner, acl, err := backgroundPrivateSecurity()
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		owner,
		nil,
		acl,
		nil,
	)
}

func backgroundPrivateSecurity() (*windows.SID, *windows.ACL, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, nil, err
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
		return nil, nil, err
	}
	return user.User.Sid, acl, nil
}
