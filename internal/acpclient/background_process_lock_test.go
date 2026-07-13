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
		{name: "audit", mode: "audit", state: "background-audit.jsonl", lockPath: func(path string) string { return path + ".lock" }},
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
		err = NewBackgroundStore(statePath).putTransition(backgroundTransition{Policy: policy, Action: BackgroundAuditStart})
	case "audit":
		err = NewBackgroundAudit(statePath).Append(BackgroundAuditRecord{
			At: now, Action: BackgroundAuditStart, SessionID: policy.SessionID,
			Profile: policy.Profile, DescriptorHash: policy.DescriptorHash, Outcome: policy.Outcome,
		})
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
