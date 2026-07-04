//go:build !windows

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
	"github.com/GoCodeAlone/ratchet-cli/internal/harnessredact"
)

func TestHarnessSmokeStartupOnboardingAndRPCShutdown(t *testing.T) {
	if raceEnabled {
		t.Skip("release startup PTY smoke is disabled under -race")
	}
	bin := buildRatchetSmokeBinary(t)
	root := t.TempDir()
	home := filepath.Join(root, "home")
	state := filepath.Join(root, "state")
	work := filepath.Join(root, "work")
	for _, dir := range []string{home, state, work} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", state)
	red := harnessredact.New(home, work, root, daemon.SocketPath(), bin, daemon.PIDPath(), state).String
	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_STATE_HOME=" + state,
	}

	s := startRatchetSmokePTY(t, bin, work, env, red, "--reconfigure")
	if match, _ := s.waitForAny([]string{"press any key", "Welcome to ratchet", "Select your AI provider"}, 10*time.Second); match == "press any key" {
		s.send(" ")
	}
	out := s.waitFor("Select your AI provider", 10*time.Second)
	if strings.Contains(out, "ratchet-tui-smoke") || strings.Contains(out, "tui_smoke") {
		t.Fatalf("release-shaped startup output leaked smoke marker:\n%s", red(out))
	}
	assertSocketAndPIDContained(t, home)

	c, err := client.Connect()
	if err != nil {
		t.Fatalf("connect to startup daemon: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Shutdown(ctx); err != nil {
		_ = c.Close()
		t.Fatalf("shutdown startup daemon: %v", err)
	}
	_ = c.Close()
	waitForRatchetSmokeMissing(t, daemon.SocketPath(), 5*time.Second)
	waitForRatchetSmokeMissing(t, daemon.PIDPath(), 5*time.Second)

	s.sendCtrl('c')
	s.waitExit(5 * time.Second)
}

func assertSocketAndPIDContained(t *testing.T, home string) {
	t.Helper()
	for _, path := range []string{daemon.SocketPath(), daemon.PIDPath()} {
		if !strings.HasPrefix(path, filepath.Join(home, ".ratchet")+string(os.PathSeparator)) {
			t.Fatalf("daemon path %s is outside test home %s", path, home)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected daemon file %s: %v", path, err)
		}
	}
	if info, err := os.Stat(daemon.SocketPath()); err != nil {
		t.Fatalf("stat daemon socket: %v", err)
	} else if info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0600 {
		t.Fatalf("daemon socket mode = %v, want socket 0600", info.Mode())
	}
}

func waitForRatchetSmokeMissing(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to be removed", path)
}
