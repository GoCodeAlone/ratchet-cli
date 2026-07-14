//go:build unix

package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestManagedHookAuditNamespaceNormalizesRestrictiveUmask(t *testing.T) {
	if os.Getenv("RATCHET_AUDIT_UMASK_CHILD") == "1" {
		syscall.Umask(0o777)
		if err := NewHookAudit(os.Getenv("RATCHET_AUDIT_UMASK_PATH")).Append(managedAuditRecord(HookAuditStarted)); err != nil {
			t.Fatalf("Append under restrictive umask: %v", err)
		}
		return
	}

	path := filepath.Join(t.TempDir(), ".ratchet", "audit", "hooks.jsonl")
	command := exec.Command(os.Args[0], "-test.run=^TestManagedHookAuditNamespaceNormalizesRestrictiveUmask$")
	command.Env = append(os.Environ(),
		"RATCHET_AUDIT_UMASK_CHILD=1",
		"RATCHET_AUDIT_UMASK_PATH="+path,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("umask child: %v\n%s", err, output)
	}
	for _, directory := range []string{filepath.Dir(path), filepath.Dir(filepath.Dir(path))} {
		info, err := os.Stat(directory)
		if err != nil {
			t.Fatalf("Stat %s: %v", directory, err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("mode %s = %04o, want 0700", directory, info.Mode().Perm())
		}
	}
}
