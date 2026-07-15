//go:build darwin

package hooks

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	hookAuditDarwinAttributeHeaderSize = 4
	hookAuditDarwinAttributeRefSize    = 8
	hookAuditDarwinFileSecuritySize    = 44
	hookAuditDarwinACEBytes            = 24
	hookAuditDarwinNoACL               = ^uint32(0)
	hookAuditDarwinMaxACLEntries       = 128
	hookAuditDarwinACEPermit           = 1
	hookAuditDarwinACEKindMask         = 0xf
	hookAuditDarwinWriteData           = 1 << 2
	hookAuditDarwinDelete              = 1 << 4
	hookAuditDarwinAppendData          = 1 << 5
	hookAuditDarwinDeleteChild         = 1 << 6
	hookAuditDarwinWriteAttributes     = 1 << 8
	hookAuditDarwinWriteExtendedAttrs  = 1 << 10
	hookAuditDarwinWriteSecurity       = 1 << 12
	hookAuditDarwinTakeOwnership       = 1 << 13
	hookAuditDarwinGenericAll          = 1 << 21
	hookAuditDarwinGenericWrite        = 1 << 23
	hookAuditDarwinMutationRights      = hookAuditDarwinWriteData | hookAuditDarwinDelete | hookAuditDarwinAppendData |
		hookAuditDarwinDeleteChild | hookAuditDarwinWriteAttributes | hookAuditDarwinWriteExtendedAttrs |
		hookAuditDarwinWriteSecurity | hookAuditDarwinTakeOwnership | hookAuditDarwinGenericAll | hookAuditDarwinGenericWrite
)

func validatePlatformMutationACL(path string) error {
	pathPointer, err := unix.BytePtrFromString(path)
	if err != nil {
		return err
	}
	attributes := unix.Attrlist{
		Bitmapcount: unix.ATTR_BIT_MAP_COUNT,
		Commonattr:  unix.ATTR_CMN_EXTENDED_SECURITY,
	}
	buffer := make([]byte, 4<<10)
	_, _, errno := unix.Syscall6(
		unix.SYS_GETATTRLIST, //nolint:staticcheck // x/sys has no pure-Go getattrlist wrapper; releases disable cgo.
		uintptr(unsafe.Pointer(pathPointer)),
		uintptr(unsafe.Pointer(&attributes)),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
		unix.FSOPT_NOFOLLOW,
		0,
	)
	runtime.KeepAlive(pathPointer)
	runtime.KeepAlive(&attributes)
	return validateDarwinMutationACLBuffer(buffer, errno)
}

func validateOpenedPlatformMutationACL(_ string, fd int) error {
	attributes := unix.Attrlist{
		Bitmapcount: unix.ATTR_BIT_MAP_COUNT,
		Commonattr:  unix.ATTR_CMN_EXTENDED_SECURITY,
	}
	buffer := make([]byte, 4<<10)
	_, _, errno := unix.Syscall6(
		unix.SYS_FGETATTRLIST, //nolint:staticcheck // x/sys has no pure-Go fgetattrlist wrapper; releases disable cgo.
		uintptr(fd),
		uintptr(unsafe.Pointer(&attributes)),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
		0,
		0,
	)
	runtime.KeepAlive(&attributes)
	return validateDarwinMutationACLBuffer(buffer, errno)
}

func validateDarwinMutationACLBuffer(buffer []byte, errno syscall.Errno) error {
	if errno != 0 {
		return fmt.Errorf("inspect filesystem ACL: %w", errno)
	}
	if len(buffer) < hookAuditDarwinAttributeHeaderSize+hookAuditDarwinAttributeRefSize {
		return errors.New("inspect filesystem ACL: truncated attributes")
	}
	returned := int(binary.LittleEndian.Uint32(buffer[:hookAuditDarwinAttributeHeaderSize]))
	if returned > len(buffer) || returned < hookAuditDarwinAttributeHeaderSize+hookAuditDarwinAttributeRefSize {
		return errors.New("inspect filesystem ACL: invalid attribute length")
	}
	referenceStart := hookAuditDarwinAttributeHeaderSize
	dataOffset := int(int32(binary.LittleEndian.Uint32(buffer[referenceStart : referenceStart+4])))
	dataLength := int(binary.LittleEndian.Uint32(buffer[referenceStart+4 : referenceStart+hookAuditDarwinAttributeRefSize]))
	if dataLength == 0 {
		return nil
	}
	dataStart := referenceStart + dataOffset
	dataEnd := dataStart + dataLength
	if dataStart < referenceStart+hookAuditDarwinAttributeRefSize || dataEnd < dataStart || dataEnd > returned || dataLength < hookAuditDarwinFileSecuritySize {
		return errors.New("inspect filesystem ACL: invalid extended-security attribute")
	}
	entryCount := binary.LittleEndian.Uint32(buffer[dataStart+36 : dataStart+40])
	if entryCount == hookAuditDarwinNoACL {
		return nil
	}
	if entryCount > hookAuditDarwinMaxACLEntries || hookAuditDarwinFileSecuritySize+int(entryCount)*hookAuditDarwinACEBytes > dataLength {
		return errors.New("inspect filesystem ACL: invalid entry count")
	}
	for i := range int(entryCount) {
		entryStart := dataStart + hookAuditDarwinFileSecuritySize + i*hookAuditDarwinACEBytes
		flags := binary.LittleEndian.Uint32(buffer[entryStart+16 : entryStart+20])
		rights := binary.LittleEndian.Uint32(buffer[entryStart+20 : entryStart+24])
		if flags&hookAuditDarwinACEKindMask != hookAuditDarwinACEPermit || rights&hookAuditDarwinMutationRights == 0 {
			continue
		}
		return errors.New("filesystem ACL grants mutation rights")
	}
	return nil
}
