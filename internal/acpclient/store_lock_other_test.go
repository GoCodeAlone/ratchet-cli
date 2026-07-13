//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package acpclient

import (
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
