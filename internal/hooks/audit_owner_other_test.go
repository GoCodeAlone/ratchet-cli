//go:build !windows

package hooks

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestManagedHookAuditOwnerUsesEffectiveUID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	uid, ok := hookAuditMetadataUint(info, "Uid")
	if !ok {
		t.Skip("platform file metadata does not expose Uid")
	}
	original := hookAuditEffectiveUID
	t.Cleanup(func() { hookAuditEffectiveUID = original })
	hookAuditEffectiveUID = func() int { return int(uid) }
	if err := validateHookAuditOwner(path, info); err != nil {
		t.Fatalf("matching effective UID: %v", err)
	}
	hookAuditEffectiveUID = func() int { return int(uid) + 1 }
	if err := validateHookAuditOwner(path, info); err == nil {
		t.Fatal("mismatching effective UID was accepted")
	}
}

func TestManagedHookAuditRejectsHardLink(t *testing.T) {
	path := managedAuditTestPath(t)
	audit := NewHookAudit(path)
	if err := audit.Append(managedAuditRecord(HookAuditStarted)); err != nil {
		t.Fatalf("seed Append: %v", err)
	}
	if err := os.Link(path, path+".link"); err != nil {
		t.Fatalf("Link: %v", err)
	}
	if err := audit.Append(managedAuditRecord(HookAuditSuccess)); err == nil {
		t.Fatal("Append accepted hard-linked audit target")
	}
}

func TestManagedHookAuditConcurrentFirstCreationReopensWinner(t *testing.T) {
	const workers = 32
	path := managedAuditTestPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	originalOpenFile := hookAuditOpenFile
	t.Cleanup(func() { hookAuditOpenFile = originalOpenFile })
	releaseCreates := make(chan struct{})
	var exclusiveCalls atomic.Int32
	var winners atomic.Int32
	var existing atomic.Int32
	hookAuditOpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		if flag&os.O_EXCL == 0 {
			return originalOpenFile(name, flag, perm)
		}
		if exclusiveCalls.Add(1) == workers {
			close(releaseCreates)
		}
		<-releaseCreates
		file, err := originalOpenFile(name, flag, perm)
		if err == nil {
			winners.Add(1)
		} else if errors.Is(err, os.ErrExist) {
			existing.Add(1)
		}
		return file, err
	}

	errs := make(chan error, workers)
	var createdResults atomic.Int32
	var group sync.WaitGroup
	for range workers {
		group.Go(func() {
			file, created, err := openHookAuditFile(path, true)
			if created {
				createdResults.Add(1)
			}
			if err == nil {
				err = file.Close()
			}
			errs <- err
		})
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent create: %v", err)
		}
	}
	if exclusiveCalls.Load() != workers || winners.Load() != 1 || existing.Load() != workers-1 || createdResults.Load() != 1 {
		t.Fatalf("creation paths = calls %d, winners %d, EEXIST %d, created results %d; want %d/1/%d/1",
			exclusiveCalls.Load(), winners.Load(), existing.Load(), createdResults.Load(), workers, workers-1)
	}
}

func TestManagedHookAuditRejectsUntrustedWritableAnchor(t *testing.T) {
	anchor := t.TempDir()
	if err := os.Chmod(anchor, 0o770); err != nil {
		t.Fatalf("Chmod anchor: %v", err)
	}
	path := filepath.Join(anchor, ".ratchet", "audit", "hooks.jsonl")

	err := NewHookAudit(path).Append(managedAuditRecord(HookAuditStarted))
	if err == nil || !strings.Contains(err.Error(), "trusted anchor") {
		t.Fatalf("Append error = %v, want trusted-anchor rejection", err)
	}
	if _, statErr := os.Stat(filepath.Join(anchor, ".ratchet")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("unsafe anchor gained audit namespace: %v", statErr)
	}
}

func TestManagedHookAuditRevalidatesTrustedAnchorIdentity(t *testing.T) {
	base := t.TempDir()
	anchor := filepath.Join(base, "home")
	if err := os.Mkdir(anchor, 0o700); err != nil {
		t.Fatalf("Mkdir anchor: %v", err)
	}
	path := filepath.Join(anchor, ".ratchet", "audit", "hooks.jsonl")
	release, err := acquireHookAuditTrustedAnchor(path)
	if err != nil {
		t.Fatalf("acquireHookAuditTrustedAnchor: %v", err)
	}
	displaced := anchor + ".displaced"
	if err := os.Rename(anchor, displaced); err != nil {
		t.Fatalf("Rename anchor: %v", err)
	}
	if err := os.Mkdir(anchor, 0o700); err != nil {
		t.Fatalf("Mkdir replacement anchor: %v", err)
	}
	if err := release(); err == nil || !strings.Contains(err.Error(), "trusted anchor changed") {
		t.Fatalf("release error = %v, want trusted-anchor identity failure", err)
	}
}
