//go:build unix

package acpclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestBackgroundManagerBlockedAuditDoesNotPreventShutdownCancellation(t *testing.T) {
	manager, store, audit, workerCanceled := newBlockedAuditManager(t)
	if _, err := manager.Start("session-2", "fixture", true); err != nil {
		t.Fatalf("Start session-2: %v", err)
	}
	fifo := filepath.Join(t.TempDir(), "audit.fifo")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatalf("Mkfifo: %v", err)
	}
	audit.path = fifo
	startDone := make(chan error, 1)
	go func() {
		_, err := manager.Start("session-1", "fixture", true)
		startDone <- err
	}()
	eventuallyBackground(t, func() bool {
		policy, err := store.Get("session-1")
		return err == nil && policy.State == BackgroundStateRunning
	})
	shutdownDone := make(chan struct{})
	go func() {
		manager.Shutdown()
		close(shutdownDone)
	}()
	select {
	case <-workerCanceled:
	case <-time.After(500 * time.Millisecond):
		unblocker := openBackgroundFIFO(t, fifo)
		_ = unblocker.Close()
		t.Fatal("Shutdown could not cancel active worker while another session audit was blocked")
	}
	returnedBeforePersistence := false
	select {
	case <-shutdownDone:
		returnedBeforePersistence = true
	case <-time.After(100 * time.Millisecond):
	}
	unblocker := openBackgroundFIFO(t, fifo)
	defer unblocker.Close() //nolint:errcheck
	select {
	case <-shutdownDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown did not complete after audit unblocked")
	}
	if err := <-startDone; err != nil && !errors.Is(err, ErrBackgroundManagerClosed) {
		t.Fatalf("blocked Start error = %v", err)
	}
	if returnedBeforePersistence {
		t.Fatal("Shutdown returned before admitted Start persistence completed")
	}
}

func TestBackgroundManagerRejectsConcurrentTransitionOnSameSession(t *testing.T) {
	manager, store, audit, _ := newBlockedAuditManager(t)
	fifo := filepath.Join(t.TempDir(), "audit.fifo")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatalf("Mkfifo: %v", err)
	}
	audit.path = fifo
	firstDone := make(chan error, 1)
	go func() {
		_, err := manager.Start("session-1", "fixture", true)
		firstDone <- err
	}()
	eventuallyBackground(t, func() bool {
		policy, err := store.Get("session-1")
		return err == nil && policy.State == BackgroundStateRunning
	})
	secondDone := make(chan error, 1)
	go func() {
		_, err := manager.Start("session-1", "fixture", true)
		secondDone <- err
	}()
	select {
	case err := <-secondDone:
		if !errors.Is(err, ErrBackgroundTransitionBusy) {
			unblocker := openBackgroundFIFO(t, fifo)
			_ = unblocker.Close()
			t.Fatalf("second Start error = %v, want ErrBackgroundTransitionBusy", err)
		}
	case <-time.After(500 * time.Millisecond):
		unblocker := openBackgroundFIFO(t, fifo)
		_ = unblocker.Close()
		t.Fatal("second Start blocked behind first transition")
	}
	unblocker := openBackgroundFIFO(t, fifo)
	defer unblocker.Close() //nolint:errcheck
	if err := <-firstDone; err != nil {
		t.Fatalf("first Start: %v", err)
	}
	manager.Shutdown()
}

func TestBackgroundManagerResumeShutdownBeforeLaunchDisablesPolicy(t *testing.T) {
	dir := t.TempDir()
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	policy := backgroundRunnablePolicy(backgroundTestNow())
	if err := store.Upsert(policy); err != nil {
		t.Fatalf("Upsert policy: %v", err)
	}
	var releaseAuditLock func() error
	auditLockReleased := false
	defer func() {
		if releaseAuditLock != nil && !auditLockReleased {
			_ = releaseAuditLock()
		}
	}()
	resolverReached := make(chan struct{})
	manager := NewBackgroundManager(backgroundSessionStore(t), store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			var err error
			releaseAuditLock, err = acquireStoreFileLock(audit.Path() + ".lock")
			if err != nil {
				return ResolvedBackgroundProfile{}, err
			}
			close(resolverReached)
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(context.Context, *Store, AgentSpec, RunOptions, string, WatchOptions, func(WatchCycle)) (WatchResult, error) {
			t.Fatal("watcher launched after shutdown")
			return WatchResult{}, nil
		},
	})
	resumeDone := make(chan error, 1)
	go func() {
		resumeDone <- manager.Resume()
	}()
	<-resolverReached
	eventuallyBackground(t, func() bool {
		persisted, err := store.Get(policy.SessionID)
		return err == nil && persisted.State == BackgroundStateRunning && persisted.Outcome == BackgroundOutcomeResumed
	})
	shutdownDone := make(chan struct{})
	go func() {
		manager.Shutdown()
		close(shutdownDone)
	}()
	returnedEarly := false
	select {
	case <-shutdownDone:
		returnedEarly = true
	case <-time.After(100 * time.Millisecond):
	}
	if err := releaseAuditLock(); err != nil {
		t.Fatalf("release audit process lock: %v", err)
	}
	auditLockReleased = true
	if err := <-resumeDone; !errors.Is(err, ErrBackgroundManagerClosed) {
		t.Fatalf("Resume error = %v, want ErrBackgroundManagerClosed", err)
	}
	<-shutdownDone
	if returnedEarly {
		t.Fatal("Shutdown returned before resumed policy persistence completed")
	}
	persisted, err := store.Get(policy.SessionID)
	if err != nil {
		t.Fatalf("Get policy: %v", err)
	}
	if persisted.Enabled || persisted.State != BackgroundStateDisabled || persisted.Outcome != BackgroundOutcomeStopped {
		t.Fatalf("policy after shutdown race = %#v, want disabled/stopped", persisted)
	}
}

func TestBackgroundManagerStopOwnsTerminalPersistenceUntilWorkerJoin(t *testing.T) {
	dir := t.TempDir()
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	releaseWorker := make(chan struct{})
	manager := NewBackgroundManager(backgroundSessionStore(t), store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(context.Context, *Store, AgentSpec, RunOptions, string, WatchOptions, func(WatchCycle)) (WatchResult, error) {
			<-releaseWorker
			return WatchResult{}, errors.New("secret independent stop race")
		},
	})
	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := os.Remove(audit.Path()); err != nil {
		t.Fatalf("Remove audit: %v", err)
	}
	if err := unix.Mkfifo(audit.Path(), 0o600); err != nil {
		t.Fatalf("Mkfifo: %v", err)
	}
	stopDone := make(chan struct {
		status BackgroundStatus
		err    error
	}, 1)
	go func() {
		status, err := manager.Stop("session-1")
		stopDone <- struct {
			status BackgroundStatus
			err    error
		}{status: status, err: err}
	}()
	eventuallyBackground(t, func() bool {
		policy, err := store.Get("session-1")
		return err == nil && policy.State == BackgroundStateDisabled && policy.Outcome == BackgroundOutcomeStopped
	})
	close(releaseWorker)
	deadline := time.Now().Add(250 * time.Millisecond)
	var concurrentPolicy BackgroundPolicy
	for time.Now().Before(deadline) {
		policy, err := store.Get("session-1")
		if err != nil {
			t.Fatalf("Get while Stop owns transition: %v", err)
		}
		if policy.State == BackgroundStateError {
			concurrentPolicy = policy
			break
		}
		time.Sleep(time.Millisecond)
	}
	unblocker := openBackgroundFIFO(t, audit.Path())
	defer unblocker.Close() //nolint:errcheck
	result := <-stopDone
	if result.err != nil {
		t.Fatalf("Stop: %v", result.err)
	}
	if result.status.Enabled || result.status.State != BackgroundStateError || result.status.Outcome != BackgroundOutcomeWorkerError {
		t.Fatalf("Stop status = %#v, want disabled worker_error", result.status)
	}
	manager.Shutdown()
	if concurrentPolicy.State == BackgroundStateError {
		t.Fatalf("worker persisted concurrently with Stop: %#v", concurrentPolicy)
	}
}

func newBlockedAuditManager(t *testing.T) (*BackgroundManager, *BackgroundStore, *BackgroundAudit, <-chan struct{}) {
	t.Helper()
	dir := t.TempDir()
	sessions := NewStore(filepath.Join(dir, "sessions.json"))
	now := backgroundTestNow()
	for _, id := range []string{"session-1", "session-2"} {
		if err := sessions.Upsert(SessionRecord{ID: id, Agent: "fixture", Status: SessionStatusQueued, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatalf("Upsert %s: %v", id, err)
		}
	}
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	canceled := make(chan struct{}, 2)
	manager := NewBackgroundManager(sessions, store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
			<-ctx.Done()
			canceled <- struct{}{}
			return WatchResult{}, ctx.Err()
		},
	})
	t.Cleanup(manager.Shutdown)
	return manager, store, audit, canceled
}

func openBackgroundFIFO(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open FIFO: %v", err)
	}
	return f
}
