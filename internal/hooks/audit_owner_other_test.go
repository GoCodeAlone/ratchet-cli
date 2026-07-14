//go:build !windows

package hooks

import (
	"errors"
	"os"
	"path/filepath"
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
	path := filepath.Join(t.TempDir(), "private", "hooks.jsonl")
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
	path := filepath.Join(t.TempDir(), "private", "hooks.jsonl")
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
