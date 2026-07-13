package acpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
		var object map[string]any
		if err := json.Unmarshal(raw, &object); err != nil {
			t.Fatalf("Unmarshal object revision %d: %v", revision, err)
		}
		if len(object) != 1 || object["policies"] == nil {
			t.Fatalf("state keys revision %d = %#v, want policies only", revision, object)
		}
		policyObjects, ok := object["policies"].([]any)
		if !ok || len(policyObjects) != 1 {
			t.Fatalf("policy objects revision %d = %#v", revision, object["policies"])
		}
		policyObject, ok := policyObjects[0].(map[string]any)
		if !ok {
			t.Fatalf("policy object revision %d = %#v", revision, policyObjects[0])
		}
		allowedPolicyKeys := map[string]bool{
			"sessionId": true, "profile": true, "descriptorHash": true, "policyVersion": true,
			"acknowledgedAt": true, "enabled": true, "state": true, "outcome": true,
			"startedAt": true, "updatedAt": true, "persistenceDegraded": true,
		}
		for key := range policyObject {
			if !allowedPolicyKeys[key] {
				t.Fatalf("policy revision %d contains unexpected key %q: %#v", revision, key, policyObject)
			}
		}
		if len(policyObject) != 10 {
			t.Fatalf("policy revision %d keys = %#v, want ten non-degraded metadata keys", revision, policyObject)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat revision %d: %v", revision, err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("policy mode revision %d = %o, want 600", revision, got)
		}
		lockInfo, err := os.Stat(requireStoreLockPhysicalPath(t, path+".lock"))
		if err != nil {
			t.Fatalf("Stat policy lock revision %d: %v", revision, err)
		}
		if got := lockInfo.Mode().Perm(); got != 0o600 {
			t.Fatalf("policy lock mode revision %d = %o, want 600", revision, got)
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

func TestBackgroundStoreCoordinatesConcurrentHandles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "background.json")
	now := backgroundTestNow()
	const count = 24
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := range count {
		wg.Go(func() {
			<-start
			policy := backgroundRunnablePolicy(now)
			policy.SessionID = fmt.Sprintf("session-%02d", i)
			errs <- NewBackgroundStore(path).Upsert(policy)
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Upsert: %v", err)
		}
	}
	policies, err := NewBackgroundStore(path).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(policies) != count {
		t.Fatalf("policies len = %d, want %d", len(policies), count)
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

func TestBackgroundManagerSerializesWorkersAcrossManagers(t *testing.T) {
	dir := t.TempDir()
	sessions := NewStore(filepath.Join(dir, "sessions.json"))
	now := backgroundTestNow()
	if err := sessions.Upsert(SessionRecord{
		ID:        "shared-session",
		Agent:     "fixture",
		Status:    SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Upsert session: %v", err)
	}
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	newManager := func(started chan struct{}) *BackgroundManager {
		return NewBackgroundManager(sessions, store, audit, BackgroundManagerOptions{
			Context: t.Context(),
			Now:     backgroundTestClock,
			Resolver: func(name string) (ResolvedBackgroundProfile, error) {
				return trustedBackgroundProfile(name, "descriptor-hash"), nil
			},
			Watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
				close(started)
				<-ctx.Done()
				return WatchResult{}, ctx.Err()
			},
		})
	}
	first := newManager(firstStarted)
	second := newManager(secondStarted)
	t.Cleanup(second.Shutdown)
	t.Cleanup(first.Shutdown)

	if _, err := first.Start("shared-session", "fixture", true); err != nil {
		if errors.Is(err, ErrStoreProcessLockUnsupported) {
			t.Skip("cross-process background leases are unsupported on this platform")
		}
		t.Fatalf("first Start: %v", err)
	}
	<-firstStarted
	if _, err := second.Start("shared-session", "fixture", true); !errors.Is(err, ErrBackgroundTransitionBusy) {
		t.Fatalf("second Start error = %v, want ErrBackgroundTransitionBusy", err)
	}
	select {
	case <-secondStarted:
		t.Fatal("second manager launched a duplicate watcher")
	default:
	}
	policy, err := store.Get("shared-session")
	if err != nil {
		t.Fatalf("Get policy: %v", err)
	}
	if !policy.Enabled || policy.State != BackgroundStateRunning {
		t.Fatalf("shared policy = %#v, want enabled/running", policy)
	}
}

func TestBackgroundManagerWorkerExitCannotLaunchReplacementWithoutLease(t *testing.T) {
	dir := t.TempDir()
	sessions := NewStore(filepath.Join(dir, "sessions.json"))
	now := backgroundTestNow()
	if err := sessions.Upsert(SessionRecord{ID: "handoff-session", Status: SessionStatusQueued, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	secondResolving := make(chan struct{})
	allowSecondResolve := make(chan struct{})
	var resolves atomic.Int32
	resolver := func(name string) (ResolvedBackgroundProfile, error) {
		if resolves.Add(1) == 2 {
			close(secondResolving)
			<-allowSecondResolve
		}
		return trustedBackgroundProfile(name, "descriptor-hash"), nil
	}
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	var watchers atomic.Int32
	manager := NewBackgroundManager(sessions, store, audit, BackgroundManagerOptions{
		Context:  t.Context(),
		Now:      backgroundTestClock,
		Resolver: resolver,
		Watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
			switch watchers.Add(1) {
			case 1:
				close(firstStarted)
				<-releaseFirst
				return WatchResult{}, nil
			case 2:
				close(secondStarted)
				<-ctx.Done()
				return WatchResult{}, ctx.Err()
			default:
				return WatchResult{}, errors.New("duplicate watcher")
			}
		},
	})
	t.Cleanup(manager.Shutdown)
	if _, err := manager.Start("handoff-session", "fixture", true); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	<-firstStarted
	secondDone := make(chan error, 1)
	go func() {
		_, err := manager.Start("handoff-session", "fixture", true)
		secondDone <- err
	}()
	<-secondResolving
	close(releaseFirst)
	eventuallyBackground(t, func() bool {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		_, active := manager.active["handoff-session"]
		return !active
	})
	close(allowSecondResolve)
	if err := <-secondDone; err != nil {
		t.Fatalf("replacement Start: %v", err)
	}
	<-secondStarted

	thirdStarted := make(chan struct{}, 1)
	third := NewBackgroundManager(sessions, store, audit, BackgroundManagerOptions{
		Context:  t.Context(),
		Now:      backgroundTestClock,
		Resolver: resolver,
		Watcher: func(context.Context, *Store, AgentSpec, RunOptions, string, WatchOptions, func(WatchCycle)) (WatchResult, error) {
			thirdStarted <- struct{}{}
			return WatchResult{}, nil
		},
	})
	t.Cleanup(third.Shutdown)
	if _, err := third.Start("handoff-session", "fixture", true); !errors.Is(err, ErrBackgroundTransitionBusy) {
		t.Fatalf("third Start error = %v, want ErrBackgroundTransitionBusy", err)
	}
	select {
	case <-thirdStarted:
		t.Fatal("third manager launched while replacement watcher was active")
	default:
	}
}

func TestBackgroundManagerAuditReconciliationCannotOverwriteOwnedStart(t *testing.T) {
	dir := t.TempDir()
	sessions := NewStore(filepath.Join(dir, "sessions.json"))
	now := backgroundTestNow()
	if err := sessions.Upsert(SessionRecord{ID: "audit-race", Status: SessionStatusQueued, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("Upsert session: %v", err)
	}
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	auditPath := filepath.Join(dir, "background-audit.jsonl")
	seedAudit := NewBackgroundAudit(auditPath)
	terminal := backgroundRunnablePolicy(now.Add(-time.Minute))
	terminal.SessionID = "audit-race"
	terminal.Enabled = false
	terminal.State = BackgroundStateDisabled
	terminal.Outcome = BackgroundOutcomeCompleted
	if err := store.Upsert(terminal); err != nil {
		t.Fatalf("Upsert terminal policy: %v", err)
	}
	if err := seedAudit.Append(BackgroundAuditRecord{
		RecordID: "audit-race-terminal",
		At:       terminal.UpdatedAt, Action: BackgroundAuditStop, SessionID: terminal.SessionID,
		Profile: terminal.Profile, DescriptorHash: terminal.DescriptorHash, Outcome: terminal.Outcome,
	}); err != nil {
		t.Fatalf("Append terminal audit: %v", err)
	}

	appendBlocked := make(chan struct{})
	allowAppend := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(allowAppend)
		}
	}()
	startAudit := NewBackgroundAudit(auditPath)
	startAudit.beforeAppend = func(record BackgroundAuditRecord) {
		if record.Action != BackgroundAuditStart {
			return
		}
		close(appendBlocked)
		<-allowAppend
	}
	workerStarted := make(chan struct{})
	starter := NewBackgroundManager(sessions, store, startAudit, BackgroundManagerOptions{
		Context: t.Context(), Now: backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, terminal.DescriptorHash), nil
		},
		Watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
			close(workerStarted)
			<-ctx.Done()
			return WatchResult{}, ctx.Err()
		},
	})
	t.Cleanup(starter.Shutdown)
	startDone := make(chan error, 1)
	go func() {
		_, err := starter.Start("audit-race", terminal.Profile, true)
		startDone <- err
	}()
	<-appendBlocked

	reconciler := NewBackgroundManager(sessions, store, NewBackgroundAudit(auditPath), BackgroundManagerOptions{
		Context: t.Context(), Now: backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, terminal.DescriptorHash), nil
		},
		Watcher: func(context.Context, *Store, AgentSpec, RunOptions, string, WatchOptions, func(WatchCycle)) (WatchResult, error) {
			t.Fatal("reconciler launched a worker owned by another manager")
			return WatchResult{}, nil
		},
	})
	t.Cleanup(reconciler.Shutdown)
	if err := reconciler.Resume(); err != nil {
		t.Fatalf("Resume reconciler: %v", err)
	}
	policy, err := store.Get("audit-race")
	if err != nil {
		t.Fatalf("Get policy: %v", err)
	}
	if !policy.Enabled || policy.State != BackgroundStateRunning || policy.Outcome != BackgroundOutcomeStarted {
		t.Fatalf("policy during owned start = %#v, want enabled/running/started", policy)
	}
	close(allowAppend)
	released = true
	if err := <-startDone; err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-workerStarted
}

func TestBackgroundManagerReleaseErrorCannotOverwriteReplacementOwner(t *testing.T) {
	dir := t.TempDir()
	sessions := NewStore(filepath.Join(dir, "sessions.json"))
	now := backgroundTestNow()
	if err := sessions.Upsert(SessionRecord{ID: "release-race", Status: SessionStatusQueued, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	releaseFirst := make(chan struct{})
	firstStarted := make(chan struct{})
	first := NewBackgroundManager(sessions, store, audit, BackgroundManagerOptions{
		Context: t.Context(), Now: backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(context.Context, *Store, AgentSpec, RunOptions, string, WatchOptions, func(WatchCycle)) (WatchResult, error) {
			close(firstStarted)
			<-releaseFirst
			return WatchResult{}, nil
		},
	})
	t.Cleanup(first.Shutdown)
	replacementStarted := make(chan struct{})
	replacement := NewBackgroundManager(sessions, store, audit, BackgroundManagerOptions{
		Context: t.Context(), Now: backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
			close(replacementStarted)
			<-ctx.Done()
			return WatchResult{}, ctx.Err()
		},
	})
	t.Cleanup(replacement.Shutdown)
	if _, err := first.Start("release-race", "fixture", true); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	<-firstStarted
	replacementResult := make(chan error, 1)
	first.mu.Lock()
	worker := first.active["release-race"]
	originalRelease := worker.releaseLease
	worker.releaseLease = func() error {
		releaseErr := originalRelease()
		_, startErr := replacement.Start("release-race", "fixture", true)
		replacementResult <- startErr
		return errors.Join(releaseErr, errors.New("lease release confirmation failed"))
	}
	first.mu.Unlock()
	close(releaseFirst)
	if err := <-replacementResult; err != nil {
		t.Fatalf("replacement Start: %v", err)
	}
	<-replacementStarted
	eventuallyBackground(t, func() bool {
		first.mu.Lock()
		defer first.mu.Unlock()
		_, active := first.active["release-race"]
		return !active
	})
	policy, err := store.Get("release-race")
	if err != nil {
		t.Fatalf("Get policy: %v", err)
	}
	if !policy.Enabled || policy.State != BackgroundStateRunning || policy.Outcome != BackgroundOutcomeStarted {
		t.Fatalf("replacement policy = %#v, want enabled/running/started", policy)
	}
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

func TestBackgroundTransitionPersistsAuditEventIDBeforeAppend(t *testing.T) {
	dir := t.TempDir()
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	checked := make(chan error, 1)
	audit.beforeAppend = func(record BackgroundAuditRecord) {
		data, err := store.loadTransitions(store.transitionPath())
		if err != nil {
			checked <- err
			return
		}
		if len(data.Transitions) != 1 {
			checked <- fmt.Errorf("transitions = %#v, want one durable transition", data.Transitions)
			return
		}
		transition := data.Transitions[0]
		if transition.EventID == "" || transition.EventID != record.RecordID {
			checked <- fmt.Errorf("transition event ID = %q, record ID = %q", transition.EventID, record.RecordID)
			return
		}
		checked <- nil
	}
	manager := NewBackgroundManager(backgroundSessionStore(t), store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
			<-ctx.Done()
			return WatchResult{}, ctx.Err()
		},
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
		assertBackgroundPolicy(t, store, "session-1", BackgroundStateBlocked, BackgroundOutcomeProfileUntrusted, false)
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
		assertBackgroundPolicy(t, store, "session-1", BackgroundStateBlocked, BackgroundOutcomeProfileDrift, false)
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
	eventuallyBackground(t, func() bool {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		_, transitionBusy := manager.transitions["session-1"]
		return !transitionBusy
	})
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
	if err := manager.Resume(); !errors.Is(err, ErrBackgroundPersistenceDegraded) {
		t.Fatalf("Resume error = %v, want ErrBackgroundPersistenceDegraded", err)
	}
	if got := watchers.Load(); got != 1 {
		t.Fatalf("watchers after Resume = %d, want 1", got)
	}
}

func TestBackgroundManagerTerminalRecordingFailureFailsClosedAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	entered := make(chan struct{})
	release := make(chan struct{})
	manager := NewBackgroundManager(backgroundSessionStore(t), store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(context.Context, *Store, AgentSpec, RunOptions, string, WatchOptions, func(WatchCycle)) (WatchResult, error) {
			close(entered)
			<-release
			return WatchResult{}, errors.New("secret terminal failure")
		},
	})
	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-entered
	stalePolicy, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatalf("ReadFile stale policy: %v", err)
	}
	for _, path := range []string{store.Path(), audit.Path()} {
		if err := os.Remove(path); err != nil {
			t.Fatalf("Remove %s: %v", path, err)
		}
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatalf("Mkdir blocker %s: %v", path, err)
		}
	}
	close(release)
	var degraded BackgroundStatus
	eventuallyBackground(t, func() bool {
		degraded, err = manager.Get("session-1")
		return err == nil && degraded.State == BackgroundStateError
	})
	if !degraded.PersistenceDegraded || degraded.Enabled || degraded.Outcome != BackgroundOutcomeWorkerError {
		t.Fatalf("degraded status = %#v", degraded)
	}
	if strings.Contains(fmt.Sprintf("%#v", degraded), "secret terminal failure") {
		t.Fatalf("degraded status leaked raw error: %#v", degraded)
	}
	transitionRaw, err := os.ReadFile(store.transitionPath())
	if err != nil {
		t.Fatalf("ReadFile transition: %v", err)
	}
	for _, forbidden := range []string{"secret terminal failure", "prompt", "response", "argv", "command", "envValue", "credential"} {
		if strings.Contains(string(transitionRaw), forbidden) {
			t.Fatalf("transition contains forbidden %q metadata: %s", forbidden, transitionRaw)
		}
	}
	manager.Shutdown()
	for _, path := range []string{store.Path(), audit.Path()} {
		if err := os.Remove(path); err != nil {
			t.Fatalf("Remove blocker %s: %v", path, err)
		}
	}
	if err := os.WriteFile(store.Path(), stalePolicy, 0o600); err != nil {
		t.Fatalf("restore stale policy: %v", err)
	}

	var watchers atomic.Int32
	restarted := NewBackgroundManager(backgroundSessionStore(t), NewBackgroundStore(store.Path()), NewBackgroundAudit(audit.Path()), BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: countingBackgroundWatcher(&watchers, errors.New("unexpected restart launch")),
	})
	t.Cleanup(restarted.Shutdown)
	if err := restarted.Resume(); err != nil {
		t.Fatalf("Resume reconciliation: %v", err)
	}
	if got := watchers.Load(); got != 0 {
		t.Fatalf("restart watchers = %d, want 0", got)
	}
	status, err := restarted.Get("session-1")
	if err != nil {
		t.Fatalf("Get reconciled: %v", err)
	}
	if status.Enabled || status.State != BackgroundStateError || status.Outcome != BackgroundOutcomeWorkerError || status.PersistenceDegraded {
		t.Fatalf("reconciled status = %#v", status)
	}
	assertBackgroundAuditActions(t, NewBackgroundAudit(audit.Path()), BackgroundAuditError)
}

func TestBackgroundManagerTerminalAuditFailsClosedWhenStateAndTransitionCoFail(t *testing.T) {
	dir := t.TempDir()
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	entered := make(chan struct{})
	release := make(chan struct{})
	manager := NewBackgroundManager(backgroundSessionStore(t), store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(context.Context, *Store, AgentSpec, RunOptions, string, WatchOptions, func(WatchCycle)) (WatchResult, error) {
			close(entered)
			<-release
			return WatchResult{}, errors.New("secret co-failure")
		},
	})
	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-entered
	if err := os.Remove(store.Path()); err != nil {
		t.Fatalf("Remove policy: %v", err)
	}
	for _, path := range []string{store.Path(), store.transitionPath()} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatalf("Mkdir blocker %s: %v", path, err)
		}
	}
	close(release)
	eventuallyBackground(t, func() bool {
		status, getErr := manager.Get("session-1")
		return getErr == nil && status.State == BackgroundStateError && status.PersistenceDegraded
	})
	assertBackgroundAuditActions(t, audit, BackgroundAuditStart)
	rawAudit, err := os.ReadFile(audit.Path())
	if err != nil {
		t.Fatalf("ReadFile audit: %v", err)
	}
	for _, forbidden := range []string{"secret co-failure", "prompt", "response", "argv", "command", "envValue", "credential"} {
		if strings.Contains(string(rawAudit), forbidden) {
			t.Fatalf("audit contains forbidden %q metadata: %s", forbidden, rawAudit)
		}
	}
	manager.Shutdown()
}

func TestBackgroundManagerRecoveryUsesAuditAppendOrderAcrossClockRollback(t *testing.T) {
	dir := t.TempDir()
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	policyTime := time.Date(2026, 7, 13, 14, 0, 0, 0, time.UTC)
	policy := backgroundRunnablePolicy(policyTime)
	if err := store.Upsert(policy); err != nil {
		t.Fatalf("Upsert policy: %v", err)
	}
	for _, record := range []BackgroundAuditRecord{
		{
			RecordID:       "clock-rollback-start",
			At:             policyTime,
			Action:         BackgroundAuditStart,
			SessionID:      policy.SessionID,
			Profile:        policy.Profile,
			DescriptorHash: policy.DescriptorHash,
			Outcome:        BackgroundOutcomeStarted,
		},
		{
			RecordID:       "clock-rollback-error",
			At:             policyTime.Add(-time.Hour),
			Action:         BackgroundAuditError,
			SessionID:      policy.SessionID,
			Profile:        policy.Profile,
			DescriptorHash: policy.DescriptorHash,
			Outcome:        BackgroundOutcomeWorkerError,
		},
	} {
		if err := audit.Append(record); err != nil {
			t.Fatalf("Append %s: %v", record.Action, err)
		}
	}
	launched := make(chan struct{}, 1)
	manager := NewBackgroundManager(backgroundSessionStore(t), store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
			launched <- struct{}{}
			<-ctx.Done()
			return WatchResult{}, ctx.Err()
		},
	})
	t.Cleanup(manager.Shutdown)
	if err := manager.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	select {
	case <-launched:
		t.Fatal("Resume launched despite latest appended terminal audit")
	case <-time.After(100 * time.Millisecond):
	}
	status, err := manager.Get(policy.SessionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if status.Enabled || status.State != BackgroundStateError || status.Outcome != BackgroundOutcomeWorkerError {
		t.Fatalf("recovered status = %#v", status)
	}
}

func TestBackgroundManagerLatestSuccessfulAuditRemainsResumable(t *testing.T) {
	for _, latestAction := range []string{BackgroundAuditStart, BackgroundAuditResume} {
		t.Run(latestAction, func(t *testing.T) {
			dir := t.TempDir()
			store := NewBackgroundStore(filepath.Join(dir, "background.json"))
			audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
			policyTime := time.Date(2026, 7, 13, 14, 0, 0, 0, time.UTC)
			policy := backgroundRunnablePolicy(policyTime)
			if err := store.Upsert(policy); err != nil {
				t.Fatalf("Upsert policy: %v", err)
			}
			for _, record := range []BackgroundAuditRecord{
				{
					RecordID:       "latest-success-error",
					At:             policyTime,
					Action:         BackgroundAuditError,
					SessionID:      policy.SessionID,
					Profile:        policy.Profile,
					DescriptorHash: policy.DescriptorHash,
					Outcome:        BackgroundOutcomeWorkerError,
				},
				{
					RecordID:       "latest-success-" + latestAction,
					At:             policyTime.Add(-time.Hour),
					Action:         latestAction,
					SessionID:      policy.SessionID,
					Profile:        policy.Profile,
					DescriptorHash: policy.DescriptorHash,
					Outcome: map[string]string{
						BackgroundAuditStart:  BackgroundOutcomeStarted,
						BackgroundAuditResume: BackgroundOutcomeResumed,
					}[latestAction],
				},
			} {
				if err := audit.Append(record); err != nil {
					t.Fatalf("Append %s: %v", record.Action, err)
				}
			}
			launched := make(chan struct{}, 1)
			manager := NewBackgroundManager(backgroundSessionStore(t), store, audit, BackgroundManagerOptions{
				Context: t.Context(),
				Now:     backgroundTestClock,
				Resolver: func(name string) (ResolvedBackgroundProfile, error) {
					return trustedBackgroundProfile(name, "descriptor-hash"), nil
				},
				Watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
					launched <- struct{}{}
					<-ctx.Done()
					return WatchResult{}, ctx.Err()
				},
			})
			t.Cleanup(manager.Shutdown)
			if err := manager.Resume(); err != nil {
				t.Fatalf("Resume: %v", err)
			}
			select {
			case <-launched:
			case <-time.After(time.Second):
				t.Fatal("latest successful audit did not resume")
			}
		})
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

func TestBackgroundManagerStopAuditFailureKeepsWorkerUntilDurableRetry(t *testing.T) {
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
	if status.Enabled || status.State != BackgroundStateDisabled || status.Outcome != BackgroundOutcomeStopped || !status.PersistenceDegraded {
		t.Fatalf("status = %#v, want degraded disabled/stopped", status)
	}
	select {
	case <-checked:
		t.Fatal("worker canceled before stop audit was durable")
	case <-time.After(100 * time.Millisecond):
	}
	if err := os.Remove(audit.Path()); err != nil {
		t.Fatalf("Remove audit blocker: %v", err)
	}
	status, err = manager.Stop("session-1")
	if err != nil {
		t.Fatalf("Stop retry: %v", err)
	}
	if status.PersistenceDegraded {
		t.Fatalf("Stop retry remains degraded: %#v", status)
	}
	if err := <-checked; err != nil {
		t.Fatal(err)
	}
}

func TestBackgroundManagerStartRejectsStoppingSessionUntilWorkerJoins(t *testing.T) {
	var watchers atomic.Int32
	started := make(chan int, 2)
	canceled := make(chan int, 2)
	releaseFirst := make(chan struct{})
	manager, _, _ := newBackgroundTestManager(t, func(name string) (ResolvedBackgroundProfile, error) {
		return trustedBackgroundProfile(name, "descriptor-hash"), nil
	}, func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
		invocation := int(watchers.Add(1))
		started <- invocation
		<-ctx.Done()
		canceled <- invocation
		if invocation == 1 {
			<-releaseFirst
		}
		return WatchResult{}, ctx.Err()
	})
	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := <-started; got != 1 {
		t.Fatalf("first invocation = %d", got)
	}
	stopResult := make(chan error, 1)
	go func() {
		_, err := manager.Stop("session-1")
		stopResult <- err
	}()
	if got := <-canceled; got != 1 {
		t.Fatalf("canceled invocation = %d", got)
	}
	startResult := make(chan error, 1)
	go func() {
		_, err := manager.Start("session-1", "fixture", true)
		startResult <- err
	}()
	select {
	case err := <-startResult:
		if !errors.Is(err, ErrBackgroundTransitionBusy) {
			t.Fatalf("Start while stopping error = %v, want ErrBackgroundTransitionBusy", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Start blocked behind stopping worker")
	}
	close(releaseFirst)
	if err := <-stopResult; err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("Start after join: %v", err)
	}
	if got := <-started; got != 2 {
		t.Fatalf("second invocation = %d", got)
	}
	manager.Shutdown()
}

func TestBackgroundManagerCancellationDoesNotMaskUnrelatedTerminalFailure(t *testing.T) {
	tests := map[string]struct {
		watcher     BackgroundWatcher
		wantOutcome string
	}{
		"error": {
			watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
				<-ctx.Done()
				return WatchResult{}, errors.New("secret error after cancel")
			},
			wantOutcome: BackgroundOutcomeWorkerError,
		},
		"panic": {
			watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
				<-ctx.Done()
				panic("secret panic after cancel")
			},
			wantOutcome: BackgroundOutcomeWorkerPanic,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			manager, _, audit := newBackgroundTestManager(t, func(name string) (ResolvedBackgroundProfile, error) {
				return trustedBackgroundProfile(name, "descriptor-hash"), nil
			}, test.watcher)
			if _, err := manager.Start("session-1", "fixture", true); err != nil {
				t.Fatalf("Start: %v", err)
			}
			status, err := manager.Stop("session-1")
			if err != nil {
				t.Fatalf("Stop: %v", err)
			}
			if status.Enabled || status.State != BackgroundStateError || status.Outcome != test.wantOutcome {
				t.Fatalf("Stop status = %#v", status)
			}
			raw, err := os.ReadFile(audit.Path())
			if err != nil {
				t.Fatalf("ReadFile audit: %v", err)
			}
			if strings.Contains(string(raw), "secret error after cancel") || strings.Contains(string(raw), "secret panic after cancel") {
				t.Fatalf("audit leaked terminal content: %s", raw)
			}
		})
	}
}

func TestBackgroundManagerExplicitStopIgnoresCancellationDerivedError(t *testing.T) {
	manager, _, _ := newBackgroundTestManager(t, func(name string) (ResolvedBackgroundProfile, error) {
		return trustedBackgroundProfile(name, "descriptor-hash"), nil
	}, func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
		<-ctx.Done()
		return WatchResult{}, ctx.Err()
	})
	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	status, err := manager.Stop("session-1")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if status.Enabled || status.State != BackgroundStateDisabled || status.Outcome != BackgroundOutcomeStopped {
		t.Fatalf("Stop status = %#v", status)
	}
}

func TestBackgroundManagerExplicitStopPreservesJoinedIndependentError(t *testing.T) {
	manager, _, audit := newBackgroundTestManager(t, func(name string) (ResolvedBackgroundProfile, error) {
		return trustedBackgroundProfile(name, "descriptor-hash"), nil
	}, func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
		<-ctx.Done()
		return WatchResult{}, errors.Join(ctx.Err(), errors.New("secret joined failure"))
	})
	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	status, err := manager.Stop("session-1")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if status.Enabled || status.State != BackgroundStateError || status.Outcome != BackgroundOutcomeWorkerError {
		t.Fatalf("Stop status = %#v, want disabled worker_error", status)
	}
	raw, err := os.ReadFile(audit.Path())
	if err != nil {
		t.Fatalf("ReadFile audit: %v", err)
	}
	if strings.Contains(string(raw), "secret joined failure") {
		t.Fatalf("audit leaked joined worker error: %s", raw)
	}
}

func TestBackgroundManagerParentCancellationLeavesPolicyResumable(t *testing.T) {
	parent, cancelParent := context.WithCancel(t.Context())
	dir := t.TempDir()
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	workerCanceled := make(chan struct{})
	manager := NewBackgroundManager(backgroundSessionStore(t), store, audit, BackgroundManagerOptions{
		Context: parent,
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
			<-ctx.Done()
			close(workerCanceled)
			return WatchResult{}, ctx.Err()
		},
	})
	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	cancelParent()
	<-workerCanceled
	eventuallyBackground(t, func() bool {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		_, active := manager.active["session-1"]
		return !active
	})
	status, err := manager.Get("session-1")
	if err != nil {
		t.Fatalf("Get after parent cancellation: %v", err)
	}
	if !status.Enabled || status.State != BackgroundStateRunning || status.Outcome != BackgroundOutcomeStarted {
		t.Fatalf("status after parent cancellation = %#v, want resumable running policy", status)
	}
	assertBackgroundAuditActions(t, audit, BackgroundAuditStart)
	manager.Shutdown()

	resumed := make(chan struct{}, 1)
	restarted := NewBackgroundManager(backgroundSessionStore(t), NewBackgroundStore(store.Path()), NewBackgroundAudit(audit.Path()), BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
			resumed <- struct{}{}
			<-ctx.Done()
			return WatchResult{}, ctx.Err()
		},
	})
	t.Cleanup(restarted.Shutdown)
	if err := restarted.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	select {
	case <-resumed:
	case <-time.After(time.Second):
		t.Fatal("parent-canceled policy did not resume")
	}
}

func TestBackgroundManagerRejectsStartWhenContextAlreadyDone(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	manager := NewBackgroundManager(backgroundSessionStore(t), NewBackgroundStore(filepath.Join(t.TempDir(), "background.json")), NewBackgroundAudit(filepath.Join(t.TempDir(), "background-audit.jsonl")), BackgroundManagerOptions{
		Context: ctx,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(context.Context, *Store, AgentSpec, RunOptions, string, WatchOptions, func(WatchCycle)) (WatchResult, error) {
			t.Fatal("watcher launched with canceled manager context")
			return WatchResult{}, nil
		},
	})
	t.Cleanup(manager.Shutdown)
	if _, err := manager.Start("session-1", "fixture", true); !errors.Is(err, ErrBackgroundManagerClosed) {
		t.Fatalf("Start error = %v, want ErrBackgroundManagerClosed", err)
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

func TestBackgroundManagerClassifiesFiniteWatchQueueCompletion(t *testing.T) {
	dir := t.TempDir()
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	manager := NewBackgroundManager(backgroundSessionStore(t), store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		WatchOptions: WatchOptions{StopWhenEmpty: true, Interval: time.Millisecond},
	})
	t.Cleanup(manager.Shutdown)
	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	eventuallyBackground(t, func() bool {
		status, err := manager.Get("session-1")
		return err == nil && status.Outcome == BackgroundOutcomeCompleted
	})
	status, err := manager.Get("session-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if status.Enabled || status.State != BackgroundStateDisabled || status.Outcome != BackgroundOutcomeCompleted {
		t.Fatalf("completion status = %#v", status)
	}
	assertBackgroundPolicy(t, store, "session-1", BackgroundStateDisabled, BackgroundOutcomeCompleted, false)
	assertBackgroundAuditActions(t, audit, BackgroundAuditStart, BackgroundAuditStop)
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
		wantEnabled bool
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
			wantEnabled: false,
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
			assertBackgroundPolicy(t, store, policy.SessionID, test.wantState, test.wantOutcome, test.wantEnabled)
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
			assertBackgroundPolicy(t, store, policy.SessionID, BackgroundStateBlocked, test.wantOutcome, false)
			assertBackgroundAuditActions(t, audit, BackgroundAuditBlock)
			if got := watchers.Load(); got != 0 {
				t.Fatalf("watchers = %d, want 0", got)
			}
		})
	}
}

func TestBackgroundManagerBlockedPolicyRequiresExplicitStartAfterRestart(t *testing.T) {
	dir := t.TempDir()
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	sessions := backgroundSessionStore(t)
	manager := NewBackgroundManager(sessions, store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			profile := trustedBackgroundProfile(name, "descriptor-hash")
			profile.TrustValid = false
			return profile, nil
		},
		Watcher: func(context.Context, *Store, AgentSpec, RunOptions, string, WatchOptions, func(WatchCycle)) (WatchResult, error) {
			t.Fatal("blocked Start launched watcher")
			return WatchResult{}, nil
		},
	})
	status, err := manager.Start("session-1", "fixture", true)
	if !errors.Is(err, ErrBackgroundProfileUntrusted) {
		t.Fatalf("Start error = %v, want ErrBackgroundProfileUntrusted", err)
	}
	if status.Enabled || status.State != BackgroundStateBlocked {
		t.Fatalf("blocked status = %#v", status)
	}
	manager.Shutdown()

	var resumed atomic.Int32
	restarted := NewBackgroundManager(sessions, NewBackgroundStore(store.Path()), NewBackgroundAudit(audit.Path()), BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: countingBackgroundWatcher(&resumed, errors.New("explicit start terminal")),
	})
	t.Cleanup(restarted.Shutdown)
	if err := restarted.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if got := resumed.Load(); got != 0 {
		t.Fatalf("restart watchers = %d, want 0", got)
	}
	if _, err := restarted.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("explicit Start: %v", err)
	}
	eventuallyBackground(t, func() bool { return resumed.Load() == 1 })
}

func TestBackgroundManagerExplicitStartSupersedesStaleTerminalTransition(t *testing.T) {
	dir := t.TempDir()
	sessions := NewStore(filepath.Join(dir, "sessions.json"))
	now := backgroundTestNow()
	if err := sessions.Upsert(SessionRecord{ID: "session-1", Agent: "fixture", Status: SessionStatusQueued, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("Upsert session: %v", err)
	}
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	var calls atomic.Int32
	manager := NewBackgroundManager(sessions, store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
			if calls.Add(1) == 1 {
				return WatchResult{}, errors.New("terminal failure")
			}
			<-ctx.Done()
			return WatchResult{}, ctx.Err()
		},
	})
	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("initial Start: %v", err)
	}
	eventuallyBackground(t, func() bool {
		policy, err := store.Get("session-1")
		return err == nil && policy.State == BackgroundStateError
	})
	eventuallyBackground(t, func() bool {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		_, active := manager.active["session-1"]
		_, transitioning := manager.transitions["session-1"]
		return !active && !transitioning
	})
	terminal, err := store.Get("session-1")
	if err != nil {
		t.Fatalf("Get terminal policy: %v", err)
	}
	terminal.PersistenceDegraded = true
	if err := store.putTransition(backgroundTransition{EventID: "stale-terminal", Policy: terminal, Action: BackgroundAuditError}); err != nil {
		t.Fatalf("put stale transition: %v", err)
	}
	if _, err := os.Stat(store.transitionPath()); err != nil {
		t.Fatalf("stale transition is not durable before explicit Start: %v", err)
	}

	if _, err := manager.Start("session-1", "fixture", true); err != nil {
		t.Fatalf("explicit Start: %v", err)
	}
	eventuallyBackground(t, func() bool { return calls.Load() == 2 })
	manager.Shutdown()

	var resumed atomic.Int32
	restarted := NewBackgroundManager(sessions, NewBackgroundStore(store.Path()), NewBackgroundAudit(audit.Path()), BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
			resumed.Add(1)
			<-ctx.Done()
			return WatchResult{}, ctx.Err()
		},
	})
	t.Cleanup(restarted.Shutdown)
	if err := restarted.Resume(); err != nil {
		t.Fatalf("Resume after explicit Start: %v", err)
	}
	status, err := restarted.Get("session-1")
	if err != nil {
		t.Fatalf("Get resumed policy: %v", err)
	}
	if !status.Enabled || status.State != BackgroundStateRunning || status.Outcome != BackgroundOutcomeResumed {
		t.Fatalf("stale terminal transition overrode explicit Start: %#v", status)
	}
	eventuallyBackground(t, func() bool { return resumed.Load() == 1 })
}

func TestBackgroundManagerLaunchFailsClosedWhenTransitionCannotClear(t *testing.T) {
	dir := t.TempDir()
	sessions := NewStore(filepath.Join(dir, "sessions.json"))
	now := backgroundTestNow()
	if err := sessions.Upsert(SessionRecord{ID: "session-1", Agent: "fixture", Status: SessionStatusQueued, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("Upsert session: %v", err)
	}
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	if err := os.Mkdir(store.transitionPath(), 0o700); err != nil {
		t.Fatalf("Mkdir transition blocker: %v", err)
	}
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	var watchers atomic.Int32
	manager := NewBackgroundManager(sessions, store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
			watchers.Add(1)
			<-ctx.Done()
			return WatchResult{}, ctx.Err()
		},
	})
	t.Cleanup(manager.Shutdown)
	status, err := manager.Start("session-1", "fixture", true)
	if !errors.Is(err, ErrBackgroundPersistenceDegraded) {
		t.Fatalf("Start error = %v, want ErrBackgroundPersistenceDegraded", err)
	}
	if got := watchers.Load(); got != 0 {
		t.Fatalf("watchers = %d, want zero when stale transition cannot clear", got)
	}
	if status.Enabled || status.State != BackgroundStateError || status.Outcome != BackgroundOutcomeStateWriteFailed || !status.PersistenceDegraded {
		t.Fatalf("fail-closed status = %#v", status)
	}
	assertBackgroundAuditActions(t, audit)
}

func TestBackgroundTransitionRecoveryMissingAfterLeaseIsNoOp(t *testing.T) {
	dir := t.TempDir()
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	policy := backgroundRunnablePolicy(backgroundTestNow())
	transition, err := newBackgroundTransition(policy, BackgroundAuditStart)
	if err != nil {
		t.Fatalf("newBackgroundTransition: %v", err)
	}
	if err := store.putTransition(transition); err != nil {
		t.Fatalf("putTransition: %v", err)
	}
	store.afterListTransitionIDs = func() {
		if err := store.removeTransition(policy.SessionID); err != nil {
			t.Errorf("removeTransition: %v", err)
		}
	}
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	manager := NewBackgroundManager(backgroundSessionStore(t), store, audit, BackgroundManagerOptions{Context: t.Context()})
	t.Cleanup(manager.Shutdown)
	if err := manager.reconcileTransitions(); err != nil {
		t.Fatalf("reconcileTransitions: %v", err)
	}
	if _, err := os.Stat(store.Path()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("policy path after missing transition = %v, want not exist", err)
	}
	if records, err := audit.Read(); err != nil || len(records) != 0 {
		t.Fatalf("audit after missing transition = %#v, %v", records, err)
	}
}

func TestBackgroundManagerResumeRechecksPolicyAfterReservation(t *testing.T) {
	dir := t.TempDir()
	sessions := NewStore(filepath.Join(dir, "sessions.json"))
	now := backgroundTestNow()
	for _, sessionID := range []string{"session-1", "session-2"} {
		if err := sessions.Upsert(SessionRecord{ID: sessionID, Agent: "fixture", Status: SessionStatusQueued, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatalf("Upsert %s: %v", sessionID, err)
		}
	}
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	blockerReached := make(chan struct{})
	releaseBlocker := make(chan struct{})
	releaseTerminal := make(chan struct{})
	session2Started := make(chan struct{})
	var session2Calls atomic.Int32
	manager := NewBackgroundManager(sessions, store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     backgroundTestClock,
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			if name == "blocker" {
				close(blockerReached)
				<-releaseBlocker
			}
			return trustedBackgroundProfile(name, "descriptor-hash"), nil
		},
		Watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, sessionID string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
			if sessionID == "session-2" {
				invocation := session2Calls.Add(1)
				if invocation == 1 {
					close(session2Started)
					<-releaseTerminal
					return WatchResult{}, errors.New("secret terminal between snapshot and reservation")
				}
			}
			<-ctx.Done()
			return WatchResult{}, ctx.Err()
		},
	})
	t.Cleanup(manager.Shutdown)
	if _, err := manager.Start("session-2", "fixture", true); err != nil {
		t.Fatalf("Start session-2: %v", err)
	}
	<-session2Started
	blockerPolicy := backgroundRunnablePolicy(now)
	blockerPolicy.Profile = "blocker"
	if err := store.Upsert(blockerPolicy); err != nil {
		t.Fatalf("Upsert blocker policy: %v", err)
	}
	resumeDone := make(chan error, 1)
	go func() {
		resumeDone <- manager.Resume()
	}()
	<-blockerReached
	close(releaseTerminal)
	eventuallyBackground(t, func() bool {
		policy, err := store.Get("session-2")
		if err != nil || policy.Enabled || policy.State != BackgroundStateError {
			return false
		}
		manager.mu.Lock()
		defer manager.mu.Unlock()
		_, active := manager.active["session-2"]
		_, transitioning := manager.transitions["session-2"]
		return !active && !transitioning
	})
	close(releaseBlocker)
	if err := <-resumeDone; err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if got := session2Calls.Load(); got != 1 {
		t.Fatalf("session-2 watcher calls after Resume = %d, want 1", got)
	}
	assertBackgroundPolicy(t, store, "session-2", BackgroundStateError, BackgroundOutcomeWorkerError, false)
	if _, err := manager.Start("session-2", "fixture", true); err != nil {
		t.Fatalf("explicit Start session-2: %v", err)
	}
	eventuallyBackground(t, func() bool { return session2Calls.Load() == 2 })
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
	if resolvedBuiltin.DescriptorHash != builtin.Fingerprint() || !resolvedBuiltin.TrustValid || resolvedBuiltin.WithTrustedProfile != nil {
		t.Fatalf("resolved builtin = %#v", resolvedBuiltin)
	}
	resolvedProfile, err := resolve(profile.Name)
	if err != nil {
		t.Fatalf("resolve profile: %v", err)
	}
	if resolvedProfile.DescriptorHash != stored.DescriptorHash() || !resolvedProfile.TrustValid || resolvedProfile.Options.Cwd != profile.Cwd || resolvedProfile.WithTrustedProfile == nil {
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
