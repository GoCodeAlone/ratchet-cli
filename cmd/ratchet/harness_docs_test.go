package main

import (
	"os"
	"strings"
	"testing"
)

func TestHarnessEmulationDocsCoverSupportedModesAndParity(t *testing.T) {
	readme := readHarnessDoc(t, "../../README.md")
	doc := readHarnessDoc(t, "../../docs/harness-emulation.md")

	for _, mode := range []string{"TUI", "one-shot", "daemon", "blackboard", "ACP", "MCP", "team"} {
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
		"ACP archive/compare/replay artifacts",
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
		"raw ACPX event logs",
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
		"ratchet acp client sessions events",
		"--history raw",
		"compare --save",
		"flow replay",
		"raw ACPX event logs",
		"hook trust",
		"ACP launch profiles",
		"ratchet acp client watch",
		"ratchet blackboard write coordination status ready",
		"ratchet blackboard read coordination status",
		"daemon-scoped volatile",
		"sensitive local coordination data",
		"workflow-plugin-notify",
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

func TestHarnessDocsDescribeTUIBinaryEvidenceBoundaries(t *testing.T) {
	readme := readHarnessDoc(t, "../../README.md")
	harness := readHarnessDoc(t, "../../docs/harness-emulation.md")
	ratchet := readHarnessDoc(t, "../../RATCHET.md")
	parity := readHarnessDoc(t, "../../docs/competitor-parity.md")
	matrix := readHarnessDoc(t, "../../docs/policy-matrix.md")

	for _, doc := range []struct {
		name string
		body string
	}{
		{name: "README.md", body: readme},
		{name: "docs/harness-emulation.md", body: harness},
	} {
		for _, required := range []string{
			"release-shaped startup smoke",
			"`ratchet-tui-smoke` is build-tagged test-only",
			"Unix PTY binary smoke",
			"release-shaped startup smoke is not full TUI PTY proof",
			"Windows cross-build/package archive inspection",
			"Windows ConPTY binary smoke",
			"before the GitHub release is made public",
		} {
			if !containsWords(doc.body, required) {
				t.Fatalf("%s missing TUI binary evidence boundary %q", doc.name, required)
			}
		}
	}

	publicDocs := strings.Join([]string{ratchet, parity, matrix}, "\n")
	for _, forbidden := range []string{
		"full release TUI PTY proof",
		"packaged release `ratchet.exe` runtime is proven",
	} {
		if strings.Contains(publicDocs, forbidden) {
			t.Fatalf("public docs overclaim deferred evidence with %q", forbidden)
		}
	}
	for _, requiredLink := range []string{
		"docs/harness-emulation.md",
		"docs/policy-matrix.md",
	} {
		if !strings.Contains(publicDocs, requiredLink) {
			t.Fatalf("public docs missing evidence boundary link %q", requiredLink)
		}
	}

	releaseEvidenceDocs := []struct {
		name string
		body string
	}{
		{name: "README.md", body: readme},
		{name: "RATCHET.md", body: ratchet},
		{name: "docs/harness-emulation.md", body: harness},
		{name: "docs/competitor-parity.md", body: parity},
		{name: "docs/policy-matrix.md", body: matrix},
	}
	for _, required := range []string{
		"GoReleaser snapshot release-check",
		"draft release asset postcheck",
		"tap preflight",
		"tap postcheck",
		"generated-cask publish",
		"packaged release `ratchet.exe` runtime remains deferred",
	} {
		for _, doc := range releaseEvidenceDocs {
			if !containsWords(doc.body, required) {
				t.Fatalf("%s missing final release evidence wording %q", doc.name, required)
			}
		}
	}
}

func containsWords(body, phrase string) bool {
	normalizedBody := strings.Join(strings.Fields(body), " ")
	normalizedPhrase := strings.Join(strings.Fields(phrase), " ")
	return strings.Contains(normalizedBody, normalizedPhrase)
}

func readHarnessDoc(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
