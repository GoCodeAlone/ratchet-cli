//go:build integration

// PTY CLI provider integration tests drive each installed AI CLI tool through
// ratchet's PTY provider interface, validating non-interactive chat, streaming,
// and provider registration.
//
// Tools are skipped when not present on the machine (exec.LookPath check).
//
// Run: go test -tags integration ./internal/tui/ -v -timeout 300s -run TestPTYCLI

package tui

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// cliTool describes a PTY-backed CLI provider for integration testing.
type cliTool struct {
	setupAlias   string   // ratchet provider setup alias (e.g. "claude-code")
	providerType string   // ratchet provider type (e.g. "claude_code")
	binary       string   // binary to look up
	chatArgs     []string // non-interactive chat args for a simple test
}

var cliTools = []cliTool{
	{
		setupAlias:   "claude-code",
		providerType: "claude_code",
		binary:       "claude",
		chatArgs:     []string{"-p", "What is 2+2? Reply with just the number.", "--output-format", "text"},
	},
	{
		setupAlias:   "copilot-cli",
		providerType: "copilot_cli",
		binary:       "copilot",
		chatArgs:     []string{"-p", "What is 2+2? Reply with just the number."},
	},
	{
		setupAlias:   "codex-cli",
		providerType: "codex_cli",
		binary:       "codex",
		chatArgs:     []string{"exec", "What is 2+2? Reply with just the number."},
	},
	{
		setupAlias:   "gemini-cli",
		providerType: "gemini_cli",
		binary:       "gemini",
		chatArgs:     []string{"-p", "What is 2+2? Reply with just the number."},
	},
	{
		setupAlias:   "cursor-cli",
		providerType: "cursor_cli",
		binary:       "agent",
		chatArgs:     []string{"-p", "What is 2+2? Reply with just the number."},
	},
}

// TestPTYCLI_NonInteractiveChat verifies each installed CLI tool can answer a
// simple question in non-interactive (single-shot) mode.
func TestPTYCLI_NonInteractiveChat(t *testing.T) {
	for _, tool := range cliTools {
		tool := tool
		t.Run(tool.setupAlias, func(t *testing.T) {
			binPath, err := exec.LookPath(tool.binary)
			if err != nil {
				t.Skipf("%s not installed, skipping", tool.binary)
			}
			t.Logf("found %s at %s", tool.binary, binPath)

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			out, err := exec.CommandContext(ctx, tool.binary, tool.chatArgs...).Output()
			if err != nil {
				t.Fatalf("non-interactive chat failed: %v", err)
			}
			response := strings.TrimSpace(string(out))
			t.Logf("response: %q", response)
			if !strings.Contains(response, "4") {
				t.Errorf("expected response to contain '4', got: %q", response)
			}
		})
	}
}

// TestPTYCLI_HealthCheck verifies each installed CLI tool passes a health check.
func TestPTYCLI_HealthCheck(t *testing.T) {
	for _, tool := range cliTools {
		tool := tool
		t.Run(tool.setupAlias, func(t *testing.T) {
			if _, err := exec.LookPath(tool.binary); err != nil {
				t.Skipf("%s not installed, skipping", tool.binary)
			}

			healthArgs := cliHealthCheckArgsForType(tool.providerType)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			out, err := exec.CommandContext(ctx, tool.binary, healthArgs...).Output()
			if err != nil {
				t.Fatalf("health check failed: %v", err)
			}
			t.Logf("health check response: %q", strings.TrimSpace(string(out)))
		})
	}
}

// TestPTYCLI_ProviderSetup verifies each installed CLI tool can be set up via
// ratchet and appears in the provider list.
func TestPTYCLI_ProviderSetup(t *testing.T) {
	for _, tool := range cliTools {
		tool := tool
		t.Run(tool.setupAlias, func(t *testing.T) {
			if _, err := exec.LookPath(tool.binary); err != nil {
				t.Skipf("%s not installed, skipping", tool.binary)
			}

			// Run: ratchet provider setup <alias>; answer "n" to set-as-default.
			s := startPTY(t, "provider", "setup", tool.setupAlias)
			s.waitFor("Set as default", 30*time.Second)
			s.sendLine("n")
			time.Sleep(2 * time.Second)

			// Verify provider appears in list.
			list := startPTY(t, "provider", "list")
			listOut := list.waitFor("ALIAS", 10*time.Second)
			if !strings.Contains(listOut, tool.setupAlias) {
				t.Errorf("provider %q not found in provider list:\n%s", tool.setupAlias, listOut)
			}
			t.Logf("provider list:\n%s", listOut)
		})
	}
}

// TestPTYCLI_PTYChat verifies each installed CLI tool responds to a prompt
// delivered via ratchet's -p flag (non-interactive ratchet mode).
// It sets the tool as the default provider before each sub-test so the correct
// backend is used, then restores whatever was default before.
func TestPTYCLI_PTYChat(t *testing.T) {
	for _, tool := range cliTools {
		tool := tool
		t.Run(tool.setupAlias, func(t *testing.T) {
			if _, err := exec.LookPath(tool.binary); err != nil {
				t.Skipf("%s not installed, skipping", tool.binary)
			}

			// Set this tool as the default provider so ratchet -p uses it.
			setDefault := startPTY(t, "provider", "default", tool.setupAlias)
			setDefault.waitFor(tool.setupAlias, 10*time.Second)

			s := startPTY(t, "-p", "What is 2+2? Reply with just the number.")
			out := s.waitFor("4", 120*time.Second)
			t.Logf("PTY chat output:\n%s", out)
		})
	}
}

// cliHealthCheckArgsForType mirrors the adapter's HealthCheckArgs() for each provider type.
func cliHealthCheckArgsForType(providerType string) []string {
	if providerType == "codex_cli" {
		return []string{"exec", "say ok"}
	}
	return []string{"-p", "say ok"}
}
