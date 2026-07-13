//go:build windows

package acpclient

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestBackgroundWindowsAtomicReplacementUsesPrivateACL(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	path := filepath.Join(dir, "background.json")
	for _, data := range [][]byte{[]byte("first\r\n"), []byte("second\r\n")} {
		if err := backgroundWriteFileAtomic(path, data); err != nil {
			t.Fatalf("backgroundWriteFileAtomic: %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(got) != string(data) {
			t.Fatalf("content = %q, want %q", got, data)
		}
	}
	if backgroundMoveFileFlags&windows.MOVEFILE_REPLACE_EXISTING == 0 {
		t.Fatalf("replacement flags = %#x, missing MOVEFILE_REPLACE_EXISTING", backgroundMoveFileFlags)
	}
	if backgroundMoveFileFlags&windows.MOVEFILE_WRITE_THROUGH == 0 {
		t.Fatalf("replacement flags = %#x, missing MOVEFILE_WRITE_THROUGH", backgroundMoveFileFlags)
	}
	assertBackgroundWindowsPrivateDACL(t, dir)
	assertBackgroundWindowsPrivateDACL(t, path)
}

func TestBackgroundWindowsAtomicReplacementDoesNotRewriteExistingParentDACL(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "custom")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir parent: %v", err)
	}
	before := backgroundWindowsSecurityDescriptor(t, dir)
	path := filepath.Join(dir, "background.json")
	if err := backgroundWriteFileAtomic(path, []byte("private\r\n")); err != nil {
		t.Fatalf("backgroundWriteFileAtomic: %v", err)
	}
	if after := backgroundWindowsSecurityDescriptor(t, dir); after != before {
		t.Fatalf("parent DACL changed:\nbefore: %s\nafter:  %s", before, after)
	}
	assertBackgroundWindowsPrivateDACL(t, path)
}

func TestBackgroundWindowsAuditAppendUsesPrivateDirectoryACL(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	path := filepath.Join(dir, "background-audit.jsonl")
	if err := NewBackgroundAudit(path).Append(BackgroundAuditRecord{
		RecordID:       "windows-private-audit",
		At:             time.Date(2026, 7, 13, 20, 0, 0, 0, time.UTC),
		Action:         BackgroundAuditError,
		SessionID:      "session-1",
		Profile:        "fixture",
		DescriptorHash: "descriptor-hash",
		Outcome:        BackgroundOutcomeWorkerError,
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	assertBackgroundWindowsPrivateDACL(t, dir)
	assertBackgroundWindowsPrivateDACL(t, path)
}

func TestBackgroundWindowsAuditRejectsReparsePoint(t *testing.T) {
	audit := NewBackgroundAudit(filepath.Join(t.TempDir(), "background-audit.jsonl"))
	if err := backgroundEnsureOwnedPrivateDir(filepath.Dir(audit.Path())); err != nil {
		t.Fatalf("create audit parent: %v", err)
	}
	target := filepath.Join(t.TempDir(), "target.jsonl")
	const targetData = "target must not change\r\n"
	if err := os.WriteFile(target, []byte(targetData), 0o600); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}
	if err := os.Symlink(target, audit.Path()); err != nil {
		t.Fatalf("Symlink audit target: %v", err)
	}
	if err := audit.Append(backgroundWindowsAuditRecord("reparse-event")); !errors.Is(err, ErrStoreLockPathUnsafe) {
		t.Fatalf("Append error = %v, want ErrStoreLockPathUnsafe", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != targetData {
		t.Fatalf("reparse target = %q, %v", got, err)
	}
}

func TestBackgroundWindowsAuditRejectsHardLink(t *testing.T) {
	audit := NewBackgroundAudit(filepath.Join(t.TempDir(), "background-audit.jsonl"))
	if err := audit.Append(backgroundWindowsAuditRecord("hard-link-seed")); err != nil {
		t.Fatalf("seed Append: %v", err)
	}
	before, err := os.ReadFile(audit.Path())
	if err != nil {
		t.Fatalf("ReadFile before: %v", err)
	}
	if err := os.Link(audit.Path(), filepath.Join(t.TempDir(), "linked.jsonl")); err != nil {
		t.Fatalf("Link audit target: %v", err)
	}
	if err := audit.Append(backgroundWindowsAuditRecord("hard-link-event")); !errors.Is(err, ErrStoreLockPathUnsafe) {
		t.Fatalf("Append error = %v, want ErrStoreLockPathUnsafe", err)
	}
	if after, err := os.ReadFile(audit.Path()); err != nil || string(after) != string(before) {
		t.Fatalf("hard-linked audit changed: before %q, after %q, err %v", before, after, err)
	}
}

func TestBackgroundWindowsAuditRejectsParentReplacement(t *testing.T) {
	audit := NewBackgroundAudit(filepath.Join(t.TempDir(), "background-audit.jsonl"))
	if err := audit.Append(backgroundWindowsAuditRecord("parent-seed")); err != nil {
		t.Fatalf("seed Append: %v", err)
	}
	parent := filepath.Dir(audit.Path())
	moved := parent + "-moved"
	var once sync.Once
	var replacementErr error
	audit.beforeMutation = func() {
		once.Do(func() {
			replacementErr = os.Rename(parent, moved)
			if replacementErr == nil {
				if err := backgroundEnsureOwnedPrivateDir(parent); err != nil {
					t.Errorf("create replacement parent: %v", err)
				}
			}
		})
	}
	appendErr := audit.Append(backgroundWindowsAuditRecord("parent-event"))
	if replacementErr == nil {
		if !errors.Is(appendErr, ErrStoreLockPathUnsafe) {
			t.Fatalf("Append after successful parent replacement = %v, want ErrStoreLockPathUnsafe", appendErr)
		}
		return
	}
	if appendErr != nil {
		t.Fatalf("Append after OS denied parent replacement: %v", appendErr)
	}
}

func TestBackgroundWindowsAuditRejectsWeakDACL(t *testing.T) {
	audit := NewBackgroundAudit(filepath.Join(t.TempDir(), "background-audit.jsonl"))
	if err := audit.Append(backgroundWindowsAuditRecord("weak-dacl-seed")); err != nil {
		t.Fatalf("seed Append: %v", err)
	}
	before, err := os.ReadFile(audit.Path())
	if err != nil {
		t.Fatalf("ReadFile before: %v", err)
	}
	backgroundSetWindowsWeakDACL(t, audit.Path())
	if err := audit.Append(backgroundWindowsAuditRecord("weak-dacl-event")); !errors.Is(err, ErrStoreLockPathUnsafe) {
		t.Fatalf("Append error = %v, want ErrStoreLockPathUnsafe", err)
	}
	if after, err := os.ReadFile(audit.Path()); err != nil || string(after) != string(before) {
		t.Fatalf("weak-DACL audit changed: before %q, after %q, err %v", before, after, err)
	}
}

func backgroundWindowsAuditRecord(recordID string) BackgroundAuditRecord {
	return BackgroundAuditRecord{
		RecordID:       recordID,
		At:             time.Date(2026, 7, 13, 20, 0, 0, 0, time.UTC),
		Action:         BackgroundAuditError,
		SessionID:      "session-1",
		Profile:        "fixture",
		DescriptorHash: "descriptor-hash",
		Outcome:        BackgroundOutcomeWorkerError,
	}
}

func backgroundSetWindowsWeakDACL(t *testing.T, path string) {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("GetTokenUser: %v", err)
	}
	everyone, err := windows.StringToSid("S-1-1-0")
	if err != nil {
		t.Fatalf("StringToSid Everyone: %v", err)
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{
		{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.GRANT_ACCESS,
			Trustee: windows.TRUSTEE{
				TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_USER,
				TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid),
			},
		},
		{
			AccessPermissions: windows.GENERIC_READ,
			AccessMode:        windows.GRANT_ACCESS,
			Trustee: windows.TRUSTEE{
				TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(everyone),
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("ACLFromEntries: %v", err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	); err != nil {
		t.Fatalf("SetNamedSecurityInfo weak DACL: %v", err)
	}
}

func TestBackgroundWindowsPrivateDirectoryACLIsInheritedByRawChild(t *testing.T) {
	if backgroundPrivateInheritance != windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT {
		t.Fatalf("private inheritance = %#x, want containers and objects", backgroundPrivateInheritance)
	}
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := backgroundSetPrivateACL(dir); err != nil {
		t.Fatalf("backgroundSetPrivateACL: %v", err)
	}
	child := filepath.Join(dir, "raw-child.jsonl")
	f, err := os.OpenFile(child, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("OpenFile raw child: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close raw child: %v", err)
	}
	assertBackgroundWindowsPrivateDACL(t, dir)

	descriptor, err := windows.GetNamedSecurityInfo(child, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("GetNamedSecurityInfo child: %v", err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatalf("DACL child: %v", err)
	}
	if dacl == nil || dacl.AceCount != 1 {
		t.Fatalf("child ACE count = %v, want inherited owner-only ACE", dacl)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatalf("GetAce child: %v", err)
	}
	if ace.Header.AceFlags&windows.INHERITED_ACE == 0 {
		t.Fatalf("child ACE flags = %#x, want INHERITED_ACE", ace.Header.AceFlags)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("GetTokenUser: %v", err)
	}
	aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !aceSID.Equals(user.User.Sid) {
		t.Fatalf("child ACE SID = %s, want current user %s", aceSID, user.User.Sid)
	}
}

func assertBackgroundWindowsPrivateDACL(t *testing.T, path string) {
	t.Helper()
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("GetNamedSecurityInfo: %v", err)
	}
	control, _, err := descriptor.Control()
	if err != nil {
		t.Fatalf("Control: %v", err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("DACL is inherited: control=%#x", control)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatalf("DACL: %v", err)
	}
	if dacl == nil || dacl.AceCount != 1 {
		t.Fatalf("ACE count = %v, want owner-only ACE", dacl)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatalf("GetAce: %v", err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("GetTokenUser: %v", err)
	}
	aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !aceSID.Equals(user.User.Sid) {
		t.Fatalf("private ACE SID = %s, want current user %s", aceSID, user.User.Sid)
	}
}

func backgroundWindowsSecurityDescriptor(t *testing.T, path string) string {
	t.Helper()
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.GROUP_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		t.Fatalf("GetNamedSecurityInfo: %v", err)
	}
	if sddl := descriptor.String(); sddl != "" {
		return sddl
	}
	t.Fatal("security descriptor has empty SDDL")
	return ""
}
