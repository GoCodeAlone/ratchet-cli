package acpclient

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

const (
	ownerLeaseHelperEnv    = "RATCHET_OWNER_LEASE_HELPER"
	ownerLeaseStorePathEnv = "RATCHET_OWNER_LEASE_STORE_PATH"
	ownerLeaseSessionIDEnv = "RATCHET_OWNER_LEASE_SESSION_ID"
	ownerLeaseReadyPathEnv = "RATCHET_OWNER_LEASE_READY_PATH"
)

func TestOwnerLeaseRejectsConcurrentClaimAndReleasesProjection(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "sessions.json")
	first := NewStore(storePath)
	second := NewStore(storePath)
	now := time.Date(2026, 7, 13, 18, 30, 0, 0, time.UTC)
	lease, err := first.AcquireOwnerLease(OwnerLock{SessionID: "leased", PID: 101, StartedAt: now})
	if err != nil {
		t.Fatalf("AcquireOwnerLease first: %v", err)
	}

	owner, err := second.Owner("leased")
	if err != nil {
		t.Fatalf("Owner while leased: %v", err)
	}
	if owner.PID != 101 {
		t.Fatalf("Owner PID = %d, want 101", owner.PID)
	}
	if _, err := second.AcquireOwnerLease(OwnerLock{SessionID: "leased", PID: 202, StartedAt: now.Add(time.Second)}); !errors.Is(err, ErrOwnerLeaseBusy) {
		t.Fatalf("AcquireOwnerLease second error = %v, want ErrOwnerLeaseBusy", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := second.Owner("leased"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Owner after release error = %v, want os.ErrNotExist", err)
	}
	replacement, err := second.AcquireOwnerLease(OwnerLock{SessionID: "leased", PID: 202, StartedAt: now.Add(time.Second)})
	if err != nil {
		t.Fatalf("AcquireOwnerLease replacement: %v", err)
	}
	if err := replacement.Release(); err != nil {
		t.Fatalf("Release replacement: %v", err)
	}
}

func TestRequestCancelActiveExecutionHoldsOwnerClaimThroughCommit(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 13, 22, 0, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID:        "cancel-race",
		Status:    SessionStatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	lease, err := store.AcquireOwnerLease(OwnerLock{
		SessionID: "cancel-race", PID: 101, Kind: OwnerKindExecution, StartedAt: now,
	})
	if errors.Is(err, ErrStoreProcessLockUnsupported) {
		t.Skip("cross-process owner leases are unsupported on this platform")
	}
	if err != nil {
		t.Fatalf("AcquireOwnerLease: %v", err)
	}

	mutationEntered := make(chan struct{})
	continueMutation := make(chan struct{})
	store.beforeMutation = func() {
		close(mutationEntered)
		<-continueMutation
	}
	cancelDone := make(chan error, 1)
	go func() {
		cancelDone <- store.RequestCancelActiveExecution("cancel-race", now.Add(time.Second))
	}()
	<-mutationEntered

	replacementDone := make(chan struct {
		lease *OwnerLease
		err   error
	}, 1)
	go func() {
		if err := lease.Release(); err != nil {
			replacementDone <- struct {
				lease *OwnerLease
				err   error
			}{err: err}
			return
		}
		replacement, err := store.AcquireOwnerLease(OwnerLock{
			SessionID: "cancel-race", PID: 202, Kind: OwnerKindSnapshot, StartedAt: now.Add(2 * time.Second),
		})
		replacementDone <- struct {
			lease *OwnerLease
			err   error
		}{lease: replacement, err: err}
	}()
	select {
	case replacement := <-replacementDone:
		if replacement.lease != nil {
			_ = replacement.lease.Release()
		}
		t.Fatalf("owner changed before cancellation committed: %v", replacement.err)
	case <-time.After(100 * time.Millisecond):
	}
	close(continueMutation)
	if err := <-cancelDone; err != nil {
		t.Fatalf("RequestCancelActiveExecution: %v", err)
	}
	replacement := <-replacementDone
	if replacement.err != nil {
		t.Fatalf("replace owner after cancellation: %v", replacement.err)
	}
	defer func() { _ = replacement.lease.Release() }()
	request, err := store.CancelRequest("cancel-race")
	if err != nil {
		t.Fatalf("CancelRequest: %v", err)
	}
	if request.RequestedAt != now.Add(time.Second) {
		t.Fatalf("RequestedAt = %s, want %s", request.RequestedAt, now.Add(time.Second))
	}
}

func TestOwnerLeaseReleaseUnlocksWhenProjectionCleanupFails(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 13, 21, 0, 0, 0, time.UTC)
	lease, err := store.AcquireOwnerLease(OwnerLock{SessionID: "cleanup-failure", PID: 101, StartedAt: now})
	if err != nil {
		t.Fatalf("AcquireOwnerLease: %v", err)
	}
	projectionPath := store.ownerPath("cleanup-failure")
	if err := os.Remove(projectionPath); err != nil {
		t.Fatalf("remove owner projection: %v", err)
	}
	if err := os.Mkdir(projectionPath, 0o700); err != nil {
		t.Fatalf("replace projection with directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectionPath, "blocker"), []byte("block\n"), 0o600); err != nil {
		t.Fatalf("write projection blocker: %v", err)
	}
	if err := lease.Release(); err == nil {
		t.Fatal("Release with projection cleanup failure succeeded")
	}
	if err := os.RemoveAll(projectionPath); err != nil {
		t.Fatalf("remove projection blocker: %v", err)
	}
	replacement, err := store.AcquireOwnerLease(OwnerLock{SessionID: "cleanup-failure", PID: 202, StartedAt: now.Add(time.Second)})
	if err != nil {
		t.Fatalf("AcquireOwnerLease after cleanup failure: %v", err)
	}
	if err := replacement.Release(); err != nil {
		t.Fatalf("Release replacement: %v", err)
	}
}

func TestOwnerIgnoresAndCleansStaleProjection(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 13, 18, 35, 0, 0, time.UTC)
	if err := backgroundWriteJSONAtomic(store.ownerPath("stale"), OwnerLock{SessionID: "stale", PID: 999, StartedAt: now}); err != nil {
		t.Fatalf("write stale owner projection: %v", err)
	}
	if _, err := store.Owner("stale"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Owner stale projection error = %v, want os.ErrNotExist", err)
	}
	if _, err := os.Stat(store.ownerPath("stale")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale owner projection remains: %v", err)
	}
}

func TestOwnerFailsClosedWhenLiveLeaseProjectionIsUnavailable(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	release, acquired, err := tryStoreFileLock(store.ownerLeasePath("claiming"))
	if errors.Is(err, ErrStoreProcessLockUnsupported) {
		t.Skip("cross-process owner leases are unsupported on this platform")
	}
	if err != nil || !acquired {
		t.Fatalf("hold owner lease lock = %t, %v", acquired, err)
	}
	defer func() { _ = release() }()
	if _, err := store.Owner("claiming"); !errors.Is(err, ErrInvalidOwnerLock) {
		t.Fatalf("Owner during projection write error = %v, want ErrInvalidOwnerLock", err)
	}
}

func TestDrainQueueReclaimsOwnerLeaseAfterProcessCrash(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "sessions.json")
	store := NewStore(storePath)
	now := time.Date(2026, 7, 13, 18, 40, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID:        "crash-reclaim",
		Status:    SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		PromptQueue: []QueuedPrompt{{
			ID:        "q-1",
			Prompt:    "after crash",
			Status:    QueuePromptStatusPending,
			CreatedAt: now,
		}},
	}); err != nil {
		t.Fatalf("seed Upsert: %v", err)
	}

	output := killOwnerLeaseHelper(t, dir, storePath, "crash-reclaim")

	runner := &fakeDrainRunner{sessionID: acpsdk.SessionId("acp-reclaimed")}
	result, err := DrainQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "crash-reclaim", DrainOptions{
		Now: fixedClock(now.Add(time.Minute)),
		StartRunner: func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error) {
			return runner, func() error { return nil }, nil
		},
	})
	if err != nil {
		t.Fatalf("DrainQueue after owner crash: %v\nhelper output: %s", err, output)
	}
	if result.Completed != 1 || len(runner.prompts) != 1 || runner.prompts[0] != "after crash" {
		t.Fatalf("result/prompts = %#v/%#v, want reclaimed prompt once", result, runner.prompts)
	}
}

func TestWatchQueueReclaimsOwnerLeaseAfterProcessCrash(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "sessions.json")
	store := NewStore(storePath)
	now := time.Date(2026, 7, 13, 18, 45, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID:        "watch-crash-reclaim",
		Status:    SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		PromptQueue: []QueuedPrompt{{
			ID:        "q-1",
			Prompt:    "watch after crash",
			Status:    QueuePromptStatusPending,
			CreatedAt: now,
		}},
	}); err != nil {
		t.Fatalf("seed Upsert: %v", err)
	}
	output := killOwnerLeaseHelper(t, dir, storePath, "watch-crash-reclaim")

	runner := &fakeDrainRunner{sessionID: acpsdk.SessionId("acp-watch-reclaimed")}
	result, err := WatchQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "watch-crash-reclaim", WatchOptions{
		Interval:      time.Millisecond,
		MaxPerCycle:   1,
		StopWhenEmpty: true,
		Now:           fixedClock(now.Add(time.Minute)),
		Sleep:         instantWatchSleep,
		StartRunner: func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error) {
			return runner, func() error { return nil }, nil
		},
	}, nil)
	if err != nil {
		t.Fatalf("WatchQueue after owner crash: %v\nhelper output: %s", err, output)
	}
	if result.Completed != 1 || len(runner.prompts) != 1 || runner.prompts[0] != "watch after crash" {
		t.Fatalf("result/prompts = %#v/%#v, want reclaimed prompt once", result, runner.prompts)
	}
}

func killOwnerLeaseHelper(t *testing.T, dir, storePath, sessionID string) string {
	t.Helper()
	readyPath := filepath.Join(dir, "owner-ready-"+storeKey(sessionID))
	cmd := exec.Command(os.Args[0], "-test.run=^TestOwnerLeaseSubprocessHelper$")
	cmd.Env = append(os.Environ(),
		ownerLeaseHelperEnv+"=1",
		ownerLeaseStorePathEnv+"="+storePath,
		ownerLeaseSessionIDEnv+"="+sessionID,
		ownerLeaseReadyPathEnv+"="+readyPath,
	)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start owner helper: %v", err)
	}
	waitForStoreLockHelperReady(t, readyPath)
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill owner helper: %v", err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("killed owner helper exited successfully")
	}
	return output.String()
}

func TestOwnerLeaseSubprocessHelper(t *testing.T) {
	if os.Getenv(ownerLeaseHelperEnv) != "1" {
		return
	}
	store := NewStore(os.Getenv(ownerLeaseStorePathEnv))
	lease, err := store.AcquireOwnerLease(OwnerLock{
		SessionID: os.Getenv(ownerLeaseSessionIDEnv),
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("AcquireOwnerLease: %v", err)
	}
	if lease == nil {
		t.Fatal("AcquireOwnerLease returned nil lease")
	}
	if err := os.WriteFile(os.Getenv(ownerLeaseReadyPathEnv), []byte("ready\n"), 0o600); err != nil {
		t.Fatalf("write ready marker: %v", err)
	}
	for {
		time.Sleep(time.Hour)
	}
}
