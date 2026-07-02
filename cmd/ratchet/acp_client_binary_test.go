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

	"github.com/GoCodeAlone/ratchet-cli/internal/acpclient"
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

	home := t.TempDir()
	cwd := filepath.Join(home, "project")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}
	env := append(os.Environ(), "HOME="+home, "XDG_STATE_HOME="+filepath.Join(home, ".state"))
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

	archivePath := filepath.Join(t.TempDir(), "queued-binary.archive.json")
	exportCmd := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "sessions", "export", "queued-binary", "--output", archivePath, "--json")
	exportCmd.Dir = repoRoot
	exportCmd.Env = env
	exportOut, err := exportCmd.Output()
	if err != nil {
		t.Fatalf("sessions export: %v\n%s", err, exportOut)
	}
	var exportPayload struct {
		SessionID string `json:"session_id"`
		Path      string `json:"path"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(exportOut, &exportPayload); err != nil {
		t.Fatalf("export json output: %v\n%s", err, exportOut)
	}
	if exportPayload.SessionID != "queued-binary" || exportPayload.Path != archivePath || exportPayload.Status != "exported" {
		t.Fatalf("export payload = %#v", exportPayload)
	}
	var archive acpclient.Archive
	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if err := json.Unmarshal(archiveBytes, &archive); err != nil {
		t.Fatalf("archive json: %v\n%s", err, archiveBytes)
	}
	if archive.FormatVersion != 1 || archive.ExportedBy != "ratchet-cli" || archive.Session.RecordID != "queued-binary" {
		t.Fatalf("archive = %#v", archive)
	}
	if filepath.IsAbs(archive.Session.CWDRelative) || archive.Session.CWDRelative != "project" {
		t.Fatalf("archive CWDRelative = %q, want project", archive.Session.CWDRelative)
	}
	if len(archive.History) == 0 {
		t.Fatalf("archive history empty: %#v", archive)
	}

	importedCWD := filepath.Join(home, "imported-project")
	importCmd := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "sessions", "import", archivePath, "--session", "imported-binary", "--cwd", importedCWD, "--json")
	importCmd.Dir = repoRoot
	importCmd.Env = env
	importOut, err := importCmd.Output()
	if err != nil {
		t.Fatalf("sessions import: %v\n%s", err, importOut)
	}
	var importPayload struct {
		SessionID string `json:"session_id"`
		Path      string `json:"path"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(importOut, &importPayload); err != nil {
		t.Fatalf("import json output: %v\n%s", err, importOut)
	}
	if importPayload.SessionID != "imported-binary" || importPayload.Path != archivePath {
		t.Fatalf("import payload = %#v", importPayload)
	}

	importShow := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "sessions", "show", "--json", "imported-binary")
	importShow.Dir = repoRoot
	importShow.Env = env
	importShowOut, err := importShow.Output()
	if err != nil {
		t.Fatalf("imported sessions show json: %v\n%s", err, importShowOut)
	}
	var importShowPayload struct {
		ID           string `json:"id"`
		ACPSessionID string `json:"acpSessionId"`
		Cwd          string `json:"cwd"`
		PromptQueue  []struct {
			Prompt   string `json:"prompt"`
			Status   string `json:"status"`
			Response string `json:"response"`
		} `json:"promptQueue"`
	}
	if err := json.Unmarshal(importShowOut, &importShowPayload); err != nil {
		t.Fatalf("imported show json output: %v\n%s", err, importShowOut)
	}
	if importShowPayload.ID != "imported-binary" || importShowPayload.ACPSessionID != "fixture-session" ||
		importShowPayload.Cwd != importedCWD || len(importShowPayload.PromptQueue) != 2 ||
		importShowPayload.PromptQueue[0].Status != "completed" {
		t.Fatalf("imported show payload = %#v", importShowPayload)
	}

	compare := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "compare",
		"--command", fixtureBin,
		"--command", fixtureBin,
		"--arg", "--echo-session",
		"--cwd", cwd,
		"--json",
		"binary compare")
	compare.Dir = repoRoot
	compare.Env = env
	compareOut, err := compare.Output()
	if err != nil {
		t.Fatalf("compare: %v\n%s", err, compareOut)
	}
	var compareRows []acpclient.CompareRow
	if err := json.Unmarshal(compareOut, &compareRows); err != nil {
		t.Fatalf("compare json output: %v\n%s", err, compareOut)
	}
	if len(compareRows) != 2 {
		t.Fatalf("compare rows = %#v, want 2 rows", compareRows)
	}
	for i, row := range compareRows {
		if row.Status != "ok" || row.StopReason != "end_turn" || !strings.Contains(row.Final, "fixture-session: binary compare") {
			t.Fatalf("compare row %d = %#v", i, row)
		}
	}

	flowPath := filepath.Join(t.TempDir(), "flow.json")
	if err := os.WriteFile(flowPath, []byte(`{
		"format_version": 1,
		"start_at": "first",
		"nodes": [
			{"id": "first", "type": "acp", "session": "main", "prompt": "first {{.Input.task}}"},
			{"id": "second", "type": "acp", "session": "main", "prompt": "second {{.Outputs.first.text}}"}
		],
		"edges": [{"from": "first", "to": "second"}]
	}`), 0o600); err != nil {
		t.Fatalf("write flow: %v", err)
	}
	flow := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "flow", "run", flowPath,
		"--input-json", `{"task":"binary flow"}`,
		"--command", fixtureBin,
		"--arg", "--echo-session",
		"--cwd", cwd,
		"--json")
	flow.Dir = repoRoot
	flow.Env = env
	flowOut, err := flow.Output()
	if err != nil {
		t.Fatalf("flow run: %v\n%s", err, flowOut)
	}
	var flowResult acpclient.FlowRunResult
	if err := json.Unmarshal(flowOut, &flowResult); err != nil {
		t.Fatalf("flow json output: %v\n%s", err, flowOut)
	}
	if flowResult.Status != acpclient.FlowRunStatusCompleted || flowResult.RunID == "" || flowResult.RunDir == "" {
		t.Fatalf("flow result = %#v", flowResult)
	}
	var secondOutput struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(flowResult.Outputs["second"], &secondOutput); err != nil {
		t.Fatalf("second output json: %v\n%s", err, flowResult.Outputs["second"])
	}
	if !strings.Contains(secondOutput.Text, "fixture-session: second fixture: fixture-session: first binary flow") {
		t.Fatalf("second output = %#v", secondOutput)
	}
	for _, rel := range []string{"flow.json", "input.json", "state.json", filepath.Join("steps", "second.json")} {
		if _, err := os.Stat(filepath.Join(flowResult.RunDir, rel)); err != nil {
			t.Fatalf("flow bundle missing %s: %v", rel, err)
		}
	}
}
