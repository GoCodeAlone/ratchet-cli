//go:build windows

package acpclient

import (
	"os"
	"path/filepath"
	"testing"
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

func TestBackgroundWindowsAuditAppendUsesPrivateDirectoryACL(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	path := filepath.Join(dir, "background-audit.jsonl")
	if err := NewBackgroundAudit(path).Append(BackgroundAuditRecord{
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
