package acpclient

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
		{name: "audit", mode: "audit", state: "background-audit.jsonl", lockPath: func(path string) string {
			return NewBackgroundAudit(path).Path() + ".lock"
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
			waitForStoreLockHelperReady(t, readyPath)
			time.Sleep(250 * time.Millisecond)
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
	release, err := acquireStoreFileLock(NewBackgroundAudit(auditPath).Path() + ".lock")
	if err != nil {
		t.Fatalf("hold audit lock: %v", err)
	}
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
	waitForStoreLockHelperReady(t, readyPath)
	time.Sleep(250 * time.Millisecond)
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

func TestBackgroundProcessLockHelper(t *testing.T) {
	if os.Getenv(backgroundLockHelperEnv) != "1" {
		return
	}
	if err := os.WriteFile(os.Getenv(backgroundLockReadyPathEnv), []byte("ready\n"), 0o600); err != nil {
		t.Fatalf("write ready marker: %v", err)
	}
	statePath := os.Getenv(backgroundLockStatePathEnv)
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
	var err error
	switch os.Getenv(backgroundLockModeEnv) {
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
		t.Fatalf("unknown helper mode %q", os.Getenv(backgroundLockModeEnv))
	}
	if err != nil {
		t.Fatalf("%s transaction: %v", os.Getenv(backgroundLockModeEnv), err)
	}
	if err := os.WriteFile(os.Getenv(backgroundLockDonePathEnv), []byte("done\n"), 0o600); err != nil {
		t.Fatalf("write done marker: %v", err)
	}
}
