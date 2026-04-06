package plugins

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAllEmpty(t *testing.T) {
	l := NewLoader(t.TempDir())
	result, err := l.LoadAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(result.Skills))
	}
	if len(result.Tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(result.Tools))
	}
}

func TestLoadAllMissing(t *testing.T) {
	l := NewLoader(filepath.Join(t.TempDir(), "nonexistent"))
	result, err := l.LoadAll(context.Background())
	if err != nil {
		t.Fatalf("expected no error for missing dir: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestLoadAllSkipsNonPluginDirs(t *testing.T) {
	dir := t.TempDir()

	// Create a directory that has no manifest (should be silently skipped)
	notAPlugin := filepath.Join(dir, "not-a-plugin")
	if err := os.MkdirAll(notAPlugin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notAPlugin, "readme.txt"), []byte("docs"), 0o644); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(dir)
	result, err := l.LoadAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skills) != 0 || len(result.Agents) != 0 || len(result.Tools) != 0 {
		t.Error("expected no capabilities from non-plugin directory")
	}
}

func TestLoadAllSkillsAndAgents(t *testing.T) {
	pluginsBase := t.TempDir()

	// Build a plugin with skills and agents
	pluginDir := filepath.Join(pluginsBase, "my-plugin")
	manifest := `{"name":"my-plugin","version":"1.0.0","description":"test","author":{"name":"test"},"capabilities":{"skills":"./skills/","agents":"./agents/"}}`
	writeJSON(t, filepath.Join(pluginDir, ".ratchet-plugin", "plugin.json"), manifest)

	// Add a skill
	if err := os.MkdirAll(filepath.Join(pluginDir, "skills", "hello"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "skills", "hello", "SKILL.md"), []byte("# Hello skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Add an agent YAML
	if err := os.MkdirAll(filepath.Join(pluginDir, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	agentYAML := "name: reviewer\nrole: code reviewer\nsystem_prompt: Review code carefully.\n"
	if err := os.WriteFile(filepath.Join(pluginDir, "agents", "reviewer.yaml"), []byte(agentYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(pluginsBase)
	result, err := l.LoadAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(result.Skills))
	} else if result.Skills[0].Name != "hello" {
		t.Errorf("skill name = %q, want %q", result.Skills[0].Name, "hello")
	}

	if len(result.Agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(result.Agents))
	} else if result.Agents[0].Name != "reviewer" {
		t.Errorf("agent name = %q, want %q", result.Agents[0].Name, "reviewer")
	}
}

func TestLoadAllCommands(t *testing.T) {
	pluginsBase := t.TempDir()
	pluginDir := filepath.Join(pluginsBase, "cmd-plugin")
	manifest := `{"name":"cmd-plugin","version":"1.0.0","description":"test","author":{"name":"test"},"capabilities":{"commands":"./commands/"}}`
	writeJSON(t, filepath.Join(pluginDir, ".ratchet-plugin", "plugin.json"), manifest)

	if err := os.MkdirAll(filepath.Join(pluginDir, "commands"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "commands", "deploy.md"), []byte("# Deploy\nDeploy the app."), 0o644); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(pluginsBase)
	result, err := l.LoadAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Commands) != 1 {
		t.Errorf("expected 1 command, got %d", len(result.Commands))
	} else if result.Commands[0].Name != "deploy" {
		t.Errorf("command name = %q, want %q", result.Commands[0].Name, "deploy")
	}
}
