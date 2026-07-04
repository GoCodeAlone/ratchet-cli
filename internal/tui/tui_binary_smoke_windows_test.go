//go:build tui_smoke && windows

package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ActiveState/termtest"

	"github.com/GoCodeAlone/ratchet-cli/internal/harnessredact"
)

func TestTUIBinaryWindowsConPTYSmoke(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary ConPTY smoke is disabled under -race")
	}
	root := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	home := filepath.Join(tempRoot, "home")
	state := filepath.Join(tempRoot, "state")
	work := filepath.Join(tempRoot, "work")
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke.exe")
	for _, dir := range []string{home, state, work} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	red := harnessredact.New(home, root, tempRoot, work, bin, "smoke prompt body").String

	build := exec.Command("go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke binary: %v\n%s", err, red(string(out)))
	}

	cp, err := termtest.New(termtest.Options{
		CmdName:        bin,
		WorkDirectory:  work,
		Environment:    append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "XDG_STATE_HOME="+state),
		DefaultTimeout: 8 * time.Second,
		HideCmdLine:    true,
	})
	if err != nil {
		t.Fatalf("start ConPTY smoke binary: %v", err)
	}
	defer cp.Close()

	if err := expectConPTY(cp, "Message ratchet", 8*time.Second); err != nil {
		if splashErr := expectConPTY(cp, "press any key", 2*time.Second); splashErr != nil {
			t.Fatalf("wait for TUI prompt or splash: %v / %v", err, splashErr)
		}
		cp.SendUnterminated(" ")
		if err := expectConPTY(cp, "Message ratchet", 8*time.Second); err != nil {
			t.Fatalf("wait for TUI prompt after splash: %v", err)
		}
	}

	sendConPTYLine(cp, "smoke prompt body")
	if err := expectConPTY(cp, "I have completed the task.", 15*time.Second); err != nil {
		t.Fatalf("wait for smoke response: %v", err)
	}
	sendConPTYLine(cp, "/help ")
	if err := expectConPTY(cp, "Quit ratchet", 8*time.Second); err != nil {
		t.Fatalf("wait for help command output: %v", err)
	}
	sendConPTYLine(cp, "/exit ")
	if err := expectConPTYExit(cp, 8*time.Second); err != nil {
		t.Fatalf("wait for clean exit: %v", err)
	}
}

func sendConPTYLine(cp *termtest.ConsoleProcess, value string) {
	cp.SendUnterminated(value + "\r")
}

func expectConPTY(cp *termtest.ConsoleProcess, value string, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		_, err := cp.Expect(value, timeout)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout + time.Second):
		_ = cp.Close()
		return termtest.ErrWaitTimeout
	}
}

func expectConPTYExit(cp *termtest.ConsoleProcess, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		_, err := cp.ExpectExitCode(0, timeout)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout + time.Second):
		_ = cp.Close()
		return termtest.ErrWaitTimeout
	}
}
