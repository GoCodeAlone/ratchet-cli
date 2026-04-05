package plugins

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// buildPluginDir creates a complete plugin directory for integration testing.
// It has a manifest, skill, agent, command, and exec tool (shell cat).
func buildPluginDir(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()

	// Manifest
	manifest := map[string]any{
		"name":        name,
		"version":     "1.0.0",
		"description": "Integration test plugin",
		"author":      map[string]any{"name": "test"},
		"capabilities": map[string]any{
			"skills":   "./skills/",
			"agents":   "./agents/",
			"commands": "./commands/",
			"tools":    "./tools/",
		},
	}
	manifestData, _ := json.Marshal(manifest)
	writeJSON(t, filepath.Join(dir, ".ratchet-plugin", "plugin.json"), string(manifestData))

	// Skill
	mustMkdir(t, filepath.Join(dir, "skills", "greet"))
	mustWrite(t, filepath.Join(dir, "skills", "greet", "SKILL.md"), "# Greet\nSay hello.")

	// Agent
	mustMkdir(t, filepath.Join(dir, "agents"))
	mustWrite(t, filepath.Join(dir, "agents", "helper.yaml"), "name: helper\nrole: assistant\nsystem_prompt: Help the user.\n")

	// Command
	mustMkdir(t, filepath.Join(dir, "commands"))
	mustWrite(t, filepath.Join(dir, "commands", "greet.md"), "# /greet\nSay hello to the user.")

	// Tool: echo_tool using exec protocol (cat echoes stdin)
	mustMkdir(t, filepath.Join(dir, "tools", "echo_tool"))
	toolDef := ToolDef{
		Name:        "echo_tool",
		Description: "echoes arguments",
		Protocol:    "exec",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"msg": map[string]any{"type": "string"}},
		},
	}
	toolData, _ := json.Marshal(toolDef)
	mustWrite(t, filepath.Join(dir, "tools", "echo_tool", "tool.json"), string(toolData))
	mustWrite(t, filepath.Join(dir, "tools", "echo_tool", "echo_tool"), "#!/bin/sh\ncat\n")
	if err := os.Chmod(filepath.Join(dir, "tools", "echo_tool", "echo_tool"), 0o755); err != nil {
		t.Fatal(err)
	}

	return dir
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestIntegration_LoadAllCapabilities verifies that LoadAll discovers all
// capability types from a well-formed plugin directory.
func TestIntegration_LoadAllCapabilities(t *testing.T) {
	pluginsBase := t.TempDir()
	src := buildPluginDir(t, "int-plugin")

	// Place the plugin directory inside pluginsBase
	dest := filepath.Join(pluginsBase, "int-plugin")
	if err := copyDir(src, dest); err != nil {
		t.Fatalf("copy plugin dir: %v", err)
	}

	l := NewLoader(pluginsBase)
	result, err := l.LoadAll(context.Background())
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	if len(result.Skills) != 1 {
		t.Errorf("skills: got %d, want 1", len(result.Skills))
	} else if result.Skills[0].Name != "greet" {
		t.Errorf("skill name = %q, want greet", result.Skills[0].Name)
	}

	if len(result.Agents) != 1 {
		t.Errorf("agents: got %d, want 1", len(result.Agents))
	} else if result.Agents[0].Name != "helper" {
		t.Errorf("agent name = %q, want helper", result.Agents[0].Name)
	}

	if len(result.Commands) != 1 {
		t.Errorf("commands: got %d, want 1", len(result.Commands))
	} else if result.Commands[0].Name != "greet" {
		t.Errorf("command name = %q, want greet", result.Commands[0].Name)
	}

	if len(result.Tools) != 1 {
		t.Errorf("tools: got %d, want 1", len(result.Tools))
	} else if result.Tools[0].Name() != "echo_tool" {
		t.Errorf("tool name = %q, want echo_tool", result.Tools[0].Name())
	}
}

// TestIntegration_ExecuteTool verifies that a plugin tool can be executed
// and returns a JSON result.
func TestIntegration_ExecuteTool(t *testing.T) {
	pluginsBase := t.TempDir()
	src := buildPluginDir(t, "exec-plugin")
	dest := filepath.Join(pluginsBase, "exec-plugin")
	if err := copyDir(src, dest); err != nil {
		t.Fatalf("copy plugin dir: %v", err)
	}

	l := NewLoader(pluginsBase)
	result, err := l.LoadAll(context.Background())
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(result.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result.Tools))
	}

	tool := result.Tools[0]
	out, err := tool.Execute(context.Background(), map[string]any{"msg": "integration"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out == nil {
		t.Error("expected non-nil output from tool execute")
	}
}

// TestIntegration_InstallAndRemoveLifecycle tests the full install → verify → remove cycle.
func TestIntegration_InstallAndRemoveLifecycle(t *testing.T) {
	src := buildPluginDir(t, "lifecycle-plugin")

	// Override home dir so registry and plugins dir are isolated.
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	// Install from local
	if err := InstallFromLocal(src); err != nil {
		t.Fatalf("InstallFromLocal: %v", err)
	}

	// Verify plugin directory was created
	destDir := filepath.Join(fakeHome, ".ratchet", "plugins", "lifecycle-plugin")
	if _, err := os.Stat(destDir); err != nil {
		t.Errorf("plugin dir not found after install: %v", err)
	}

	// Verify manifest is present
	if _, err := LoadManifest(destDir); err != nil {
		t.Errorf("manifest not found at installed path: %v", err)
	}

	// Verify registry entry
	reg, err := Load()
	if err != nil {
		t.Fatalf("Load registry: %v", err)
	}
	entry, ok := reg.Get("lifecycle-plugin")
	if !ok {
		t.Fatal("expected registry entry for lifecycle-plugin")
	}
	if entry.Version != "1.0.0" {
		t.Errorf("version = %q, want 1.0.0", entry.Version)
	}

	// LoadAll should find it
	l := NewLoader(filepath.Join(fakeHome, ".ratchet", "plugins"))
	loadResult, err := l.LoadAll(context.Background())
	if err != nil {
		t.Fatalf("LoadAll after install: %v", err)
	}
	if len(loadResult.Skills) == 0 {
		t.Error("expected skills from installed plugin")
	}

	// Remove
	if err := Uninstall("lifecycle-plugin"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	// Verify directory is gone
	if _, err := os.Stat(destDir); !os.IsNotExist(err) {
		t.Error("plugin dir should be removed after uninstall")
	}

	// Verify registry entry is gone
	reg2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reg2.Get("lifecycle-plugin"); ok {
		t.Error("registry entry should be removed after uninstall")
	}
}
