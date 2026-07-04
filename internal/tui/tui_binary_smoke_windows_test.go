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

	if _, err := cp.Expect("Message ratchet", 8*time.Second); err != nil {
		if _, splashErr := cp.Expect("press any key", 2*time.Second); splashErr != nil {
			t.Fatalf("wait for TUI prompt or splash: %v / %v", err, splashErr)
		}
		cp.SendUnterminated(" ")
		if _, err := cp.Expect("Message ratchet", 8*time.Second); err != nil {
			t.Fatalf("wait for TUI prompt after splash: %v", err)
		}
	}

	cp.SendLine("smoke prompt body")
	if _, err := cp.Expect("I have completed the task.", 15*time.Second); err != nil {
		t.Fatalf("wait for smoke response: %v", err)
	}
	cp.SendLine("/help ")
	if _, err := cp.Expect("Quit ratchet", 8*time.Second); err != nil {
		t.Fatalf("wait for help command output: %v", err)
	}
	cp.SendLine("/exit ")
	if _, err := cp.ExpectExitCode(0, 8*time.Second); err != nil {
		t.Fatalf("wait for clean exit: %v", err)
	}
}
