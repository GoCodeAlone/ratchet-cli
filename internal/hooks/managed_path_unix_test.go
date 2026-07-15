//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package hooks

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestManagedUnixMetadataRequiresRootOwnedRegularFile(t *testing.T) {
	tests := []struct {
		name    string
		uid     uint32
		mode    uint32
		wantErr bool
	}{
		{name: "root read only", uid: 0, mode: unix.S_IFREG | 0o400},
		{name: "root world readable", uid: 0, mode: unix.S_IFREG | 0o644},
		{name: "non root owner", uid: 501, mode: unix.S_IFREG | 0o600, wantErr: true},
		{name: "group writable", uid: 0, mode: unix.S_IFREG | 0o620, wantErr: true},
		{name: "other writable", uid: 0, mode: unix.S_IFREG | 0o602, wantErr: true},
		{name: "directory", uid: 0, mode: unix.S_IFDIR | 0o755, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateManagedUnixMetadata(test.uid, test.mode)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateManagedUnixMetadata(%d, %#o) = %v, wantErr %v", test.uid, test.mode, err, test.wantErr)
			}
		})
	}
}

func TestManagedUnixSnapshotRevalidatesAfterRead(t *testing.T) {
	initial := managedUnixSnapshot{
		uid:     0,
		mode:    unix.S_IFREG | 0o400,
		size:    3,
		modTime: time.Unix(100, 0),
	}
	tests := []struct {
		name   string
		mutate func(*managedUnixSnapshot)
	}{
		{name: "stable"},
		{name: "size", mutate: func(snapshot *managedUnixSnapshot) { snapshot.size++ }},
		{name: "modification time", mutate: func(snapshot *managedUnixSnapshot) { snapshot.modTime = snapshot.modTime.Add(time.Second) }},
		{name: "owner", mutate: func(snapshot *managedUnixSnapshot) { snapshot.uid = 501 }},
		{name: "mode", mutate: func(snapshot *managedUnixSnapshot) { snapshot.mode |= 0o020 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			post := initial
			if test.mutate != nil {
				test.mutate(&post)
			}
			inspections := 0
			data, err := readManagedUnixSnapshot(strings.NewReader("abc"), initial, func() (managedUnixSnapshot, error) {
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

func TestManagedUnixSnapshotChecksPlatformMutationACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "managed-hooks.yaml")
	if err := os.WriteFile(path, []byte("mode: additive\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer file.Close()

	wantErr := errors.New("mutation ACL")
	original := managedPolicyValidatePlatformACL
	t.Cleanup(func() { managedPolicyValidatePlatformACL = original })
	managedPolicyValidatePlatformACL = func(gotPath string, gotFD int) error {
		if gotPath != path {
			t.Fatalf("ACL path = %q, want %q", gotPath, path)
		}
		if gotFD != int(file.Fd()) {
			t.Fatalf("ACL fd = %d, want %d", gotFD, file.Fd())
		}
		return wantErr
	}

	_, err = inspectManagedUnixSnapshot(file, int(file.Fd()))
	if !errors.Is(err, wantErr) {
		t.Fatalf("inspectManagedUnixSnapshot error = %v, want mutation ACL", err)
	}
}

func TestManagedDarwinPolicyRejectsMutationACL(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin ACL contract")
	}
	path := filepath.Join(t.TempDir(), "managed-hooks.yaml")
	if err := os.WriteFile(path, []byte("mode: additive\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	command := exec.Command("/bin/chmod", "+a", "everyone allow write", path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("install mutation ACL: %v\n%s", err, output)
	}
	t.Cleanup(func() { _ = exec.Command("/bin/chmod", "-N", path).Run() })

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer file.Close()
	if err := validateOpenedPlatformMutationACL(path, int(file.Fd())); err == nil || !strings.Contains(err.Error(), "ACL") {
		t.Fatalf("mutation ACL error = %v, want rejection", err)
	}
}

func TestSecurePolicyUnixReaderRejectsSymlinkWithoutFollowing(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.yaml")
	if err := os.WriteFile(target, []byte("mode: additive\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	link := filepath.Join(dir, "managed-hooks.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	_, err := secureReadManagedFile(link)
	wantErr := error(unix.ELOOP)
	if runtime.GOOS == "freebsd" {
		wantErr = unix.EMLINK
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("symlink error = %v, want %v", err, wantErr)
	}
}

func TestSecurePolicyUnixReaderRejectsNonRegularFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadManagedPolicy(LoadOptions{ManagedPath: dir}); !errors.Is(err, ErrManagedPolicy) {
		t.Fatalf("directory error = %v, want ErrManagedPolicy", err)
	}
}

func TestSecurePolicyUnixReaderPreservesMissingFile(t *testing.T) {
	_, err := secureReadManagedFile(filepath.Join(t.TempDir(), "missing.yaml"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want os.ErrNotExist", err)
	}
}

func TestSecurePolicyUnixReaderRejectsOversizeBeforeReading(t *testing.T) {
	path := filepath.Join(t.TempDir(), "managed-hooks.yaml")
	if err := os.WriteFile(path, make([]byte, maxManagedPolicyBytes+1), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := LoadManagedPolicy(LoadOptions{ManagedPath: path})
	assertManagedPolicySizeError(t, err, path)
}

func TestManagedDefaultPolicyPathUnix(t *testing.T) {
	path, err := defaultManagedPolicyPath()
	if err != nil {
		t.Fatalf("defaultManagedPolicyPath: %v", err)
	}
	want := "/etc/ratchet/managed-hooks.yaml"
	if runtime.GOOS == "darwin" {
		want = "/Library/Application Support/ratchet/managed-hooks.yaml"
	}
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}
