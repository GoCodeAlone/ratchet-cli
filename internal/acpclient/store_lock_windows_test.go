//go:build windows

package acpclient

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestBackgroundWindowsStoreCrossProcessLockBlocks(t *testing.T) {
	runSessionStoreCrossProcessLockBlocks(t, holdSessionStoreTestLock)
}

func TestBackgroundWindowsStoreLockDoesNotRewriteExistingParentDACL(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "custom")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir parent: %v", err)
	}
	before := backgroundWindowsSecurityDescriptor(t, dir)
	logicalPath := filepath.Join(dir, "sessions.json.lock")
	release, err := acquireStoreFileLock(logicalPath)
	if err != nil {
		t.Fatalf("acquireStoreFileLock: %v", err)
	}
	defer func() { _ = release() }()
	if after := backgroundWindowsSecurityDescriptor(t, dir); after != before {
		t.Fatalf("parent DACL changed:\nbefore: %s\nafter:  %s", before, after)
	}
	lockDir := filepath.Join(dir, ".ratchet-locks")
	assertBackgroundWindowsPrivateDACL(t, lockDir)
	entries, err := os.ReadDir(lockDir)
	if err != nil {
		t.Fatalf("ReadDir dedicated lock directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("dedicated lock entries = %d, want 1", len(entries))
	}
	assertBackgroundWindowsPrivateDACL(t, filepath.Join(lockDir, entries[0].Name()))
}

func TestBackgroundWindowsStoreLockRejectsUnsafeDedicatedLockPath(t *testing.T) {
	t.Run("reparse-point", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(t.TempDir(), "target")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("Mkdir target: %v", err)
		}
		before := backgroundWindowsSecurityDescriptor(t, target)
		lockPath := filepath.Join(dir, storeLockDirectoryName)
		if err := os.Symlink(target, lockPath); err != nil {
			t.Skipf("directory symlink unavailable: %v", err)
		}
		pathPtr, err := windows.UTF16PtrFromString(lockPath)
		if err != nil {
			t.Fatalf("UTF16PtrFromString: %v", err)
		}
		attributes, err := windows.GetFileAttributes(pathPtr)
		if err != nil {
			t.Fatalf("GetFileAttributes: %v", err)
		}
		if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
			t.Fatal("test symlink is not a reparse point")
		}
		if release, err := acquireStoreFileLock(filepath.Join(dir, "sessions.json.lock")); !errors.Is(err, ErrStoreLockPathUnsafe) {
			_ = release()
			t.Fatalf("acquireStoreFileLock error = %v, want ErrStoreLockPathUnsafe", err)
		}
		if after := backgroundWindowsSecurityDescriptor(t, target); after != before {
			t.Fatalf("reparse target DACL changed:\nbefore: %s\nafter:  %s", before, after)
		}
	})

	t.Run("non-directory", func(t *testing.T) {
		dir := t.TempDir()
		lockPath := filepath.Join(dir, storeLockDirectoryName)
		if err := os.WriteFile(lockPath, []byte("not a directory\r\n"), 0o600); err != nil {
			t.Fatalf("WriteFile dedicated lock path: %v", err)
		}
		if release, err := acquireStoreFileLock(filepath.Join(dir, "sessions.json.lock")); !errors.Is(err, ErrStoreLockPathUnsafe) {
			_ = release()
			t.Fatalf("acquireStoreFileLock error = %v, want ErrStoreLockPathUnsafe", err)
		}
	})
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
	physicalLockPath := requireStoreLockPhysicalPath(t, path+".lock")
	assertBackgroundWindowsPrivateDACL(t, filepath.Dir(physicalLockPath))
	assertBackgroundWindowsPrivateDACL(t, physicalLockPath)
}

func TestBackgroundWindowsEventLogLockAndReplacementArePrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	store := NewStore(filepath.Join(dir, "sessions.json"))
	if err := store.WriteEventLog("private", []EventLogLine{{
		Direction: EventDirectionInbound,
		Message:   json.RawMessage(`{"jsonrpc":"2.0","id":1,"result":{}}`),
	}}); err != nil {
		t.Fatalf("WriteEventLog: %v", err)
	}
	path := store.eventLogPath("private")
	assertBackgroundWindowsPrivateDACL(t, filepath.Dir(path))
	assertBackgroundWindowsPrivateDACL(t, path)
	physicalLockPath := requireStoreLockPhysicalPath(t, path+".lock")
	assertBackgroundWindowsPrivateDACL(t, filepath.Dir(physicalLockPath))
	assertBackgroundWindowsPrivateDACL(t, physicalLockPath)
}

func TestBackgroundWindowsOwnerLeaseIsExclusiveAndPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	store := NewStore(filepath.Join(dir, "sessions.json"))
	lease, err := store.AcquireOwnerLease(OwnerLock{SessionID: "private", PID: os.Getpid(), StartedAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("AcquireOwnerLease: %v", err)
	}
	defer func() { _ = lease.Release() }()
	other, err := NewStore(store.Path()).AcquireOwnerLease(OwnerLock{SessionID: "private", PID: os.Getpid(), StartedAt: time.Now().UTC()})
	if other != nil {
		_ = other.Release()
	}
	if !errors.Is(err, ErrOwnerLeaseBusy) {
		t.Fatalf("second owner lease error = %v, want ErrOwnerLeaseBusy", err)
	}
	assertBackgroundWindowsPrivateDACL(t, filepath.Dir(store.ownerPath("private")))
	assertBackgroundWindowsPrivateDACL(t, store.ownerPath("private"))
	for _, logicalPath := range []string{store.ownerLeasePath("private"), store.ownerClaimPath("private")} {
		physicalLockPath := requireStoreLockPhysicalPath(t, logicalPath)
		assertBackgroundWindowsPrivateDACL(t, filepath.Dir(physicalLockPath))
		assertBackgroundWindowsPrivateDACL(t, physicalLockPath)
	}
}

func TestBackgroundWindowsWorkerLeaseIsPrivate(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "private", "sessions.json"))
	manager := NewBackgroundManager(store, NewBackgroundStore(filepath.Join(filepath.Dir(store.Path()), "background.json")), NewBackgroundAudit(filepath.Join(filepath.Dir(store.Path()), "background-audit.jsonl")), BackgroundManagerOptions{})
	t.Cleanup(manager.Shutdown)
	path := manager.workerLeasePath("private")
	release, acquired, err := tryStoreFileLock(path)
	if err != nil || !acquired {
		t.Fatalf("tryStoreFileLock = %t, %v", acquired, err)
	}
	defer func() { _ = release() }()
	physicalLockPath := requireStoreLockPhysicalPath(t, path)
	assertBackgroundWindowsPrivateDACL(t, filepath.Dir(path))
	assertBackgroundWindowsPrivateDACL(t, filepath.Dir(physicalLockPath))
	assertBackgroundWindowsPrivateDACL(t, physicalLockPath)
}

func TestBackgroundWindowsPolicyTransitionAuditLocksArePrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	now := time.Date(2026, 7, 13, 20, 30, 0, 0, time.UTC)
	policy := BackgroundPolicy{
		SessionID: "private", Profile: "fixture", DescriptorHash: "hash",
		PolicyVersion: BackgroundPolicyVersion, AcknowledgedAt: now,
		Enabled: true, State: BackgroundStateRunning, Outcome: BackgroundOutcomeStarted, UpdatedAt: now,
	}
	if err := store.Upsert(policy); err != nil {
		t.Fatalf("Upsert policy: %v", err)
	}
	if err := store.putTransition(backgroundTransition{Policy: policy, Action: BackgroundAuditStart}); err != nil {
		t.Fatalf("putTransition: %v", err)
	}
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	if err := audit.Append(BackgroundAuditRecord{
		At: now, Action: BackgroundAuditStart, SessionID: policy.SessionID,
		Profile: policy.Profile, DescriptorHash: policy.DescriptorHash, Outcome: policy.Outcome,
	}); err != nil {
		t.Fatalf("Append audit: %v", err)
	}
	for _, path := range []string{
		dir,
		store.Path(),
		store.transitionPath(),
		audit.Path(),
	} {
		assertBackgroundWindowsPrivateDACL(t, path)
	}
	for _, logicalPath := range []string{store.Path() + ".lock", store.transitionPath() + ".lock", audit.Path() + ".lock"} {
		physicalLockPath := requireStoreLockPhysicalPath(t, logicalPath)
		assertBackgroundWindowsPrivateDACL(t, filepath.Dir(physicalLockPath))
		assertBackgroundWindowsPrivateDACL(t, physicalLockPath)
	}
}

func holdSessionStoreTestLock(path string) (func() error, error) {
	physicalPath, err := storeLockPhysicalPath(path)
	if err != nil {
		return nil, err
	}
	if err := backgroundEnsureOwnedPrivateDir(filepath.Dir(physicalPath)); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(physicalPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := backgroundSetPrivateACL(physicalPath); err != nil {
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
