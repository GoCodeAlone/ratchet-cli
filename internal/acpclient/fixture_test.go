package acpclient

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

const acpClientProcessSmokeTimeout = 30 * time.Second

func waitACPClientProcessValue[T any](t *testing.T, values <-chan T, action string) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(acpClientProcessSmokeTimeout):
		t.Fatalf("timed out waiting for %s", action)
		var zero T
		return zero
	}
}

func BuildFixtureAgent(t *testing.T) string {
	t.Helper()

	out := filepath.Join(t.TempDir(), "fixture-agent")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", out, "./testdata/fixture-agent")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build fixture agent: %v\n%s", err, output)
	}
	return out
}
