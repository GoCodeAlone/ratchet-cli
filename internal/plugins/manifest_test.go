package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func writeJSON(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const validManifestJSON = `{
	"name": "test-plugin",
	"version": "1.0.0",
	"description": "A test plugin",
	"author": {"name": "GoCodeAlone", "email": "test@example.com"},
	"capabilities": {
		"skills": "./skills/",
		"agents": "./agents/",
		"commands": "./commands/",
		"tools": "./tools/",
		"hooks": "./hooks/hooks.json",
		"mcp": "./.mcp.json"
	}
}`

func TestLoadManifest_Valid(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".ratchet-plugin", "plugin.json"), validManifestJSON)

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "test-plugin" {
		t.Errorf("name = %q, want %q", m.Name, "test-plugin")
	}
	if m.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", m.Version, "1.0.0")
	}
	if m.Author.Name != "GoCodeAlone" {
		t.Errorf("author = %q, want %q", m.Author.Name, "GoCodeAlone")
	}
	if m.Capabilities.Skills != "./skills/" {
		t.Errorf("skills = %q, want %q", m.Capabilities.Skills, "./skills/")
	}
	if m.Capabilities.MCP != "./.mcp.json" {
		t.Errorf("mcp = %q, want %q", m.Capabilities.MCP, "./.mcp.json")
	}
}

func TestLoadManifest_MissingManifest(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadManifest(dir)
	if err == nil {
		t.Fatal("expected error for missing manifest, got nil")
	}
}

func TestLoadManifest_ClaudePluginFallback(t *testing.T) {
	dir := t.TempDir()
	// Only write .claude-plugin/plugin.json, not .ratchet-plugin/plugin.json
	writeJSON(t, filepath.Join(dir, ".claude-plugin", "plugin.json"), validManifestJSON)

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "test-plugin" {
		t.Errorf("name = %q, want %q", m.Name, "test-plugin")
	}
}

func TestLoadManifest_RatchetPluginPreferredOverClaude(t *testing.T) {
	dir := t.TempDir()
	ratchetJSON := `{"name":"ratchet-plugin","version":"2.0.0","description":"ratchet","author":{"name":"test"},"capabilities":{}}`
	claudeJSON := `{"name":"claude-plugin","version":"1.0.0","description":"claude","author":{"name":"test"},"capabilities":{}}`

	writeJSON(t, filepath.Join(dir, ".ratchet-plugin", "plugin.json"), ratchetJSON)
	writeJSON(t, filepath.Join(dir, ".claude-plugin", "plugin.json"), claudeJSON)

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "ratchet-plugin" {
		t.Errorf("expected ratchet-plugin to take precedence, got %q", m.Name)
	}
}

func TestLoadManifest_PartialCapabilities(t *testing.T) {
	dir := t.TempDir()
	partial := `{"name":"partial","version":"0.1.0","description":"partial","author":{"name":"test"},"capabilities":{"skills":"./skills/"}}`
	writeJSON(t, filepath.Join(dir, ".ratchet-plugin", "plugin.json"), partial)

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Capabilities.Skills != "./skills/" {
		t.Errorf("skills = %q, want %q", m.Capabilities.Skills, "./skills/")
	}
	if m.Capabilities.Tools != "" {
		t.Errorf("tools should be empty, got %q", m.Capabilities.Tools)
	}
	if m.Capabilities.MCP != "" {
		t.Errorf("mcp should be empty, got %q", m.Capabilities.MCP)
	}
}
