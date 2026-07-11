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
