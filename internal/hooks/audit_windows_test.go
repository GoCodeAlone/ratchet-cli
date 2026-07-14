//go:build windows

package hooks

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestManagedHookAuditWindowsCreatesOwnerOnlyDACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "hooks.jsonl")
	if err := NewHookAudit(path).Append(managedAuditRecord(HookAuditStarted)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	assertManagedHookAuditWindowsPrivate(t, filepath.Dir(path))
	assertManagedHookAuditWindowsPrivate(t, path)
}

func TestManagedHookAuditWindowsReaderAllowsRotation(t *testing.T) {
	if hookAuditWindowsFileShare&windows.FILE_SHARE_DELETE == 0 {
		t.Fatal("audit file share mask does not permit rotation")
	}
	path := filepath.Join(t.TempDir(), "private", "hooks.jsonl")
	audit := NewHookAudit(path)
	if err := audit.Append(managedAuditRecord(HookAuditStarted)); err != nil {
		t.Fatalf("seed Append: %v", err)
	}
	reader, _, err := openHookAuditFile(path, false)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	defer reader.Close() //nolint:errcheck
	writer, _, err := openHookAuditFile(path, true)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	next, err := audit.rotate(writer)
	if err != nil {
		t.Fatalf("rotate with open reader: %v", err)
	}
	if err := next.Close(); err != nil {
		t.Fatalf("close rotated active audit: %v", err)
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seek retained reader: %v", err)
	}
	if data, err := io.ReadAll(reader); err != nil || len(data) == 0 {
		t.Fatalf("retained reader data = %d bytes, %v", len(data), err)
	}
}

func TestManagedHookAuditWindowsRejectsWeakDACL(t *testing.T) {
	t.Run("namespace", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "private")
		if err := os.Mkdir(parent, 0o700); err != nil {
			t.Fatal(err)
		}
		setManagedHookAuditWindowsWeakDACL(t, parent)
		path := filepath.Join(parent, "hooks.jsonl")
		if err := NewHookAudit(path).Append(managedAuditRecord(HookAuditStarted)); err == nil {
			t.Fatal("Append accepted weak namespace DACL")
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("audit created in weak namespace: %v", err)
		}
	})

	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "private", "hooks.jsonl")
		audit := NewHookAudit(path)
		if err := audit.Append(managedAuditRecord(HookAuditStarted)); err != nil {
			t.Fatalf("seed Append: %v", err)
		}
		setManagedHookAuditWindowsWeakDACL(t, path)
		if err := audit.Append(managedAuditRecord(HookAuditSuccess)); err == nil {
			t.Fatal("Append accepted weak file DACL")
		}
	})

	t.Run("protected file with extra principal", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "private", "hooks.jsonl")
		audit := NewHookAudit(path)
		if err := audit.Append(managedAuditRecord(HookAuditStarted)); err != nil {
			t.Fatalf("seed Append: %v", err)
		}
		setManagedHookAuditWindowsProtectedExtraPrincipalDACL(t, path)
		if err := audit.Append(managedAuditRecord(HookAuditSuccess)); err == nil {
			t.Fatal("Append accepted protected file DACL with extra principal")
		}
	})
}

func TestManagedHookAuditWindowsRejectsReparseAndNonRegularTargets(t *testing.T) {
	t.Run("reparse", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "private")
		if err := hookAuditWindowsEnsurePrivateDir(parent); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(t.TempDir(), "target.jsonl")
		if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(parent, "hooks.jsonl")
		if err := os.Symlink(target, path); err != nil {
			t.Skipf("create Windows symlink: %v", err)
		}
		if err := NewHookAudit(path).Append(managedAuditRecord(HookAuditStarted)); err == nil {
			t.Fatal("Append accepted reparse target")
		}
		if got, err := os.ReadFile(target); err != nil || string(got) != "unchanged" {
			t.Fatalf("reparse target = %q, %v", got, err)
		}
	})

	t.Run("directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "private", "hooks.jsonl")
		if err := hookAuditWindowsEnsurePrivateDir(filepath.Dir(path)); err != nil {
			t.Fatal(err)
		}
		if err := hookAuditWindowsEnsurePrivateDir(path); err != nil {
			t.Fatal(err)
		}
		if err := NewHookAudit(path).Append(managedAuditRecord(HookAuditStarted)); err == nil {
			t.Fatal("Append accepted directory target")
		}
	})
}

func setManagedHookAuditWindowsWeakDACL(t *testing.T, path string) {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	everyone, err := windows.StringToSid("S-1-1-0")
	if err != nil {
		t.Fatal(err)
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{
		{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.GRANT_ACCESS,
			Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_USER,
				TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid)},
		},
		{
			AccessPermissions: windows.GENERIC_READ,
			AccessMode:        windows.GRANT_ACCESS,
			Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(everyone)},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
}

func setManagedHookAuditWindowsProtectedExtraPrincipalDACL(t *testing.T, path string) {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	everyone, err := windows.StringToSid("S-1-1-0")
	if err != nil {
		t.Fatal(err)
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{
		{
			AccessPermissions: hookAuditWindowsFileAllAccess,
			AccessMode:        windows.GRANT_ACCESS,
			Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_USER,
				TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid)},
		},
		{
			AccessPermissions: windows.GENERIC_READ,
			AccessMode:        windows.GRANT_ACCESS,
			Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(everyone)},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
}

func assertManagedHookAuditWindowsPrivate(t *testing.T, path string) {
	t.Helper()
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil || !owner.Equals(user.User.Sid) {
		t.Fatalf("owner = %v, err %v; want current user", owner, err)
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("DACL control = %#x, err %v; want protected", control, err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil || dacl.AceCount == 0 {
		t.Fatalf("DACL = %v, err %v", dacl, err)
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			t.Fatal(err)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE ||
			ace.Mask != hookAuditWindowsFileAllAccess || !sid.Equals(user.User.Sid) {
			t.Fatalf("ACE %d = type %#x mask %#x sid %s", i, ace.Header.AceType, ace.Mask, sid)
		}
	}
}
