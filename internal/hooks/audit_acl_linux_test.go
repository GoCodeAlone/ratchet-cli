//go:build linux

package hooks

import (
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestManagedHookAuditLinuxACLInspectionUsesListxattr(t *testing.T) {
	path := t.TempDir()
	if err := unix.Setxattr(path, "user.ratchet-test", []byte("value"), 0); err != nil {
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
			t.Skipf("xattrs unsupported: %v", err)
		}
		t.Fatalf("Setxattr: %v", err)
	}
	if err := validatePlatformMutationACL(path); err != nil {
		t.Fatalf("unrelated native xattr: %v", err)
	}

	original := hookAuditLinuxListxattr
	t.Cleanup(func() { hookAuditLinuxListxattr = original })
	data := []byte("system.nfs4_acl\x00")
	hookAuditLinuxListxattr = func(gotPath string, destination []byte) (int, error) {
		if gotPath != path {
			t.Fatalf("Listxattr path = %q, want %q", gotPath, path)
		}
		if destination == nil {
			return len(data), nil
		}
		return copy(destination, data), nil
	}
	if err := validatePlatformMutationACL(path); err == nil {
		t.Fatal("syscall-backed ACL inspection accepted NFSv4 ACL xattr")
	}
}

func TestManagedHookAuditLinuxACLInspectionFailsClosed(t *testing.T) {
	original := hookAuditLinuxListxattr
	t.Cleanup(func() { hookAuditLinuxListxattr = original })
	hookAuditLinuxListxattr = func(string, []byte) (int, error) {
		return 0, os.ErrPermission
	}
	if err := validatePlatformMutationACL(t.TempDir()); err == nil {
		t.Fatal("ACL inspection accepted Listxattr permission failure")
	}
}
