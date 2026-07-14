//go:build !windows

package hooks

import (
	"os"
	"path/filepath"
	"sync"
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
	for attempt := range 20 {
		path := filepath.Join(t.TempDir(), "private", "hooks.jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("attempt %d: MkdirAll: %v", attempt, err)
		}
		start := make(chan struct{})
		errs := make(chan error, 32)
		var group sync.WaitGroup
		for range 32 {
			group.Go(func() {
				<-start
				file, _, err := openHookAuditFile(path, true)
				if err == nil {
					err = file.Close()
				}
				errs <- err
			})
		}
		close(start)
		group.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("attempt %d: concurrent create: %v", attempt, err)
			}
		}
	}
}
