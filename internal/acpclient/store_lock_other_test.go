//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package acpclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreProcessLocksFailExplicitlyOnUnsupportedPlatforms(t *testing.T) {
	if release, err := acquireStoreFileLock("sessions.json.lock"); release != nil || !errors.Is(err, ErrStoreProcessLockUnsupported) {
		t.Fatalf("acquireStoreFileLock = %p, %v; want nil, ErrStoreProcessLockUnsupported", release, err)
	}
	if release, acquired, err := tryStoreFileLock("owner.lock"); release != nil || acquired || !errors.Is(err, ErrStoreProcessLockUnsupported) {
		t.Fatalf("tryStoreFileLock = %p, %t, %v; want nil, false, ErrStoreProcessLockUnsupported", release, acquired, err)
	}
}

func TestBackgroundAuditUnsupportedPlatformFailsBeforeMutation(t *testing.T) {
	audit := NewBackgroundAudit(filepath.Join(t.TempDir(), "background-audit.jsonl"))
	err := audit.Append(BackgroundAuditRecord{
		RecordID:       "unsupported-platform",
		At:             time.Date(2026, 7, 13, 20, 0, 0, 0, time.UTC),
		Action:         BackgroundAuditError,
		SessionID:      "session-1",
		Profile:        "fixture",
		DescriptorHash: "descriptor-hash",
		Outcome:        BackgroundOutcomeWorkerError,
	})
	if !errors.Is(err, ErrStoreProcessLockUnsupported) {
		t.Fatalf("Append error = %v, want ErrStoreProcessLockUnsupported", err)
	}
	if _, err := os.Stat(filepath.Dir(audit.Path())); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("audit namespace after unsupported process lock = %v, want not exist", err)
	}
}

func TestProfileLaunchUnsupportedPlatformFailsBeforeMutationOrStart(t *testing.T) {
	dir := t.TempDir()
	mutationPath := filepath.Join(dir, "mutation-profiles.json")
	mutationStore := NewProfileStore(mutationPath)
	err := mutationStore.Add(Profile{Name: "fixture", Spec: AgentSpec{Name: "fixture", Command: "fixture-agent"}})
	if !errors.Is(err, ErrStoreProcessLockUnsupported) {
		t.Fatalf("Add error = %v, want ErrStoreProcessLockUnsupported", err)
	}
	if _, err := os.Stat(mutationPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("profile state after unsupported mutation = %v, want not exist", err)
	}

	launchPath := filepath.Join(dir, "launch-profiles.json")
	profile := Profile{Name: "fixture", Spec: AgentSpec{Name: "fixture", Command: "fixture-agent"}, Trusted: true}
	profile.Hash = profile.DescriptorHash()
	data, err := json.Marshal(profileFile{Profiles: []Profile{profile}})
	if err != nil {
		t.Fatalf("Marshal profile: %v", err)
	}
	if err := os.WriteFile(launchPath, data, 0o600); err != nil {
		t.Fatalf("WriteFile launch profile: %v", err)
	}
	started := false
	err = NewProfileStore(launchPath).WithTrustedProfile(profile.Name, profile.Hash, func(Profile) error {
		started = true
		return nil
	})
	if !errors.Is(err, ErrStoreProcessLockUnsupported) {
		t.Fatalf("WithTrustedProfile error = %v, want ErrStoreProcessLockUnsupported", err)
	}
	if started {
		t.Fatal("unsupported profile lease invoked launch callback")
	}
	after, err := os.ReadFile(launchPath)
	if err != nil {
		t.Fatalf("ReadFile launch profile: %v", err)
	}
	if !bytes.Equal(after, data) {
		t.Fatalf("launch profile changed on unsupported platform: %s", after)
	}
}
