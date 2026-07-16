package acpclient

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	backgroundLockHelperEnv    = "RATCHET_BACKGROUND_LOCK_HELPER"
	backgroundLockModeEnv      = "RATCHET_BACKGROUND_LOCK_MODE"
	backgroundLockStatePathEnv = "RATCHET_BACKGROUND_LOCK_STATE_PATH"
	backgroundLockReadyPathEnv = "RATCHET_BACKGROUND_LOCK_READY_PATH"
	backgroundLockDonePathEnv  = "RATCHET_BACKGROUND_LOCK_DONE_PATH"
	backgroundLockAuditPathEnv = "RATCHET_BACKGROUND_LOCK_AUDIT_PATH"
)

func TestBackgroundStateTransactionsHonorProcessLocks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     string
		state    string
		lockPath func(string) string
	}{
		{name: "policy", mode: "policy", state: "background.json", lockPath: func(path string) string { return path + ".lock" }},
		{name: "transition", mode: "transition", state: "background.json", lockPath: func(path string) string {
			return NewBackgroundStore(path).transitionPath() + ".lock"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			statePath := filepath.Join(dir, tc.state)
			release, err := acquireStoreFileLock(tc.lockPath(statePath))
			if errors.Is(err, ErrStoreProcessLockUnsupported) {
				t.Skip("cross-process state locks are unsupported on this platform")
			}
			if err != nil {
				t.Fatalf("hold process lock: %v", err)
			}
			released := false
			defer func() {
				if !released {
					_ = release()
				}
			}()

			readyPath := filepath.Join(dir, "ready")
			donePath := filepath.Join(dir, "done")
			cmd := exec.Command(os.Args[0], "-test.run=^TestBackgroundProcessLockHelper$")
			cmd.Env = append(os.Environ(),
				backgroundLockHelperEnv+"=1",
				backgroundLockModeEnv+"="+tc.mode,
				backgroundLockStatePathEnv+"="+statePath,
				backgroundLockReadyPathEnv+"="+readyPath,
				backgroundLockDonePathEnv+"="+donePath,
			)
			var output bytes.Buffer
			cmd.Stdout = &output
			cmd.Stderr = &output
			if err := cmd.Start(); err != nil {
				t.Fatalf("start helper: %v", err)
			}
			waitForBackgroundLockContention(t, readyPath)
			if _, err := os.Stat(donePath); err == nil {
				_ = cmd.Wait()
				t.Fatalf("%s transaction ignored held process lock\n%s", tc.mode, output.String())
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("stat done marker: %v", err)
			}
			if err := release(); err != nil {
				t.Fatalf("release process lock: %v", err)
			}
			released = true
			if err := cmd.Wait(); err != nil {
				t.Fatalf("helper: %v\n%s", err, output.String())
			}
			if _, err := os.Stat(donePath); err != nil {
				t.Fatalf("done marker after release: %v", err)
			}
		})
	}
}

func runBackgroundAuditProcessLockBlocks(t *testing.T, hold func(*BackgroundAudit) (func() error, error)) {
	t.Helper()
	dir := t.TempDir()
	requestedPath := filepath.Join(dir, "background-audit.jsonl")
	audit := NewBackgroundAudit(requestedPath)
	if err := audit.Append(backgroundAuditTestRecord("process-lock-seed", BackgroundAuditStart, BackgroundOutcomeStarted)); err != nil {
		t.Fatalf("seed Append: %v", err)
	}
	release, err := hold(audit)
	if err != nil {
		t.Fatalf("hold audit lock: %v", err)
	}
	released := false
	defer func() {
		if !released {
			_ = release()
		}
	}()

	readyPath := filepath.Join(dir, "ready")
	donePath := filepath.Join(dir, "done")
	cmd := exec.Command(os.Args[0], "-test.run=^TestBackgroundProcessLockHelper$")
	cmd.Env = append(os.Environ(),
		backgroundLockHelperEnv+"=1",
		backgroundLockModeEnv+"=audit",
		backgroundLockStatePathEnv+"="+requestedPath,
		backgroundLockReadyPathEnv+"="+readyPath,
		backgroundLockDonePathEnv+"="+donePath,
	)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start audit helper: %v", err)
	}
	waitForBackgroundLockContention(t, readyPath)
	if _, err := os.Stat(donePath); err == nil {
		_ = cmd.Wait()
		t.Fatalf("audit append ignored held namespace lock\n%s", output.String())
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat done marker: %v", err)
	}
	if err := release(); err != nil {
		t.Fatalf("release audit lock: %v", err)
	}
	released = true
	if err := cmd.Wait(); err != nil {
		t.Fatalf("audit helper: %v\n%s", err, output.String())
	}
}

func TestBackgroundAuditTransitionReplayAcrossFreshProcesses(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "background.json")
	auditPath := filepath.Join(dir, "background-audit.jsonl")
	run := func(mode string) {
		t.Helper()
		readyPath := filepath.Join(dir, mode+"-ready")
		donePath := filepath.Join(dir, mode+"-done")
		cmd := exec.Command(os.Args[0], "-test.run=^TestBackgroundProcessLockHelper$")
		cmd.Env = append(os.Environ(),
			backgroundLockHelperEnv+"=1",
			backgroundLockModeEnv+"="+mode,
			backgroundLockStatePathEnv+"="+statePath,
			backgroundLockAuditPathEnv+"="+auditPath,
			backgroundLockReadyPathEnv+"="+readyPath,
			backgroundLockDonePathEnv+"="+donePath,
		)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s helper: %v\n%s", mode, err, output)
		}
	}

	run("audit-a")
	store := NewBackgroundStore(statePath)
	transition, found, err := store.getTransition("audit-a")
	if err != nil || !found {
		t.Fatalf("durable A transition = %#v, %t, %v", transition, found, err)
	}
	heldAudit := NewBackgroundAudit(auditPath)
	tx, err := backgroundOpenAuditTransaction(heldAudit.Path(), false)
	if err != nil {
		t.Fatalf("hold audit lock: %v", err)
	}
	release := tx.Close
	readyPath := filepath.Join(dir, "audit-b-ready")
	donePath := filepath.Join(dir, "audit-b-done")
	cmd := exec.Command(os.Args[0], "-test.run=^TestBackgroundProcessLockHelper$")
	cmd.Env = append(os.Environ(),
		backgroundLockHelperEnv+"=1",
		backgroundLockModeEnv+"=audit-b",
		backgroundLockStatePathEnv+"="+statePath,
		backgroundLockAuditPathEnv+"="+auditPath,
		backgroundLockReadyPathEnv+"="+readyPath,
		backgroundLockDonePathEnv+"="+donePath,
	)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		_ = release()
		t.Fatalf("start B helper: %v", err)
	}
	waitForBackgroundLockContention(t, readyPath)
	if _, err := os.Stat(donePath); err == nil {
		_ = release()
		_ = cmd.Wait()
		t.Fatalf("B append ignored A-held audit lock\n%s", output.String())
	} else if !errors.Is(err, os.ErrNotExist) {
		_ = release()
		t.Fatalf("stat B done marker: %v", err)
	}
	if err := release(); err != nil {
		t.Fatalf("release audit lock: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("B helper: %v\n%s", err, output.String())
	}
	run("audit-a2")

	records, err := NewBackgroundAudit(auditPath).Read()
	if err != nil {
		t.Fatalf("Read audit: %v", err)
	}
	counts := map[string]int{}
	for _, record := range records {
		counts[record.RecordID]++
	}
	if len(records) != 2 || counts[transition.EventID] != 1 || counts["audit-b-event"] != 1 {
		t.Fatalf("replayed audit records = %#v, counts = %#v", records, counts)
	}
	if _, found, err := store.getTransition("audit-a"); err != nil || found {
		t.Fatalf("A transition after A2 = found %t, err %v", found, err)
	}
}

func TestBackgroundTransitionRecoveryReloadsAfterProcessReplacement(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "background.json")
	store := NewBackgroundStore(statePath)
	now := time.Date(2026, 7, 13, 20, 0, 0, 0, time.UTC)
	stale := BackgroundPolicy{
		SessionID: "process-lock", Profile: "fixture", DescriptorHash: "hash",
		PolicyVersion: BackgroundPolicyVersion, AcknowledgedAt: now,
		State: BackgroundStateError, Outcome: BackgroundOutcomeWorkerError, UpdatedAt: now,
	}
	if err := store.putTransition(backgroundTransition{EventID: "stale-event", Policy: stale, Action: BackgroundAuditError}); err != nil {
		t.Fatalf("put stale transition: %v", err)
	}
	listed := make(chan struct{})
	reload := make(chan struct{})
	store.afterListTransitionIDs = func() {
		close(listed)
		<-reload
	}
	manager := NewBackgroundManager(
		NewStore(filepath.Join(dir, "sessions.json")),
		store,
		NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl")),
		BackgroundManagerOptions{Context: t.Context()},
	)
	t.Cleanup(manager.Shutdown)
	reconciled := make(chan error, 1)
	go func() { reconciled <- manager.reconcileTransitions() }()
	<-listed

	readyPath := filepath.Join(dir, "replace-ready")
	donePath := filepath.Join(dir, "replace-done")
	cmd := exec.Command(os.Args[0], "-test.run=^TestBackgroundProcessLockHelper$")
	cmd.Env = append(os.Environ(),
		backgroundLockHelperEnv+"=1",
		backgroundLockModeEnv+"=replace-transition",
		backgroundLockStatePathEnv+"="+statePath,
		backgroundLockReadyPathEnv+"="+readyPath,
		backgroundLockDonePathEnv+"="+donePath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		close(reload)
		t.Fatalf("replace transition helper: %v\n%s", err, output)
	}
	close(reload)
	if err := <-reconciled; err != nil {
		t.Fatalf("reconcileTransitions: %v", err)
	}
	records, err := manager.audit.Read()
	if err != nil {
		t.Fatalf("Read audit: %v", err)
	}
	if len(records) != 1 || records[0].RecordID != "current-event" || records[0].Action != BackgroundAuditStop {
		t.Fatalf("audit records = %#v, want current replacement only", records)
	}
}

func TestBackgroundProfileStartAndResumeCommitHoldProcessLease(t *testing.T) {
	for _, action := range []string{"start", "resume"} {
		t.Run(action, func(t *testing.T) {
			dir := t.TempDir()
			now := time.Date(2026, 7, 13, 22, 0, 0, 0, time.UTC)
			sessions := NewStore(filepath.Join(dir, "sessions.json"))
			if err := sessions.Upsert(SessionRecord{ID: "profile-commit", Status: SessionStatusQueued, CreatedAt: now, UpdatedAt: now}); err != nil {
				t.Fatalf("seed session: %v", err)
			}
			profiles := NewProfileStore(filepath.Join(dir, "profiles.json"))
			profile := Profile{Name: "fixture", Spec: AgentSpec{Name: "fixture", Command: "fixture-agent"}}
			if err := profiles.Add(profile); err != nil {
				t.Fatalf("Add profile: %v", err)
			}
			if err := profiles.Trust(profile.Name); err != nil {
				t.Fatalf("Trust profile: %v", err)
			}
			stored, err := profiles.Get(profile.Name)
			if err != nil {
				t.Fatalf("Get profile: %v", err)
			}
			store := NewBackgroundStore(filepath.Join(dir, "background.json"))
			if action == "resume" {
				policy := BackgroundPolicy{
					SessionID: "profile-commit", Profile: profile.Name, DescriptorHash: stored.DescriptorHash(),
					PolicyVersion: BackgroundPolicyVersion, AcknowledgedAt: now, Enabled: true,
					State: BackgroundStateRunning, Outcome: BackgroundOutcomeStarted, StartedAt: now, UpdatedAt: now,
				}
				if err := store.Upsert(policy); err != nil {
					t.Fatalf("seed background policy: %v", err)
				}
			}
			audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
			commitReached := make(chan struct{})
			allowCommit := make(chan struct{}, 1)
			var commitOnce sync.Once
			audit.syncParent = func(string) error {
				block := false
				commitOnce.Do(func() {
					block = true
					close(commitReached)
				})
				if block {
					<-allowCommit
				}
				return nil
			}
			manager := NewBackgroundManager(sessions, store, audit, BackgroundManagerOptions{
				Context:  t.Context(),
				Now:      func() time.Time { return now },
				Resolver: NewBackgroundProfileResolver(Registry{}, profiles),
				Watcher: func(ctx context.Context, _ *Store, _ AgentSpec, _ RunOptions, _ string, _ WatchOptions, _ func(WatchCycle)) (WatchResult, error) {
					<-ctx.Done()
					return WatchResult{}, ctx.Err()
				},
			})
			t.Cleanup(manager.Shutdown)
			t.Cleanup(func() {
				select {
				case allowCommit <- struct{}{}:
				default:
				}
			})

			operationDone := make(chan error, 1)
			go func() {
				if action == "start" {
					_, err := manager.Start("profile-commit", profile.Name, true)
					operationDone <- err
					return
				}
				operationDone <- manager.Resume()
			}()
			select {
			case <-commitReached:
			case <-time.After(3 * time.Second):
				t.Fatal("background commit did not reach audit durability acknowledgement")
			}

			child := startProfileProcess(t, profiles.Path(), "add", "peer")
			assertProfileProcessBlocked(t, child, action+" commit")
			allowCommit <- struct{}{}
			select {
			case err := <-operationDone:
				if err != nil {
					t.Fatalf("%s background policy: %v", action, err)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("%s did not acknowledge durable commit", action)
			}
			waitProfileProcess(t, child)
		})
	}
}

func TestBackgroundProfileUnsupportedLeaseFailsBeforeDurableWriteOrStart(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 13, 22, 15, 0, 0, time.UTC)
	sessions := NewStore(filepath.Join(dir, "sessions.json"))
	if err := sessions.Upsert(SessionRecord{
		ID: "profile-unsupported", Status: SessionStatusQueued, CreatedAt: now, UpdatedAt: now,
		PromptQueue: []QueuedPrompt{{ID: "q-1", Prompt: "must not launch", Status: QueuePromptStatusPending, CreatedAt: now}},
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	store := NewBackgroundStore(filepath.Join(dir, "background.json"))
	audit := NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl"))
	var leaseCallbacks atomic.Int32
	var starts atomic.Int32
	manager := NewBackgroundManager(sessions, store, audit, BackgroundManagerOptions{
		Context: t.Context(),
		Now:     func() time.Time { return now },
		Resolver: func(name string) (ResolvedBackgroundProfile, error) {
			return ResolvedBackgroundProfile{
				Spec:           AgentSpec{Name: name, Command: "fixture-agent"},
				DescriptorHash: "descriptor-hash",
				TrustValid:     true,
				WithTrustedProfile: func(string, func(Profile) error) error {
					leaseCallbacks.Add(1)
					return ErrStoreProcessLockUnsupported
				},
			}, nil
		},
		WatchOptions: WatchOptions{
			StartRunner: func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error) {
				starts.Add(1)
				return &fakeDrainRunner{sessionID: "must-not-start"}, func() error { return nil }, nil
			},
		},
	})
	t.Cleanup(manager.Shutdown)

	status, err := manager.Start("profile-unsupported", "fixture", true)
	if !errors.Is(err, ErrStoreProcessLockUnsupported) {
		t.Fatalf("Start error = %v, want ErrStoreProcessLockUnsupported", err)
	}
	if status != (BackgroundStatus{}) {
		t.Fatalf("Start status = %#v, want zero status", status)
	}
	if got := leaseCallbacks.Load(); got != 1 {
		t.Fatalf("profile lease calls = %d, want 1", got)
	}
	if got := starts.Load(); got != 0 {
		t.Fatalf("StartRunner calls = %d, want zero", got)
	}
	policies, listErr := store.List()
	if listErr != nil {
		t.Fatalf("List policies: %v", listErr)
	}
	if len(policies) != 0 {
		t.Fatalf("policies = %#v, want none", policies)
	}
	records, readErr := audit.Read()
	if readErr != nil {
		t.Fatalf("Read audit: %v", readErr)
	}
	if len(records) != 0 {
		t.Fatalf("audit records = %#v, want none", records)
	}
}

func TestACPClientLifecycleBinarySmokeBackgroundProfileLaunchHoldsProcessLeaseThroughRealStart(t *testing.T) {
	dir := t.TempDir()
	fixture := BuildFixtureAgent(t)
	manager, sessions, profiles, barriers := newBackgroundProfileProcessManager(t, dir, fixture, func(
		ctx context.Context,
		spec AgentSpec,
		opts RunOptions,
		existingID string,
		entered chan<- struct{},
		allowStart <-chan struct{},
		startReturned chan<- error,
		allowReturn <-chan struct{},
	) (DrainPromptRunner, func() error, error) {
		entered <- struct{}{}
		select {
		case <-allowStart:
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
		runner, closeRunner, err := defaultDrainStartRunner(ctx, spec, opts, existingID)
		startReturned <- err
		select {
		case <-allowReturn:
		case <-ctx.Done():
			if closeRunner != nil {
				_ = closeRunner()
			}
			return nil, nil, ctx.Err()
		}
		return runner, closeRunner, err
	})

	if _, err := manager.Start("profile-launch", "fixture", true); err != nil {
		t.Fatalf("Start background manager: %v", err)
	}
	waitBackgroundLaunchEntered(t, barriers.entered)
	child := startProfileProcess(t, profiles.Path(), "remove", "fixture")
	assertProfileProcessBlocked(t, child, "real launch before Start")
	close(barriers.allowStart)
	if err := waitBackgroundStartReturned(t, barriers.startReturned); err != nil {
		t.Fatalf("defaultDrainStartRunner: %v", err)
	}
	assertProfileProcessBlocked(t, child, "real launch after Start acknowledgement")
	close(barriers.allowReturn)
	waitProfileProcess(t, child)

	eventuallyBackground(t, func() bool {
		record, err := sessions.Get("profile-launch")
		return err == nil && len(record.PromptQueue) == 1 && record.PromptQueue[0].Status == QueuePromptStatusCompleted
	})
}

func TestACPClientLifecycleBinarySmokeBackgroundProfileLaunchReleasesProcessLeaseAfterStartFailure(t *testing.T) {
	dir := t.TempDir()
	missingCommand := filepath.Join(dir, "missing-agent")
	manager, _, profiles, barriers := newBackgroundProfileProcessManager(t, dir, missingCommand, func(
		ctx context.Context,
		spec AgentSpec,
		opts RunOptions,
		existingID string,
		entered chan<- struct{},
		allowStart <-chan struct{},
		startReturned chan<- error,
		allowReturn <-chan struct{},
	) (DrainPromptRunner, func() error, error) {
		entered <- struct{}{}
		select {
		case <-allowStart:
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
		runner, closeRunner, err := defaultDrainStartRunner(ctx, spec, opts, existingID)
		startReturned <- err
		select {
		case <-allowReturn:
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
		return runner, closeRunner, err
	})

	if _, err := manager.Start("profile-launch", "fixture", true); err != nil {
		t.Fatalf("Start background manager: %v", err)
	}
	waitBackgroundLaunchEntered(t, barriers.entered)
	child := startProfileProcess(t, profiles.Path(), "add", "peer")
	assertProfileProcessBlocked(t, child, "failed launch before Start")
	close(barriers.allowStart)
	if err := waitBackgroundStartReturned(t, barriers.startReturned); err == nil {
		t.Fatal("defaultDrainStartRunner error = nil for missing command")
	}
	assertProfileProcessBlocked(t, child, "failed launch after Start acknowledgement")
	close(barriers.allowReturn)
	waitProfileProcess(t, child)
	eventuallyBackground(t, func() bool {
		status, err := manager.Get("profile-launch")
		return err == nil && status.State == BackgroundStateError && status.Outcome == BackgroundOutcomeWorkerError
	})
}

func TestBackgroundProfileLaunchRejectsMissingUntrustedAndDriftedProfileBeforeStart(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *ProfileStore)
	}{
		{
			name: "missing",
			mutate: func(t *testing.T, profiles *ProfileStore) {
				if err := profiles.Remove("fixture"); err != nil {
					t.Fatalf("Remove profile: %v", err)
				}
			},
		},
		{
			name: "untrusted",
			mutate: func(t *testing.T, profiles *ProfileStore) {
				if err := profiles.Add(Profile{Name: "fixture", Spec: AgentSpec{Name: "fixture", Command: "fixture-agent"}}); err != nil {
					t.Fatalf("replace profile: %v", err)
				}
			},
		},
		{
			name: "drifted",
			mutate: func(t *testing.T, profiles *ProfileStore) {
				if err := profiles.Add(Profile{Name: "fixture", Spec: AgentSpec{Name: "fixture", Command: "drifted-agent"}}); err != nil {
					t.Fatalf("replace profile: %v", err)
				}
				if err := profiles.Trust("fixture"); err != nil {
					t.Fatalf("Trust drifted profile: %v", err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			now := time.Date(2026, 7, 13, 23, 0, 0, 0, time.UTC)
			sessions := NewStore(filepath.Join(dir, "sessions.json"))
			if err := sessions.Upsert(SessionRecord{ID: "profile-revalidate", Status: SessionStatusQueued, CreatedAt: now, UpdatedAt: now}); err != nil {
				t.Fatalf("seed session: %v", err)
			}
			profiles := NewProfileStore(filepath.Join(dir, "profiles.json"))
			if err := profiles.Add(Profile{Name: "fixture", Spec: AgentSpec{Name: "fixture", Command: "fixture-agent"}}); err != nil {
				t.Fatalf("Add profile: %v", err)
			}
			if err := profiles.Trust("fixture"); err != nil {
				t.Fatalf("Trust profile: %v", err)
			}
			idle := make(chan struct{}, 1)
			proceed := make(chan struct{})
			var starts atomic.Int32
			manager := NewBackgroundManager(
				sessions,
				NewBackgroundStore(filepath.Join(dir, "background.json")),
				NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl")),
				BackgroundManagerOptions{
					Context:  t.Context(),
					Now:      func() time.Time { return now },
					Resolver: NewBackgroundProfileResolver(Registry{}, profiles),
					WatchOptions: WatchOptions{
						Interval: time.Millisecond,
						Now:      func() time.Time { return now },
						Sleep: func(ctx context.Context, _ time.Duration) error {
							select {
							case idle <- struct{}{}:
							default:
							}
							select {
							case <-proceed:
								return nil
							case <-ctx.Done():
								return ctx.Err()
							}
						},
						StartRunner: func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error) {
							starts.Add(1)
							return &fakeDrainRunner{sessionID: "must-not-start"}, func() error { return nil }, nil
						},
					},
				},
			)
			t.Cleanup(manager.Shutdown)
			if _, err := manager.Start("profile-revalidate", "fixture", true); err != nil {
				t.Fatalf("Start background manager: %v", err)
			}
			select {
			case <-idle:
			case <-time.After(3 * time.Second):
				t.Fatal("WatchQueue did not reach idle barrier")
			}
			test.mutate(t, profiles)
			if _, err := sessions.AppendQueuedPrompt(SessionRecord{ID: "profile-revalidate"}, QueuedPrompt{
				ID: "q-1", Prompt: "must not launch", CreatedAt: now.Add(time.Second),
			}); err != nil {
				t.Fatalf("AppendQueuedPrompt: %v", err)
			}
			close(proceed)
			eventuallyBackground(t, func() bool {
				status, err := manager.Get("profile-revalidate")
				return err == nil && status.State == BackgroundStateError
			})
			if got := starts.Load(); got != 0 {
				t.Fatalf("StartRunner calls = %d, want zero", got)
			}
		})
	}
}

type backgroundLaunchBarriers struct {
	entered       chan struct{}
	allowStart    chan struct{}
	startReturned chan error
	allowReturn   chan struct{}
}

func waitBackgroundLaunchEntered(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	waitACPClientProcessValue(t, entered, "profile launch StartRunner barrier")
}

func waitBackgroundStartReturned(t *testing.T, returned <-chan error) error {
	t.Helper()
	return waitACPClientProcessValue(t, returned, "profile launch StartRunner acknowledgment")
}

type backgroundBarrierStartRunner func(
	context.Context,
	AgentSpec,
	RunOptions,
	string,
	chan<- struct{},
	<-chan struct{},
	chan<- error,
	<-chan struct{},
) (DrainPromptRunner, func() error, error)

func newBackgroundProfileProcessManager(t *testing.T, dir, command string, start backgroundBarrierStartRunner) (*BackgroundManager, *Store, *ProfileStore, *backgroundLaunchBarriers) {
	t.Helper()
	now := time.Date(2026, 7, 13, 22, 30, 0, 0, time.UTC)
	sessions := NewStore(filepath.Join(dir, "sessions.json"))
	if err := sessions.Upsert(SessionRecord{
		ID: "profile-launch", Status: SessionStatusQueued, CreatedAt: now, UpdatedAt: now,
		PromptQueue: []QueuedPrompt{{ID: "q-1", Prompt: "release-shaped launch", Status: QueuePromptStatusPending, CreatedAt: now}},
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	profiles := NewProfileStore(filepath.Join(dir, "profiles.json"))
	profile := Profile{Name: "fixture", Spec: AgentSpec{Name: "fixture", Command: command}, Cwd: dir}
	if err := profiles.Add(profile); err != nil {
		t.Fatalf("Add profile: %v", err)
	}
	if err := profiles.Trust(profile.Name); err != nil {
		t.Fatalf("Trust profile: %v", err)
	}
	barriers := &backgroundLaunchBarriers{
		entered: make(chan struct{}, 1), allowStart: make(chan struct{}),
		startReturned: make(chan error, 1), allowReturn: make(chan struct{}),
	}
	manager := NewBackgroundManager(
		sessions,
		NewBackgroundStore(filepath.Join(dir, "background.json")),
		NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl")),
		BackgroundManagerOptions{
			Context:  t.Context(),
			Now:      func() time.Time { return now },
			Resolver: NewBackgroundProfileResolver(Registry{}, profiles),
			WatchOptions: WatchOptions{
				Interval: time.Millisecond, MaxPerCycle: 1, StopWhenEmpty: true,
				Now: func() time.Time { return now }, Sleep: instantWatchSleep,
				StartRunner: func(ctx context.Context, spec AgentSpec, opts RunOptions, existingID string) (DrainPromptRunner, func() error, error) {
					return start(ctx, spec, opts, existingID, barriers.entered, barriers.allowStart, barriers.startReturned, barriers.allowReturn)
				},
			},
		},
	)
	t.Cleanup(manager.Shutdown)
	return manager, sessions, profiles, barriers
}

func TestBackgroundProcessLockHelper(t *testing.T) {
	if os.Getenv(backgroundLockHelperEnv) != "1" {
		return
	}
	statePath := os.Getenv(backgroundLockStatePathEnv)
	mode := os.Getenv(backgroundLockModeEnv)
	contended, err := backgroundProcessLockContended(mode, statePath, os.Getenv(backgroundLockAuditPathEnv))
	if err != nil {
		t.Fatalf("probe %s process lock: %v", mode, err)
	}
	if contended {
		if err := backgroundWriteFileAtomic(os.Getenv(backgroundLockReadyPathEnv), []byte("contended\n")); err != nil {
			t.Fatalf("write contention marker: %v", err)
		}
	}
	now := time.Date(2026, 7, 13, 20, 0, 0, 0, time.UTC)
	policy := BackgroundPolicy{
		SessionID:      "process-lock",
		Profile:        "fixture",
		DescriptorHash: "hash",
		PolicyVersion:  BackgroundPolicyVersion,
		AcknowledgedAt: now,
		Enabled:        true,
		State:          BackgroundStateRunning,
		Outcome:        BackgroundOutcomeStarted,
		UpdatedAt:      now,
	}
	switch mode {
	case "policy":
		err = NewBackgroundStore(statePath).Upsert(policy)
	case "transition":
		err = NewBackgroundStore(statePath).putTransition(backgroundTransition{EventID: "process-lock-transition", Policy: policy, Action: BackgroundAuditStart})
	case "replace-transition":
		policy.Enabled = false
		policy.State = BackgroundStateDisabled
		policy.Outcome = BackgroundOutcomeCompleted
		err = NewBackgroundStore(statePath).putTransition(backgroundTransition{EventID: "current-event", Policy: policy, Action: BackgroundAuditStop})
	case "audit":
		err = NewBackgroundAudit(statePath).Append(BackgroundAuditRecord{
			RecordID: "process-lock-audit",
			At:       now, Action: BackgroundAuditStart, SessionID: policy.SessionID,
			Profile: policy.Profile, DescriptorHash: policy.DescriptorHash, Outcome: policy.Outcome,
		})
	case "audit-a":
		policy.SessionID = "audit-a"
		policy.Enabled = false
		policy.State = BackgroundStateError
		policy.Outcome = BackgroundOutcomeWorkerError
		audit := NewBackgroundAudit(os.Getenv(backgroundLockAuditPathEnv))
		audit.syncParent = func(string) error { return errors.New("injected parent sync uncertainty") }
		manager := NewBackgroundManager(
			NewStore(filepath.Join(filepath.Dir(statePath), "sessions.json")),
			NewBackgroundStore(statePath),
			audit,
			BackgroundManagerOptions{},
		)
		result := manager.persistTerminal(policy, BackgroundAuditError)
		manager.Shutdown()
		if !errors.Is(result.err, ErrStoreCommitUnconfirmed) {
			t.Fatalf("A persistence error = %v, want ErrStoreCommitUnconfirmed", result.err)
		}
	case "audit-b":
		err = NewBackgroundAudit(os.Getenv(backgroundLockAuditPathEnv)).Append(BackgroundAuditRecord{
			RecordID: "audit-b-event",
			At:       now, Action: BackgroundAuditError, SessionID: "audit-b",
			Profile: "fixture", DescriptorHash: "hash", Outcome: BackgroundOutcomeWorkerError,
		})
	case "audit-a2":
		manager := NewBackgroundManager(
			NewStore(filepath.Join(filepath.Dir(statePath), "sessions.json")),
			NewBackgroundStore(statePath),
			NewBackgroundAudit(os.Getenv(backgroundLockAuditPathEnv)),
			BackgroundManagerOptions{},
		)
		err = manager.reconcileTransitions()
		manager.Shutdown()
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}
	if err != nil {
		t.Fatalf("%s transaction: %v", mode, err)
	}
	if err := os.WriteFile(os.Getenv(backgroundLockDonePathEnv), []byte("done\n"), 0o600); err != nil {
		t.Fatalf("write done marker: %v", err)
	}
}

func backgroundProcessLockContended(mode, statePath, auditPath string) (bool, error) {
	var logicalPath string
	switch mode {
	case "policy":
		logicalPath = statePath + ".lock"
	case "transition":
		logicalPath = NewBackgroundStore(statePath).transitionPath() + ".lock"
	case "audit":
		return backgroundAuditLockContended(NewBackgroundAudit(statePath).Path())
	case "audit-b":
		return backgroundAuditLockContended(NewBackgroundAudit(auditPath).Path())
	default:
		return false, nil
	}
	release, acquired, err := tryStoreFileLock(logicalPath)
	if err != nil {
		return false, err
	}
	if !acquired {
		return true, nil
	}
	return false, errors.Join(errors.New("expected process lock contention"), release())
}

func waitForBackgroundLockContention(t *testing.T, path string) {
	t.Helper()
	waitForStoreLockHelperReady(t, path)
	marker, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read lock contention marker: %v", err)
	}
	if string(marker) != "contended\n" {
		t.Fatalf("lock contention marker = %q, want %q", marker, "contended\\n")
	}
}
