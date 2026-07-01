package main

import (
	"bytes"
	"context"
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

func TestParseACPClientSessionCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		kind acpClientCommandKind
		id   string
	}{
		{name: "sessions list", args: []string{"sessions", "list"}, kind: acpClientCommandSessionsList},
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
