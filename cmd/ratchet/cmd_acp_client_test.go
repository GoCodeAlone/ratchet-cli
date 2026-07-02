package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/acpclient"
	acpsdk "github.com/coder/acp-go-sdk"
)

func TestParseACPClientExecCommand(t *testing.T) {
	cmd, err := parseACPClientCommand([]string{
		"exec",
		"--agent", "codex",
		"--cwd", "/tmp/project",
		"--timeout", "2s",
		"--json",
		"hello", "agent",
	})
	if err != nil {
		t.Fatalf("parseACPClientCommand: %v", err)
	}
	if cmd.kind != acpClientCommandExec {
		t.Fatalf("kind = %q, want exec", cmd.kind)
	}
	if cmd.exec.Agent != "codex" {
		t.Fatalf("Agent = %q, want codex", cmd.exec.Agent)
	}
	if cmd.exec.Cwd != "/tmp/project" {
		t.Fatalf("Cwd = %q", cmd.exec.Cwd)
	}
	if cmd.exec.Timeout != 2*time.Second {
		t.Fatalf("Timeout = %v, want 2s", cmd.exec.Timeout)
	}
	if !cmd.exec.JSON {
		t.Fatal("JSON = false, want true")
	}
	if cmd.exec.Prompt != "hello agent" {
		t.Fatalf("Prompt = %q", cmd.exec.Prompt)
	}
}

func TestParseACPClientExecCommandPreservesRepeatedArgs(t *testing.T) {
	cmd, err := parseACPClientCommand([]string{
		"exec",
		"--command", "/bin/acp-agent",
		"--arg", "--stdio",
		"--arg", "--profile=work",
		"--session", "sess-existing",
		"--no-wait",
		"hello",
	})
	if err != nil {
		t.Fatalf("parseACPClientCommand: %v", err)
	}
	want := []string{"--stdio", "--profile=work"}
	if len(cmd.exec.Args) != len(want) {
		t.Fatalf("Args = %#v, want %#v", cmd.exec.Args, want)
	}
	for i := range want {
		if cmd.exec.Args[i] != want[i] {
			t.Fatalf("Args[%d] = %q, want %q", i, cmd.exec.Args[i], want[i])
		}
	}
	if cmd.exec.SessionID != "sess-existing" {
		t.Fatalf("SessionID = %q, want sess-existing", cmd.exec.SessionID)
	}
	if !cmd.exec.NoWait {
		t.Fatal("NoWait = false, want true")
	}
}

func TestParseACPClientExecRejectsPromptAndFile(t *testing.T) {
	_, err := parseACPClientCommand([]string{"exec", "--command", "agent", "--file", "prompt.txt", "inline"})
	if err == nil || !strings.Contains(err.Error(), "cannot combine") {
		t.Fatalf("error = %v, want prompt/file exclusivity", err)
	}
}

func TestParseACPClientExecHelpPrintsFlagsAndSucceeds(t *testing.T) {
	var out bytes.Buffer
	cmd, err := parseACPClientCommandWithOutput([]string{"exec", "--help"}, &out)
	if err != nil {
		t.Fatalf("parseACPClientCommandWithOutput: %v", err)
	}
	if cmd.kind != acpClientCommandHandled {
		t.Fatalf("kind = %q, want handled", cmd.kind)
	}
	help := out.String()
	for _, want := range []string{"Usage: ratchet acp client exec", "--command", "--agent", "--json"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help output missing %q:\n%s", want, help)
		}
	}
}

func TestACPClientUsageLabelsHistoryAsShowAlias(t *testing.T) {
	var out bytes.Buffer
	cmd, err := parseACPClientCommandWithOutput(nil, &out)
	if err != nil {
		t.Fatalf("parseACPClientCommandWithOutput: %v", err)
	}
	if cmd.kind != acpClientCommandHelp {
		t.Fatalf("kind = %q, want help", cmd.kind)
	}
	printACPClientUsage(&out)
	if !strings.Contains(out.String(), "history (alias for show)") {
		t.Fatalf("usage output missing history alias label:\n%s", out.String())
	}
}

func TestParseACPClientSessionCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		kind acpClientCommandKind
		id   string
	}{
		{name: "sessions list", args: []string{"sessions", "list"}, kind: acpClientCommandSessionsList},
		{name: "sessions list json shorthand", args: []string{"sessions", "--json"}, kind: acpClientCommandSessionsList},
		{name: "sessions show", args: []string{"sessions", "show", "s1"}, kind: acpClientCommandSessionsShow, id: "s1"},
		{name: "status", args: []string{"status", "s1"}, kind: acpClientCommandStatus, id: "s1"},
		{name: "cancel", args: []string{"cancel", "s1"}, kind: acpClientCommandCancel, id: "s1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := parseACPClientCommand(tt.args)
			if err != nil {
				t.Fatalf("parseACPClientCommand: %v", err)
			}
			if cmd.kind != tt.kind {
				t.Fatalf("kind = %q, want %q", cmd.kind, tt.kind)
			}
			if cmd.sessionID != tt.id {
				t.Fatalf("sessionID = %q, want %q", cmd.sessionID, tt.id)
			}
		})
	}
}

func TestParseACPClientArchiveSessionCommands(t *testing.T) {
	exportCmd, err := parseACPClientCommand([]string{"sessions", "export", "s-export", "--output", "archive.json", "--json"})
	if err != nil {
		t.Fatalf("parse export: %v", err)
	}
	if exportCmd.kind != acpClientCommandSessionsExport || exportCmd.sessionID != "s-export" || exportCmd.archive.Output != "archive.json" || !exportCmd.json {
		t.Fatalf("export command = %#v", exportCmd)
	}

	importCmd, err := parseACPClientCommand([]string{
		"sessions", "import", "archive.json",
		"--session", "s-imported",
		"--cwd", "/tmp/imported",
		"--agent", "custom",
		"--command", "/bin/fixture-agent",
		"--arg", "--load-session",
		"--json",
	})
	if err != nil {
		t.Fatalf("parse import: %v", err)
	}
	if importCmd.kind != acpClientCommandSessionsImport || importCmd.archive.Path != "archive.json" ||
		importCmd.archive.SessionID != "s-imported" || importCmd.archive.Cwd != "/tmp/imported" ||
		importCmd.archive.Agent != "custom" || importCmd.archive.Command != "/bin/fixture-agent" ||
		len(importCmd.archive.Args) != 1 || importCmd.archive.Args[0] != "--load-session" || !importCmd.json {
		t.Fatalf("import command = %#v", importCmd)
	}
}

func TestParseACPClientArchiveSessionCommandsRejectInvalidArgs(t *testing.T) {
	tests := [][]string{
		{"sessions", "export", "s1"},
		{"sessions", "export", "s1", "--output"},
		{"sessions", "export", "s1", "--output", "archive.json", "extra"},
		{"sessions", "import"},
		{"sessions", "import", "archive.json", "--session"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, err := parseACPClientCommand(args); err == nil {
				t.Fatalf("parseACPClientCommand(%#v) succeeded, want error", args)
			}
		})
	}
}

func TestParseACPClientQueueCommand(t *testing.T) {
	cmd, err := parseACPClientCommand([]string{"queue", "fifo-session", "--json"})
	if err != nil {
		t.Fatalf("parseACPClientCommand: %v", err)
	}
	if cmd.kind != acpClientCommandQueue {
		t.Fatalf("kind = %q, want queue", cmd.kind)
	}
	if cmd.sessionID != "fifo-session" {
		t.Fatalf("sessionID = %q, want fifo-session", cmd.sessionID)
	}
	if !cmd.json {
		t.Fatal("json = false, want true")
	}
}

func TestParseACPClientDrainCommand(t *testing.T) {
	cmd, err := parseACPClientCommand([]string{
		"drain",
		"fifo-session",
		"--command", "/bin/acp-agent",
		"--arg", "--stdio",
		"--arg", "--profile=work",
		"--agent", "custom",
		"--cwd", "/tmp/project",
		"--timeout", "5s",
		"--max", "2",
	})
	if err != nil {
		t.Fatalf("parseACPClientCommand: %v", err)
	}
	if cmd.kind != acpClientCommandDrain {
		t.Fatalf("kind = %q, want drain", cmd.kind)
	}
	if cmd.sessionID != "fifo-session" {
		t.Fatalf("sessionID = %q, want fifo-session", cmd.sessionID)
	}
	if cmd.drain.Command != "/bin/acp-agent" || cmd.drain.Agent != "custom" || cmd.drain.Cwd != "/tmp/project" {
		t.Fatalf("drain options = %#v", cmd.drain)
	}
	if got, want := strings.Join(cmd.drain.Args, ","), "--stdio,--profile=work"; got != want {
		t.Fatalf("drain args = %q, want %q", got, want)
	}
	if cmd.drain.Timeout != 5*time.Second || cmd.drain.Max != 2 {
		t.Fatalf("drain timeout/max = %s/%d, want 5s/2", cmd.drain.Timeout, cmd.drain.Max)
	}
}

func TestParseACPClientDrainRejectsInvalidArgs(t *testing.T) {
	tests := [][]string{
		{"drain"},
		{"drain", "fifo-session", "--max", "0"},
		{"drain", "fifo-session", "--command", "agent", "extra"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, err := parseACPClientCommand(args); err == nil {
				t.Fatalf("parseACPClientCommand(%#v) succeeded, want error", args)
			}
		})
	}
}

func TestExecuteACPClientExecHumanOutput(t *testing.T) {
	runner := &fakeACPClientExecRunner{
		result: acpclient.Result{
			SessionID:  "s1",
			StopReason: acpsdk.StopReasonEndTurn,
			Text:       "hello from fixture",
		},
	}
	var out bytes.Buffer
	err := executeACPClientExec(t.Context(), acpClientExecOptions{
		Command: "/bin/fixture-agent",
		Prompt:  "hello",
		Cwd:     ".",
		Timeout: time.Second,
	}, runner, &out)
	if err != nil {
		t.Fatalf("executeACPClientExec: %v", err)
	}
	if got := out.String(); got != "hello from fixture\n[stop: end_turn]\n" {
		t.Fatalf("output = %q", got)
	}
	if runner.prompt != "hello" {
		t.Fatalf("prompt = %q", runner.prompt)
	}
	if runner.spec.Command != "/bin/fixture-agent" {
		t.Fatalf("command = %q", runner.spec.Command)
	}
}

func TestExecuteACPClientExecJSONOutput(t *testing.T) {
	runner := &fakeACPClientExecRunner{
		result: acpclient.Result{
			SessionID:  "s1",
			StopReason: acpsdk.StopReasonEndTurn,
			Text:       "json fixture",
			Duration:   25 * time.Millisecond,
		},
	}
	var out bytes.Buffer
	err := executeACPClientExec(t.Context(), acpClientExecOptions{
		Command: "/bin/fixture-agent",
		Prompt:  "hello",
		Cwd:     ".",
		Timeout: time.Second,
		JSON:    true,
	}, runner, &out)
	if err != nil {
		t.Fatalf("executeACPClientExec: %v", err)
	}
	var payload struct {
		Command         string `json:"command"`
		SessionID       string `json:"session_id"`
		StopReason      string `json:"stop_reason"`
		Text            string `json:"text"`
		DurationMillis  int64  `json:"duration_ms"`
		CommandFpPrefix string `json:"command_fp_prefix"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json output: %v\n%s", err, out.String())
	}
	if payload.Command != "/bin/fixture-agent" || payload.SessionID != "s1" || payload.StopReason != "end_turn" || payload.Text != "json fixture" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.DurationMillis != 25 {
		t.Fatalf("DurationMillis = %d, want 25", payload.DurationMillis)
	}
	if payload.CommandFpPrefix == "" {
		t.Fatal("CommandFpPrefix is empty")
	}
}

func TestExecuteACPClientExecPersistsSessionRecord(t *testing.T) {
	store := acpclient.NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	runner := &fakeACPClientExecRunner{
		result: acpclient.Result{
			SessionID:  "s-persisted",
			StopReason: acpsdk.StopReasonEndTurn,
			Text:       "persisted response",
			Duration:   25 * time.Millisecond,
		},
	}
	var out bytes.Buffer
	err := executeACPClientExecWithStore(t.Context(), acpClientExecOptions{
		Agent:   "custom",
		Command: "/bin/fixture-agent",
		Prompt:  "hello persisted",
		Cwd:     ".",
		Timeout: time.Second,
	}, runner, store, &out)
	if err != nil {
		t.Fatalf("executeACPClientExecWithStore: %v", err)
	}
	rec, err := store.Get("s-persisted")
	if err != nil {
		t.Fatalf("Get persisted session: %v", err)
	}
	if rec.Status != acpclient.SessionStatusCompleted || rec.Summary != "persisted response" || rec.LastStopReason != "end_turn" {
		t.Fatalf("record = %#v", rec)
	}
	if len(rec.Turns) != 1 || rec.Turns[0].Prompt != "hello persisted" || rec.Turns[0].Response != "persisted response" {
		t.Fatalf("Turns = %#v", rec.Turns)
	}
}

func TestExecuteACPClientExecNoWaitQueuesPrompt(t *testing.T) {
	store := acpclient.NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	runner := &fakeACPClientExecRunner{}
	var out bytes.Buffer
	err := executeACPClientExecWithStore(t.Context(), acpClientExecOptions{
		SessionID: "s-queued",
		Command:   "/bin/fixture-agent",
		Prompt:    "queued prompt",
		Cwd:       ".",
		Timeout:   time.Second,
		NoWait:    true,
	}, runner, store, &out)
	if err != nil {
		t.Fatalf("executeACPClientExecWithStore: %v", err)
	}
	if runner.called {
		t.Fatal("runner called for --no-wait")
	}
	rec, err := store.Get("s-queued")
	if err != nil {
		t.Fatalf("Get queued session: %v", err)
	}
	if rec.Status != acpclient.SessionStatusQueued || len(rec.PromptQueue) != 1 {
		t.Fatalf("queued record = %#v", rec)
	}
	if rec.PendingPrompt != nil {
		t.Fatalf("PendingPrompt = %#v, want nil for new no-wait writes", rec.PendingPrompt)
	}
	if rec.PromptQueue[0].Prompt != "queued prompt" || rec.PromptQueue[0].Status != acpclient.QueuePromptStatusPending {
		t.Fatalf("PromptQueue = %#v", rec.PromptQueue)
	}
	if got := out.String(); !strings.Contains(got, "queued prompt") || !strings.Contains(got, "queue depth: 1") {
		t.Fatalf("output = %q", got)
	}
}

func TestExecuteACPClientExecNoWaitAppendsPromptQueue(t *testing.T) {
	store := acpclient.NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	runner := &fakeACPClientExecRunner{}
	for _, prompt := range []string{"first queued", "second queued"} {
		var out bytes.Buffer
		if err := executeACPClientExecWithStore(t.Context(), acpClientExecOptions{
			SessionID: "s-fifo",
			Command:   "/bin/fixture-agent",
			Prompt:    prompt,
			Cwd:       ".",
			Timeout:   time.Second,
			NoWait:    true,
		}, runner, store, &out); err != nil {
			t.Fatalf("executeACPClientExecWithStore %q: %v", prompt, err)
		}
	}
	if runner.called {
		t.Fatal("runner called for --no-wait")
	}
	rec, err := store.Get("s-fifo")
	if err != nil {
		t.Fatalf("Get queued session: %v", err)
	}
	if rec.PendingPrompt != nil {
		t.Fatalf("PendingPrompt = %#v, want new writes to use PromptQueue only", rec.PendingPrompt)
	}
	if len(rec.PromptQueue) != 2 {
		t.Fatalf("PromptQueue len = %d, want 2: %#v", len(rec.PromptQueue), rec.PromptQueue)
	}
	if rec.PromptQueue[0].Prompt != "first queued" || rec.PromptQueue[1].Prompt != "second queued" {
		t.Fatalf("PromptQueue = %#v, want FIFO prompts", rec.PromptQueue)
	}
	if !rec.CreatedAt.Equal(rec.PromptQueue[0].CreatedAt) {
		t.Fatalf("CreatedAt = %s, want first queue item CreatedAt %s", rec.CreatedAt, rec.PromptQueue[0].CreatedAt)
	}
}

func TestExecuteACPClientExecNoWaitRequiresStore(t *testing.T) {
	runner := &fakeACPClientExecRunner{}
	var out bytes.Buffer
	err := executeACPClientExec(t.Context(), acpClientExecOptions{
		SessionID: "s-queued",
		Command:   "/bin/fixture-agent",
		Prompt:    "queued prompt",
		Cwd:       ".",
		Timeout:   time.Second,
		NoWait:    true,
	}, runner, &out)
	if err == nil || !strings.Contains(err.Error(), "store is required") {
		t.Fatalf("executeACPClientExec no-wait without store error = %v, want store required", err)
	}
	if runner.called {
		t.Fatal("runner called for failed --no-wait")
	}
}

func TestExecuteACPClientQueueOutputsHumanAndJSON(t *testing.T) {
	store := acpclient.NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 23, 0, 0, 0, time.UTC)
	if err := store.Upsert(acpclient.SessionRecord{
		ID:        "s-queue",
		Status:    acpclient.SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		PromptQueue: []acpclient.QueuedPrompt{
			{ID: "q-1", Prompt: "first", Status: acpclient.QueuePromptStatusPending, CreatedAt: now},
			{ID: "q-2", Prompt: "second", Status: acpclient.QueuePromptStatusRunning, CreatedAt: now.Add(time.Second)},
		},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	var human bytes.Buffer
	if err := executeACPClientQueue(store, "s-queue", false, &human); err != nil {
		t.Fatalf("executeACPClientQueue human: %v", err)
	}
	if got := human.String(); !strings.Contains(got, "q-1") || !strings.Contains(got, "pending") || !strings.Contains(got, "q-2") || !strings.Contains(got, "running") {
		t.Fatalf("queue human output = %q", got)
	}

	var jsonOut bytes.Buffer
	if err := executeACPClientQueue(store, "s-queue", true, &jsonOut); err != nil {
		t.Fatalf("executeACPClientQueue json: %v", err)
	}
	var payload struct {
		SessionID string `json:"session_id"`
		Items     []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(jsonOut.Bytes(), &payload); err != nil {
		t.Fatalf("queue json: %v\n%s", err, jsonOut.String())
	}
	if payload.SessionID != "s-queue" || len(payload.Items) != 2 || payload.Items[0].ID != "q-1" || payload.Items[1].Status != "running" {
		t.Fatalf("queue payload = %#v", payload)
	}
}

func TestACPClientStatusAndCancelPromptQueue(t *testing.T) {
	store := acpclient.NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 23, 5, 0, 0, time.UTC)
	if err := store.Upsert(acpclient.SessionRecord{
		ID:        "s-queue-status",
		Status:    acpclient.SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		PromptQueue: []acpclient.QueuedPrompt{
			{ID: "q-1", Prompt: "first", Status: acpclient.QueuePromptStatusPending, CreatedAt: now},
			{ID: "q-2", Prompt: "second", Status: acpclient.QueuePromptStatusPending, CreatedAt: now.Add(time.Second)},
			{ID: "q-3", Prompt: "done", Status: acpclient.QueuePromptStatusCompleted, CreatedAt: now.Add(2 * time.Second)},
		},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	var statusOut bytes.Buffer
	if err := executeACPClientStatus(store, "s-queue-status", false, &statusOut); err != nil {
		t.Fatalf("executeACPClientStatus: %v", err)
	}
	if got := statusOut.String(); !strings.Contains(got, "queue: 2 pending, 0 running, 1 completed") {
		t.Fatalf("status output = %q", got)
	}

	var cancelOut bytes.Buffer
	if err := executeACPClientCancel(store, "s-queue-status", false, &cancelOut); err != nil {
		t.Fatalf("executeACPClientCancel: %v", err)
	}
	if got := cancelOut.String(); !strings.Contains(got, "canceled 2 pending prompts") {
		t.Fatalf("cancel output = %q", got)
	}
	rec, err := store.Get("s-queue-status")
	if err != nil {
		t.Fatalf("Get canceled queue: %v", err)
	}
	if rec.PromptQueue[0].Status != acpclient.QueuePromptStatusCanceled || rec.PromptQueue[1].Status != acpclient.QueuePromptStatusCanceled {
		t.Fatalf("PromptQueue after cancel = %#v", rec.PromptQueue)
	}
}

func TestExecuteACPClientDrainUsesInjectedRunner(t *testing.T) {
	store := acpclient.NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 23, 10, 0, 0, time.UTC)
	if err := store.Upsert(acpclient.SessionRecord{
		ID:        "s-drain",
		Status:    acpclient.SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		PromptQueue: []acpclient.QueuedPrompt{{
			ID: "q-1", Prompt: "drain me", Status: acpclient.QueuePromptStatusPending, CreatedAt: now,
		}},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	runner := &fakeDrainPromptRunner{sessionID: "acp-drain"}
	var out bytes.Buffer
	if err := executeACPClientDrain(t.Context(), store, "s-drain", acpClientDrainOptions{
		Command: "/bin/fixture-agent",
		Cwd:     ".",
		Timeout: time.Second,
		Max:     1,
	}, func(context.Context, acpclient.AgentSpec, acpclient.RunOptions, string) (acpclient.DrainPromptRunner, func() error, error) {
		return runner, func() error { return nil }, nil
	}, &out); err != nil {
		t.Fatalf("executeACPClientDrain: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "drained 1 prompt") || !strings.Contains(got, "remaining: 0") {
		t.Fatalf("drain output = %q", got)
	}
	rec, err := store.Get("s-drain")
	if err != nil {
		t.Fatalf("Get drained: %v", err)
	}
	if rec.ACPSessionID != "acp-drain" || rec.PromptQueue[0].Status != acpclient.QueuePromptStatusCompleted {
		t.Fatalf("record after drain = %#v", rec)
	}
}

func TestExecuteACPClientArchiveExportAndImport(t *testing.T) {
	store := acpclient.NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	if err := store.Upsert(acpclient.SessionRecord{
		ID:                 "s-export",
		ACPSessionID:       "provider-session",
		Agent:              "fixture",
		CommandFingerprint: "fp-export",
		Cwd:                "/tmp/source",
		Status:             acpclient.SessionStatusCompleted,
		CreatedAt:          now,
		UpdatedAt:          now,
		Summary:            "export me",
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	archivePath := filepath.Join(t.TempDir(), "archive.json")

	var exportOut bytes.Buffer
	if err := executeACPClientSessionExport(store, "s-export", acpClientArchiveOptions{Output: archivePath}, false, &exportOut); err != nil {
		t.Fatalf("execute export: %v", err)
	}
	if got := exportOut.String(); !strings.Contains(got, "exported s-export") || !strings.Contains(got, archivePath) {
		t.Fatalf("export output = %q", got)
	}
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("stat archive: %v", err)
	}

	var importOut bytes.Buffer
	if err := executeACPClientSessionImport(store, acpClientArchiveOptions{
		Path:      archivePath,
		SessionID: "s-imported",
		Cwd:       "/tmp/imported",
		Agent:     "custom",
		Command:   "/bin/fixture-agent",
		Args:      []string{"--load-session"},
	}, true, &importOut); err != nil {
		t.Fatalf("execute import: %v", err)
	}
	var payload struct {
		SessionID string `json:"session_id"`
		Path      string `json:"path"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(importOut.Bytes(), &payload); err != nil {
		t.Fatalf("import json: %v\n%s", err, importOut.String())
	}
	if payload.SessionID != "s-imported" || payload.Path != archivePath || payload.Status != acpclient.SessionStatusCompleted {
		t.Fatalf("payload = %#v", payload)
	}
	imported, err := store.Get("s-imported")
	if err != nil {
		t.Fatalf("Get imported: %v", err)
	}
	if imported.Cwd != "/tmp/imported" || imported.Agent != "custom" || imported.CommandFingerprint == "" || imported.ACPSessionID != "provider-session" {
		t.Fatalf("imported = %#v", imported)
	}
}

func TestACPClientSessionsStatusAndCancelCommands(t *testing.T) {
	store := acpclient.NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 19, 30, 0, 0, time.UTC)
	if err := store.Upsert(acpclient.SessionRecord{
		ID:                 "s-one",
		Agent:              "custom",
		CommandFingerprint: "abcdef123456",
		Cwd:                "/tmp/project",
		Status:             acpclient.SessionStatusQueued,
		CreatedAt:          now,
		UpdatedAt:          now,
		Summary:            "queued prompt",
		PendingPrompt: &acpclient.PendingPrompt{
			ID:        "pending-1",
			Prompt:    "queued prompt",
			Status:    acpclient.PendingPromptStatusPending,
			CreatedAt: now,
		},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	var listOut bytes.Buffer
	if err := executeACPClientSessionsList(store, false, &listOut); err != nil {
		t.Fatalf("executeACPClientSessionsList: %v", err)
	}
	if got := listOut.String(); !strings.Contains(got, "s-one") || !strings.Contains(got, "queued") {
		t.Fatalf("list output = %q", got)
	}

	var showOut bytes.Buffer
	if err := executeACPClientSessionShow(store, "s-one", true, &showOut); err != nil {
		t.Fatalf("executeACPClientSessionShow: %v", err)
	}
	var payload struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(showOut.Bytes(), &payload); err != nil {
		t.Fatalf("show json: %v\n%s", err, showOut.String())
	}
	if payload.ID != "s-one" || payload.Status != acpclient.SessionStatusQueued {
		t.Fatalf("payload = %#v", payload)
	}

	var statusOut bytes.Buffer
	if err := executeACPClientStatus(store, "s-one", false, &statusOut); err != nil {
		t.Fatalf("executeACPClientStatus: %v", err)
	}
	if got := statusOut.String(); !strings.Contains(got, "pending prompt") {
		t.Fatalf("status output = %q", got)
	}

	var cancelOut bytes.Buffer
	if err := executeACPClientCancel(store, "s-one", false, &cancelOut); err != nil {
		t.Fatalf("executeACPClientCancel: %v", err)
	}
	if got := cancelOut.String(); !strings.Contains(got, "canceled pending prompt") {
		t.Fatalf("cancel output = %q", got)
	}
	rec, err := store.Get("s-one")
	if err != nil {
		t.Fatalf("Get canceled: %v", err)
	}
	if rec.Status != acpclient.SessionStatusCanceled || rec.PendingPrompt.Status != acpclient.PendingPromptStatusCanceled {
		t.Fatalf("record after cancel = %#v", rec)
	}
}

func TestACPClientStatusAndCancelActiveOwner(t *testing.T) {
	store := acpclient.NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 19, 45, 0, 0, time.UTC)
	if err := store.Upsert(acpclient.SessionRecord{
		ID:                 "s-active",
		Agent:              "custom",
		CommandFingerprint: "abcdef123456",
		Cwd:                "/tmp/project",
		Status:             acpclient.SessionStatusRunning,
		CreatedAt:          now,
		UpdatedAt:          now,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := store.WriteOwner(acpclient.OwnerLock{
		SessionID:          "s-active",
		PID:                12345,
		CommandFingerprint: "abcdef123456",
		StartedAt:          now,
	}); err != nil {
		t.Fatalf("WriteOwner: %v", err)
	}

	var statusOut bytes.Buffer
	if err := executeACPClientStatus(store, "s-active", false, &statusOut); err != nil {
		t.Fatalf("executeACPClientStatus: %v", err)
	}
	if got := statusOut.String(); !strings.Contains(got, "owner pid: 12345") {
		t.Fatalf("status output = %q", got)
	}

	var cancelOut bytes.Buffer
	if err := executeACPClientCancel(store, "s-active", false, &cancelOut); err != nil {
		t.Fatalf("executeACPClientCancel: %v", err)
	}
	if got := cancelOut.String(); !strings.Contains(got, "requested cancel for active session s-active") {
		t.Fatalf("cancel output = %q", got)
	}
	req, err := store.CancelRequest("s-active")
	if err != nil {
		t.Fatalf("CancelRequest: %v", err)
	}
	if req.SessionID != "s-active" {
		t.Fatalf("CancelRequest = %#v", req)
	}
}

func TestACPClientCancelReturnsCorruptOwnerError(t *testing.T) {
	store := acpclient.NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 19, 50, 0, 0, time.UTC)
	if err := store.Upsert(acpclient.SessionRecord{
		ID:        "s-corrupt-owner",
		Status:    acpclient.SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		PendingPrompt: &acpclient.PendingPrompt{
			ID:        "pending-1",
			Prompt:    "queued prompt",
			Status:    acpclient.PendingPromptStatusPending,
			CreatedAt: now,
		},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	ownerPath := filepath.Join(filepath.Dir(store.Path()), "owners", base64.RawURLEncoding.EncodeToString([]byte("s-corrupt-owner"))+".json")
	if err := os.MkdirAll(filepath.Dir(ownerPath), 0o755); err != nil {
		t.Fatalf("mkdir owner dir: %v", err)
	}
	if err := os.WriteFile(ownerPath, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("write corrupt owner: %v", err)
	}

	var out bytes.Buffer
	err := executeACPClientCancel(store, "s-corrupt-owner", false, &out)
	if err == nil || !strings.Contains(err.Error(), "read") {
		t.Fatalf("executeACPClientCancel error = %v, want corrupt owner error", err)
	}
	rec, err := store.Get("s-corrupt-owner")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.PendingPrompt.Status != acpclient.PendingPromptStatusPending {
		t.Fatalf("pending prompt status = %q, want pending", rec.PendingPrompt.Status)
	}
}

func TestACPClientSessionsEmptyAndInvalidID(t *testing.T) {
	store := acpclient.NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	var out bytes.Buffer
	if err := executeACPClientSessionsList(store, false, &out); err != nil {
		t.Fatalf("executeACPClientSessionsList: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "No ACP client sessions.") {
		t.Fatalf("empty output = %q", got)
	}
	if err := executeACPClientSessionShow(store, "missing", false, &out); err == nil {
		t.Fatal("executeACPClientSessionShow missing succeeded, want error")
	}
}

func TestExecuteACPClientExecReadsPromptFile(t *testing.T) {
	promptFile := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(promptFile, []byte("from file"), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}
	runner := &fakeACPClientExecRunner{result: acpclient.Result{StopReason: acpsdk.StopReasonEndTurn}}
	var out bytes.Buffer
	err := executeACPClientExec(t.Context(), acpClientExecOptions{
		Command: "/bin/fixture-agent",
		File:    promptFile,
		Cwd:     ".",
		Timeout: time.Second,
	}, runner, &out)
	if err != nil {
		t.Fatalf("executeACPClientExec: %v", err)
	}
	if runner.prompt != "from file" {
		t.Fatalf("prompt = %q, want file contents", runner.prompt)
	}
}

func TestExecuteACPClientExecRejectsMissingCommand(t *testing.T) {
	runner := &fakeACPClientExecRunner{}
	var out bytes.Buffer
	err := executeACPClientExec(t.Context(), acpClientExecOptions{
		Agent:   "custom",
		Prompt:  "hello",
		Cwd:     ".",
		Timeout: time.Second,
	}, runner, &out)
	if err == nil || !strings.Contains(err.Error(), "command is required") {
		t.Fatalf("error = %v, want missing command", err)
	}
	if runner.called {
		t.Fatal("runner was called despite missing command")
	}
}

type fakeACPClientExecRunner struct {
	called bool
	spec   acpclient.AgentSpec
	opts   acpclient.RunOptions
	prompt string
	result acpclient.Result
	err    error
}

func (r *fakeACPClientExecRunner) RunPrompt(_ context.Context, spec acpclient.AgentSpec, opts acpclient.RunOptions, prompt string) (acpclient.Result, error) {
	r.called = true
	r.spec = spec
	r.opts = opts
	r.prompt = prompt
	return r.result, r.err
}

type fakeDrainPromptRunner struct {
	sessionID acpsdk.SessionId
	prompts   []string
}

func (r *fakeDrainPromptRunner) SessionID() acpsdk.SessionId {
	return r.sessionID
}

func (r *fakeDrainPromptRunner) Prompt(_ context.Context, prompt string) (acpclient.Result, error) {
	r.prompts = append(r.prompts, prompt)
	return acpclient.Result{
		SessionID:  r.sessionID,
		StopReason: acpsdk.StopReasonEndTurn,
		Text:       "drained: " + prompt,
	}, nil
}
