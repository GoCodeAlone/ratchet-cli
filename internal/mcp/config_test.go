package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteClaudeCodeMCPConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".claude", "mcp.json")

	err := WriteMCPConfig(configPath, "ratchet-blackboard", MCPServerEntry{
		Command: "ratchet",
		Args:    []string{"mcp", "blackboard", "--team-id", "team-1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config ClaudeCodeMCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	entry, ok := config.MCPServers["ratchet-blackboard"]
	if !ok {
		t.Fatal("missing ratchet-blackboard entry")
	}
	if entry.Command != "ratchet" {
		t.Fatalf("expected command=ratchet, got %s", entry.Command)
	}
}

func TestWriteClaudeCodeMCPConfig_MergeExisting(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".claude", "mcp.json")

	// Write an existing config.
	os.MkdirAll(filepath.Dir(configPath), 0o755)
	existing := `{"mcpServers":{"existing-server":{"command":"existing","args":[]}}}`
	os.WriteFile(configPath, []byte(existing), 0o644)

	err := WriteMCPConfig(configPath, "ratchet-blackboard", MCPServerEntry{
		Command: "ratchet",
		Args:    []string{"mcp", "blackboard"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(configPath)
	var config ClaudeCodeMCPConfig
	json.Unmarshal(data, &config)

	if _, ok := config.MCPServers["existing-server"]; !ok {
		t.Fatal("existing server was clobbered")
	}
	if _, ok := config.MCPServers["ratchet-blackboard"]; !ok {
		t.Fatal("ratchet-blackboard not added")
	}
}

func TestWriteGenericMCPConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp.json")

	err := WriteGenericMCPConfig(configPath, "ratchet-daemon", MCPServerEntry{
		Command: "ratchet",
		Args:    []string{"mcp", "daemon"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config GenericMCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	entry, ok := config.Servers["ratchet-daemon"]
	if !ok {
		t.Fatal("missing ratchet-daemon entry")
	}
	if entry.Command != "ratchet" {
		t.Fatalf("expected command=ratchet, got %s", entry.Command)
	}
}

func TestWriteZedMCPConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".zed", "settings.json")

	err := WriteZedMCPConfig(configPath, "ratchet", ZedMCPServerEntry{
		Command: "ratchet",
		Args:    []string{"mcp", "daemon"},
		Env:     map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config ZedMCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	entry, ok := config.ContextServers["ratchet"]
	if !ok {
		t.Fatal("missing ratchet context server")
	}
	if entry.Command != "ratchet" {
		t.Fatalf("command = %q, want ratchet", entry.Command)
	}
	if got := len(entry.Args); got != 2 || entry.Args[0] != "mcp" || entry.Args[1] != "daemon" {
		t.Fatalf("args = %#v, want [mcp daemon]", entry.Args)
	}
	if entry.Env == nil {
		t.Fatal("env map should be present for Zed MCP config")
	}
}

func TestWriteZedMCPConfigMergesExistingSettings(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".zed", "settings.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"theme":"Ayu Dark","context_servers":{"other":{"command":"other","args":[],"env":{}}}}`
	if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	err := WriteZedMCPConfig(configPath, "ratchet", ZedMCPServerEntry{
		Command: "ratchet",
		Args:    []string{"mcp", "daemon"},
		Env:     map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["theme"] != "Ayu Dark" {
		t.Fatalf("theme was not preserved: %#v", raw["theme"])
	}
	servers, ok := raw["context_servers"].(map[string]any)
	if !ok {
		t.Fatalf("context_servers missing or wrong type: %#v", raw["context_servers"])
	}
	if _, ok := servers["other"]; !ok {
		t.Fatal("existing context server was clobbered")
	}
	if _, ok := servers["ratchet"]; !ok {
		t.Fatal("ratchet context server not added")
	}
}

func TestRemoveMCPConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".claude", "mcp.json")

	// Write config with two entries.
	os.MkdirAll(filepath.Dir(configPath), 0o755)
	config := ClaudeCodeMCPConfig{
		MCPServers: map[string]MCPServerEntry{
			"ratchet-blackboard": {Command: "ratchet"},
			"other-server":       {Command: "other"},
		},
	}
	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(configPath, data, 0o644)

	err := RemoveMCPConfig(configPath, "ratchet-blackboard")
	if err != nil {
		t.Fatal(err)
	}

	data, _ = os.ReadFile(configPath)
	var updated ClaudeCodeMCPConfig
	json.Unmarshal(data, &updated)

	if _, ok := updated.MCPServers["ratchet-blackboard"]; ok {
		t.Fatal("ratchet-blackboard should have been removed")
	}
	if _, ok := updated.MCPServers["other-server"]; !ok {
		t.Fatal("other-server should still exist")
	}
}

func TestBackupRestore(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp.json")
	os.WriteFile(configPath, []byte(`{"original":true}`), 0o644)

	backupPath, err := BackupConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}

	// Overwrite original.
	os.WriteFile(configPath, []byte(`{"modified":true}`), 0o644)

	err = RestoreConfig(configPath, backupPath)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(configPath)
	if string(data) != `{"original":true}` {
		t.Fatalf("expected original content, got: %s", data)
	}
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatal("backup should have been removed after restore")
	}
}
