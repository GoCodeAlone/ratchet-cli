package acpclient

import (
	"errors"
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
	_, err = DefaultRegistry().WithProfiles([]Profile{shadow})
	if !errors.Is(err, ErrProfileShadowsBuiltin) {
		t.Fatalf("WithProfiles shadow error = %v, want ErrProfileShadowsBuiltin", err)
	}
}
