package acpclient

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProfileStoreAddListRemovePersistsJSON(t *testing.T) {
	store := NewProfileStore(filepath.Join(t.TempDir(), "profiles.json"))
	profile := Profile{
		Name: "fixture",
		Spec: AgentSpec{
			Name:    "fixture",
			Command: "/tmp/acp-agent",
			Args:    []string{"--stdio"},
			EnvKeys: []string{"ANTHROPIC_API_KEY"},
		},
		Cwd:        "/tmp/project",
		SourceID:   "local:fixture",
		SourceKind: "local",
	}
	if err := store.Add(profile); err != nil {
		t.Fatalf("Add: %v", err)
	}

	reopened := NewProfileStore(store.Path())
	profiles, err := reopened.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1", len(profiles))
	}
	got := profiles[0]
	if got.Name != "fixture" || got.Spec.Command != "/tmp/acp-agent" || got.Cwd != "/tmp/project" {
		t.Fatalf("profile = %#v", got)
	}
	if got.Hash == "" || got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("profile missing generated metadata: %#v", got)
	}
	if got.Spec.EnvKeys[0] != "ANTHROPIC_API_KEY" {
		t.Fatalf("env keys not persisted: %#v", got.Spec.EnvKeys)
	}

	if err := reopened.Remove("fixture"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	after, err := reopened.List()
	if err != nil {
		t.Fatalf("List after remove: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("profiles after remove = %#v", after)
	}
}

func TestProfileStoreTrustAndRemoveTrimProfileName(t *testing.T) {
	store := NewProfileStore(filepath.Join(t.TempDir(), "profiles.json"))
	if err := store.Add(Profile{
		Name: "fixture",
		Spec: AgentSpec{Name: "fixture", Command: "/tmp/acp-agent"},
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := store.Trust(" fixture "); err != nil {
		t.Fatalf("Trust with whitespace: %v", err)
	}
	profile, err := store.Get("fixture")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !profile.Trusted {
		t.Fatalf("profile trusted = false, want true")
	}

	if err := store.Remove("\tfixture\n"); err != nil {
		t.Fatalf("Remove with whitespace: %v", err)
	}
	profiles, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("profiles after remove = %#v", profiles)
	}
}

func TestProfileStoreAddValidatesAgentSpec(t *testing.T) {
	store := NewProfileStore(filepath.Join(t.TempDir(), "profiles.json"))
	err := store.Add(Profile{Name: "bad", Spec: AgentSpec{Command: "codex acp"}})
	if !errors.Is(err, ErrShellCommand) {
		t.Fatalf("Add error = %v, want ErrShellCommand", err)
	}
}

func TestProfileHashChangesWithLaunchInputs(t *testing.T) {
	base := Profile{
		Name: "fixture",
		Spec: AgentSpec{Command: "codex", Args: []string{"acp"}, EnvKeys: []string{"A"}},
		Cwd:  "/tmp/project",
	}
	withArg := base
	withArg.Spec.Args = []string{"acp", "--model", "gpt-5"}
	withEnv := base
	withEnv.Spec.EnvKeys = []string{"B"}

	if base.DescriptorHash() == "" {
		t.Fatal("DescriptorHash is empty")
	}
	if base.DescriptorHash() == withArg.DescriptorHash() {
		t.Fatal("hash should change when args change")
	}
	if base.DescriptorHash() == withEnv.DescriptorHash() {
		t.Fatal("hash should change when env keys change")
	}
}

func TestProfileTrustInvalidatedByLaunchDescriptorDrift(t *testing.T) {
	base := Profile{
		Name: "fixture",
		Spec: AgentSpec{
			Name:    "fixture",
			Command: "/tmp/acp-agent",
			Args:    []string{"--stdio"},
			EnvKeys: []string{"ACP_TOKEN"},
		},
		Cwd:     "/tmp/project",
		Trusted: true,
	}
	base.Hash = base.DescriptorHash()
	if !base.TrustValid() {
		t.Fatal("unchanged profile trust is invalid")
	}

	tests := map[string]func(*Profile){
		"command": func(profile *Profile) { profile.Spec.Command = "/tmp/other-agent" },
		"args":    func(profile *Profile) { profile.Spec.Args = []string{"--stdio", "--verbose"} },
		"env key": func(profile *Profile) { profile.Spec.EnvKeys = []string{"OTHER_TOKEN"} },
		"cwd":     func(profile *Profile) { profile.Cwd = "/tmp/other-project" },
	}
	for name, drift := range tests {
		t.Run(name, func(t *testing.T) {
			profile := base
			profile.Spec.Args = append([]string(nil), base.Spec.Args...)
			profile.Spec.EnvKeys = append([]string(nil), base.Spec.EnvKeys...)
			drift(&profile)
			if profile.TrustValid() {
				t.Fatal("TrustValid = true after launch descriptor drift")
			}
		})
	}

	untrusted := base
	untrusted.Trusted = false
	if untrusted.TrustValid() {
		t.Fatal("TrustValid = true for untrusted profile")
	}
}

func TestProfileStorePreservesStaleTrustedHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	const staleHash = "0123456789abcdef"
	if err := os.WriteFile(path, []byte(`{
  "profiles": [{
    "name": "fixture",
    "spec": {"name": "fixture", "command": "/tmp/acp-agent"},
    "hash": "0123456789abcdef",
    "trusted": true,
    "createdAt": "2026-07-10T12:00:00Z",
    "updatedAt": "2026-07-10T12:00:00Z"
  }]
}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	profile, err := NewProfileStore(path).Get("fixture")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if profile.Hash != staleHash {
		t.Fatalf("Hash = %q, want preserved stale hash %q", profile.Hash, staleHash)
	}
	if profile.TrustValid() {
		t.Fatal("TrustValid = true for stale stored hash")
	}
}

func TestProfileStorePreservesEmptyTrustedHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	if err := os.WriteFile(path, []byte(`{
  "profiles": [{
    "name": "fixture",
    "spec": {"name": "fixture", "command": "/tmp/acp-agent"},
    "hash": "",
    "trusted": true,
    "createdAt": "2026-07-10T12:00:00Z",
    "updatedAt": "2026-07-10T12:00:00Z"
  }]
}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	profile, err := NewProfileStore(path).Get("fixture")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if profile.Hash != "" {
		t.Fatalf("Hash = %q, want empty trusted hash preserved", profile.Hash)
	}
	if profile.TrustValid() {
		t.Fatal("TrustValid = true for empty trusted hash")
	}
	reg, err := DefaultRegistry().WithProfiles([]Profile{profile})
	if err != nil {
		t.Fatalf("WithProfiles: %v", err)
	}
	if _, err := reg.Resolve(RunOptions{Agent: profile.Name}); !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("Resolve empty-hash trusted profile error = %v, want ErrUnknownAgent", err)
	}
}

func TestRegistryWithProfilesRequiresTrustAndRejectsBuiltinShadowing(t *testing.T) {
	untrusted := Profile{
		Name: "fixture",
		Spec: AgentSpec{Name: "fixture", Command: "/tmp/acp-agent"},
		Hash: "hash",
	}
	reg, err := DefaultRegistry().WithProfiles([]Profile{untrusted})
	if err != nil {
		t.Fatalf("WithProfiles untrusted: %v", err)
	}
	if _, err := reg.Resolve(RunOptions{Agent: "fixture"}); !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("Resolve untrusted profile error = %v, want ErrUnknownAgent", err)
	}

	trusted := untrusted
	trusted.Trusted = true
	trusted.Hash = trusted.DescriptorHash()
	reg, err = DefaultRegistry().WithProfiles([]Profile{trusted})
	if err != nil {
		t.Fatalf("WithProfiles trusted: %v", err)
	}
	spec, err := reg.Resolve(RunOptions{Agent: "fixture"})
	if err != nil {
		t.Fatalf("Resolve trusted profile: %v", err)
	}
	if spec.Command != "/tmp/acp-agent" {
		t.Fatalf("profile command = %q", spec.Command)
	}

	shadow := trusted
	shadow.Name = "codex"
	shadow.Spec.Name = "codex"
	shadow.Hash = shadow.DescriptorHash()
	_, err = DefaultRegistry().WithProfiles([]Profile{shadow})
	if !errors.Is(err, ErrProfileShadowsBuiltin) {
		t.Fatalf("WithProfiles shadow error = %v, want ErrProfileShadowsBuiltin", err)
	}
}
