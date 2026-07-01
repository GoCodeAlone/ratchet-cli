package main

import (
	"os"
	"strings"
	"testing"
)

func TestHarnessEmulationDocsCoverSupportedModesAndParity(t *testing.T) {
	readme := readHarnessDoc(t, "../../README.md")
	doc := readHarnessDoc(t, "../../docs/harness-emulation.md")

	for _, mode := range []string{"TUI", "one-shot", "daemon", "ACP", "MCP", "team"} {
		if !strings.Contains(readme, mode) && !strings.Contains(doc, mode) {
			t.Fatalf("harness docs missing command mode %q", mode)
		}
	}
	for _, required := range []string{
		"mock provider",
		"temp home",
		"Competitor parity",
		"Zed",
		"Pi",
		"Codex",
		"Claude Code",
		"OpenClaw",
		"ACP",
		"MCP",
	} {
		if !strings.Contains(doc, required) {
			t.Fatalf("harness emulation doc missing %q", required)
		}
	}
}

func readHarnessDoc(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
