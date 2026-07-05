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

	profileAdd := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "profiles", "add", "fixture-profile", "--command", fixtureBin, "--trust")
	profileAdd.Dir = repoRoot
	profileAdd.Env = env
	if out, err := profileAdd.CombinedOutput(); err != nil {
		t.Fatalf("profiles add fixture-profile: %v\n%s", err, out)
	}
	profileVerify := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "profiles", "verify", "fixture-profile", "--prompt", "binary profile verify secret", "--json")
	profileVerify.Dir = repoRoot
	profileVerify.Env = env
	var profileVerifyErr bytes.Buffer
	profileVerify.Stderr = &profileVerifyErr
	profileVerifyOut, err := profileVerify.Output()
	if err != nil {
		t.Fatalf("profiles verify fixture-profile: %v\nstdout:\n%s\nstderr:\n%s", err, profileVerifyOut, profileVerifyErr.String())
	}
	var verifyPayload struct {
		Name         string `json:"name"`
		Status       string `json:"status"`
		ACPSessionID string `json:"acpSessionId"`
		StopReason   string `json:"stopReason"`
		TextBytes    int    `json:"textBytes"`
	}
	if err := json.Unmarshal(profileVerifyOut, &verifyPayload); err != nil {
		t.Fatalf("profiles verify json output: %v\n%s", err, profileVerifyOut)
	}
	if verifyPayload.Name != "fixture-profile" || verifyPayload.Status != "ok" ||
		verifyPayload.ACPSessionID != "fixture-session" || verifyPayload.StopReason != "end_turn" ||
		verifyPayload.TextBytes == 0 {
		t.Fatalf("verify payload = %#v", verifyPayload)
	}
	if got := string(profileVerifyOut); strings.Contains(got, "binary profile verify secret") ||
		strings.Contains(got, "fixture: binary profile verify secret") {
		t.Fatalf("profiles verify leaked prompt/response: %s", got)
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

	rawArchivePath := filepath.Join(t.TempDir(), "fixture-session.raw.archive.json")
	rawExportCmd := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "sessions", "export", "fixture-session", "--output", rawArchivePath, "--history", "raw", "--json")
	rawExportCmd.Dir = repoRoot
	rawExportCmd.Env = env
	rawExportOut, err := rawExportCmd.Output()
	if err != nil {
		t.Fatalf("sessions raw export: %v\n%s", err, rawExportOut)
	}
	var rawExportPayload struct {
		SessionID   string `json:"session_id"`
		Path        string `json:"path"`
		HistoryMode string `json:"history_mode"`
	}
	if err := json.Unmarshal(rawExportOut, &rawExportPayload); err != nil {
		t.Fatalf("raw export json output: %v\n%s", err, rawExportOut)
	}
	if rawExportPayload.SessionID != "fixture-session" || rawExportPayload.Path != rawArchivePath || rawExportPayload.HistoryMode != "raw" {
		t.Fatalf("raw export payload = %#v", rawExportPayload)
	}
	var rawArchive struct {
		History []json.RawMessage `json:"history"`
	}
	rawArchiveBytes, err := os.ReadFile(rawArchivePath)
	if err != nil {
		t.Fatalf("read raw archive: %v", err)
	}
	if err := json.Unmarshal(rawArchiveBytes, &rawArchive); err != nil {
		t.Fatalf("raw archive json: %v\n%s", err, rawArchiveBytes)
	}
	if len(rawArchive.History) == 0 {
		t.Fatalf("raw archive history empty: %s", rawArchiveBytes)
	}
	for i, msg := range rawArchive.History {
		if err := acpclient.ValidateJSONRPCMessage(msg); err != nil {
			t.Fatalf("raw archive history %d invalid: %v\n%s", i, err, msg)
		}
	}

	eventsJSON := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "sessions", "events", "fixture-session", "--json")
	eventsJSON.Dir = repoRoot
	eventsJSON.Env = env
	eventsOut, err := eventsJSON.Output()
	if err != nil {
		t.Fatalf("sessions events json: %v\n%s", err, eventsOut)
	}
	var eventsPayload struct {
		SessionID string                   `json:"session_id"`
		Status    string                   `json:"status"`
		Events    []acpclient.EventLogLine `json:"events"`
	}
	if err := json.Unmarshal(eventsOut, &eventsPayload); err != nil {
		t.Fatalf("events json output: %v\n%s", err, eventsOut)
	}
	if eventsPayload.SessionID != "fixture-session" || eventsPayload.Status != "ok" || len(eventsPayload.Events) == 0 {
		t.Fatalf("events payload = %#v", eventsPayload)
	}

	acpxArchivePath := filepath.Join(t.TempDir(), "acpx.archive.json")
	acpxArchive := map[string]any{
		"format_version": 1,
		"exported_at":    "2026-07-02T10:00:00Z",
		"exported_by":    "acpx",
		"session": map[string]any{
			"record_id":    "acpx-binary-source",
			"agent":        "fixture",
			"cwd_relative": "project",
			"cwd_original": "project",
			"created_at":   "2026-07-02T10:00:00Z",
			"updated_at":   "2026-07-02T10:00:00Z",
			"state": map[string]any{
				"id":           "acpx-binary-source",
				"acpSessionId": "fixture-session",
				"agent":        "fixture",
				"cwd":          cwd,
				"status":       "completed",
				"createdAt":    "2026-07-02T10:00:00Z",
				"updatedAt":    "2026-07-02T10:00:00Z",
			},
		},
		"history": []json.RawMessage{
			json.RawMessage(`{"jsonrpc":"2.0","id":"prompt-1","method":"session/prompt","params":{"sessionId":"fixture-session"}}`),
			json.RawMessage(`{"jsonrpc":"2.0","id":"prompt-1","result":{"stopReason":"end_turn"}}`),
		},
	}
	acpxBytes, err := json.Marshal(acpxArchive)
	if err != nil {
		t.Fatalf("marshal acpx archive: %v", err)
	}
	if err := os.WriteFile(acpxArchivePath, acpxBytes, 0o600); err != nil {
		t.Fatalf("write acpx archive: %v", err)
	}
	importACPX := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "sessions", "import", acpxArchivePath, "--session", "imported-acpx-binary", "--cwd", cwd, "--json")
	importACPX.Dir = repoRoot
	importACPX.Env = env
	importACPXOut, err := importACPX.Output()
	if err != nil {
		t.Fatalf("import acpx archive: %v\n%s", err, importACPXOut)
	}
	importedRawPath := filepath.Join(t.TempDir(), "imported-acpx.raw.archive.json")
	exportImportedRaw := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "sessions", "export", "imported-acpx-binary", "--output", importedRawPath, "--history", "raw", "--json")
	exportImportedRaw.Dir = repoRoot
	exportImportedRaw.Env = env
	if out, err := exportImportedRaw.CombinedOutput(); err != nil {
		t.Fatalf("export imported acpx raw: %v\n%s", err, out)
	}
	importedRawBytes, err := os.ReadFile(importedRawPath)
	if err != nil {
		t.Fatalf("read imported raw archive: %v", err)
	}
	var importedRawArchive struct {
		History []json.RawMessage `json:"history"`
	}
	if err := json.Unmarshal(importedRawBytes, &importedRawArchive); err != nil {
		t.Fatalf("imported raw archive json: %v\n%s", err, importedRawBytes)
	}
	if len(importedRawArchive.History) != 2 {
		t.Fatalf("imported raw history len = %d, want 2", len(importedRawArchive.History))
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

	compareRunRoot := filepath.Join(t.TempDir(), "compare-runs")
	compareSave := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "compare",
		"--command", fixtureBin,
		"--command", fixtureBin,
		"--arg", "--echo-session",
		"--cwd", cwd,
		"--save",
		"--run-id", "binary-compare",
		"--run-root", compareRunRoot,
		"--json",
		"binary compare saved")
	compareSave.Dir = repoRoot
	compareSave.Env = env
	compareSaveOut, err := compareSave.Output()
	if err != nil {
		t.Fatalf("compare save: %v\n%s", err, compareSaveOut)
	}
	var compareSavePayload struct {
		RunID  string                 `json:"run_id"`
		RunDir string                 `json:"run_dir"`
		Status string                 `json:"status"`
		Rows   []acpclient.CompareRow `json:"rows"`
	}
	if err := json.Unmarshal(compareSaveOut, &compareSavePayload); err != nil {
		t.Fatalf("compare save json output: %v\n%s", err, compareSaveOut)
	}
	if compareSavePayload.RunID != "binary-compare" || compareSavePayload.Status != "completed" || len(compareSavePayload.Rows) != 2 {
		t.Fatalf("compare save payload = %#v", compareSavePayload)
	}
	if _, err := os.Stat(filepath.Join(compareSavePayload.RunDir, "compare.json")); err != nil {
		t.Fatalf("compare bundle missing compare.json: %v", err)
	}
	compareEvents, err := filepath.Glob(filepath.Join(compareSavePayload.RunDir, "agents", "*", "events.ndjson"))
	if err != nil {
		t.Fatalf("glob compare events: %v", err)
	}
	if len(compareEvents) != 2 {
		t.Fatalf("compare event files = %#v, want 2", compareEvents)
	}

	flowPath := filepath.Join(t.TempDir(), "flow.json")
	flowDef := map[string]any{
		"format_version": 1,
		"start_at":       "prepare",
		"nodes": []map[string]any{
			{"id": "prepare", "type": "action", "command": ratchetBin, "args": []string{"version"}},
			{"id": "first", "type": "acp", "session": "main", "prompt": "first {{.Input.task}} after {{.Outputs.prepare.stdout}}"},
			{"id": "second", "type": "acp", "session": "main", "prompt": "second {{.Outputs.first.text}}"},
		},
		"edges": []map[string]string{
			{"from": "prepare", "to": "first"},
			{"from": "first", "to": "second"},
		},
	}
	flowBytes, err := json.Marshal(flowDef)
	if err != nil {
		t.Fatalf("marshal flow: %v", err)
	}
	if err := os.WriteFile(flowPath, flowBytes, 0o600); err != nil {
		t.Fatalf("write flow: %v", err)
	}
	flow := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "flow", "run", flowPath,
		"--input-json", `{"task":"binary flow"}`,
		"--command", fixtureBin,
		"--arg", "--echo-session",
		"--allow", "shell",
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
	var actionOutput struct {
		Stdout string `json:"stdout"`
	}
	if err := json.Unmarshal(flowResult.Outputs["prepare"], &actionOutput); err != nil {
		t.Fatalf("action output json: %v\n%s", err, flowResult.Outputs["prepare"])
	}
	if !strings.Contains(actionOutput.Stdout, "ratchet ") {
		t.Fatalf("action output = %#v", actionOutput)
	}
	var secondOutput struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(flowResult.Outputs["second"], &secondOutput); err != nil {
		t.Fatalf("second output json: %v\n%s", err, flowResult.Outputs["second"])
	}
	if !strings.Contains(secondOutput.Text, "fixture-session: second fixture: fixture-session: first binary flow after ratchet ") {
		t.Fatalf("second output = %#v", secondOutput)
	}
	for _, rel := range []string{
		"flow.json",
		"input.json",
		"state.json",
		"manifest.json",
		"trace.ndjson",
		filepath.Join("projections", "run.json"),
		filepath.Join("projections", "live.json"),
		filepath.Join("projections", "steps.json"),
		filepath.Join("steps", "prepare.json"),
		filepath.Join("steps", "second.json"),
		filepath.Join("sessions", "main", "events.ndjson"),
	} {
		if _, err := os.Stat(filepath.Join(flowResult.RunDir, rel)); err != nil {
			t.Fatalf("flow bundle missing %s: %v", rel, err)
		}
	}
	replay := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "flow", "replay", flowResult.RunDir, "--json")
	replay.Dir = repoRoot
	replay.Env = env
	replayOut, err := replay.Output()
	if err != nil {
		t.Fatalf("flow replay: %v\n%s", err, replayOut)
	}
	var replaySummary acpclient.FlowReplaySummary
	if err := json.Unmarshal(replayOut, &replaySummary); err != nil {
		t.Fatalf("flow replay json output: %v\n%s", err, replayOut)
	}
	if replaySummary.RunID != flowResult.RunID || replaySummary.Status != acpclient.FlowRunStatusCompleted ||
		replaySummary.StepCount != 3 || replaySummary.TraceCount != 7 || replaySummary.SessionCount != 1 {
		t.Fatalf("flow replay summary = %#v", replaySummary)
	}
	replayText := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "flow", "replay", flowResult.RunDir)
	replayText.Dir = repoRoot
	replayText.Env = env
	replayTextOut, err := replayText.Output()
	if err != nil {
		t.Fatalf("flow replay text: %v\n%s", err, replayTextOut)
	}
	if got := string(replayTextOut); !strings.Contains(got, "trace events: 7") ||
		strings.Contains(got, "binary flow") || strings.Contains(got, "fixture-session") || strings.Contains(got, "ratchet ") {
		t.Fatalf("flow replay human output leaked payload or missed counts: %q", got)
	}
}

func TestACPClientWatchBinarySmoke(t *testing.T) {
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
	for _, prompt := range []string{"watch binary one", "watch binary two"} {
		queue := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "exec", "--command", fixtureBin, "--cwd", cwd, "--session", "watch-binary", "--no-wait", prompt)
		queue.Dir = repoRoot
		queue.Env = env
		queueOut, err := queue.CombinedOutput()
		if err != nil {
			t.Fatalf("queue exec %q: %v\n%s", prompt, err, queueOut)
		}
	}

	watch := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "watch", "watch-binary",
		"--command", fixtureBin,
		"--arg", "--echo-session",
		"--arg", "--load-session",
		"--cwd", cwd,
		"--stop-when-empty",
		"--max-per-cycle", "2",
		"--max-cycles", "2")
	watch.Dir = repoRoot
	watch.Env = env
	watchOut, err := watch.CombinedOutput()
	if err != nil {
		t.Fatalf("watch: %v\n%s", err, watchOut)
	}
	gotWatch := string(watchOut)
	if !strings.Contains(gotWatch, "watch cycle 1") || !strings.Contains(gotWatch, "completed: 2") || !strings.Contains(gotWatch, "remaining: 0") {
		t.Fatalf("watch output = %q", gotWatch)
	}
	if strings.Contains(gotWatch, "watch binary one") || strings.Contains(gotWatch, "watch binary two") {
		t.Fatalf("watch output leaked prompt bodies: %q", gotWatch)
	}

	statusDone := exec.CommandContext(t.Context(), ratchetBin, "acp", "client", "status", "watch-binary")
	statusDone.Dir = repoRoot
	statusDone.Env = env
	statusDoneOut, err := statusDone.CombinedOutput()
	if err != nil {
		t.Fatalf("status done: %v\n%s", err, statusDoneOut)
	}
	if got := string(statusDoneOut); !strings.Contains(got, "queue: 0 pending, 0 running, 2 completed") {
		t.Fatalf("status done output = %q", got)
	}
}
