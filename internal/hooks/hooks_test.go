package hooks

import (
	"os"
	"path/filepath"
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
