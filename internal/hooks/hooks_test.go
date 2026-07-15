package hooks

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg, err := Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Hooks) != 0 {
		t.Errorf("expected empty hooks, got %d", len(cfg.Hooks))
	}
}

func TestLoadFromFile(t *testing.T) {
	tmp := t.TempDir()
	homeDir := filepath.Join(tmp, "home")
	workDir := filepath.Join(tmp, "work")
	os.MkdirAll(homeDir, 0700)
	os.MkdirAll(workDir, 0700)
	t.Setenv("HOME", homeDir)

	ratchetDir := filepath.Join(homeDir, ".ratchet")
	os.MkdirAll(ratchetDir, 0700)
	os.WriteFile(filepath.Join(ratchetDir, "hooks.yaml"), []byte(`
hooks:
  post-edit:
    - command: "echo edited {{.file}}"
      glob: "*.go"
`), 0600)

	cfg, err := Load(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Hooks[PostEdit]) != 1 {
		t.Errorf("expected 1 post-edit hook, got %d", len(cfg.Hooks[PostEdit]))
	}
	if cfg.Hooks[PostEdit][0].Glob != "*.go" {
		t.Errorf("expected glob '*.go', got %s", cfg.Hooks[PostEdit][0].Glob)
	}
}

func TestExpandTemplate(t *testing.T) {
	result, err := expandTemplate("echo {{.file}} {{.command}}", map[string]string{
		"file":    "main.go",
		"command": "build",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != "echo main.go build" {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestHookConfig_NewEvents(t *testing.T) {
	newEvents := []Event{
		PrePlan, PostPlan,
		PreFleet, PostFleet,
		OnAgentSpawn, OnAgentComplete,
		OnTokenLimit, OnCronTick,
	}
	cfg := &HookConfig{
		Hooks: make(map[Event][]Hook),
	}
	for _, e := range newEvents {
		cfg.Hooks[e] = []Hook{{Command: "true"}}
	}
	for _, e := range newEvents {
		if err := cfg.Run(e, map[string]string{}); err != nil {
			t.Errorf("Run(%s): %v", e, err)
		}
	}
}

func TestHookConfig_TemplateExpansion_NewKeys(t *testing.T) {
	cases := []struct {
		tmpl string
		data map[string]string
		want string
	}{
		{"plan {{.plan_id}}", map[string]string{"plan_id": "p123"}, "plan p123"},
		{"fleet {{.fleet_id}}", map[string]string{"fleet_id": "f456"}, "fleet f456"},
		{"agent {{.agent_name}} {{.agent_role}}", map[string]string{"agent_name": "worker-1", "agent_role": "executor"}, "agent worker-1 executor"},
		{"cron {{.cron_id}}", map[string]string{"cron_id": "c789"}, "cron c789"},
		{"tokens {{.tokens_used}}/{{.tokens_limit}}", map[string]string{"tokens_used": "4000", "tokens_limit": "8192"}, "tokens 4000/8192"},
	}
	for _, tc := range cases {
		t.Run(tc.tmpl, func(t *testing.T) {
			got, err := expandTemplate(tc.tmpl, tc.data)
			if err != nil {
				t.Fatalf("expandTemplate: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAllEventsComplete(t *testing.T) {
	// Ensure AllEvents contains all defined constants.
	if len(AllEvents) < 19 {
		t.Errorf("AllEvents has %d entries, expected at least 19", len(AllEvents))
	}
}

func TestRunGlobFilter(t *testing.T) {
	cfg := &HookConfig{
		Hooks: map[Event][]Hook{
			PostEdit: {
				{Command: "true", Glob: "*.go"},
			},
		},
	}

	// Matching file - should run (true always succeeds)
	if err := cfg.Run(PostEdit, map[string]string{"file": "main.go"}); err != nil {
		t.Errorf("expected no error for matching glob: %v", err)
	}

	// Non-matching file - hook should be skipped
	if err := cfg.Run(PostEdit, map[string]string{"file": "readme.md"}); err != nil {
		t.Errorf("expected no error for non-matching glob: %v", err)
	}
}

func TestHookDescriptorHashStableWithoutAbsoluteHomePath(t *testing.T) {
	a := Hook{
		Command:    "echo {{.file}}",
		Glob:       "*.go",
		Event:      PostCommand,
		SourceKind: SourceProject,
		SourceID:   "project:.ratchet/hooks.yaml",
		SourcePath: filepath.Join(t.TempDir(), "work-a", ".ratchet", "hooks.yaml"),
	}
	b := a
	b.SourcePath = filepath.Join(t.TempDir(), "work-b", ".ratchet", "hooks.yaml")

	if a.DescriptorHash() == "" {
		t.Fatal("DescriptorHash is empty")
	}
	if a.DescriptorHash() != b.DescriptorHash() {
		t.Fatalf("hash includes machine-specific path: %q != %q", a.DescriptorHash(), b.DescriptorHash())
	}
}

func TestHookDescriptorHashIncludesEvent(t *testing.T) {
	a := Hook{
		Event:      PreCommand,
		Command:    "echo shared",
		SourceKind: SourceProject,
		SourceID:   "project:.ratchet/hooks.yaml",
	}
	b := a
	b.Event = PostCommand

	if a.DescriptorHash() == b.DescriptorHash() {
		t.Fatal("hash should differ when event differs")
	}
}

func TestHookTrustStoreTrustDisableUntrust(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trust.json")
	store, err := LoadTrustStore(path)
	if err != nil {
		t.Fatalf("LoadTrustStore: %v", err)
	}

	const hash = "abc123"
	if store.IsTrusted(hash) {
		t.Fatal("new store should not trust hash")
	}
	if err := store.Trust(hash); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	if !store.IsTrusted(hash) {
		t.Fatal("trusted hash not recorded")
	}
	if err := store.Disable(hash); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if !store.IsDisabled(hash) {
		t.Fatal("disabled hash not recorded")
	}
	if store.IsTrusted(hash) {
		t.Fatal("disabled hash must not remain trusted")
	}
	if err := store.Untrust(hash); err != nil {
		t.Fatalf("Untrust: %v", err)
	}

	reloaded, err := LoadTrustStore(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reloaded.IsDisabled(hash) {
		t.Fatal("disabled hash not persisted")
	}
	if reloaded.IsTrusted(hash) {
		t.Fatal("untrusted hash was persisted as trusted")
	}
}

func TestLoadProjectHookStartsUntrusted(t *testing.T) {
	tmp := t.TempDir()
	homeDir := filepath.Join(tmp, "home")
	workDir := filepath.Join(tmp, "work")
	if err := os.MkdirAll(filepath.Join(workDir, ".ratchet"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", homeDir)
	if err := os.WriteFile(filepath.Join(workDir, ".ratchet", "hooks.yaml"), []byte(`
hooks:
  post-command:
    - command: "echo project"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := LoadTrustStore(filepath.Join(tmp, "trust.json"))
	if err != nil {
		t.Fatalf("LoadTrustStore: %v", err)
	}
	cfg, err := LoadWithOptions(LoadOptions{WorkingDir: workDir, TrustStore: store})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}
	got := cfg.Hooks[PostCommand]
	if len(got) != 1 {
		t.Fatalf("project hooks = %d, want 1", len(got))
	}
	if got[0].SourceKind != SourceProject {
		t.Fatalf("SourceKind = %q, want project", got[0].SourceKind)
	}
	if got[0].Trusted {
		t.Fatal("project hook should start untrusted")
	}
	if got[0].Hash == "" {
		t.Fatal("project hook hash is empty")
	}
}

func TestHookCommandForGOOS(t *testing.T) {
	h := Hook{Command: "echo posix", CommandWindows: "Write-Output windows"}
	if got, ok := h.commandForGOOS("windows"); !ok || got != "Write-Output windows" {
		t.Fatalf("windows command = %q/%v, want command/true", got, ok)
	}
	if got, ok := h.commandForGOOS("linux"); !ok || got != "echo posix" {
		t.Fatalf("linux command = %q/%v, want command/true", got, ok)
	}

	windowsOnlyMissing := Hook{Command: "echo posix", SourceKind: SourceProject}
	if got, ok := windowsOnlyMissing.commandForGOOS("windows"); ok || got != "" {
		t.Fatalf("windows missing command = %q/%v, want empty/false", got, ok)
	}
}

func TestManagedHookTrustIsImmutable(t *testing.T) {
	hook := Hook{
		Command:        "echo managed",
		CommandWindows: "Write-Output managed",
		Event:          PreCommand,
		SourceKind:     SourceManaged,
		SourceID:       "managed:managed-hooks.yaml",
	}
	hook.Hash = hook.DescriptorHash()
	store := &TrustStore{
		Trusted:  map[string]bool{},
		Disabled: map[string]bool{hook.Hash: true},
	}
	cfg := &HookConfig{Hooks: map[Event][]Hook{PreCommand: {hook}}}

	cfg.ApplyTrust(store)

	got := cfg.Hooks[PreCommand][0]
	if !got.Trusted {
		t.Fatal("managed hook was untrusted by local state")
	}
	if got.Disabled {
		t.Fatal("managed hook was disabled by local state")
	}
	if !got.runnable() {
		t.Fatal("managed hook is not runnable")
	}
}

func TestManagedHookRunCompatibilityWrapperRequiresAuditBeforeLaunch(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "launched")
	hook := managedAuditTestHook(managedAuditMarkerCommand(marker))
	cfg := &HookConfig{Hooks: map[Event][]Hook{PreCommand: {hook}}}

	err := cfg.Run(PreCommand, map[string]string{})
	if !errors.Is(err, ErrHookAuditDegraded) {
		t.Fatalf("Run error = %v, want ErrHookAuditDegraded", err)
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("managed command launched without audit: %v", statErr)
	}
}

func TestRunCompatibilityWrapperPreservesUnmanagedErrorOutput(t *testing.T) {
	cfg := &HookConfig{Hooks: map[Event][]Hook{
		PreCommand: {{
			Command:        "printf 'compat-output' >&2; exit 9",
			CommandWindows: "Write-Error 'compat-output'; exit 9",
		}},
	}}
	err := cfg.Run(PreCommand, map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "compat-output") {
		t.Fatalf("Run compatibility error = %v", err)
	}
}

func TestEscapeDataForGOOS(t *testing.T) {
	data := map[string]string{"value": "it's ok"}
	if got := escapeDataForGOOS(data, "linux")["value"]; got != "'it'\\''s ok'" {
		t.Fatalf("sh escaped value = %q", got)
	}
	if got := escapeDataForGOOS(data, "windows")["value"]; got != "'it''s ok'" {
		t.Fatalf("powershell escaped value = %q", got)
	}
}
