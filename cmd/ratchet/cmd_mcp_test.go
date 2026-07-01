package main

import (
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
