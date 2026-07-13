package acpclient

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	reorderedArgs := base
	reorderedArgs.Spec.Args = []string{"--model", "acp"}
	reorderedEnv := base
	reorderedEnv.Spec.EnvKeys = []string{"B", "A"}
	sortedEnv := base
	sortedEnv.Spec.EnvKeys = []string{"A", "B"}

	if base.DescriptorHash() == "" {
		t.Fatal("DescriptorHash is empty")
	}
	if base.DescriptorHash() == withArg.DescriptorHash() {
		t.Fatal("hash should change when args change")
	}
	if base.DescriptorHash() == withEnv.DescriptorHash() {
		t.Fatal("hash should change when env keys change")
	}
	if base.DescriptorHash() == reorderedArgs.DescriptorHash() {
		t.Fatal("hash should preserve args order")
	}
	if reorderedEnv.DescriptorHash() != sortedEnv.DescriptorHash() {
		t.Fatal("hash should sort env keys")
	}
}

func TestProfileDescriptorHashPreservesCommandAndCWDBytes(t *testing.T) {
	base := Profile{Name: "fixture", Spec: AgentSpec{Command: "fixture"}, Cwd: "work"}
	commandVariant := base
	commandVariant.Spec.Command = " fixture"
	cwdVariant := base
	cwdVariant.Cwd = " work"
	if commandVariant.DescriptorHash() == base.DescriptorHash() {
		t.Fatal("descriptor hash normalized command bytes")
	}
	if cwdVariant.DescriptorHash() == base.DescriptorHash() {
		t.Fatal("descriptor hash normalized cwd bytes")
	}
}

func TestProfileDescriptorHashPreservesFieldOrderAndNilEmptyEncoding(t *testing.T) {
	nilSlices := Profile{Name: "fixture", Spec: AgentSpec{Command: "fixture"}}
	emptySlices := nilSlices
	emptySlices.Spec.Args = []string{}
	emptySlices.Spec.EnvKeys = []string{}
	if got, want := nilSlices.DescriptorHash(), "2ccfc18db1dd6d167fe25cd9554f2db129fec8e415e47a81e5fa5d441d52dd7a"; got != want {
		t.Fatalf("nil-slice descriptor hash = %q, want stable field-order hash %q", got, want)
	}
	if nilSlices.DescriptorHash() == emptySlices.DescriptorHash() {
		t.Fatal("descriptor hash collapsed nil and empty slices")
	}
}

func TestProfileLegacyNormalizedTrustRequiresExplicitRetrust(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	profile := Profile{
		Name:      "fixture",
		Spec:      AgentSpec{Name: "fixture", Command: "fixture-agent"},
		Cwd:       " work",
		Trusted:   true,
		CreatedAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
	}
	legacyNormalized := profile
	legacyNormalized.Cwd = strings.TrimSpace(legacyNormalized.Cwd)
	profile.Hash = legacyNormalized.DescriptorHash()
	b, err := json.Marshal(profileFile{Profiles: []Profile{profile}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	store := NewProfileStore(path)
	stored, err := store.Get(profile.Name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.TrustValid() {
		t.Fatal("legacy normalized descriptor remained trusted")
	}
	if err := store.Trust(profile.Name); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	stored, err = store.Get(profile.Name)
	if err != nil {
		t.Fatalf("Get retrusted: %v", err)
	}
	if !stored.TrustValid() || stored.Hash != stored.DescriptorHash() {
		t.Fatalf("explicitly retrusted profile = %#v", stored)
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

func TestProfileWithTrustedProfileRequiresCurrentTrustedPinnedDescriptor(t *testing.T) {
	store := NewProfileStore(filepath.Join(t.TempDir(), "profiles.json"))
	profile := Profile{
		Name: "fixture",
		Spec: AgentSpec{Name: "fixture", Command: "/tmp/acp-agent"},
	}
	if err := store.Add(profile); err != nil {
		t.Fatalf("Add: %v", err)
	}
	stored, err := store.Get(profile.Name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	called := false
	callback := func(Profile) error {
		called = true
		return nil
	}
	if err := store.WithTrustedProfile(profile.Name, stored.DescriptorHash(), callback); err == nil {
		t.Fatal("WithTrustedProfile untrusted error = nil")
	}
	if called {
		t.Fatal("WithTrustedProfile invoked callback for untrusted profile")
	}
	if err := store.Trust(profile.Name); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	stored, err = store.Get(profile.Name)
	if err != nil {
		t.Fatalf("Get trusted: %v", err)
	}
	if err := store.WithTrustedProfile(profile.Name, "wrong-pinned-hash", callback); err == nil {
		t.Fatal("WithTrustedProfile mismatched pin error = nil")
	}
	if called {
		t.Fatal("WithTrustedProfile invoked callback for mismatched pin")
	}
	if err := store.WithTrustedProfile("missing", stored.DescriptorHash(), callback); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("WithTrustedProfile missing error = %v, want ErrProfileNotFound", err)
	}
	if called {
		t.Fatal("WithTrustedProfile invoked callback for missing profile")
	}

	if err := store.WithTrustedProfile(profile.Name, stored.DescriptorHash(), func(current Profile) error {
		called = true
		if current.Name != stored.Name || current.Hash != stored.Hash || !current.TrustValid() {
			t.Fatalf("leased profile = %#v, want current trusted profile %#v", current, stored)
		}
		return nil
	}); err != nil {
		t.Fatalf("WithTrustedProfile: %v", err)
	}
	if !called {
		t.Fatal("WithTrustedProfile did not invoke callback")
	}
}

func TestProfileWithTrustedProfileReleasesLockAfterErrorAndPanic(t *testing.T) {
	store := NewProfileStore(filepath.Join(t.TempDir(), "profiles.json"))
	profile := Profile{Name: "fixture", Spec: AgentSpec{Name: "fixture", Command: "/tmp/acp-agent"}}
	if err := store.Add(profile); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := store.Trust(profile.Name); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	stored, err := store.Get(profile.Name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	callbackErr := errors.New("callback failed")
	if err := store.WithTrustedProfile(profile.Name, stored.DescriptorHash(), func(Profile) error {
		return callbackErr
	}); !errors.Is(err, callbackErr) {
		t.Fatalf("WithTrustedProfile callback error = %v, want %v", err, callbackErr)
	}
	if _, err := store.List(); err != nil {
		t.Fatalf("List after callback error: %v", err)
	}

	func() {
		defer func() {
			if recovered := recover(); recovered != "lease panic" {
				t.Fatalf("recovered = %#v, want lease panic", recovered)
			}
		}()
		_ = store.WithTrustedProfile(profile.Name, stored.DescriptorHash(), func(Profile) error {
			panic("lease panic")
		})
	}()
	if _, err := store.List(); err != nil {
		t.Fatalf("List after callback panic: %v", err)
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
