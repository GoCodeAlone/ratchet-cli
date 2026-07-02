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

func TestHarnessEmulationDocsCoverPolicyMatrixLayers(t *testing.T) {
	readme := readHarnessDoc(t, "../../README.md")
	harness := readHarnessDoc(t, "../../docs/harness-emulation.md")
	parity := readHarnessDoc(t, "../../docs/competitor-parity.md")
	matrix := readHarnessDoc(t, "../../docs/policy-matrix.md")

	for _, required := range []string{
		"Static config trust rules",
		"Runtime trust rules",
		"Persistent trust grants",
		"Permission prompts",
		"ACP client queue/drain",
		"Sandbox/path/network controls",
		"Hooks/extensions",
		"Retro/self-improvement",
		"Per-agent/team scopes",
		"Supported",
		"Partial",
		"Deferred",
		"Explicit watch/drain only",
		"sensitive local policy metadata",
		"action nodes",
		"--allow shell",
		"outside-cwd",
		"sensitive local command output",
	} {
		if !strings.Contains(matrix, required) {
			t.Fatalf("policy matrix doc missing %q", required)
		}
	}

	publicDocs := strings.Join([]string{readme, harness, parity}, "\n")
	for _, required := range []string{
		"docs/policy-matrix.md",
		"ratchet hooks list",
		"ratchet hooks trust",
		"ratchet acp client profiles",
		"hook trust",
		"ACP launch profiles",
		"ratchet acp client watch",
		"background drain",
		"extension hooks",
		"action nodes",
		"--allow shell",
		"managed hooks remain deferred",
		"TypeScript extension SDK remains deferred",
		"ACPX TypeScript flow runtime compatibility remains deferred",
	} {
		if !strings.Contains(publicDocs, required) {
			t.Fatalf("public harness docs missing %q", required)
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
