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
