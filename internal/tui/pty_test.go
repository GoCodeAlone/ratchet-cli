//go:build integration

// PTY integration tests drive the real ratchet TUI through a pseudo-terminal.
// They test the exact same code path a user sees — Bubbletea renders to the PTY
// and we read/parse the terminal output.
//
// Requires: Ollama running with at least one model, and a configured provider.
//
// Run: go test -tags integration ./internal/tui/ -v -timeout 300s -run TestPTY

package tui

import (
	"bytes"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

// stripANSI removes terminal escape sequences and control characters from output.
var ansiRegex = regexp.MustCompile(`\x1b[\[\]()][0-9;?]*[a-zA-Z@]|\x1b\].*?\x07|\x1b[=>]|\r`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// ptySession manages an interactive ratchet session via PTY.
type ptySession struct {
	t      *testing.T
	ptmx   *os.File
	cmd    *exec.Cmd
	output bytes.Buffer
}

// startPTY launches ratchet in a PTY with the given args.
func startPTY(t *testing.T, args ...string) *ptySession {
	t.Helper()

	// Build ratchet binary
	bin := t.TempDir() + "/ratchet-pty-test"
	build := exec.Command("go", "build", "-o", bin, "./cmd/ratchet/")
	build.Dir = findRepoRoot(t)
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build ratchet: %v\n%s", err, out)
	}

	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 100})
	if err != nil {
		t.Fatalf("start pty: %v", err)
	}

	s := &ptySession{t: t, ptmx: ptmx, cmd: cmd}

	// Background reader that accumulates all output.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				s.output.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	t.Cleanup(func() {
		ptmx.Close()
		cmd.Process.Kill()
		cmd.Wait()
	})

	return s
}

// send writes text to the PTY (simulates typing).
func (s *ptySession) send(text string) {
	s.t.Helper()
	_, err := s.ptmx.Write([]byte(text))
	if err != nil {
		s.t.Fatalf("pty write: %v", err)
	}
}

// sendLine writes text followed by Enter.
func (s *ptySession) sendLine(text string) {
	s.send(text + "\r")
}

// sendCtrl sends a control character (e.g., sendCtrl('c') for Ctrl+C).
func (s *ptySession) sendCtrl(c byte) {
	s.send(string([]byte{c & 0x1f}))
}

// waitFor waits until the output contains the given substring (case-insensitive).
// Returns the full output at the time of match.
func (s *ptySession) waitFor(substr string, timeout time.Duration) string {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	lower := strings.ToLower(substr)
	for time.Now().Before(deadline) {
		text := stripANSI(s.output.String())
		if strings.Contains(strings.ToLower(text), lower) {
			return text
		}
		time.Sleep(100 * time.Millisecond)
	}
	text := stripANSI(s.output.String())
	s.t.Fatalf("timeout waiting for %q in output:\n%s", substr, text)
	return ""
}

// snapshot returns the current output (ANSI-stripped).
func (s *ptySession) snapshot() string {
	return stripANSI(s.output.String())
}

// clearOutput resets the output buffer.
func (s *ptySession) clearOutput() {
	s.output.Reset()
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(dir + "/go.mod"); err == nil {
			return dir
		}
		parent := dir[:strings.LastIndex(dir, "/")]
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}

// --- Tests ---

func TestPTY_OneShotChat(t *testing.T) {
	s := startPTY(t, "-p", "What is 2+2? Reply with just the number.")

	// Wait for the response — should contain "4".
	out := s.waitFor("4", 120*time.Second)
	t.Logf("One-shot output:\n%s", out)
}

func TestPTY_TUILaunches(t *testing.T) {
	s := startPTY(t)

	// Wait for splash screen then press any key to continue.
	s.waitFor("press any key", 30*time.Second)
	s.send(" ")

	// Wait for the chat input prompt.
	s.waitFor("Message ratchet", 15*time.Second)

	// Send a message.
	s.sendLine("What is 2+2? Reply with just the number.")

	// Wait for a response containing "4".
	out := s.waitFor("4", 120*time.Second)
	t.Logf("TUI output after message:\n%s", out)

	// Quit with Ctrl+C.
	s.sendCtrl('c')
	time.Sleep(500 * time.Millisecond)
}

func TestPTY_TUIMultiTurn(t *testing.T) {
	s := startPTY(t)

	// Pass splash screen.
	s.waitFor("press any key", 30*time.Second)
	s.send(" ")
	s.waitFor("Message ratchet", 15*time.Second)

	// Turn 1
	s.sendLine("My name is Alice. Remember that.")
	s.waitFor("Alice", 120*time.Second)

	// Wait for response to finish streaming before sending next message.
	time.Sleep(5 * time.Second)
	s.clearOutput()

	// Turn 2 — the model should recall the name from conversation history.
	s.sendLine("What is my name?")
	out := s.waitFor("Alice", 120*time.Second)
	t.Logf("Multi-turn output:\n%s", out)

	s.sendCtrl('c')
}

func TestPTY_NoThinkingInOutput(t *testing.T) {
	// With think:false, there should be NO thinking panel in one-shot output.
	s := startPTY(t, "-p", "What is 1+1? Reply with just the number.")
	out := s.waitFor("2", 120*time.Second)

	lower := strings.ToLower(out)
	if strings.Contains(lower, "thinking") || strings.Contains(lower, "▶ thinking") {
		t.Errorf("thinking panel should not appear with think:false, got:\n%s", out)
	}
	// Verify no reasoning leak (model shouldn't output "let me think" etc.)
	if strings.Contains(lower, "let me") || strings.Contains(lower, "okay,") {
		t.Errorf("reasoning content leaked into output:\n%s", out)
	}
	t.Logf("Output (no thinking expected):\n%s", out)
}

func TestPTY_ProviderList(t *testing.T) {
	s := startPTY(t, "provider", "list")
	out := s.waitFor("ALIAS", 10*time.Second)
	if !strings.Contains(out, "ollama") {
		t.Logf("warning: no ollama provider in list — may need setup:\n%s", out)
	}
	t.Logf("Provider list:\n%s", out)
}

func TestPTY_DaemonStatus(t *testing.T) {
	s := startPTY(t, "daemon", "status")
	out := s.waitFor("daemon", 10*time.Second)
	t.Logf("Daemon status:\n%s", out)
}

func TestPTY_ModelList(t *testing.T) {
	s := startPTY(t, "model", "list")
	// Should show installed models or "No models installed"
	s.waitFor("NAME", 15*time.Second)
	t.Logf("Model list:\n%s", s.snapshot())
}

// TestPTY_TextWrapping verifies that long text wraps within the terminal width.
func TestPTY_TextWrapping(t *testing.T) {
	s := startPTY(t, "-p", "Write a single paragraph of exactly 200 words about Go programming.")
	out := s.waitFor("Go", 120*time.Second)

	// Check that no line exceeds 100 columns (our PTY width).
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		clean := stripANSI(line)
		if len(clean) > 105 { // small margin for terminal rendering
			t.Errorf("line %d exceeds terminal width (len=%d): %q", i, len(clean), clean[:50]+"...")
		}
	}
	t.Logf("Wrapping test passed (%d lines)", len(lines))
}
