package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestACPClientExecBinarySmoke(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	binDir := t.TempDir()
	ratchetBin := filepath.Join(binDir, "ratchet")
	fixtureBin := filepath.Join(binDir, "fixture-agent")
	if runtime.GOOS == "windows" {
		ratchetBin += ".exe"
		fixtureBin += ".exe"
	}

	buildRatchet := exec.CommandContext(t.Context(), "go", "build", "-o", ratchetBin, "./cmd/ratchet")
	buildRatchet.Dir = repoRoot
	if out, err := buildRatchet.CombinedOutput(); err != nil {
		t.Fatalf("build ratchet: %v\n%s", err, out)
	}

	buildFixture := exec.CommandContext(t.Context(), "go", "build", "-o", fixtureBin, "./internal/acpclient/testdata/fixture-agent")
	buildFixture.Dir = repoRoot
	if out, err := buildFixture.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v\n%s", err, out)
	}

	cwd := t.TempDir()
	human := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "exec", "--command", fixtureBin, "--cwd", cwd, "binary hello")
	human.Dir = repoRoot
	humanOut, err := human.CombinedOutput()
	if err != nil {
		t.Fatalf("human exec: %v\n%s", err, humanOut)
	}
	if got := string(humanOut); !strings.Contains(got, "fixture: binary hello") || !strings.Contains(got, "[stop: end_turn]") {
		t.Fatalf("human output = %q", got)
	}

	jsonCmd := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "exec", "--command", fixtureBin, "--cwd", cwd, "--json", "json hello")
	jsonCmd.Dir = repoRoot
	var jsonErr bytes.Buffer
	jsonCmd.Stderr = &jsonErr
	jsonOut, err := jsonCmd.Output()
	if err != nil {
		t.Fatalf("json exec: %v\nstdout:\n%s\nstderr:\n%s", err, jsonOut, jsonErr.String())
	}
	var payload struct {
		Command    string `json:"command"`
		StopReason string `json:"stop_reason"`
		Text       string `json:"text"`
	}
	if err := json.Unmarshal(jsonOut, &payload); err != nil {
		t.Fatalf("json output: %v\n%s", err, jsonOut)
	}
	if payload.Command != fixtureBin || payload.StopReason != "end_turn" || payload.Text != "fixture: json hello" {
		t.Fatalf("payload = %#v", payload)
	}
}
