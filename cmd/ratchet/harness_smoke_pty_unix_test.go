//go:build !windows

package main

import (
	"bytes"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

var ratchetSmokeANSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\].*?\x07|\x1b[=>]|\r`)

type ratchetSmokePTY struct {
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

func startRatchetSmokePTY(t *testing.T, bin string, dir string, env []string, red func(string) string, args ...string) *ratchetSmokePTY {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = ratchetSmokeEnv(env, dir)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 42, Cols: 120})
	if err != nil {
		t.Fatalf("start pty: %s", red(err.Error()))
	}
	waitCh := make(chan error, 1)
	s := &ratchetSmokePTY{t: t, ptmx: ptmx, cmd: cmd, waitCh: waitCh, red: red, alive: true}
	go func() { waitCh <- cmd.Wait() }()
	go s.readLoop()
	t.Cleanup(func() {
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
	return s
}

func (s *ratchetSmokePTY) readLoop() {
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

func (s *ratchetSmokePTY) send(text string) {
	s.t.Helper()
	if _, err := s.ptmx.Write([]byte(text)); err != nil {
		s.t.Fatalf("pty write: %v", err)
	}
}

func (s *ratchetSmokePTY) sendLine(text string) {
	s.send(text + "\r")
}

func (s *ratchetSmokePTY) sendCtrl(c byte) {
	s.send(string([]byte{c & 0x1f}))
}

func (s *ratchetSmokePTY) waitFor(substr string, timeout time.Duration) string {
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

func (s *ratchetSmokePTY) waitForAny(substrs []string, timeout time.Duration) (string, string) {
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

func (s *ratchetSmokePTY) waitExit(timeout time.Duration) string {
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

func (s *ratchetSmokePTY) snapshot() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ratchetSmokeANSI.ReplaceAllString(s.out.String(), "")
}

func ratchetSmokeEnv(overrides []string, dir string) []string {
	blocked := map[string]bool{"TERM": true, "PWD": true}
	for _, kv := range overrides {
		blocked[ratchetSmokeEnvKey(kv)] = true
	}
	env := make([]string, 0, len(os.Environ())+len(overrides)+2)
	for _, kv := range os.Environ() {
		if !blocked[ratchetSmokeEnvKey(kv)] {
			env = append(env, kv)
		}
	}
	env = append(env, overrides...)
	if dir != "" {
		env = append(env, "PWD="+dir)
	}
	return append(env, "TERM=xterm-256color")
}

func ratchetSmokeEnvKey(kv string) string {
	if i := strings.IndexByte(kv, '='); i >= 0 {
		return kv[:i]
	}
	return kv
}
