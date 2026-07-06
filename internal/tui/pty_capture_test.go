//go:build !windows

package tui

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"github.com/creack/pty"
	"golang.org/x/term"
)

var tuiANSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\].*?\x07|\x1b[=>]|\r`)

func TestStartTUITestPTYDisablesSoftwareFlowControl(t *testing.T) {
	if _, err := exec.LookPath("stty"); err != nil {
		t.Skipf("stty not available: %v", err)
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "show-stty")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 1\nstty -a\n"), 0700); err != nil {
		t.Fatalf("write stty script: %v", err)
	}

	s := startTUITestPTY(t, script, dir, nil, func(text string) string { return text })
	out := strings.ToLower(s.waitExit(5 * time.Second))
	if hasSttyFlag(out, "-ixon") {
		return
	}
	t.Fatalf("expected PTY software flow control to be disabled; stty output:\n%s", out)
}

type tuiPTY struct {
	t      *testing.T
	ptmx   *os.File
	cmd    *exec.Cmd
	waitCh chan error
	mu     sync.Mutex
	out    bytes.Buffer
	red    func(string) string
	alive  bool
	waited bool
}

func startTUITestPTY(t *testing.T, bin string, dir string, env []string, red func(string) string) *tuiPTY {
	t.Helper()
	cmd := exec.Command(bin)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = tuiPTYEnv(env, dir)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 42, Cols: 120})
	if err != nil {
		t.Fatalf("start pty: %v", err)
	}
	waitCh := make(chan error, 1)
	s := &tuiPTY{t: t, ptmx: ptmx, cmd: cmd, waitCh: waitCh, red: red, alive: true}
	go func() { waitCh <- cmd.Wait() }()
	var oldState *term.State
	t.Cleanup(func() {
		if oldState != nil {
			_ = term.Restore(int(ptmx.Fd()), oldState)
		}
		_ = ptmx.Close()
		s.mu.Lock()
		alive := s.alive
		waited := s.waited
		s.mu.Unlock()
		if alive && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		if !waited {
			select {
			case <-waitCh:
			case <-time.After(2 * time.Second):
			}
		}
	})
	oldState, err = term.MakeRaw(int(ptmx.Fd()))
	if err != nil {
		t.Fatalf("set pty raw mode: %v", err)
	}
	go s.readLoop()
	return s
}

func (s *tuiPTY) readLoop() {
	buf := make([]byte, 8192)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			s.mu.Lock()
			_, _ = s.out.Write(buf[:n])
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (s *tuiPTY) send(text string) {
	s.t.Helper()
	if _, err := s.ptmx.Write([]byte(text)); err != nil {
		s.t.Fatalf("pty write: %v", err)
	}
}

func (s *tuiPTY) sendLine(text string) {
	s.send(text + "\r")
}

func (s *tuiPTY) sendCtrl(c byte) {
	s.send(string([]byte{c & 0x1f}))
}

func (s *tuiPTY) waitFor(substr string, timeout time.Duration) string {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	want := strings.ToLower(substr)
	for time.Now().Before(deadline) {
		snap := s.snapshot()
		if strings.Contains(strings.ToLower(snap), want) {
			return snap
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.t.Fatalf("timeout waiting for %q in output:\n%s", substr, s.red(s.snapshot()))
	return ""
}

func (s *tuiPTY) waitForAny(substrs []string, timeout time.Duration) (string, string) {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	wants := make([]string, len(substrs))
	for i, substr := range substrs {
		wants[i] = strings.ToLower(substr)
	}
	for time.Now().Before(deadline) {
		snap := s.snapshot()
		lower := strings.ToLower(snap)
		for i, want := range wants {
			if strings.Contains(lower, want) {
				return substrs[i], snap
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.t.Fatalf("timeout waiting for any of %q in output:\n%s", substrs, s.red(s.snapshot()))
	return "", ""
}

func (s *tuiPTY) waitExit(timeout time.Duration) string {
	s.t.Helper()
	select {
	case err := <-s.waitCh:
		s.mu.Lock()
		s.alive = false
		s.waited = true
		s.mu.Unlock()
		if err != nil {
			s.t.Fatalf("process exited with error: %v\n%s", err, s.red(s.snapshot()))
		}
		return s.snapshot()
	case <-time.After(timeout):
		s.t.Fatalf("timeout waiting for process exit:\n%s", s.red(s.snapshot()))
		return ""
	}
}

func (s *tuiPTY) clear() {
	s.mu.Lock()
	s.out.Reset()
	s.mu.Unlock()
}

func (s *tuiPTY) snapshot() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return tuiANSI.ReplaceAllString(s.out.String(), "")
}

func tuiPTYEnv(overrides []string, dir string) []string {
	blocked := map[string]bool{
		"TERM":                         true,
		"PWD":                          true,
		"RATCHET_TUI_SMOKE_EMPTY_JOBS": true,
	}
	emptyJobsSet := false
	for _, kv := range overrides {
		key := envKey(kv)
		blocked[key] = true
		if key == "RATCHET_TUI_SMOKE_EMPTY_JOBS" {
			emptyJobsSet = true
		}
	}
	env := make([]string, 0, len(os.Environ())+len(overrides)+3)
	for _, kv := range os.Environ() {
		if !blocked[envKey(kv)] {
			env = append(env, kv)
		}
	}
	if !emptyJobsSet {
		env = append(env, "RATCHET_TUI_SMOKE_EMPTY_JOBS=0")
	}
	env = append(env, overrides...)
	if dir != "" {
		env = append(env, "PWD="+dir)
	}
	return append(env, "TERM=xterm-256color")
}

func envKey(kv string) string {
	if i := strings.IndexByte(kv, '='); i >= 0 {
		return kv[:i]
	}
	return kv
}

func hasSttyFlag(output string, flag string) bool {
	for _, token := range strings.FieldsFunc(output, func(r rune) bool {
		return unicode.IsSpace(r) || r == ';'
	}) {
		if token == flag {
			return true
		}
	}
	return false
}
