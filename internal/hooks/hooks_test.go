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
