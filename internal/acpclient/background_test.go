package acpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackgroundStoreRepeatedReplacementPersistsMinimalOwnerOnlyAtomicPolicyState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "background.json")
	store := NewBackgroundStore(path)
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	policy := BackgroundPolicy{
		SessionID:      "session-1",
		Profile:        "fixture",
		DescriptorHash: "descriptor-hash",
		PolicyVersion:  BackgroundPolicyVersion,
		AcknowledgedAt: now,
		Enabled:        true,
		State:          BackgroundStateRunning,
		Outcome:        BackgroundOutcomeStarted,
		StartedAt:      now,
		UpdatedAt:      now,
	}
	var raw []byte
	for revision := range 25 {
		policy.Outcome = fmt.Sprintf("revision_%d", revision)
		policy.UpdatedAt = now.Add(time.Duration(revision) * time.Second)
		if err := store.Upsert(policy); err != nil {
			t.Fatalf("Upsert revision %d: %v", revision, err)
		}
		var err error
		raw, err = os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile revision %d: %v", revision, err)
		}
		var persisted backgroundFile
		if err := json.Unmarshal(raw, &persisted); err != nil {
			t.Fatalf("Unmarshal revision %d: %v\n%s", revision, err, raw)
		}
		if len(persisted.Policies) != 1 || persisted.Policies[0] != policy {
			t.Fatalf("persisted revision %d = %#v, want one complete policy %#v", revision, persisted.Policies, policy)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat revision %d: %v", revision, err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("policy mode revision %d = %o, want 600", revision, got)
		}
		temps, err := filepath.Glob(filepath.Join(dir, ".background.json.*.tmp"))
		if err != nil {
			t.Fatalf("Glob revision %d: %v", revision, err)
		}
		if len(temps) != 0 {
			t.Fatalf("atomic temp files remain after revision %d: %#v", revision, temps)
		}
	}

	got, err := store.Get(policy.SessionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != policy {
		t.Fatalf("policy = %#v, want %#v", got, policy)
	}
	for _, forbidden := range []string{"prompt", "response", "argv", "args", "command", "env", "credential", "secret"} {
		if strings.Contains(strings.ToLower(string(raw)), forbidden) {
			t.Fatalf("policy contains forbidden %q metadata: %s", forbidden, raw)
		}
	}
}

func TestBackgroundManagerStartIsIdempotentOnlyForSameActivePolicy(t *testing.T) {
	var watchers atomic.Int32
	started := make(chan struct{}, 2)
	manager, _, _ := newBackgroundTestManager(t, func(name string) (ResolvedBackgroundProfile, error) {
		return trustedBackgroundProfile(name, "hash-"+name), nil
	}, func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
		watchers.Add(1)
		started <- struct{}{}
		<-ctx.Done()
		return WatchResult{}, ctx.Err()
	})

	first, err := manager.Start("session-1", "fixture", true)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-started
	second, err := manager.Start("session-1", "fixture", true)
	if err != nil {
		t.Fatalf("idempotent Start: %v", err)
	}
	if first != second {
		t.Fatalf("idempotent statuses differ: %#v != %#v", first, second)
	}
	if got := watchers.Load(); got != 1 {
		t.Fatalf("watchers = %d, want 1", got)
	}
	if _, err := manager.Start("session-1", "other", true); !errors.Is(err, ErrBackgroundPolicyConflict) {
		t.Fatalf("different active Start error = %v, want ErrBackgroundPolicyConflict", err)
	}
	if got := watchers.Load(); got != 1 {
		t.Fatalf("watchers after conflict = %d, want 1", got)
	}
	manager.Shutdown()
}

func TestBackgroundManagerStartRequiresAcknowledgement(t *testing.T) {
	manager, _, _ := newBackgroundTestManager(t, func(name string) (ResolvedBackgroundProfile, error) {
		return trustedBackgroundProfile(name, "hash"), nil
	}, nil)

	if _, err := manager.Start("session-1", "fixture", false); !errors.Is(err, ErrBackgroundAcknowledgementRequired) {
		t.Fatalf("Start error = %v, want ErrBackgroundAcknowledgementRequired", err)
	}
}

func TestBackgroundManagerStartPersistsAndAuditsBeforeWatcherEntry(t *testing.T) {
	dir := t.TempDir()
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	checked := make(chan error, 1)
	manager := NewBackgroundManager(backgroundSessionStore(t), store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: backgroundDurabilityWatcher(store, audit, BackgroundAuditStart, checked),
	})
	t.Cleanup(manager.Shutdown)

	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := <-checked; err != nil {
		t.Fatal(err)
	}
}

func TestBackgroundManagerStartStateWriteFailureDoesNotLaunch(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatalf("WriteFile blocker: %v", err)
	}
	var watchers atomic.Int32
	manager := NewBackgroundManager(
		backgroundSessionStore(t),
		NewBackgroundStore(filepath.Join(blocker, "background.json")),
		NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl")),
		BackgroundManagerOptions{
			Context: t.Context(),
			Now:     backgroundTestClock,
			Resolver: func(name string) (ResolvedBackgroundProfile, error) {
				return trustedBackgroundProfile(name, "descriptor-hash"), nil
			},
			Watcher: countingBackgroundWatcher(&watchers, nil),
		},
	)
	t.Cleanup(manager.Shutdown)

	status, err := manager.Start("session-1", "fixture", true)
	if err == nil {
		t.Fatal("Start error = nil, want state write failure")
	}
	assertBackgroundFailureStatus(t, status, "state_write_failed")
	if got := watchers.Load(); got != 0 {
		t.Fatalf("watchers = %d, want 0", got)
	}
}

func TestBackgroundManagerStartAuditFailureDisablesPolicyWithoutLaunch(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatalf("WriteFile blocker: %v", err)
	}
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	var watchers atomic.Int32
	manager := NewBackgroundManager(
		backgroundSessionStore(t),
		store,
		NewBackgroundAudit(filepath.Join(blocker, "background-audit.jsonl")),
		BackgroundManagerOptions{
			Context: t.Context(),
			Now:     backgroundTestClock,
			Resolver: func(name string) (ResolvedBackgroundProfile, error) {
				return trustedBackgroundProfile(name, "descriptor-hash"), nil
			},
			Watcher: countingBackgroundWatcher(&watchers, nil),
		},
	)
	t.Cleanup(manager.Shutdown)

	status, err := manager.Start("session-1", "fixture", true)
	if err == nil {
		t.Fatal("Start error = nil, want audit append failure")
	}
	assertBackgroundFailureStatus(t, status, "audit_append_failed")
	assertBackgroundPolicy(t, store, "session-1", BackgroundStateError, "audit_append_failed", false)
	if got := watchers.Load(); got != 0 {
		t.Fatalf("watchers = %d, want 0", got)
	}
}

func TestBackgroundManagerProfileDriftBlocksStartAndResume(t *testing.T) {
	t.Run("start", func(t *testing.T) {
		var watchers atomic.Int32
		manager, store, audit := newBackgroundTestManager(t, func(name string) (ResolvedBackgroundProfile, error) {
			profile := trustedBackgroundProfile(name, "current-hash")
			profile.TrustValid = false
			return profile, nil
		}, countingBackgroundWatcher(&watchers, errors.New("unexpected watcher")))

		status, err := manager.Start("session-1", "fixture", true)
		if !errors.Is(err, ErrBackgroundProfileUntrusted) {
			t.Fatalf("Start error = %v, want ErrBackgroundProfileUntrusted", err)
		}
		if status.State != BackgroundStateBlocked || status.Outcome != BackgroundOutcomeProfileUntrusted {
			t.Fatalf("status = %#v, want blocked/untrusted", status)
		}
		assertBackgroundPolicy(t, store, "session-1", BackgroundStateBlocked, BackgroundOutcomeProfileUntrusted, true)
		assertBackgroundAuditActions(t, audit, BackgroundAuditBlock)
		if got := watchers.Load(); got != 0 {
			t.Fatalf("watchers = %d, want 0", got)
		}
	})

	t.Run("resume", func(t *testing.T) {
		var watchers atomic.Int32
		manager, store, audit := newBackgroundTestManager(t, func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "current-hash"), nil
		}, countingBackgroundWatcher(&watchers, errors.New("unexpected watcher")))
		now := backgroundTestNow()
		if err := store.Upsert(BackgroundPolicy{
			SessionID:      "session-1",
			Profile:        "fixture",
			DescriptorHash: "pinned-hash",
			PolicyVersion:  BackgroundPolicyVersion,
			AcknowledgedAt: now,
			Enabled:        true,
			State:          BackgroundStateRunning,
			Outcome:        BackgroundOutcomeStarted,
			StartedAt:      now,
			UpdatedAt:      now,
		}); err != nil {
			t.Fatalf("Upsert: %v", err)
		}

		if err := manager.Resume(); err != nil {
			t.Fatalf("Resume: %v", err)
		}
		assertBackgroundPolicy(t, store, "session-1", BackgroundStateBlocked, BackgroundOutcomeProfileDrift, true)
		assertBackgroundAuditActions(t, audit, BackgroundAuditBlock)
		if got := watchers.Load(); got != 0 {
			t.Fatalf("watchers = %d, want 0", got)
		}
	})
}

func TestBackgroundManagerWorkerErrorPersistsErrorWithoutRetry(t *testing.T) {
	var watchers atomic.Int32
	manager, store, audit := newBackgroundTestManager(t, func(name string) (ResolvedBackgroundProfile, error) {
		return trustedBackgroundProfile(name, "descriptor-hash"), nil
	}, countingBackgroundWatcher(&watchers, errors.New("secret worker failure")))

	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	eventuallyBackground(t, func() bool {
		policy, err := store.Get("session-1")
		return err == nil && policy.State == BackgroundStateError
	})
	if got := watchers.Load(); got != 1 {
		t.Fatalf("watchers = %d, want exactly 1", got)
	}
	policy := assertBackgroundPolicy(t, store, "session-1", BackgroundStateError, BackgroundOutcomeWorkerError, false)
	if strings.Contains(policy.Outcome, "secret worker failure") {
		t.Fatalf("worker outcome leaked raw error: %q", policy.Outcome)
	}
	assertBackgroundAuditActions(t, audit, BackgroundAuditStart, BackgroundAuditError)
	rawAudit, err := os.ReadFile(audit.Path())
	if err != nil {
		t.Fatalf("ReadFile audit: %v", err)
	}
	if strings.Contains(string(rawAudit), "secret worker failure") {
		t.Fatalf("worker audit leaked raw error: %s", rawAudit)
	}
	if err := manager.Resume(); err != nil {
		t.Fatalf("Resume after worker error: %v", err)
	}
	if got := watchers.Load(); got != 1 {
		t.Fatalf("watchers after Resume = %d, want 1", got)
	}
	manager.Shutdown()
}

func TestBackgroundManagerPanickingWatcherIsGuardedUntilExplicitStart(t *testing.T) {
	var watchers atomic.Int32
	manager, store, audit := newBackgroundTestManager(t, func(name string) (ResolvedBackgroundProfile, error) {
		return trustedBackgroundProfile(name, "descriptor-hash"), nil
	}, func(context.Context, *Store, AgentSpec, RunOptions, string, WatchOptions, func(WatchCycle)) (WatchResult, error) {
		watchers.Add(1)
		panic("secret panic payload")
	})

	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	eventuallyBackground(t, func() bool {
		status, err := manager.Get("session-1")
		return err == nil && status.State == BackgroundStateError
	})
	status, err := manager.Get("session-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if status.Enabled || status.Outcome != BackgroundOutcomeWorkerPanic {
		t.Fatalf("status = %#v, want disabled error/worker_panic", status)
	}
	assertBackgroundPolicy(t, store, "session-1", BackgroundStateError, BackgroundOutcomeWorkerPanic, false)
	assertBackgroundAuditActions(t, audit, BackgroundAuditStart, BackgroundAuditError)
	rawAudit, err := os.ReadFile(audit.Path())
	if err != nil {
		t.Fatalf("ReadFile audit: %v", err)
	}
	if strings.Contains(string(rawAudit), "secret panic payload") {
		t.Fatalf("panic audit leaked raw panic: %s", rawAudit)
	}
	if err := manager.Resume(); err != nil {
		t.Fatalf("Resume after panic: %v", err)
	}
	if got := watchers.Load(); got != 1 {
		t.Fatalf("watchers after Resume = %d, want 1", got)
	}
	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("explicit Start after panic: %v", err)
	}
	eventuallyBackground(t, func() bool { return watchers.Load() == 2 })
}

func TestBackgroundManagerWorkerErrorStateWriteFailureGuardsResume(t *testing.T) {
	dir := t.TempDir()
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	var watchers atomic.Int32
	watcherReturned := make(chan struct{})
	manager := NewBackgroundManager(backgroundSessionStore(t), store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(context.Context, *Store, AgentSpec, RunOptions, string, WatchOptions, func(WatchCycle)) (WatchResult, error) {
			watchers.Add(1)
			if err := os.Remove(store.Path()); err != nil {
				return WatchResult{}, err
			}
			if err := os.Mkdir(store.Path(), 0o700); err != nil {
				return WatchResult{}, err
			}
			close(watcherReturned)
			return WatchResult{}, errors.New("secret worker state failure")
		},
	})
	t.Cleanup(manager.Shutdown)

	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-watcherReturned
	var status BackgroundStatus
	eventuallyBackground(t, func() bool {
		var err error
		status, err = manager.Get("session-1")
		return err == nil && status.State == BackgroundStateError
	})
	if status.Enabled || status.Outcome != BackgroundOutcomeWorkerError {
		t.Fatalf("status = %#v, want disabled worker_error", status)
	}
	if strings.Contains(fmt.Sprintf("%#v", status), "secret worker state failure") {
		t.Fatalf("terminal status leaked raw worker error: %#v", status)
	}
	assertBackgroundTerminalRecording(t, manager, "session-1", false, true)
	assertBackgroundAuditActions(t, audit, BackgroundAuditStart, BackgroundAuditError)
	_ = manager.Resume()
	if got := watchers.Load(); got != 1 {
		t.Fatalf("watchers after Resume = %d, want 1", got)
	}
}

func TestBackgroundManagerWorkerErrorAuditFailureGuardsResume(t *testing.T) {
	dir := t.TempDir()
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	var watchers atomic.Int32
	watcherReturned := make(chan struct{})
	manager := NewBackgroundManager(backgroundSessionStore(t), store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(context.Context, *Store, AgentSpec, RunOptions, string, WatchOptions, func(WatchCycle)) (WatchResult, error) {
			watchers.Add(1)
			if err := os.Remove(audit.Path()); err != nil {
				return WatchResult{}, err
			}
			if err := os.Mkdir(audit.Path(), 0o700); err != nil {
				return WatchResult{}, err
			}
			close(watcherReturned)
			return WatchResult{}, errors.New("secret worker audit failure")
		},
	})
	t.Cleanup(manager.Shutdown)

	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-watcherReturned
	eventuallyBackground(t, func() bool {
		policy, err := store.Get("session-1")
		return err == nil && policy.State == BackgroundStateError
	})
	status, err := manager.Get("session-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if status.Enabled || status.Outcome != BackgroundOutcomeWorkerError {
		t.Fatalf("status = %#v, want disabled worker_error", status)
	}
	if strings.Contains(fmt.Sprintf("%#v", status), "secret worker audit failure") {
		t.Fatalf("terminal status leaked raw worker error: %#v", status)
	}
	assertBackgroundTerminalRecording(t, manager, "session-1", true, false)
	if err := manager.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if got := watchers.Load(); got != 1 {
		t.Fatalf("watchers after Resume = %d, want 1", got)
	}
}

func TestBackgroundManagerStopPersistsAndAuditsBeforeCancellation(t *testing.T) {
	policyPath := filepath.Join(t.TempDir(), "background.json")
	auditPath := filepath.Join(filepath.Dir(policyPath), "background-audit.jsonl")
	policyStore := NewBackgroundStore(policyPath)
	audit := NewBackgroundAudit(auditPath)
	sessions := backgroundSessionStore(t)
	checked := make(chan error, 1)
	watcher := func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
		<-ctx.Done()
		policy, err := policyStore.Get("session-1")
		if err == nil && (policy.Enabled || policy.State != BackgroundStateDisabled) {
			err = errors.New("policy was not disabled before cancellation")
		}
		if err == nil {
			records, readErr := audit.Read()
			if readErr != nil {
				err = readErr
			} else if len(records) < 2 || records[len(records)-1].Action != BackgroundAuditStop {
				err = errors.New("stop audit was not durable before cancellation")
			}
		}
		checked <- err
		return WatchResult{}, ctx.Err()
	}
	manager := NewBackgroundManager(sessions, policyStore, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     func() time.Time { return backgroundTestNow() },
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: watcher,
	})

	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	status, err := manager.Stop("session-1")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if status.Enabled || status.State != BackgroundStateDisabled || status.Outcome != BackgroundOutcomeStopped {
		t.Fatalf("status = %#v, want disabled/stopped", status)
	}
	if err := <-checked; err != nil {
		t.Fatal(err)
	}
	assertBackgroundAuditActions(t, audit, BackgroundAuditStart, BackgroundAuditStop)
	manager.Shutdown()
}

func TestBackgroundManagerStopAuditFailureStillCancelsAfterPersist(t *testing.T) {
	dir := t.TempDir()
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	started := make(chan struct{})
	checked := make(chan error, 1)
	manager := NewBackgroundManager(backgroundSessionStore(t), store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
			close(started)
			<-ctx.Done()
			policy, err := store.Get("session-1")
			if err == nil && (policy.Enabled || policy.State != BackgroundStateDisabled) {
				err = errors.New("policy was not disabled before cancellation")
			}
			checked <- err
			return WatchResult{}, ctx.Err()
		},
	})
	t.Cleanup(manager.Shutdown)

	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-started
	if err := os.Remove(audit.Path()); err != nil {
		t.Fatalf("Remove audit: %v", err)
	}
	if err := os.Mkdir(audit.Path(), 0o700); err != nil {
		t.Fatalf("Mkdir audit blocker: %v", err)
	}
	status, err := manager.Stop("session-1")
	if err == nil {
		t.Fatal("Stop error = nil, want attempted audit append failure")
	}
	if status.Enabled || status.State != BackgroundStateDisabled || status.Outcome != BackgroundOutcomeStopped {
		t.Fatalf("status = %#v, want disabled/stopped", status)
	}
	select {
	case err := <-checked:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker was not canceled after stop audit failure")
	}
}

func TestBackgroundManagerResumeLaunchesAcknowledgedTrustedPinnedPolicy(t *testing.T) {
	var watchers atomic.Int32
	started := make(chan struct{}, 1)
	manager, store, audit := newBackgroundTestManager(t, func(name string) (ResolvedBackgroundProfile, error) {
		return trustedBackgroundProfile(name, "descriptor-hash"), nil
	}, func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
		watchers.Add(1)
		started <- struct{}{}
		<-ctx.Done()
		return WatchResult{}, ctx.Err()
	})
	now := backgroundTestNow()
	if err := store.Upsert(BackgroundPolicy{
		SessionID:      "session-1",
		Profile:        "fixture",
		DescriptorHash: "descriptor-hash",
		PolicyVersion:  BackgroundPolicyVersion,
		AcknowledgedAt: now,
		Enabled:        true,
		State:          BackgroundStateRunning,
		Outcome:        BackgroundOutcomeStarted,
		StartedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := manager.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	<-started
	if got := watchers.Load(); got != 1 {
		t.Fatalf("watchers = %d, want 1", got)
	}
	assertBackgroundPolicy(t, store, "session-1", BackgroundStateRunning, BackgroundOutcomeResumed, true)
	assertBackgroundAuditActions(t, audit, BackgroundAuditResume)
	manager.Shutdown()
}

func TestBackgroundManagerResumePersistsAndAuditsBeforeWatcherEntry(t *testing.T) {
	dir := t.TempDir()
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	now := backgroundTestNow()
	if err := store.Upsert(backgroundRunnablePolicy(now)); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	checked := make(chan error, 1)
	manager := NewBackgroundManager(backgroundSessionStore(t), store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: backgroundDurabilityWatcher(store, audit, BackgroundAuditResume, checked),
	})
	t.Cleanup(manager.Shutdown)

	if err := manager.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if err := <-checked; err != nil {
		t.Fatal(err)
	}
}

func TestBackgroundManagerResumeAuditFailureDisablesPolicyWithoutLaunch(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatalf("WriteFile blocker: %v", err)
	}
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	if err := store.Upsert(backgroundRunnablePolicy(backgroundTestNow())); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	var watchers atomic.Int32
	manager := NewBackgroundManager(
		backgroundSessionStore(t),
		store,
		NewBackgroundAudit(filepath.Join(blocker, "background-audit.jsonl")),
		BackgroundManagerOptions{
			Context: t.Context(),
			Now:     backgroundTestClock,
			Resolver: func(name string) (ResolvedBackgroundProfile, error) {
				return trustedBackgroundProfile(name, "descriptor-hash"), nil
			},
			Watcher: countingBackgroundWatcher(&watchers, nil),
		},
	)
	t.Cleanup(manager.Shutdown)

	if err := manager.Resume(); err == nil {
		t.Fatal("Resume error = nil, want audit append failure")
	}
	assertBackgroundPolicy(t, store, "session-1", BackgroundStateError, "audit_append_failed", false)
	if got := watchers.Load(); got != 0 {
		t.Fatalf("watchers = %d, want 0", got)
	}
}

func TestBackgroundManagerResumeStateWriteFailureDoesNotLaunch(t *testing.T) {
	dir := t.TempDir()
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	if err := store.Upsert(backgroundRunnablePolicy(backgroundTestNow())); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	var watchers atomic.Int32
	manager := NewBackgroundManager(
		backgroundSessionStore(t),
		store,
		NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl")),
		BackgroundManagerOptions{
			Context: t.Context(),
			Now:     backgroundTestClock,
			Resolver: func(name string) (ResolvedBackgroundProfile, error) {
				if err := os.Remove(store.Path()); err != nil {
					return ResolvedBackgroundProfile{}, err
				}
				if err := os.Mkdir(store.Path(), 0o700); err != nil {
					return ResolvedBackgroundProfile{}, err
				}
				return trustedBackgroundProfile(name, "descriptor-hash"), nil
			},
			Watcher: countingBackgroundWatcher(&watchers, nil),
		},
	)
	t.Cleanup(manager.Shutdown)

	if err := manager.Resume(); err == nil {
		t.Fatal("Resume error = nil, want state write failure")
	}
	status, err := manager.Get("session-1")
	if err != nil {
		t.Fatalf("Get guarded status: %v", err)
	}
	assertBackgroundFailureStatus(t, status, BackgroundOutcomeStateWriteFailed)
	if got := watchers.Load(); got != 0 {
		t.Fatalf("watchers = %d, want 0", got)
	}
}

func TestBackgroundManagerResumeRequiresEnabledAcknowledgedPolicy(t *testing.T) {
	tests := map[string]struct {
		policy      BackgroundPolicy
		wantState   string
		wantOutcome string
		wantAudit   []string
	}{
		"disabled": {
			policy: BackgroundPolicy{
				Enabled: false,
				State:   BackgroundStateDisabled,
				Outcome: BackgroundOutcomeStopped,
			},
			wantState:   BackgroundStateDisabled,
			wantOutcome: BackgroundOutcomeStopped,
		},
		"unacknowledged": {
			policy: BackgroundPolicy{
				Enabled: true,
				State:   BackgroundStateRunning,
				Outcome: BackgroundOutcomeStarted,
			},
			wantState:   BackgroundStateBlocked,
			wantOutcome: BackgroundOutcomePolicyInvalid,
			wantAudit:   []string{BackgroundAuditBlock},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var watchers atomic.Int32
			manager, store, audit := newBackgroundTestManager(t, func(name string) (ResolvedBackgroundProfile, error) {
				return trustedBackgroundProfile(name, "descriptor-hash"), nil
			}, countingBackgroundWatcher(&watchers, errors.New("unexpected watcher")))
			now := backgroundTestNow()
			policy := test.policy
			policy.SessionID = "session-1"
			policy.Profile = "fixture"
			policy.DescriptorHash = "descriptor-hash"
			policy.PolicyVersion = BackgroundPolicyVersion
			policy.StartedAt = now
			policy.UpdatedAt = now
			if err := store.Upsert(policy); err != nil {
				t.Fatalf("Upsert: %v", err)
			}

			if err := manager.Resume(); err != nil {
				t.Fatalf("Resume: %v", err)
			}
			assertBackgroundPolicy(t, store, policy.SessionID, test.wantState, test.wantOutcome, policy.Enabled)
			assertBackgroundAuditActions(t, audit, test.wantAudit...)
			if got := watchers.Load(); got != 0 {
				t.Fatalf("watchers = %d, want 0", got)
			}
		})
	}
}

func TestBackgroundManagerResumeBlocksInvalidPolicyMatrix(t *testing.T) {
	tests := map[string]struct {
		preparePolicy  func(*BackgroundPolicy)
		prepareSession func(*testing.T) *Store
		resolver       BackgroundProfileResolver
		wantOutcome    string
	}{
		"invalid version": {
			preparePolicy: func(policy *BackgroundPolicy) { policy.PolicyVersion++ },
			wantOutcome:   BackgroundOutcomePolicyInvalid,
		},
		"trust invalid": {
			resolver: func(name string) (ResolvedBackgroundProfile, error) {
				profile := trustedBackgroundProfile(name, "descriptor-hash")
				profile.TrustValid = false
				return profile, nil
			},
			wantOutcome: BackgroundOutcomeProfileUntrusted,
		},
		"missing session": {
			prepareSession: func(t *testing.T) *Store {
				return NewStore(filepath.Join(t.TempDir(), "sessions.json"))
			},
			wantOutcome: BackgroundOutcomeSessionMissing,
		},
		"missing profile": {
			resolver: func(name string) (ResolvedBackgroundProfile, error) {
				return ResolvedBackgroundProfile{}, fmt.Errorf("%w: %s", ErrProfileNotFound, name)
			},
			wantOutcome: BackgroundOutcomeProfileMissing,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			store := NewBackgroundStore(filepath.Join(dir, "background.json"))
			audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
			policy := backgroundRunnablePolicy(backgroundTestNow())
			if test.preparePolicy != nil {
				test.preparePolicy(&policy)
			}
			if err := store.Upsert(policy); err != nil {
				t.Fatalf("Upsert: %v", err)
			}
			sessions := backgroundSessionStore(t)
			if test.prepareSession != nil {
				sessions = test.prepareSession(t)
			}
			resolver := test.resolver
			if resolver == nil {
				resolver = func(name string) (ResolvedBackgroundProfile, error) {
					return trustedBackgroundProfile(name, "descriptor-hash"), nil
				}
			}
			var watchers atomic.Int32
			manager := NewBackgroundManager(sessions, store, audit, BackgroundManagerOptions{
				Context:  t.Context(),
				Now:      backgroundTestClock,
				Resolver: resolver,
				Watcher:  countingBackgroundWatcher(&watchers, nil),
			})
			t.Cleanup(manager.Shutdown)

			if err := manager.Resume(); err != nil {
				t.Fatalf("Resume: %v", err)
			}
			assertBackgroundPolicy(t, store, policy.SessionID, BackgroundStateBlocked, test.wantOutcome, true)
			assertBackgroundAuditActions(t, audit, BackgroundAuditBlock)
			if got := watchers.Load(); got != 0 {
				t.Fatalf("watchers = %d, want 0", got)
			}
		})
	}
}

func TestBackgroundProfileResolverPinsBuiltinFingerprintAndProfileDescriptor(t *testing.T) {
	profiles := NewProfileStore(filepath.Join(t.TempDir(), "profiles.json"))
	profile := Profile{
		Name: "fixture",
		Spec: AgentSpec{Name: "fixture", Command: "/tmp/acp-agent", Args: []string{"--stdio"}, EnvKeys: []string{"ACP_TOKEN"}},
		Cwd:  "/tmp/project",
	}
	if err := profiles.Add(profile); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := profiles.Trust(profile.Name); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	stored, err := profiles.Get(profile.Name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	builtin := AgentSpec{Name: "builtin", Command: "builtin", Args: []string{"acp"}}
	resolve := NewBackgroundProfileResolver(NewRegistry([]AgentSpec{builtin}), profiles)

	resolvedBuiltin, err := resolve(builtin.Name)
	if err != nil {
		t.Fatalf("resolve builtin: %v", err)
	}
	if resolvedBuiltin.DescriptorHash != builtin.Fingerprint() || !resolvedBuiltin.TrustValid {
		t.Fatalf("resolved builtin = %#v", resolvedBuiltin)
	}
	resolvedProfile, err := resolve(profile.Name)
	if err != nil {
		t.Fatalf("resolve profile: %v", err)
	}
	if resolvedProfile.DescriptorHash != stored.DescriptorHash() || !resolvedProfile.TrustValid || resolvedProfile.Options.Cwd != profile.Cwd {
		t.Fatalf("resolved profile = %#v", resolvedProfile)
	}
}

func TestBackgroundProfileResolverReturnsTrustBeforeValidatingDriftedSpec(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	if err := os.WriteFile(path, []byte(`{
  "profiles": [{
    "name": "fixture",
    "spec": {"name": "fixture", "command": "codex acp"},
    "hash": "stale-trust-hash",
    "trusted": true,
    "createdAt": "2026-07-10T12:00:00Z",
    "updatedAt": "2026-07-10T12:00:00Z"
  }]
}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	resolve := NewBackgroundProfileResolver(Registry{}, NewProfileStore(path))

	profile, err := resolve("fixture")
	if err != nil {
		t.Fatalf("resolve drifted profile: %v", err)
	}
	if profile.TrustValid {
		t.Fatal("resolved drifted profile trust is valid")
	}
}

func newBackgroundTestManager(t *testing.T, resolver BackgroundProfileResolver, watcher BackgroundWatcher) (*BackgroundManager, *BackgroundStore, *BackgroundAudit) {
	t.Helper()
	dir := t.TempDir()
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	manager := NewBackgroundManager(backgroundSessionStore(t), store, audit, BackgroundManagerOptions{
		Context:  t.Context(),
		Now:      func() time.Time { return backgroundTestNow() },
		Resolver: resolver,
		Watcher:  watcher,
	})
	t.Cleanup(manager.Shutdown)
	return manager, store, audit
}

func backgroundSessionStore(t *testing.T) *Store {
	t.Helper()
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := backgroundTestNow()
	if err := store.Upsert(SessionRecord{ID: "session-1", Agent: "fixture", Status: SessionStatusQueued, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("Upsert session: %v", err)
	}
	return store
}

func trustedBackgroundProfile(name, hash string) ResolvedBackgroundProfile {
	return ResolvedBackgroundProfile{
		Spec:           AgentSpec{Name: name, Command: "/tmp/acp-agent"},
		DescriptorHash: hash,
		TrustValid:     true,
	}
}

func backgroundRunnablePolicy(now time.Time) BackgroundPolicy {
	return BackgroundPolicy{
		SessionID:      "session-1",
		Profile:        "fixture",
		DescriptorHash: "descriptor-hash",
		PolicyVersion:  BackgroundPolicyVersion,
		AcknowledgedAt: now,
		Enabled:        true,
		State:          BackgroundStateRunning,
		Outcome:        BackgroundOutcomeStarted,
		StartedAt:      now,
		UpdatedAt:      now,
	}
}

func backgroundDurabilityWatcher(store *BackgroundStore, audit *BackgroundAudit, action string, checked chan<- error) BackgroundWatcher {
	return func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, sessionID string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
		policy, err := store.Get(sessionID)
		if err == nil && (!policy.Enabled || policy.State != BackgroundStateRunning) {
			err = errors.New("running policy was not durable before watcher entry")
		}
		if err == nil {
			records, readErr := audit.Read()
			if readErr != nil {
				err = readErr
			} else if len(records) == 0 || records[len(records)-1].Action != action {
				err = errors.New("launch audit was not durable before watcher entry")
			}
		}
		checked <- err
		<-ctx.Done()
		return WatchResult{}, ctx.Err()
	}
}

func assertBackgroundFailureStatus(t *testing.T, status BackgroundStatus, outcome string) {
	t.Helper()
	if status.Enabled || status.State != BackgroundStateError || status.Outcome != outcome {
		t.Fatalf("status = %#v, want disabled/error/%s", status, outcome)
	}
}

func assertBackgroundTerminalRecording(t *testing.T, manager *BackgroundManager, sessionID string, wantState, wantAudit bool) {
	t.Helper()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	guard, ok := manager.terminal[sessionID]
	if !ok {
		t.Fatalf("terminal guard %q is missing", sessionID)
	}
	if guard.stateRecorded != wantState || guard.auditRecorded != wantAudit {
		t.Fatalf("terminal recording = state:%t audit:%t, want state:%t audit:%t", guard.stateRecorded, guard.auditRecorded, wantState, wantAudit)
	}
}

func countingBackgroundWatcher(count *atomic.Int32, result error) BackgroundWatcher {
	return func(context.Context, *Store, AgentSpec, RunOptions, string, WatchOptions, func(WatchCycle)) (WatchResult, error) {
		count.Add(1)
		return WatchResult{}, result
	}
}

func assertBackgroundPolicy(t *testing.T, store *BackgroundStore, sessionID, state, outcome string, enabled bool) BackgroundPolicy {
	t.Helper()
	policy, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("Get(%q): %v", sessionID, err)
	}
	if policy.State != state || policy.Outcome != outcome || policy.Enabled != enabled {
		t.Fatalf("policy = %#v, want state=%q outcome=%q enabled=%t", policy, state, outcome, enabled)
	}
	return policy
}

func assertBackgroundAuditActions(t *testing.T, audit *BackgroundAudit, want ...string) {
	t.Helper()
	records, err := audit.Read()
	if err != nil {
		t.Fatalf("Read audit: %v", err)
	}
	if len(records) != len(want) {
		t.Fatalf("audit records = %#v, want actions %#v", records, want)
	}
	for i, action := range want {
		if records[i].Action != action {
			t.Fatalf("audit action[%d] = %q, want %q", i, records[i].Action, action)
		}
	}
}

func eventuallyBackground(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition not satisfied before timeout")
		}
		time.Sleep(time.Millisecond)
	}
}

func backgroundTestNow() time.Time {
	return time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
}

func backgroundTestClock() time.Time {
	return backgroundTestNow()
}
