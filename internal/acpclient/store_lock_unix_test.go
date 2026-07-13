//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package acpclient

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSessionStoreCrossProcessLockBlocks(t *testing.T) {
	runSessionStoreCrossProcessLockBlocks(t, holdSessionStoreTestLock)
}

func TestSessionStoreReplacementAndLockAreOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	store := NewStore(path)
	for i := range 20 {
		if err := store.Upsert(SessionRecord{ID: "session-a", Summary: string(rune('a' + i%26))}); err != nil {
			t.Fatalf("Upsert session-a revision %d: %v", i, err)
		}
		if err := store.Upsert(SessionRecord{ID: "session-b", Summary: string(rune('A' + i%26))}); err != nil {
			t.Fatalf("Upsert session-b revision %d: %v", i, err)
		}
		var data storeFile
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile revision %d: %v", i, err)
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			t.Fatalf("Unmarshal revision %d: %v", i, err)
		}
		if len(data.Sessions) != 2 {
			t.Fatalf("revision %d sessions = %#v, want two complete records", i, data.Sessions)
		}
	}
	for _, privatePath := range []string{path, path + ".lock"} {
		info, err := os.Stat(privatePath)
		if err != nil {
			t.Fatalf("Stat %s: %v", privatePath, err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("mode %s = %o, want 600", privatePath, got)
		}
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".sessions.json.*.tmp"))
	if err != nil {
		t.Fatalf("Glob temp files: %v", err)
	}
	if len(temps) != 0 {
		t.Fatalf("temporary session files remain: %#v", temps)
	}
}

func holdSessionStoreTestLock(path string) (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() error {
		return errors.Join(unix.Flock(int(f.Fd()), unix.LOCK_UN), f.Close())
	}, nil
}
