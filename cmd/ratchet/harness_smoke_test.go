package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
)

func TestHarnessSmokeVersionHelpAndDaemonStatus(t *testing.T) {
	if raceEnabled {
		t.Skip("binary-build smoke is covered by normal tests; skip expensive subprocess build under -race")
	}
	bin := buildRatchetSmokeBinary(t)
	home := t.TempDir()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "version", args: []string{"version"}, want: "ratchet"},
		{name: "version flag", args: []string{"--version"}, want: "ratchet"},
		{name: "help", args: []string{"help"}, want: "Commands:"},
		{name: "daemon status", args: []string{"daemon", "status"}, want: "daemon is not running"},
		{name: "doctor json", args: []string{"doctor", "--json"}, want: `"daemon_status"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := runRatchetSmoke(t, bin, home, tt.args...)
			if err != nil {
				t.Fatalf("ratchet %v: %v\n%s", tt.args, err, out)
			}
			if !strings.Contains(out, tt.want) {
				t.Fatalf("ratchet %v output = %q, want substring %q", tt.args, out, tt.want)
			}
		})
	}
}

func TestHarnessSmokeDaemonRestartWaitsForReplacement(t *testing.T) {
	if raceEnabled {
		t.Skip("binary-build smoke is covered by normal tests; skip expensive subprocess build under -race")
	}
	bin := buildRatchetSmokeBinary(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Cleanup(func() {
		_, _ = runRatchetSmoke(t, bin, home, "daemon", "stop")
	})

	out, err := runRatchetSmoke(t, bin, home, "daemon", "start", "--background")
	if err != nil {
		t.Fatalf("start daemon: %v\n%s", err, out)
	}
	oldPID, err := daemon.ReadPID()
	if err != nil {
		t.Fatalf("read old daemon PID: %v", err)
	}

	out, err = runRatchetSmoke(t, bin, home, "daemon", "restart")
	if err != nil {
		t.Fatalf("restart daemon: %v\n%s", err, out)
	}
	if !strings.Contains(out, "daemon restarted") {
		t.Fatalf("restart output = %q, want success", out)
	}
	newPID, err := daemon.ReadPID()
	if err != nil {
		t.Fatalf("read replacement daemon PID: %v", err)
	}
	if newPID == oldPID {
		t.Fatalf("replacement PID = old PID %d", oldPID)
	}

	out, err = runRatchetSmoke(t, bin, home, "daemon", "status")
	if err != nil {
		t.Fatalf("status replacement daemon: %v\n%s", err, out)
	}
	if !strings.Contains(out, strconv.Itoa(newPID)) {
		t.Fatalf("status output = %q, want PID %d", out, newPID)
	}

	out, err = runRatchetSmoke(t, bin, home, "daemon", "stop")
	if err != nil {
		t.Fatalf("stop replacement daemon: %v\n%s", err, out)
	}
	for _, path := range []string{daemon.PIDPath(), daemon.SocketPath()} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("daemon path %s remains after stop: %v", path, err)
		}
	}
}

func buildRatchetSmokeBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "ratchet-smoke")
	if os.PathSeparator == '\\' {
		bin += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, ".")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("build ratchet smoke binary: %v\n%s", err, buf.String())
	}
	return bin
}

func runRatchetSmoke(t *testing.T, bin, home string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"USERPROFILE="+home,
		"XDG_STATE_HOME="+filepath.Join(home, ".local", "state"),
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}
