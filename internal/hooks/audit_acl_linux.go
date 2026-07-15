//go:build linux

package hooks

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/unix"
)

var (
	hookAuditLinuxListxattr      = unix.Listxattr
	managedPolicyLinuxFlistxattr = unix.Flistxattr
)

func validatePlatformMutationACL(path string) error {
	return validateLinuxMutationACL(func(destination []byte) (int, error) {
		return hookAuditLinuxListxattr(path, destination)
	})
}

func validateOpenedPlatformMutationACL(_ string, fd int) error {
	return validateLinuxMutationACL(func(destination []byte) (int, error) {
		return managedPolicyLinuxFlistxattr(fd, destination)
	})
}

func validateLinuxMutationACL(list func([]byte) (int, error)) error {
	for range 3 {
		size, err := list(nil)
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect filesystem ACLs: %w", err)
		}
		if size == 0 {
			return nil
		}
		buffer := make([]byte, size)
		read, err := list(buffer)
		if errors.Is(err, unix.ERANGE) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect filesystem ACLs: %w", err)
		}
		names := make([]string, 0, 2)
		for name := range strings.SplitSeq(string(buffer[:read]), "\x00") {
			if name != "" {
				names = append(names, name)
			}
		}
		return validateLinuxMutationACLNames(names)
	}
	return errors.New("inspect filesystem ACLs: attributes changed repeatedly")
}
