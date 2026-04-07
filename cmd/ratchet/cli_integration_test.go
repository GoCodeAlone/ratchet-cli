//go:build integration

// CLI integration tests exercise the real ratchet binary as a subprocess.
// They require a running Ollama instance with at least one model pulled.
//
// Run: go test -tags integration ./cmd/ratchet/ -v -timeout 120s

package main

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// ratchetBin builds the binary once and returns its path.
func ratchetBin(t *testing.T) string {
	t.Helper()
	bin := t.TempDir() + "/ratchet-test"
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build ratchet: %v\n%s", err, out)
	}
	return bin
}

// run executes the ratchet binary with args and optional stdin, returns stdout+stderr.
func run(t *testing.T, bin string, stdin string, timeout time.Duration, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func TestCLI_ProviderAddOllama_SetsModel(t *testing.T) {
	bin := ratchetBin(t)

	// Ensure daemon is running.
	run(t, bin, "", 10*time.Second, "daemon", "start", "--background")
	time.Sleep(2 * time.Second)
	t.Cleanup(func() {
		run(t, bin, "", 5*time.Second, "provider", "remove", "cli-test-ollama")
	})

	// Add Ollama provider — pipe base URL + model selection.
	out, err := run(t, bin, "http://localhost:11434\n1\n", 15*time.Second,
		"provider", "add", "ollama", "cli-test-ollama")
	if err != nil {
		t.Fatalf("provider add: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Added provider: cli-test-ollama") {
		t.Fatalf("expected 'Added provider' in output, got: %s", out)
	}

	// Verify model was set (not empty).
	out, err = run(t, bin, "", 10*time.Second, "provider", "list")
	if err != nil {
		t.Fatalf("provider list: %v\n%s", err, out)
	}
	// Find the cli-test-ollama line and verify model column is not empty.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "cli-test-ollama") {
			fields := strings.Fields(line)
			// ALIAS TYPE MODEL DEFAULT — model is the 3rd field
			if len(fields) < 3 || fields[2] == "" {
				t.Fatalf("model not set in provider list: %s", line)
			}
			t.Logf("Provider registered with model: %s", fields[2])
			return
		}
	}
	t.Fatalf("cli-test-ollama not found in provider list:\n%s", out)
}

func TestCLI_ProviderAddWithModelFlag(t *testing.T) {
	bin := ratchetBin(t)

	run(t, bin, "", 10*time.Second, "daemon", "start", "--background")
	time.Sleep(2 * time.Second)
	t.Cleanup(func() {
		run(t, bin, "", 5*time.Second, "provider", "remove", "cli-flag-test")
	})

	// Add with explicit --model flag.
	out, err := run(t, bin, "http://localhost:11434\n", 15*time.Second,
		"provider", "add", "ollama", "cli-flag-test", "--model", "qwen3:1.7b")
	if err != nil {
		t.Fatalf("provider add with --model: %v\n%s", err, out)
	}

	// Verify model is set correctly.
	out, err = run(t, bin, "", 10*time.Second, "provider", "list")
	if err != nil {
		t.Fatalf("provider list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "qwen3:1.7b") {
		t.Fatalf("expected model qwen3:1.7b in list, got:\n%s", out)
	}
}

func TestCLI_OneShotChat(t *testing.T) {
	bin := ratchetBin(t)

	run(t, bin, "", 10*time.Second, "daemon", "start", "--background")
	time.Sleep(2 * time.Second)

	// Add provider if not exists (may already be there from prior test).
	run(t, bin, "http://localhost:11434\n1\n", 15*time.Second,
		"provider", "add", "ollama", "chat-test")
	run(t, bin, "", 5*time.Second, "provider", "default", "chat-test")
	t.Cleanup(func() {
		run(t, bin, "", 5*time.Second, "provider", "remove", "chat-test")
	})

	// Send a one-shot message and verify we get a non-empty response.
	out, err := run(t, bin, "", 120*time.Second, "-p", "What is 2+2? Reply with just the number.")
	if err != nil {
		t.Fatalf("one-shot chat: %v\n%s", err, out)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		t.Fatal("expected non-empty response from one-shot chat")
	}
	if !strings.Contains(out, "4") {
		t.Logf("warning: expected '4' in response, got: %s", out)
	}
	t.Logf("Chat response: %s", out)
}

func TestCLI_DaemonRestart(t *testing.T) {
	bin := ratchetBin(t)

	out, err := run(t, bin, "", 10*time.Second, "daemon", "restart")
	if err != nil {
		t.Fatalf("daemon restart: %v\n%s", err, out)
	}
	if !strings.Contains(out, "restarted") {
		t.Fatalf("expected 'restarted' in output, got: %s", out)
	}

	// Verify it's running.
	time.Sleep(2 * time.Second)
	out, err = run(t, bin, "", 10*time.Second, "daemon", "status")
	if err != nil {
		t.Fatalf("daemon status after restart: %v\n%s", err, out)
	}
	t.Logf("Daemon status: %s", out)
}

func TestCLI_ModelList(t *testing.T) {
	bin := ratchetBin(t)

	out, err := run(t, bin, "", 15*time.Second, "model", "list")
	if err != nil {
		t.Fatalf("model list: %v\n%s", err, out)
	}
	// Should either list models or say "No models installed"
	if out == "" {
		t.Fatal("expected non-empty output from model list")
	}
	t.Logf("Model list output: %s", out)
}
