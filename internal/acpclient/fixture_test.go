package acpclient

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

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
