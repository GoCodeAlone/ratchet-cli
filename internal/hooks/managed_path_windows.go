//go:build windows

package hooks

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const managedWindowsWriteRights windows.ACCESS_MASK = windows.GENERIC_WRITE |
	windows.GENERIC_ALL |
	windows.FILE_WRITE_DATA |
	windows.FILE_APPEND_DATA |
	windows.FILE_WRITE_EA |
	windows.FILE_WRITE_ATTRIBUTES |
	windows.DELETE |
	windows.WRITE_DAC |
	windows.WRITE_OWNER

func defaultManagedPolicyPath() (string, error) {
	programData, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil {
		return "", err
	}
	return filepath.Join(programData, "ratchet", "managed-hooks.yaml"), nil
}

func secureReadManagedFile(path string) (data []byte, err error) {
	return secureReadManagedFileWithSnapshotReader(path, readManagedPolicySnapshot)
}

func secureReadManagedFileWithSnapshotReader(
	path string,
	readSnapshot managedPolicySnapshotReader,
) (data []byte, err error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ|windows.READ_CONTROL,
		windows.FILE_SHARE_READ,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("create managed policy file handle")
	}
	defer finishManagedPolicyRead(&data, &err, file)

	initial, err := inspectManagedWindowsSnapshot(handle)
	if err != nil {
		return nil, err
	}
	data, err = readManagedWindowsSnapshotWith(file, initial, func() (managedWindowsSnapshot, error) {
		return inspectManagedWindowsSnapshot(handle)
	}, readSnapshot)
	return data, err
}

type managedWindowsSnapshot struct {
	attributes uint32
	size       uint64
	lastWrite  windows.Filetime
	security   string
}

func inspectManagedWindowsSnapshot(handle windows.Handle) (managedWindowsSnapshot, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return managedWindowsSnapshot{}, err
	}
	if err := validateManagedWindowsFileAttributes(info.FileAttributes); err != nil {
		return managedWindowsSnapshot{}, err
	}
	fileSize := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
	if err := validateManagedPolicySize(fileSize); err != nil {
		return managedWindowsSnapshot{}, err
	}
	descriptor, err := windows.GetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return managedWindowsSnapshot{}, err
	}
	if err := validateManagedWindowsDescriptor(descriptor); err != nil {
		return managedWindowsSnapshot{}, err
	}
	security, err := validateManagedWindowsDescriptorSerialization(descriptor.String())
	if err != nil {
		return managedWindowsSnapshot{}, err
	}
	return managedWindowsSnapshot{
		attributes: info.FileAttributes,
		size:       fileSize,
		lastWrite:  info.LastWriteTime,
		security:   security,
	}, nil
}

func readManagedWindowsSnapshot(
	reader io.Reader,
	initial managedWindowsSnapshot,
	inspect func() (managedWindowsSnapshot, error),
) ([]byte, error) {
	return readManagedWindowsSnapshotWith(reader, initial, inspect, readManagedPolicySnapshot)
}

func readManagedWindowsSnapshotWith(
	reader io.Reader,
	initial managedWindowsSnapshot,
	inspect func() (managedWindowsSnapshot, error),
	readSnapshot managedPolicySnapshotReader,
) ([]byte, error) {
	return readSnapshot(reader, initial.size, func() error {
		current, err := inspect()
		if err != nil {
			return err
		}
		if current != initial {
			return errManagedPolicyChanged
		}
		return nil
	})
}

func validateManagedWindowsFileAttributes(attributes uint32) error {
	if attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("managed policy is not a regular non-reparse file")
	}
	return nil
}

func validateManagedWindowsDescriptor(descriptor *windows.SECURITY_DESCRIPTOR) error {
	return validateManagedWindowsDescriptorWithACEValidator(descriptor, validateManagedWindowsACE)
}

func validateManagedWindowsDescriptorWithACEValidator(
	descriptor *windows.SECURITY_DESCRIPTOR,
	validateACE func(uint8, bool, bool, bool) error,
) error {
	if descriptor == nil {
		return errors.New("managed policy has no security descriptor")
	}
	adminSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return err
	}
	systemSID, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return err
	}
	if owner == nil || (!owner.Equals(adminSID) && !owner.Equals(systemSID)) {
		return errors.New("managed policy owner is not Administrators or SYSTEM")
	}
	control, _, err := descriptor.Control()
	if err != nil {
		return err
	}
	if err := validateManagedWindowsDACLProtection(control&windows.SE_DACL_PROTECTED != 0); err != nil {
		return err
	}
	dacl, defaulted, err := descriptor.DACL()
	if err != nil {
		return err
	}
	if dacl == nil || defaulted {
		return errors.New("managed policy has no explicit DACL")
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return err
		}
		administrative := false
		if ace.Header.AceType == windows.ACCESS_ALLOWED_ACE_TYPE {
			aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
			administrative = aceSID.Equals(adminSID) || aceSID.Equals(systemSID)
		}
		if err := validateACE(
			ace.Header.AceType,
			ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0,
			ace.Mask&managedWindowsWriteRights != 0,
			administrative,
		); err != nil {
			return err
		}
	}
	return nil
}
