//go:build windows

package hooks

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	hookAuditWindowsFileAllAccess windows.ACCESS_MASK = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff
	hookAuditWindowsInheritance                       = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	hookAuditWindowsFileShare                         = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE
)

type hookAuditWindowsFileID struct {
	VolumeSerialNumber uint64
	FileID             [16]byte
}

var (
	hookAuditWindowsCreateDirectory = windows.CreateDirectory
	hookAuditWindowsCreateFile      = windows.CreateFile
	hookAuditWindowsMoveFileEx      = windows.MoveFileEx
)

func rotateHookAuditPath(source, destination string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return hookAuditWindowsMoveFileEx(
		from,
		to,
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	)
}

func openHookAuditFile(path string, create bool) (*os.File, bool, error) {
	parent := filepath.Dir(path)
	if create {
		if err := hookAuditWindowsEnsurePrivateDir(parent); err != nil {
			return nil, false, err
		}
	} else if err := hookAuditWindowsValidatePrivatePath(parent, true); err != nil {
		if hookAuditWindowsPathNotExist(err) || errors.Is(err, os.ErrNotExist) {
			return nil, false, os.ErrNotExist
		}
		return nil, false, err
	}

	file, created, err := hookAuditWindowsOpenFile(path, create)
	if err != nil {
		return nil, false, err
	}
	if err := validateHookAuditIdentity(path, file); err != nil {
		return nil, false, errors.Join(err, file.Close())
	}
	return file, created, nil
}

func hookAuditWindowsOpenFile(path string, create bool) (*os.File, bool, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, false, err
	}
	access := uint32(windows.GENERIC_READ | windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES)
	if create {
		access |= windows.GENERIC_WRITE
	}
	handle, err := hookAuditWindowsCreateFile(
		pathPtr,
		access,
		hookAuditWindowsFileShare,
		nil,
		windows.OPEN_EXISTING,
		hookAuditWindowsOpenAttributes(create),
		0,
	)
	created := false
	if hookAuditWindowsPathNotExist(err) && create {
		security, securityErr := hookAuditWindowsPrivateSecurityAttributes()
		if securityErr != nil {
			return nil, false, securityErr
		}
		handle, err = hookAuditWindowsCreateFile(
			pathPtr,
			access|windows.WRITE_DAC|windows.WRITE_OWNER,
			hookAuditWindowsFileShare,
			security,
			windows.CREATE_NEW,
			hookAuditWindowsOpenAttributes(true),
			0,
		)
		created = err == nil
		if errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			handle, err = hookAuditWindowsCreateFile(
				pathPtr,
				access,
				hookAuditWindowsFileShare,
				nil,
				windows.OPEN_EXISTING,
				hookAuditWindowsOpenAttributes(true),
				0,
			)
			created = false
		}
	}
	if err != nil {
		if hookAuditWindowsPathNotExist(err) && !create {
			return nil, false, os.ErrNotExist
		}
		return nil, false, fmt.Errorf("open managed hook audit: %w", err)
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, false, errors.New("create managed hook audit handle")
	}
	if created {
		if err := hookAuditWindowsSetPrivateHandle(handle); err != nil {
			return nil, false, errors.Join(err, file.Close(), os.Remove(path))
		}
	}
	if err := hookAuditWindowsValidateHandle(handle, false); err != nil {
		closeErr := file.Close()
		if created {
			closeErr = errors.Join(closeErr, os.Remove(path))
		}
		return nil, false, errors.Join(err, closeErr)
	}
	return file, created, nil
}

func hookAuditWindowsEnsurePrivateDir(path string) error {
	if err := hookAuditWindowsValidatePrivatePath(path, true); err == nil {
		return nil
	} else if !hookAuditWindowsPathNotExist(err) && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	missing := make([]string, 0, 2)
	current := path
	for {
		if _, err := os.Lstat(current); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect managed hook audit namespace: %w", err)
		}
		missing = append(missing, current)
		if len(missing) > maxHookAuditMissingDirs {
			return errors.New("managed hook audit namespace requires an existing anchor within two parent levels")
		}
		parent := filepath.Dir(current)
		if parent == current {
			return errors.New("managed hook audit namespace has no existing ancestor")
		}
		current = parent
	}
	for i := len(missing) - 1; i >= 0; i-- {
		if err := hookAuditWindowsCreatePrivateDir(missing[i]); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return fmt.Errorf("create managed hook audit namespace: %w", err)
		}
		if err := hookAuditWindowsValidatePrivatePath(missing[i], true); err != nil {
			return err
		}
	}
	return hookAuditWindowsValidatePrivatePath(path, true)
}

func hookAuditWindowsCreatePrivateDir(path string) error {
	security, err := hookAuditWindowsPrivateSecurityAttributes()
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	for range 8 {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return fmt.Errorf("name managed hook audit namespace: %w", err)
		}
		temporary := path + ".tmp-" + hex.EncodeToString(nonce[:])
		from, err := windows.UTF16PtrFromString(temporary)
		if err != nil {
			return err
		}
		if err := hookAuditWindowsCreateDirectory(from, security); err != nil {
			if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
				continue
			}
			return err
		}
		moveErr := hookAuditWindowsMoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)
		removeErr := error(nil)
		if moveErr != nil {
			removeErr = os.Remove(temporary)
		}
		if _, statErr := os.Lstat(path); statErr == nil && moveErr != nil {
			return errors.Join(windows.ERROR_ALREADY_EXISTS, removeErr)
		}
		return errors.Join(moveErr, removeErr)
	}
	return errors.New("create managed hook audit namespace: temporary name collisions")
}

func hookAuditWindowsOpenAttributes(write bool) uint32 {
	attributes := uint32(windows.FILE_ATTRIBUTE_NORMAL | windows.FILE_FLAG_OPEN_REPARSE_POINT)
	if write {
		attributes |= windows.FILE_FLAG_WRITE_THROUGH
	}
	return attributes
}

func hookAuditWindowsValidatePrivatePath(path string, directory bool) error {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
	if directory {
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	handle, err := hookAuditWindowsCreateFile(
		pathPtr,
		windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		flags,
		0,
	)
	if err != nil {
		if hookAuditWindowsPathNotExist(err) {
			return os.ErrNotExist
		}
		return err
	}
	defer windows.CloseHandle(handle) //nolint:errcheck
	return hookAuditWindowsValidateHandle(handle, directory)
}

func hookAuditWindowsValidateHandle(handle windows.Handle, directory bool) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return err
	}
	isDirectory := info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	if isDirectory != directory || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("managed hook audit target type or reparse attributes are unsafe")
	}
	if !directory && info.NumberOfLinks != 1 {
		return fmt.Errorf("managed hook audit target has %d links, want one", info.NumberOfLinks)
	}
	descriptor, err := windows.GetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return err
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return err
	}
	current, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	control, _, err := descriptor.Control()
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	entries := make([]hookAuditWindowsAccessEntry, 0)
	if dacl != nil {
		entries = make([]hookAuditWindowsAccessEntry, 0, dacl.AceCount)
		for i := uint32(0); i < uint32(dacl.AceCount); i++ {
			var ace *windows.ACCESS_ALLOWED_ACE
			if err := windows.GetAce(dacl, i, &ace); err != nil {
				return err
			}
			entry := hookAuditWindowsAccessEntry{
				allowed:     ace.Header.AceType == windows.ACCESS_ALLOWED_ACE_TYPE,
				inheritOnly: ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0,
			}
			if entry.allowed {
				sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
				entry.owner = sid.Equals(current.User.Sid)
				entry.fullControl = ace.Mask == hookAuditWindowsFileAllAccess
			}
			entries = append(entries, entry)
		}
	}
	return validateHookAuditWindowsAccess(
		owner != nil && owner.Equals(current.User.Sid),
		control&windows.SE_DACL_PROTECTED != 0,
		entries,
	)
}

func validateHookAuditIdentity(path string, file *os.File) error {
	want, err := hookAuditWindowsHandleIdentity(windows.Handle(file.Fd()))
	if err != nil {
		return err
	}
	current, _, err := hookAuditWindowsOpenFile(path, false)
	if err != nil {
		return err
	}
	defer current.Close() //nolint:errcheck
	got, err := hookAuditWindowsHandleIdentity(windows.Handle(current.Fd()))
	if err != nil {
		return err
	}
	if got != want {
		return errors.New("managed hook audit target changed during open")
	}
	return nil
}

func hookAuditWindowsHandleIdentity(handle windows.Handle) (hookAuditWindowsFileID, error) {
	var identity hookAuditWindowsFileID
	err := windows.GetFileInformationByHandleEx(
		handle,
		windows.FileIdInfo,
		(*byte)(unsafe.Pointer(&identity)),
		uint32(unsafe.Sizeof(identity)),
	)
	return identity, err
}

func hookAuditWindowsSetPrivateHandle(handle windows.Handle) error {
	owner, acl, err := hookAuditWindowsPrivateSecurity()
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

func hookAuditWindowsPrivateSecurity() (*windows.SID, *windows.ACL, error) {
	current, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, nil, err
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: hookAuditWindowsFileAllAccess,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       hookAuditWindowsInheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(current.User.Sid),
		},
	}}, nil)
	if err != nil {
		return nil, nil, err
	}
	return current.User.Sid, acl, nil
}

func hookAuditWindowsPrivateSecurityAttributes() (*windows.SecurityAttributes, error) {
	owner, acl, err := hookAuditWindowsPrivateSecurity()
	if err != nil {
		return nil, err
	}
	descriptor, err := windows.NewSecurityDescriptor()
	if err != nil {
		return nil, err
	}
	if err := descriptor.SetOwner(owner, false); err != nil {
		return nil, err
	}
	if err := descriptor.SetDACL(acl, true, false); err != nil {
		return nil, err
	}
	if err := descriptor.SetControl(windows.SE_DACL_PROTECTED, windows.SE_DACL_PROTECTED); err != nil {
		return nil, err
	}
	descriptor, err = descriptor.ToSelfRelative()
	if err != nil {
		return nil, err
	}
	return &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: descriptor,
	}, nil
}

func hookAuditWindowsPathNotExist(err error) bool {
	return errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)
}

// Windows has no supported directory-handle equivalent of fsync. Namespace
// creation and replacement use MOVEFILE_WRITE_THROUGH, while writer handles
// use FILE_FLAG_WRITE_THROUGH and FlushFileBuffers before this hook is called.
func syncHookAuditDirectory(string) error { return nil }
