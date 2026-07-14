//go:build !windows

package hooks

import (
	"os"
	"path/filepath"
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
