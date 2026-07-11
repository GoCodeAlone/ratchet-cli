//go:build unix

package acpclient

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackgroundWriteFileAtomicReplacesOwnerOnlyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "background.json")
	for _, data := range [][]byte{[]byte("first\n"), []byte("second\n")} {
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
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("mode = %o, want 600", got)
		}
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".background.json.*.tmp"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(temps) != 0 {
		t.Fatalf("temporary files remain: %#v", temps)
	}
}
