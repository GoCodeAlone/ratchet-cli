package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleMCPUsageIncludesDaemonAndConfig(t *testing.T) {
	out := captureStdout(t, func() {
		handleMCP(nil)
	})
	for _, want := range []string{"ratchet mcp", "blackboard", "daemon", "config"} {
		if !strings.Contains(out, want) {
			t.Fatalf("usage missing %q: %s", want, out)
		}
	}
}

func TestMCPConfigEntryDaemon(t *testing.T) {
	entry, err := mcpConfigEntry("daemon")
	if err != nil {
		t.Fatalf("mcpConfigEntry: %v", err)
	}
	if entry.Command != "ratchet" {
		t.Fatalf("command = %q, want ratchet", entry.Command)
	}
	if got := strings.Join(entry.Args, " "); got != "mcp daemon" {
		t.Fatalf("args = %q, want mcp daemon", got)
	}
}

func TestHandleMCPConfigZedWritesSettings(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".zed", "settings.json")

	out := captureStdout(t, func() {
		handleMCPConfig([]string{"zed", configPath, "daemon"})
	})
	if !strings.Contains(out, "wrote zed MCP config") {
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
	servers, ok := raw["context_servers"].(map[string]any)
	if !ok {
		t.Fatalf("context_servers missing: %#v", raw)
	}
	ratchet, ok := servers["ratchet"].(map[string]any)
	if !ok {
		t.Fatalf("ratchet context server missing: %#v", servers)
	}
	if ratchet["command"] != "ratchet" {
		t.Fatalf("command = %#v, want ratchet", ratchet["command"])
	}
	args, ok := ratchet["args"].([]any)
	if !ok || len(args) != 2 || args[0] != "mcp" || args[1] != "daemon" {
		t.Fatalf("args = %#v, want [mcp daemon]", ratchet["args"])
	}
}

func TestParseMCPConfigArgsAcceptsZedTargetWithoutPath(t *testing.T) {
	format, path, target := parseMCPConfigArgs([]string{"zed", "blackboard"})
	if format != "zed" {
		t.Fatalf("format = %q, want zed", format)
	}
	if path != "" {
		t.Fatalf("path = %q, want empty default path", path)
	}
	if target != "blackboard" {
		t.Fatalf("target = %q, want blackboard", target)
	}
}
