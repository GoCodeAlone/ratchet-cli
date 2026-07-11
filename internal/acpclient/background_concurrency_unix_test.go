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
	fifo := filepath.Join(dir, "audit.fifo")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatalf("Mkfifo: %v", err)
	}
	audit.path = fifo
	manager := NewBackgroundManager(backgroundSessionStore(t), store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
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
	eventuallyBackground(t, func() bool {
		persisted, err := store.Get(policy.SessionID)
		return err == nil && persisted.State == BackgroundStateRunning && persisted.Outcome == BackgroundOutcomeResumed
	})
	manager.Shutdown()
	unblocker := openBackgroundFIFO(t, fifo)
	defer unblocker.Close() //nolint:errcheck
	if err := <-resumeDone; !errors.Is(err, ErrBackgroundManagerClosed) {
		t.Fatalf("Resume error = %v, want ErrBackgroundManagerClosed", err)
	}
	persisted, err := store.Get(policy.SessionID)
	if err != nil {
		t.Fatalf("Get policy: %v", err)
	}
	if persisted.Enabled || persisted.State != BackgroundStateDisabled || persisted.Outcome != BackgroundOutcomeStopped {
		t.Fatalf("policy after shutdown race = %#v, want disabled/stopped", persisted)
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
