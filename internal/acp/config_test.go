package acp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteZedACPConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".zed", "settings.json")

	err := WriteZedACPConfig(configPath, "ratchet", ZedACPAgentServer{
		Type:    "custom",
		Command: "ratchet",
		Args:    []string{"acp"},
		Env:     map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config ZedACPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	entry, ok := config.AgentServers["ratchet"]
	if !ok {
		t.Fatal("missing ratchet agent server")
	}
	if entry.Type != "custom" {
		t.Fatalf("type = %q, want custom", entry.Type)
	}
	if entry.Command != "ratchet" {
		t.Fatalf("command = %q, want ratchet", entry.Command)
	}
	if got := len(entry.Args); got != 1 || entry.Args[0] != "acp" {
		t.Fatalf("args = %#v, want [acp]", entry.Args)
	}
	if entry.Env == nil {
		t.Fatal("env map should be present for Zed custom agent config")
	}
}

func TestWriteZedACPConfigMergesExistingSettings(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".zed", "settings.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"theme":"Ayu Dark","agent_servers":{"other":{"type":"custom","command":"other","args":[],"env":{}}}}`
	if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	err := WriteZedACPConfig(configPath, "ratchet", ZedACPAgentServer{
		Type:    "custom",
		Command: "ratchet",
		Args:    []string{"acp"},
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
	servers, ok := raw["agent_servers"].(map[string]any)
	if !ok {
		t.Fatalf("agent_servers missing or wrong type: %#v", raw["agent_servers"])
	}
	if _, ok := servers["other"]; !ok {
		t.Fatal("existing agent server was clobbered")
	}
	if _, ok := servers["ratchet"]; !ok {
		t.Fatal("ratchet agent server not added")
	}
}
