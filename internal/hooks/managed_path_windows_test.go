//go:build windows

package hooks

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestManagedWindowsDescriptorAllowsOnlyAdministratorsAndSystemToWrite(t *testing.T) {
	tests := []struct {
		name    string
		sddl    string
		wantErr bool
	}{
		{
			name: "admin owner with admin and system full access",
			sddl: "O:BAD:P(A;;FA;;;BA)(A;;FA;;;SY)(A;;FR;;;BU)(A;;FR;;;WD)",
		},
		{
			name: "system owner",
			sddl: "O:SYD:P(A;;FA;;;BA)(A;;FA;;;SY)",
		},
		{
			name: "basic deny grants nothing",
			sddl: "O:BAD:P(D;;FW;;;BU)(A;;FA;;;BA)(A;;FR;;;BU)",
		},
		{
			name:    "unprotected administrative dacl",
			sddl:    "O:BAD:(A;;FA;;;BA)(A;;FA;;;SY)",
			wantErr: true,
		},
		{
			name:    "inherited administrative dacl",
			sddl:    "O:BAD:AI(A;ID;FA;;;BA)(A;ID;FA;;;SY)",
			wantErr: true,
		},
		{
			name:    "users can write",
			sddl:    "O:BAD:P(A;;FA;;;BA)(A;;FW;;;BU)",
			wantErr: true,
		},
		{
			name:    "everyone has generic write",
			sddl:    "O:BAD:P(A;;FA;;;BA)(A;;GW;;;WD)",
			wantErr: true,
		},
		{
			name:    "users object allow can write",
			sddl:    "O:BAD:P(A;;FA;;;BA)(OA;;FW;00112233-4455-6677-8899-aabbccddeeff;;BU)",
			wantErr: true,
		},
		{
			name:    "users can rewrite dacl",
			sddl:    "O:BAD:P(A;;FA;;;BA)(A;;WD;;;BU)",
			wantErr: true,
		},
		{
			name:    "non administrative owner",
			sddl:    "O:BUD:P(A;;FA;;;BA)(A;;FA;;;SY)",
			wantErr: true,
		},
		{
			name:    "null dacl",
			sddl:    "O:BA",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor, err := windows.SecurityDescriptorFromString(test.sddl)
			if err != nil {
				t.Fatalf("SecurityDescriptorFromString: %v", err)
			}
			err = validateManagedWindowsDescriptor(descriptor)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateManagedWindowsDescriptor = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestManagedWindowsDescriptorRejectsCallbackAllowWriteACE(t *testing.T) {
	const callbackACEType = 0x9
	descriptor, err := windows.SecurityDescriptorFromString(
		`O:BAD:P(A;;FA;;;BA)(XA;;FW;;;BU;(@User.Title=="PM"))`,
	)
	if err != nil {
		t.Fatalf("SecurityDescriptorFromString: %v", err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatalf("DACL: %v", err)
	}
	foundCallbackWrite := false
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			t.Fatalf("GetAce %d: %v", i, err)
		}
		if ace.Header.AceType == callbackACEType && ace.Mask&managedWindowsWriteRights != 0 {
			foundCallbackWrite = true
		}
	}
	if !foundCallbackWrite {
		t.Fatal("SDDL did not produce a write-capable callback allow ACE")
	}
	oldSkipUnhandled := func(aceType uint8, inheritOnly, grantsWrite, administrative bool) error {
		if inheritOnly || aceType != managedWindowsAccessAllowedACEType {
			return nil
		}
		return validateManagedWindowsACE(aceType, false, grantsWrite, administrative)
	}
	if err := validateManagedWindowsDescriptorWithACEValidator(descriptor, oldSkipUnhandled); err != nil {
		t.Fatalf("old skip behavior rejected callback ACE: %v", err)
	}
	if err := validateManagedWindowsDescriptor(descriptor); err == nil {
		t.Fatal("callback allow write ACE was accepted")
	}
}

func TestManagedWindowsSnapshotRevalidatesAfterRead(t *testing.T) {
	initial := managedWindowsSnapshot{
		attributes: windows.FILE_ATTRIBUTE_NORMAL,
		size:       3,
		lastWrite:  windows.Filetime{LowDateTime: 100},
		security:   "protected-admin",
	}
	tests := []struct {
		name   string
		mutate func(*managedWindowsSnapshot)
	}{
		{name: "stable"},
		{name: "attributes", mutate: func(snapshot *managedWindowsSnapshot) { snapshot.attributes = windows.FILE_ATTRIBUTE_REPARSE_POINT }},
		{name: "size", mutate: func(snapshot *managedWindowsSnapshot) { snapshot.size++ }},
		{name: "modification time", mutate: func(snapshot *managedWindowsSnapshot) { snapshot.lastWrite.LowDateTime++ }},
		{name: "security descriptor", mutate: func(snapshot *managedWindowsSnapshot) { snapshot.security = "changed" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			post := initial
			if test.mutate != nil {
				test.mutate(&post)
			}
			inspections := 0
			data, err := readManagedWindowsSnapshot(strings.NewReader("abc"), initial, func() (managedWindowsSnapshot, error) {
				inspections++
				return post, nil
			})
			if inspections != 1 {
				t.Fatalf("post-read inspections = %d, want 1", inspections)
			}
			if test.mutate == nil {
				if err != nil || string(data) != "abc" {
					t.Fatalf("stable snapshot = %q, %v", data, err)
				}
			} else if !errors.Is(err, errManagedPolicyChanged) {
				t.Fatalf("changed snapshot error = %v, want errManagedPolicyChanged", err)
			}
		})
	}
}

func TestManagedWindowsFileAttributesRejectReparsePoints(t *testing.T) {
	if err := validateManagedWindowsFileAttributes(windows.FILE_ATTRIBUTE_NORMAL); err != nil {
		t.Fatalf("regular file: %v", err)
	}
	if err := validateManagedWindowsFileAttributes(windows.FILE_ATTRIBUTE_REPARSE_POINT); err == nil {
		t.Fatal("reparse point accepted")
	}
	if err := validateManagedWindowsFileAttributes(windows.FILE_ATTRIBUTE_DIRECTORY); err == nil {
		t.Fatal("directory accepted")
	}
}

func TestSecurePolicyWindowsReaderRejectsOversizeBeforeReading(t *testing.T) {
	path := filepath.Join(t.TempDir(), "managed-hooks.yaml")
	if err := os.WriteFile(path, make([]byte, maxManagedPolicyBytes+1), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := LoadManagedPolicy(LoadOptions{ManagedPath: path})
	assertManagedPolicySizeError(t, err, path)
}

func TestSecurePolicyWindowsReaderEnforcesRealSecurityBoundary(t *testing.T) {
	currentSID := currentWindowsUserSID(t).String()
	tests := []struct {
		name    string
		sddl    string
		wantErr bool
	}{
		{
			name: "protected administrator policy",
			sddl: "O:BAD:P(A;;FA;;;BA)(A;;FA;;;SY)(A;;FR;;;" + currentSID + ")",
		},
		{
			name:    "weak current user dacl",
			sddl:    "O:BAD:P(A;;FA;;;BA)(A;;FA;;;SY)(A;;FA;;;" + currentSID + ")",
			wantErr: true,
		},
		{
			name:    "unprotected dacl",
			sddl:    "O:BAD:(A;;FA;;;BA)(A;;FA;;;SY)(A;;FR;;;" + currentSID + ")",
			wantErr: true,
		},
		{
			name:    "current user owner",
			sddl:    "O:" + currentSID + "D:P(A;;FA;;;BA)(A;;FA;;;SY)(A;;FR;;;" + currentSID + ")",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "managed-hooks.yaml")
			if err := os.WriteFile(path, []byte("mode: additive\nhooks: {}\n"), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			applyManagedWindowsSecurity(t, path, test.sddl)

			policy, err := LoadManagedPolicy(LoadOptions{ManagedPath: path})
			if test.wantErr {
				if !errors.Is(err, ErrManagedPolicy) {
					t.Fatalf("LoadManagedPolicy error = %v, want ErrManagedPolicy", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadManagedPolicy: %v", err)
			}
			if policy == nil || policy.Mode != ManagedModeAdditive {
				t.Fatalf("policy = %#v", policy)
			}
		})
	}
}

func TestSecurePolicyWindowsReaderRejectsRealReparsePath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.yaml")
	if err := os.WriteFile(target, []byte("mode: additive\nhooks: {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	currentSID := currentWindowsUserSID(t).String()
	applyManagedWindowsSecurity(t, target, "O:BAD:P(A;;FA;;;BA)(A;;FA;;;SY)(A;;FR;;;"+currentSID+")")
	link := filepath.Join(dir, "managed-hooks.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	_, err := LoadManagedPolicy(LoadOptions{ManagedPath: link})
	if !errors.Is(err, ErrManagedPolicy) {
		t.Fatalf("reparse path error = %v, want ErrManagedPolicy", err)
	}
}

func TestSecurePolicyWindowsReaderDeniesConcurrentWriteOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "managed-hooks.yaml")
	if err := os.WriteFile(path, []byte("mode: additive\nhooks: {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	currentSID := currentWindowsUserSID(t).String()
	applyManagedWindowsSecurity(t, path, "O:BAD:P(A;;FA;;;BA)(A;;FA;;;SY)(A;;FR;;;"+currentSID+")")

	data, err := secureReadManagedFileWithSnapshotReader(path, func(
		reader io.Reader,
		expectedSize uint64,
		revalidate func() error,
	) ([]byte, error) {
		pathPtr, ptrErr := windows.UTF16PtrFromString(path)
		if ptrErr != nil {
			t.Fatalf("UTF16PtrFromString: %v", ptrErr)
		}
		handle, openErr := windows.CreateFile(
			pathPtr,
			windows.GENERIC_WRITE,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			nil,
			windows.OPEN_EXISTING,
			windows.FILE_ATTRIBUTE_NORMAL,
			0,
		)
		if openErr == nil {
			_ = windows.CloseHandle(handle)
			t.Fatal("concurrent write handle opened while managed policy handle was held")
		}
		if !errors.Is(openErr, windows.ERROR_SHARING_VIOLATION) {
			t.Fatalf("concurrent write error = %v, want ERROR_SHARING_VIOLATION", openErr)
		}
		return readManagedPolicySnapshot(reader, expectedSize, revalidate)
	})
	if err != nil {
		t.Fatalf("secureReadManagedFileWithSnapshotReader: %v", err)
	}
	if string(data) != "mode: additive\nhooks: {}\n" {
		t.Fatalf("data = %q", data)
	}
}

func TestManagedDefaultPolicyPathWindows(t *testing.T) {
	programData, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil {
		t.Fatalf("KnownFolderPath: %v", err)
	}
	path, err := defaultManagedPolicyPath()
	if err != nil {
		t.Fatalf("defaultManagedPolicyPath: %v", err)
	}
	want := filepath.Join(programData, "ratchet", "managed-hooks.yaml")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func currentWindowsUserSID(t *testing.T) *windows.SID {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("GetTokenUser: %v", err)
	}
	return user.User.Sid
}

func applyManagedWindowsSecurity(t *testing.T, path, sddl string) {
	t.Helper()
	descriptor, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		t.Fatalf("SecurityDescriptorFromString: %v", err)
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		t.Fatalf("Owner: %v", err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatalf("DACL: %v", err)
	}
	control, _, err := descriptor.Control()
	if err != nil {
		t.Fatalf("Control: %v", err)
	}
	securityInformation := windows.SECURITY_INFORMATION(
		windows.OWNER_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION,
	)
	if control&windows.SE_DACL_PROTECTED != 0 {
		securityInformation |= windows.PROTECTED_DACL_SECURITY_INFORMATION
	} else {
		securityInformation |= windows.UNPROTECTED_DACL_SECURITY_INFORMATION
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		securityInformation,
		owner,
		nil,
		dacl,
		nil,
	); err != nil {
		t.Fatalf("SetNamedSecurityInfo: %v", err)
	}
}
