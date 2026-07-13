//go:build windows

package acpclient

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestBackgroundWindowsStoreCrossProcessLockBlocks(t *testing.T) {
	runSessionStoreCrossProcessLockBlocks(t, holdSessionStoreTestLock)
}

func TestBackgroundWindowsStoreLockAndReplacementArePrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
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
	assertBackgroundWindowsPrivateDACL(t, dir)
	assertBackgroundWindowsPrivateDACL(t, path)
	assertBackgroundWindowsPrivateDACL(t, path+".lock")
}

func holdSessionStoreTestLock(path string) (func() error, error) {
	if err := backgroundEnsurePrivateDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := backgroundSetPrivateACL(path); err != nil {
		_ = f.Close()
		return nil, err
	}
	var overlapped windows.Overlapped
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() error {
		return errors.Join(windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &overlapped), f.Close())
	}, nil
}
