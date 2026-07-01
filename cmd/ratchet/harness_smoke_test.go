package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		{name: "help", args: []string{"help"}, want: "Commands:"},
		{name: "daemon status", args: []string{"daemon", "status"}, want: "daemon is not running"},
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
		"XDG_STATE_HOME="+filepath.Join(home, ".local", "state"),
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}
