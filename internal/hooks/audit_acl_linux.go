//go:build linux

package hooks

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/unix"
)

var hookAuditLinuxListxattr = unix.Listxattr

func validateHookAuditPlatformACL(path string) error {
	for range 3 {
		size, err := hookAuditLinuxListxattr(path, nil)
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect managed hook audit trusted anchor ACLs: %w", err)
		}
		if size == 0 {
			return nil
		}
		buffer := make([]byte, size)
		read, err := hookAuditLinuxListxattr(path, buffer)
		if errors.Is(err, unix.ERANGE) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect managed hook audit trusted anchor ACLs: %w", err)
		}
		names := make([]string, 0, 2)
		for name := range strings.SplitSeq(string(buffer[:read]), "\x00") {
			if name != "" {
				names = append(names, name)
			}
		}
		return validateHookAuditLinuxACLNames(names)
	}
	return errors.New("inspect managed hook audit trusted anchor ACLs: attributes changed repeatedly")
}
