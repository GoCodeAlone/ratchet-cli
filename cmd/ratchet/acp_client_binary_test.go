package main

import (
	"bytes"
	"encoding/json"
	"os"
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
	env := append(os.Environ(), "XDG_STATE_HOME="+t.TempDir())
	human := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "exec", "--command", fixtureBin, "--cwd", cwd, "binary hello")
	human.Dir = repoRoot
	human.Env = env
	humanOut, err := human.CombinedOutput()
	if err != nil {
		t.Fatalf("human exec: %v\n%s", err, humanOut)
	}
	if got := string(humanOut); !strings.Contains(got, "fixture: binary hello") || !strings.Contains(got, "[stop: end_turn]") {
		t.Fatalf("human output = %q", got)
	}

	jsonCmd := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "exec", "--command", fixtureBin, "--cwd", cwd, "--json", "json hello")
	jsonCmd.Dir = repoRoot
	jsonCmd.Env = env
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

	sessions := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "sessions", "list")
	sessions.Dir = repoRoot
	sessions.Env = env
	sessionsOut, err := sessions.CombinedOutput()
	if err != nil {
		t.Fatalf("sessions list: %v\n%s", err, sessionsOut)
	}
	if got := string(sessionsOut); !strings.Contains(got, "fixture-session") || !strings.Contains(got, "completed") {
		t.Fatalf("sessions output = %q", got)
	}

	for _, prompt := range []string{"queued binary one", "queued binary two"} {
		queue := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "exec", "--command", fixtureBin, "--cwd", cwd, "--session", "queued-binary", "--no-wait", prompt)
		queue.Dir = repoRoot
		queue.Env = env
		queueOut, err := queue.CombinedOutput()
		if err != nil {
			t.Fatalf("queue exec %q: %v\n%s", prompt, err, queueOut)
		}
		if got := string(queueOut); !strings.Contains(got, "queued prompt") || !strings.Contains(got, "queued-binary") {
			t.Fatalf("queue output = %q", got)
		}
	}

	queueJSON := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "queue", "queued-binary", "--json")
	queueJSON.Dir = repoRoot
	queueJSON.Env = env
	queueJSONOut, err := queueJSON.Output()
	if err != nil {
		t.Fatalf("queue json: %v\n%s", err, queueJSONOut)
	}
	var queuePayload struct {
		SessionID string `json:"session_id"`
		Items     []struct {
			ID     string `json:"id"`
			Prompt string `json:"prompt"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(queueJSONOut, &queuePayload); err != nil {
		t.Fatalf("queue json output: %v\n%s", err, queueJSONOut)
	}
	if queuePayload.SessionID != "queued-binary" || len(queuePayload.Items) != 2 ||
		queuePayload.Items[0].Prompt != "queued binary one" || queuePayload.Items[1].Prompt != "queued binary two" ||
		queuePayload.Items[0].Status != "pending" || queuePayload.Items[1].Status != "pending" {
		t.Fatalf("queue payload = %#v", queuePayload)
	}

	status := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "status", "queued-binary")
	status.Dir = repoRoot
	status.Env = env
	statusOut, err := status.CombinedOutput()
	if err != nil {
		t.Fatalf("status: %v\n%s", err, statusOut)
	}
	if got := string(statusOut); !strings.Contains(got, "queue: 2 pending") {
		t.Fatalf("status output = %q", got)
	}

	drain := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "drain", "queued-binary", "--command", fixtureBin, "--arg", "--echo-session", "--arg", "--load-session", "--cwd", cwd, "--max", "2")
	drain.Dir = repoRoot
	drain.Env = env
	drainOut, err := drain.CombinedOutput()
	if err != nil {
		t.Fatalf("drain: %v\n%s", err, drainOut)
	}
	if got := string(drainOut); !strings.Contains(got, "drained 2 prompts") || !strings.Contains(got, "remaining: 0") {
		t.Fatalf("drain output = %q", got)
	}

	statusDone := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "status", "queued-binary")
	statusDone.Dir = repoRoot
	statusDone.Env = env
	statusDoneOut, err := statusDone.CombinedOutput()
	if err != nil {
		t.Fatalf("status done: %v\n%s", err, statusDoneOut)
	}
	if got := string(statusDoneOut); !strings.Contains(got, "queue: 0 pending, 0 running, 2 completed") {
		t.Fatalf("status done output = %q", got)
	}

	showJSON := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "sessions", "show", "--json", "queued-binary")
	showJSON.Dir = repoRoot
	showJSON.Env = env
	showJSONOut, err := showJSON.Output()
	if err != nil {
		t.Fatalf("sessions show json: %v\n%s", err, showJSONOut)
	}
	var showPayload struct {
		ACPSessionID string `json:"acpSessionId"`
		PromptQueue  []struct {
			Prompt   string `json:"prompt"`
			Status   string `json:"status"`
			Response string `json:"response"`
		} `json:"promptQueue"`
	}
	if err := json.Unmarshal(showJSONOut, &showPayload); err != nil {
		t.Fatalf("sessions show json output: %v\n%s", err, showJSONOut)
	}
	if showPayload.ACPSessionID != "fixture-session" || len(showPayload.PromptQueue) != 2 ||
		showPayload.PromptQueue[0].Status != "completed" || showPayload.PromptQueue[1].Status != "completed" ||
		!strings.Contains(showPayload.PromptQueue[0].Response, "fixture-session: queued binary one") ||
		!strings.Contains(showPayload.PromptQueue[1].Response, "fixture-session: queued binary two") {
		t.Fatalf("sessions show payload = %#v", showPayload)
	}
}
