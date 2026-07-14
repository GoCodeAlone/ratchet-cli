//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package acpclient

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func TestStoreProcessLockConcurrentFirstOpen(t *testing.T) {
	const contenders = 64
	for iteration := range 20 {
		logicalPath := filepath.Join(t.TempDir(), "events", "session.events.jsonl.lock")
		start := make(chan struct{})
		errs := make(chan error, contenders)
		var wg sync.WaitGroup
		for range contenders {
			wg.Go(func() {
				<-start
				release, err := acquireStoreFileLock(logicalPath)
				if err == nil {
					err = release()
				}
				errs <- err
			})
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("iteration %d: acquire first lock: %v", iteration, err)
			}
		}
	}
}

func TestSessionStoreCrossProcessLockBlocks(t *testing.T) {
	runSessionStoreCrossProcessLockBlocks(t, holdSessionStoreTestLock)
}

func TestStoreLockDoesNotChangeExistingParentPermissions(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("Chmod parent: %v", err)
	}
	logicalPath := filepath.Join(dir, "sessions.json.lock")
	release, err := acquireStoreFileLock(logicalPath)
	if err != nil {
		t.Fatalf("acquireStoreFileLock: %v", err)
	}
	defer func() { _ = release() }()

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat parent: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("parent mode = %o, want unchanged 755", got)
	}
	lockDir := filepath.Join(dir, ".ratchet-locks")
	info, err = os.Stat(lockDir)
	if err != nil {
		t.Fatalf("Stat dedicated lock directory: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("dedicated lock directory mode = %o, want 700", got)
	}
	entries, err := os.ReadDir(lockDir)
	if err != nil {
		t.Fatalf("ReadDir dedicated lock directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("dedicated lock entries = %d, want 1", len(entries))
	}
	info, err = entries[0].Info()
	if err != nil {
		t.Fatalf("lock entry info: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("physical lock mode = %o, want 600", got)
	}
}

func TestStoreLockRejectsUnsafeDedicatedLockPath(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		dir := t.TempDir()
		target := t.TempDir()
		if err := os.Chmod(target, 0o755); err != nil {
			t.Fatalf("Chmod target: %v", err)
		}
		if err := os.Symlink(target, filepath.Join(dir, storeLockDirectoryName)); err != nil {
			t.Fatalf("Symlink dedicated lock directory: %v", err)
		}
		if release, err := acquireStoreFileLock(filepath.Join(dir, "sessions.json.lock")); !errors.Is(err, ErrStoreLockPathUnsafe) {
			_ = release()
			t.Fatalf("acquireStoreFileLock error = %v, want ErrStoreLockPathUnsafe", err)
		}
		info, err := os.Stat(target)
		if err != nil {
			t.Fatalf("Stat target: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o755 {
			t.Fatalf("symlink target mode = %o, want unchanged 755", got)
		}
	})

	t.Run("non-directory", func(t *testing.T) {
		dir := t.TempDir()
		lockPath := filepath.Join(dir, storeLockDirectoryName)
		if err := os.WriteFile(lockPath, []byte("not a directory\n"), 0o644); err != nil {
			t.Fatalf("WriteFile dedicated lock path: %v", err)
		}
		if release, err := acquireStoreFileLock(filepath.Join(dir, "sessions.json.lock")); !errors.Is(err, ErrStoreLockPathUnsafe) {
			_ = release()
			t.Fatalf("acquireStoreFileLock error = %v, want ErrStoreLockPathUnsafe", err)
		}
		info, err := os.Lstat(lockPath)
		if err != nil {
			t.Fatalf("Lstat dedicated lock path: %v", err)
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("dedicated lock path mode = %v, want regular file", info.Mode())
		}
	})
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
	physicalLockPath := requireStoreLockPhysicalPath(t, path+".lock")
	for _, privatePath := range []string{path, filepath.Dir(physicalLockPath), physicalLockPath} {
		info, err := os.Stat(privatePath)
		if err != nil {
			t.Fatalf("Stat %s: %v", privatePath, err)
		}
		want := os.FileMode(0o600)
		if info.IsDir() {
			want = 0o700
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("mode %s = %o, want %o", privatePath, got, want)
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

func TestEventLogReplacementAndLockAreOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sessions.json"))
	if err := store.WriteEventLog("private", []EventLogLine{{
		Direction: EventDirectionInbound,
		Message:   json.RawMessage(`{"jsonrpc":"2.0","id":1,"result":{}}`),
	}}); err != nil {
		t.Fatalf("WriteEventLog: %v", err)
	}
	path := store.eventLogPath("private")
	physicalLockPath := requireStoreLockPhysicalPath(t, path+".lock")
	for _, privatePath := range []string{filepath.Dir(path), path, filepath.Dir(physicalLockPath), physicalLockPath} {
		info, err := os.Stat(privatePath)
		if err != nil {
			t.Fatalf("Stat %s: %v", privatePath, err)
		}
		want := os.FileMode(0o600)
		if info.IsDir() {
			want = 0o700
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("mode %s = %o, want %o", privatePath, got, want)
		}
	}
}

func TestBackgroundWorkerLeaseIsOwnerOnly(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	manager := NewBackgroundManager(store, NewBackgroundStore(filepath.Join(filepath.Dir(store.Path()), "background.json")), NewBackgroundAudit(filepath.Join(filepath.Dir(store.Path()), "background-audit.jsonl")), BackgroundManagerOptions{})
	t.Cleanup(manager.Shutdown)
	path := manager.workerLeasePath("private")
	release, acquired, err := tryStoreFileLock(path)
	if err != nil || !acquired {
		t.Fatalf("tryStoreFileLock = %t, %v", acquired, err)
	}
	defer func() { _ = release() }()
	physicalLockPath := requireStoreLockPhysicalPath(t, path)
	for _, privatePath := range []string{filepath.Dir(path), filepath.Dir(physicalLockPath), physicalLockPath} {
		info, err := os.Stat(privatePath)
		if err != nil {
			t.Fatalf("Stat %s: %v", privatePath, err)
		}
		want := os.FileMode(0o600)
		if info.IsDir() {
			want = 0o700
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("mode %s = %o, want %o", privatePath, got, want)
		}
	}
}

func holdSessionStoreTestLock(path string) (func() error, error) {
	physicalPath, err := storeLockPhysicalPath(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(physicalPath), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(physicalPath, os.O_CREATE|os.O_RDWR, 0o600)
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
