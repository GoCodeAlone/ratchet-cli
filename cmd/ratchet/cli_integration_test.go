//go:build integration

// CLI integration tests exercise the real ratchet binary as a subprocess.
// They require a running Ollama instance with at least one model pulled.
//
// Run: go test -tags integration ./cmd/ratchet/ -v -timeout 120s

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/acpclient"
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

func TestCLI_ACPClientBackgroundDrainLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("foreground daemon runtime proof uses the Unix socket transport")
	}
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	tempRoot := t.TempDir()
	binDir := filepath.Join(tempRoot, "bin")
	home := filepath.Join(tempRoot, "home")
	stateRoot := filepath.Join(tempRoot, "state")
	work := filepath.Join(tempRoot, "work")
	for _, dir := range []string{binDir, home, stateRoot, work} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	ratchet := filepath.Join(binDir, "ratchet")
	fixture := filepath.Join(binDir, "fixture-agent")
	fixtureV2 := filepath.Join(binDir, "fixture-agent-v2")
	for output, pkg := range map[string]string{
		ratchet: "./cmd/ratchet", fixture: "./internal/acpclient/testdata/fixture-agent", fixtureV2: "./internal/acpclient/testdata/fixture-agent",
	} {
		cmd := exec.CommandContext(t.Context(), "go", "build", "-o", output, pkg)
		cmd.Dir = repoRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", pkg, err, out)
		}
	}
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "XDG_STATE_HOME="+stateRoot)
	var captured bytes.Buffer
	runCLI := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, ratchet, args...)
		cmd.Dir = work
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		captured.Write(out)
		return out, err
	}

	if out, err := runCLI("acp", "client", "profiles", "add", "fixture-background", "--command", fixture, "--arg", "--echo-session", "--arg", "--load-session", "--trust"); err != nil {
		t.Fatalf("add profile: %v\n%s", err, out)
	}
	prompts := []string{"background secret alpha", "background secret beta"}
	for _, prompt := range prompts {
		if out, err := runCLI("acp", "client", "exec", "--agent", "fixture-background", "--session", "background-session", "--no-wait", prompt); err != nil {
			t.Fatalf("queue prompt: %v\n%s", err, out)
		}
	}

	type daemonProcess struct {
		cmd *exec.Cmd
		log bytes.Buffer
	}
	var active *daemonProcess
	startDaemon := func() {
		t.Helper()
		active = &daemonProcess{}
		active.cmd = exec.CommandContext(t.Context(), ratchet, "daemon", "start")
		active.cmd.Dir = work
		active.cmd.Env = env
		active.cmd.Stdout = &active.log
		active.cmd.Stderr = &active.log
		if err := active.cmd.Start(); err != nil {
			t.Fatalf("start daemon: %v", err)
		}
		socket := filepath.Join(home, ".ratchet", "daemon.sock")
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(socket); err == nil {
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
		process := active
		active = nil
		_ = process.cmd.Process.Kill()
		_ = process.cmd.Wait()
		t.Fatalf("daemon socket did not appear:\n%s", process.log.String())
	}
	stopDaemon := func() {
		t.Helper()
		if active == nil {
			return
		}
		process := active
		active = nil
		if out, err := runCLI("daemon", "stop"); err != nil {
			_ = process.cmd.Process.Kill()
			_ = process.cmd.Wait()
			t.Fatalf("stop daemon: %v\n%s\n%s", err, out, process.log.String())
		}
		done := make(chan error, 1)
		go func() { done <- process.cmd.Wait() }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("daemon exit: %v\n%s", err, process.log.String())
			}
		case <-time.After(10 * time.Second):
			_ = process.cmd.Process.Kill()
			<-done
			t.Fatalf("daemon did not exit:\n%s", process.log.String())
		}
		captured.WriteString(process.log.String())
	}
	t.Cleanup(func() {
		if active != nil && active.cmd.Process != nil {
			_ = active.cmd.Process.Kill()
			_ = active.cmd.Wait()
		}
	})

	decodeDrain := func(out []byte) map[string]any {
		t.Helper()
		var result map[string]any
		if err := json.Unmarshal(out, &result); err != nil {
			t.Fatalf("decode background JSON: %v\n%s", err, out)
		}
		return result
	}
	waitDrain := func(state, outcome string) map[string]any {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			out, err := runCLI("acp", "client", "background", "status", "background-session", "--json")
			if err == nil {
				result := decodeDrain(out)
				if result["state"] == state && result["last_outcome"] == outcome {
					return result
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("background drain did not reach %s/%s", state, outcome)
		return nil
	}

	startDaemon()
	if out, err := runCLI("acp", "client", "background", "start", "background-session", "--agent", "fixture-background", "--acknowledge-unattended", "--json"); err != nil {
		t.Fatalf("start background drain: %v\n%s", err, out)
	}
	store := acpclient.NewStore(filepath.Join(stateRoot, "ratchet", "acp-client", "sessions.json"))
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		record, err := store.Get("background-session")
		if err == nil && len(record.PromptQueue) == 2 && record.PromptQueue[0].Status == acpclient.QueuePromptStatusCompleted && record.PromptQueue[1].Status == acpclient.QueuePromptStatusCompleted {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	record, err := store.Get("background-session")
	if err != nil || len(record.PromptQueue) != 2 || record.PromptQueue[1].Status != acpclient.QueuePromptStatusCompleted {
		t.Fatalf("background queue did not complete: record=%#v err=%v", record, err)
	}
	stopDaemon()

	startDaemon()
	waitDrain(acpclient.BackgroundStateRunning, acpclient.BackgroundOutcomeResumed)
	stopDaemon()
	if out, err := runCLI("acp", "client", "profiles", "remove", "fixture-background"); err != nil {
		t.Fatalf("remove profile: %v\n%s", err, out)
	}
	if out, err := runCLI("acp", "client", "profiles", "add", "fixture-background", "--command", fixtureV2, "--arg", "--echo-session", "--arg", "--load-session", "--trust"); err != nil {
		t.Fatalf("replace profile: %v\n%s", err, out)
	}

	startDaemon()
	waitDrain(acpclient.BackgroundStateBlocked, acpclient.BackgroundOutcomeProfileDrift)
	stopOut, err := runCLI("acp", "client", "background", "stop", "background-session", "--json")
	if err != nil {
		t.Fatalf("stop background drain: %v\n%s", err, stopOut)
	}
	stopped := decodeDrain(stopOut)
	if stopped["state"] != acpclient.BackgroundStateDisabled || stopped["last_outcome"] != acpclient.BackgroundOutcomeStopped {
		t.Fatalf("stop background drain = %#v, want disabled/stopped", stopped)
	}
	stopDaemon()
	startDaemon()
	waitDrain(acpclient.BackgroundStateDisabled, acpclient.BackgroundOutcomeStopped)
	stopDaemon()
	for _, prompt := range prompts {
		if strings.Contains(captured.String(), prompt) {
			t.Fatalf("daemon or background CLI output leaked prompt %q", prompt)
		}
	}
}
