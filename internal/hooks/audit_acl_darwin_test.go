//go:build darwin

package hooks

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedHookAuditRejectsMutationACLOnTrustedAnchor(t *testing.T) {
	for _, location := range []string{"anchor", "ancestor"} {
		t.Run(location, func(t *testing.T) {
			base := t.TempDir()
			anchor := filepath.Join(base, "home")
			if err := os.Mkdir(anchor, 0o700); err != nil {
				t.Fatalf("Mkdir anchor: %v", err)
			}
			aclPath := anchor
			if location == "ancestor" {
				aclPath = base
			}
			command := exec.Command("/bin/chmod", "+a", "everyone allow delete,delete_child", aclPath)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("install mutation ACL: %v\n%s", err, output)
			}
			t.Cleanup(func() {
				_ = exec.Command("/bin/chmod", "-N", aclPath).Run()
			})
			path := filepath.Join(anchor, ".ratchet", "audit", "hooks.jsonl")

			err := NewHookAudit(path).Append(managedAuditRecord(HookAuditStarted))
			if err == nil || !strings.Contains(err.Error(), "ACL") {
				t.Fatalf("Append error = %v, want mutation-ACL rejection", err)
			}
			if _, statErr := os.Stat(filepath.Join(anchor, ".ratchet")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("unsafe ACL gained audit namespace: %v", statErr)
			}
		})
	}
}

func TestManagedHookAuditAllowsDenyOnlyACLOnTrustedAnchor(t *testing.T) {
	anchor := t.TempDir()
	command := exec.Command("/bin/chmod", "+a", "everyone deny delete,delete_child", anchor)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("install deny ACL: %v\n%s", err, output)
	}
	t.Cleanup(func() {
		_ = exec.Command("/bin/chmod", "-N", anchor).Run()
	})
	path := filepath.Join(anchor, ".ratchet", "audit", "hooks.jsonl")
	if err := NewHookAudit(path).Append(managedAuditRecord(HookAuditStarted)); err != nil {
		t.Fatalf("Append with deny-only ACL: %v", err)
	}
}
