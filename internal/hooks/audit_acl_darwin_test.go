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
	for _, test := range []struct {
		name     string
		location string
		rights   string
	}{
		{name: "anchor delete", location: "anchor", rights: "delete,delete_child"},
		{name: "anchor write", location: "anchor", rights: "write,delete"},
		{name: "ancestor delete", location: "ancestor", rights: "delete,delete_child"},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			anchor := filepath.Join(base, "home")
			if err := os.Mkdir(anchor, 0o700); err != nil {
				t.Fatalf("Mkdir anchor: %v", err)
			}
			aclPath := anchor
			if test.location == "ancestor" {
				aclPath = base
			}
			command := exec.Command("/bin/chmod", "+a", "everyone allow "+test.rights, aclPath)
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

func TestManagedHookAuditRejectsInheritedMutationACL(t *testing.T) {
	anchor := t.TempDir()
	command := exec.Command("/bin/chmod", "+a", "everyone allow delete_child,directory_inherit,only_inherit", anchor)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("install inheritable ACL: %v\n%s", err, output)
	}
	t.Cleanup(func() {
		_ = exec.Command("/bin/chmod", "-N", anchor).Run()
	})
	path := filepath.Join(anchor, ".ratchet", "audit", "hooks.jsonl")

	err := NewHookAudit(path).Append(managedAuditRecord(HookAuditStarted))
	if err == nil || !strings.Contains(err.Error(), "ACL") {
		t.Fatalf("Append error = %v, want inheritable-ACL rejection", err)
	}
	if _, statErr := os.Stat(filepath.Join(anchor, ".ratchet")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("inheritable ACL gained audit namespace: %v", statErr)
	}
}

func TestManagedHookAuditRejectsMutationACLOnExistingPrivateObjects(t *testing.T) {
	for _, target := range []string{"namespace", "file"} {
		t.Run(target, func(t *testing.T) {
			path := managedAuditTestPath(t)
			audit := NewHookAudit(path)
			if err := audit.Append(managedAuditRecord(HookAuditStarted)); err != nil {
				t.Fatalf("seed Append: %v", err)
			}
			aclPath := path
			rights := "write"
			if target == "namespace" {
				aclPath = filepath.Dir(path)
				rights = "delete_child"
			}
			command := exec.Command("/bin/chmod", "+a", "everyone allow "+rights, aclPath)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("install object ACL: %v\n%s", err, output)
			}
			t.Cleanup(func() {
				_ = exec.Command("/bin/chmod", "-N", aclPath).Run()
			})
			if err := audit.Append(managedAuditRecord(HookAuditSuccess)); err == nil || !strings.Contains(err.Error(), "ACL") {
				t.Fatalf("Append error = %v, want existing-object ACL rejection", err)
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
