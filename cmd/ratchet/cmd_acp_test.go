package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunACPConfigZedWritesSettings(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".zed", "settings.json")

	out := captureStdout(t, func() {
		if err := runACPWithArgs([]string{"config", "zed", configPath}); err != nil {
			t.Fatalf("runACPWithArgs: %v", err)
		}
	})
	if !strings.Contains(out, "wrote zed ACP config") {
		t.Fatalf("output missing success message: %s", out)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	servers, ok := raw["agent_servers"].(map[string]any)
	if !ok {
		t.Fatalf("agent_servers missing: %#v", raw)
	}
	ratchet, ok := servers["ratchet"].(map[string]any)
	if !ok {
		t.Fatalf("ratchet agent server missing: %#v", servers)
	}
	if ratchet["command"] != "ratchet" {
		t.Fatalf("command = %#v, want ratchet", ratchet["command"])
	}
	args, ok := ratchet["args"].([]any)
	if !ok || len(args) != 1 || args[0] != "acp" {
		t.Fatalf("args = %#v, want [acp]", ratchet["args"])
	}
}

func TestACPUsageIncludesConfig(t *testing.T) {
	out := captureStdout(t, func() {
		printACPUsage(os.Stdout)
	})
	for _, want := range []string{"config", "zed", ".zed/settings.json"} {
		if !strings.Contains(out, want) {
			t.Fatalf("usage missing %q: %s", want, out)
		}
	}
}
