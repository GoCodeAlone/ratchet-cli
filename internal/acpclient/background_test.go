package acpclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackgroundStorePersistsMinimalOwnerOnlyAtomicPolicyState(t *testing.T) {
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
	if err := store.Upsert(policy); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := store.Get(policy.SessionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != policy {
		t.Fatalf("policy = %#v, want %#v", got, policy)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("policy mode = %o, want 600", got)
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".background.json.*.tmp"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(temps) != 0 {
		t.Fatalf("atomic temp files remain: %#v", temps)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
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
	manager.Shutdown()
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
