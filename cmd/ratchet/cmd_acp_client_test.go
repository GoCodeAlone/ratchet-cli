package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func TestParseACPClientCompareCommand(t *testing.T) {
	cmd, err := parseACPClientCommand([]string{
		"compare",
		"--command", "/bin/agent-a",
		"--command", "/bin/agent-b",
		"--cwd", "/tmp/project",
		"--timeout", "3s",
		"--save",
		"--run-id", "fixed-run",
		"--run-root", "/tmp/compare-runs",
		"compare", "prompt",
	})
	if err != nil {
		t.Fatalf("parseACPClientCommand: %v", err)
	}
	if cmd.kind != acpClientCommandCompare {
		t.Fatalf("kind = %q, want compare", cmd.kind)
	}
	if got, want := strings.Join(cmd.compare.Commands, ","), "/bin/agent-a,/bin/agent-b"; got != want {
		t.Fatalf("Commands = %q, want %q", got, want)
	}
	if cmd.compare.Cwd != "/tmp/project" || cmd.compare.Timeout != 3*time.Second || cmd.compare.Prompt != "compare prompt" ||
		!cmd.compare.Save || cmd.compare.RunID != "fixed-run" || cmd.compare.RunRoot != "/tmp/compare-runs" {
		t.Fatalf("compare options = %#v", cmd.compare)
	}

	fileCmd, err := parseACPClientCommand([]string{
		"compare",
		"--agent", "codex",
		"--agent", "claude",
		"--file", "prompt.txt",
		"--json",
	})
	if err != nil {
		t.Fatalf("parse file compare: %v", err)
	}
	if got, want := strings.Join(fileCmd.compare.Agents, ","), "codex,claude"; got != want {
		t.Fatalf("Agents = %q, want %q", got, want)
	}
	if fileCmd.compare.File != "prompt.txt" || !fileCmd.compare.JSON {
		t.Fatalf("file compare options = %#v", fileCmd.compare)
	}
}

func TestParseACPClientCompareRejectsInvalidArgs(t *testing.T) {
	tests := [][]string{
		{"compare", "--command", "/bin/agent", "prompt"},
		{"compare", "--agent", "codex", "--file", "prompt.txt", "inline"},
		{"compare", "--command", "/bin/a", "--command", "/bin/b"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, err := parseACPClientCommand(args); err == nil {
				t.Fatalf("parseACPClientCommand(%#v) succeeded, want error", args)
			}
		})
	}
}

func TestParseACPClientFlowRunCommand(t *testing.T) {
	cmd, err := parseACPClientCommand([]string{
		"flow", "run", "flow.json",
		"--input-json", `{"task":"x"}`,
		"--command", "/bin/fixture-agent",
		"--arg", "--echo-session",
		"--cwd", "/tmp/project",
		"--allow", "shell",
		"--allow", "outside-cwd",
		"--json",
	})
	if err != nil {
		t.Fatalf("parseACPClientCommand: %v", err)
	}
	if cmd.kind != acpClientCommandFlowRun {
		t.Fatalf("kind = %q, want flow run", cmd.kind)
	}
	if cmd.flow.Path != "flow.json" || cmd.flow.InputJSON != `{"task":"x"}` ||
		cmd.flow.Command != "/bin/fixture-agent" || len(cmd.flow.Args) != 1 ||
		cmd.flow.Args[0] != "--echo-session" || cmd.flow.Cwd != "/tmp/project" || !cmd.flow.JSON {
		t.Fatalf("flow options = %#v", cmd.flow)
	}
	if got, want := strings.Join(cmd.flow.AllowedPermissions, ","), "shell,outside-cwd"; got != want {
		t.Fatalf("AllowedPermissions = %q, want %q", got, want)
	}

	fileCmd, err := parseACPClientCommand([]string{"flow", "run", "flow.json", "--input-file", "input.json", "--default-agent", "custom"})
	if err != nil {
		t.Fatalf("parse file flow: %v", err)
	}
	if fileCmd.flow.InputFile != "input.json" || fileCmd.flow.DefaultAgent != "custom" {
		t.Fatalf("file flow options = %#v", fileCmd.flow)
	}
}

func TestParseACPClientFlowReplayCommand(t *testing.T) {
	cmd, err := parseACPClientCommand([]string{"flow", "replay", "/tmp/flow-run", "--json"})
	if err != nil {
		t.Fatalf("parseACPClientCommand: %v", err)
	}
	if cmd.kind != acpClientCommandFlowReplay {
		t.Fatalf("kind = %q, want flow replay", cmd.kind)
	}
	if cmd.flow.Path != "/tmp/flow-run" || !cmd.flow.JSON {
		t.Fatalf("flow options = %#v", cmd.flow)
	}
}

func TestParseACPClientFlowRunRejectsInvalidArgs(t *testing.T) {
	tests := [][]string{
		{"flow", "run"},
		{"flow", "run", "flow.json", "--input-json", "{}", "--input-file", "input.json"},
		{"flow", "run", "flow.json", "--allow", ""},
		{"flow", "replay"},
		{"flow", "replay", "run-dir", "extra"},
		{"flow", "show", "flow.json"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, err := parseACPClientCommand(args); err == nil {
				t.Fatalf("parseACPClientCommand(%#v) succeeded, want error", args)
			}
		})
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

func TestACPClientProfilesAddListTrustRemove(t *testing.T) {
	home := t.TempDir()
	stateHome := filepath.Join(home, "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateHome)

	if err := handleACPClient([]string{"profiles", "add", "fixture", "--command", "/tmp/acp-agent", "--arg", "--stdio", "--env-key", "API_KEY", "--cwd", "/tmp/project"}); err != nil {
		t.Fatalf("profiles add: %v", err)
	}
	listOut := captureStdout(t, func() {
		if err := handleACPClient([]string{"profiles", "list", "--json"}); err != nil {
			t.Fatalf("profiles list: %v", err)
		}
	})
	if !strings.Contains(listOut, `"name":"fixture"`) || !strings.Contains(listOut, `"trusted":false`) {
		t.Fatalf("profiles list output = %s", listOut)
	}
	if err := handleACPClient([]string{"profiles", "trust", "fixture"}); err != nil {
		t.Fatalf("profiles trust: %v", err)
	}
	listOut = captureStdout(t, func() {
		if err := handleACPClient([]string{"profiles", "list", "--json"}); err != nil {
			t.Fatalf("profiles list trusted: %v", err)
		}
	})
	if !strings.Contains(listOut, `"trusted":true`) {
		t.Fatalf("trusted profile missing from list: %s", listOut)
	}
	if err := handleACPClient([]string{"profiles", "remove", "fixture"}); err != nil {
		t.Fatalf("profiles remove: %v", err)
	}
	listOut = captureStdout(t, func() {
		if err := handleACPClient([]string{"profiles", "list", "--json"}); err != nil {
			t.Fatalf("profiles list removed: %v", err)
		}
	})
	if strings.Contains(listOut, `"name":"fixture"`) {
		t.Fatalf("removed profile still listed: %s", listOut)
	}
}

func TestACPClientProfilesInstallPluginTemplate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	pluginDir := filepath.Join(home, ".ratchet", "plugins", "profile-plugin")
	if err := os.MkdirAll(filepath.Join(pluginDir, ".ratchet-plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"profile-plugin","version":"1.0.0","description":"test","author":{"name":"test"},"capabilities":{"acpProfiles":"profiles.yaml"}}`
	if err := os.WriteFile(filepath.Join(pluginDir, ".ratchet-plugin", "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "profiles.yaml"), []byte(`
profiles:
  - name: fixture
    spec:
      command: /tmp/plugin-agent
      args: ["--stdio"]
`), 0o600); err != nil {
		t.Fatal(err)
	}

	listOut := captureStdout(t, func() {
		if err := handleACPClient([]string{"profiles", "list", "--json"}); err != nil {
			t.Fatalf("profiles list templates: %v", err)
		}
	})
	if !strings.Contains(listOut, `"template":true`) || !strings.Contains(listOut, `"pluginName":"profile-plugin"`) {
		t.Fatalf("plugin template missing from list: %s", listOut)
	}
	if err := handleACPClient([]string{"profiles", "install", "profile-plugin/fixture", "--as", "installed", "--trust"}); err != nil {
		t.Fatalf("profiles install: %v", err)
	}
	listOut = captureStdout(t, func() {
		if err := handleACPClient([]string{"profiles", "list", "--json"}); err != nil {
			t.Fatalf("profiles list installed: %v", err)
		}
	})
	if !strings.Contains(listOut, `"name":"installed"`) || !strings.Contains(listOut, `"trusted":true`) {
		t.Fatalf("installed profile missing/trust state wrong: %s", listOut)
	}
}

func TestLoadACPProfileTemplatesRequiresHomeDir(t *testing.T) {
	oldUserHomeDir := userHomeDir
	userHomeDir = func() (string, error) {
		return "", errors.New("home unavailable")
	}
	t.Cleanup(func() {
		userHomeDir = oldUserHomeDir
	})

	_, err := loadACPProfileTemplates()
	if err == nil {
		t.Fatal("loadACPProfileTemplates error = nil, want home directory error")
	}
	if !strings.Contains(err.Error(), "home directory") {
		t.Fatalf("loadACPProfileTemplates error = %v, want home directory context", err)
	}
}

func TestACPClientProfilesRejectBuiltinShadowing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	err := handleACPClient([]string{"profiles", "add", "codex", "--command", "/tmp/acp-agent"})
	if !errors.Is(err, acpclient.ErrProfileShadowsBuiltin) {
		t.Fatalf("profiles add codex error = %v, want ErrProfileShadowsBuiltin", err)
	}
}

func TestParseACPClientArchiveSessionCommands(t *testing.T) {
	exportCmd, err := parseACPClientCommand([]string{"sessions", "export", "s-export", "--output", "archive.json", "--history", "raw", "--json"})
	if err != nil {
		t.Fatalf("parse export: %v", err)
	}
	if exportCmd.kind != acpClientCommandSessionsExport || exportCmd.sessionID != "s-export" ||
		exportCmd.archive.Output != "archive.json" || exportCmd.archive.HistoryMode != "raw" || !exportCmd.json {
		t.Fatalf("export command = %#v", exportCmd)
	}

	eventsCmd, err := parseACPClientCommand([]string{"sessions", "events", "s-export", "--output", "events.ndjson", "--json"})
	if err != nil {
		t.Fatalf("parse events: %v", err)
	}
	if eventsCmd.kind != acpClientCommandSessionsEvents || eventsCmd.sessionID != "s-export" ||
		eventsCmd.archive.Output != "events.ndjson" || !eventsCmd.json {
		t.Fatalf("events command = %#v", eventsCmd)
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
		{"sessions", "export", "s1", "--output", "archive.json", "--history", "bogus"},
		{"sessions", "events"},
		{"sessions", "events", "s1", "extra"},
		{"sessions", "events", "s1", "--output"},
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

func TestParseACPClientWatchCommand(t *testing.T) {
	cmd, err := parseACPClientCommand([]string{
		"watch",
		"fifo-session",
		"--command", "/bin/acp-agent",
		"--arg", "--stdio",
		"--arg", "--profile=work",
		"--agent", "custom",
		"--cwd", "/tmp/project",
		"--timeout", "5s",
		"--interval", "100ms",
		"--max-per-cycle", "2",
		"--max-cycles", "1",
		"--stop-when-empty",
		"--json",
	})
	if err != nil {
		t.Fatalf("parseACPClientCommand: %v", err)
	}
	if cmd.kind != acpClientCommandWatch {
		t.Fatalf("kind = %q, want watch", cmd.kind)
	}
	if cmd.sessionID != "fifo-session" {
		t.Fatalf("sessionID = %q, want fifo-session", cmd.sessionID)
	}
	if cmd.watch.Command != "/bin/acp-agent" || cmd.watch.Agent != "custom" || cmd.watch.Cwd != "/tmp/project" {
		t.Fatalf("watch options = %#v", cmd.watch)
	}
	if got, want := strings.Join(cmd.watch.Args, ","), "--stdio,--profile=work"; got != want {
		t.Fatalf("watch args = %q, want %q", got, want)
	}
	if cmd.watch.Timeout != 5*time.Second || cmd.watch.Interval != 100*time.Millisecond ||
		cmd.watch.MaxPerCycle != 2 || cmd.watch.MaxCycles != 1 ||
		!cmd.watch.StopWhenEmpty || !cmd.watch.JSON {
		t.Fatalf("watch options = %#v", cmd.watch)
	}
}

func TestParseACPClientWatchRejectsInvalidArgs(t *testing.T) {
	tests := [][]string{
		{"watch"},
		{"watch", "fifo-session", "--interval", "0s"},
		{"watch", "fifo-session", "--max-per-cycle", "0"},
		{"watch", "fifo-session", "--max-cycles", "0"},
		{"watch", "fifo-session", "--command", "agent", "extra"},
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

func TestExecuteACPClientExecUsesTrustedProfile(t *testing.T) {
	setupDefaultACPProfile(t, "fixture", true)
	runner := &fakeACPClientExecRunner{
		result: acpclient.Result{SessionID: "s1", StopReason: acpsdk.StopReasonEndTurn, Text: "profile ok"},
	}
	var out bytes.Buffer
	err := executeACPClientExec(t.Context(), acpClientExecOptions{
		Agent:  "fixture",
		Prompt: "hello",
	}, runner, &out)
	if err != nil {
		t.Fatalf("executeACPClientExec: %v", err)
	}
	if runner.spec.Command != "/tmp/fixture-acp" || strings.Join(runner.spec.Args, ",") != "--stdio" {
		t.Fatalf("runner spec = %#v", runner.spec)
	}
}

func TestExecuteACPClientExecRejectsUntrustedProfile(t *testing.T) {
	setupDefaultACPProfile(t, "fixture", false)
	err := executeACPClientExec(t.Context(), acpClientExecOptions{Agent: "fixture", Prompt: "hello"}, &fakeACPClientExecRunner{}, io.Discard)
	if !errors.Is(err, acpclient.ErrUnknownAgent) {
		t.Fatalf("executeACPClientExec error = %v, want ErrUnknownAgent", err)
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

func TestExecuteACPClientDrainUsesTrustedProfile(t *testing.T) {
	setupDefaultACPProfile(t, "fixture", true)
	store := acpclient.NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 23, 10, 0, 0, time.UTC)
	if err := store.Upsert(acpclient.SessionRecord{
		ID:        "s-drain-profile",
		Status:    acpclient.SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		PromptQueue: []acpclient.QueuedPrompt{{
			ID: "q-1", Prompt: "drain profile", Status: acpclient.QueuePromptStatusPending, CreatedAt: now,
		}},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	var gotSpec acpclient.AgentSpec
	runner := &fakeDrainPromptRunner{sessionID: "acp-drain-profile"}
	if err := executeACPClientDrain(t.Context(), store, "s-drain-profile", acpClientDrainOptions{
		Agent: "fixture",
	}, func(_ context.Context, spec acpclient.AgentSpec, _ acpclient.RunOptions, _ string) (acpclient.DrainPromptRunner, func() error, error) {
		gotSpec = spec
		return runner, func() error { return nil }, nil
	}, io.Discard); err != nil {
		t.Fatalf("executeACPClientDrain: %v", err)
	}
	if gotSpec.Command != "/tmp/fixture-acp" {
		t.Fatalf("drain spec = %#v", gotSpec)
	}
}

func TestExecuteACPClientWatchUsesInjectedRunnerWithoutPromptLeak(t *testing.T) {
	store := acpclient.NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 2, 13, 0, 0, 0, time.UTC)
	const secretPrompt = "watch secret prompt"
	if err := store.Upsert(acpclient.SessionRecord{
		ID:        "s-watch",
		Status:    acpclient.SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		PromptQueue: []acpclient.QueuedPrompt{{
			ID: "q-1", Prompt: secretPrompt, Status: acpclient.QueuePromptStatusPending, CreatedAt: now,
		}},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	runner := &fakeDrainPromptRunner{sessionID: "acp-watch"}
	var out bytes.Buffer

	if err := executeACPClientWatch(t.Context(), store, "s-watch", acpClientWatchOptions{
		Command:       "/bin/fixture-agent",
		Cwd:           ".",
		Timeout:       time.Second,
		Interval:      time.Millisecond,
		MaxPerCycle:   1,
		MaxCycles:     2,
		StopWhenEmpty: true,
	}, func(context.Context, acpclient.AgentSpec, acpclient.RunOptions, string) (acpclient.DrainPromptRunner, func() error, error) {
		return runner, func() error { return nil }, nil
	}, &out); err != nil {
		t.Fatalf("executeACPClientWatch: %v", err)
	}
	if got := strings.Join(runner.prompts, ","); got != secretPrompt {
		t.Fatalf("runner prompts = %q, want secret prompt executed", got)
	}
	got := out.String()
	if !strings.Contains(got, "watch cycle 1") || !strings.Contains(got, "completed: 1") || !strings.Contains(got, "remaining: 0") {
		t.Fatalf("watch output = %q", got)
	}
	if strings.Contains(got, secretPrompt) {
		t.Fatalf("watch output leaked prompt body: %q", got)
	}
}

func TestExecuteACPClientWatchUsesTrustedProfile(t *testing.T) {
	setupDefaultACPProfile(t, "fixture", true)
	store := acpclient.NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 2, 13, 0, 0, 0, time.UTC)
	if err := store.Upsert(acpclient.SessionRecord{
		ID:        "s-watch-profile",
		Status:    acpclient.SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		PromptQueue: []acpclient.QueuedPrompt{{
			ID: "q-1", Prompt: "watch profile", Status: acpclient.QueuePromptStatusPending, CreatedAt: now,
		}},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	var gotSpec acpclient.AgentSpec
	runner := &fakeDrainPromptRunner{sessionID: "acp-watch-profile"}
	if err := executeACPClientWatch(t.Context(), store, "s-watch-profile", acpClientWatchOptions{
		Agent:         "fixture",
		Interval:      time.Millisecond,
		MaxPerCycle:   1,
		MaxCycles:     1,
		StopWhenEmpty: true,
	}, func(_ context.Context, spec acpclient.AgentSpec, _ acpclient.RunOptions, _ string) (acpclient.DrainPromptRunner, func() error, error) {
		gotSpec = spec
		return runner, func() error { return nil }, nil
	}, io.Discard); err != nil {
		t.Fatalf("executeACPClientWatch: %v", err)
	}
	if gotSpec.Command != "/tmp/fixture-acp" {
		t.Fatalf("watch spec = %#v", gotSpec)
	}
}

func TestExecuteACPClientWatchJSONOutputIsAggregateOnly(t *testing.T) {
	store := acpclient.NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 2, 13, 5, 0, 0, time.UTC)
	const secretPrompt = "json watch secret"
	if err := store.Upsert(acpclient.SessionRecord{
		ID:        "s-watch-json",
		Status:    acpclient.SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		PromptQueue: []acpclient.QueuedPrompt{{
			ID: "q-1", Prompt: secretPrompt, Status: acpclient.QueuePromptStatusPending, CreatedAt: now,
		}},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	runner := &fakeDrainPromptRunner{sessionID: "acp-watch-json"}
	var out bytes.Buffer

	if err := executeACPClientWatch(t.Context(), store, "s-watch-json", acpClientWatchOptions{
		Command:       "/bin/fixture-agent",
		Cwd:           ".",
		Timeout:       time.Second,
		Interval:      time.Millisecond,
		MaxPerCycle:   1,
		MaxCycles:     2,
		StopWhenEmpty: true,
		JSON:          true,
	}, func(context.Context, acpclient.AgentSpec, acpclient.RunOptions, string) (acpclient.DrainPromptRunner, func() error, error) {
		return runner, func() error { return nil }, nil
	}, &out); err != nil {
		t.Fatalf("executeACPClientWatch: %v", err)
	}
	if got := out.String(); strings.Contains(got, secretPrompt) {
		t.Fatalf("json output leaked prompt body: %q", got)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("json output lines = %#v, want drain and idle cycle", lines)
	}
	var first struct {
		SessionID string `json:"session_id"`
		Cycle     int    `json:"cycle"`
		Completed int    `json:"completed"`
		Remaining int    `json:"remaining"`
		Idle      bool   `json:"idle"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("first cycle json: %v\n%s", err, lines[0])
	}
	if first.SessionID != "s-watch-json" || first.Cycle != 1 || first.Completed != 1 || first.Remaining != 0 || first.Idle {
		t.Fatalf("first cycle = %#v", first)
	}
	var second struct {
		Idle bool `json:"idle"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("second cycle json: %v\n%s", err, lines[1])
	}
	if !second.Idle {
		t.Fatalf("second cycle = %#v, want idle", second)
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

func TestExecuteACPClientArchiveRawExportAndEvents(t *testing.T) {
	store := acpclient.NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 2, 9, 15, 0, 0, time.UTC)
	if err := store.Upsert(acpclient.SessionRecord{
		ID:        "s-raw",
		Agent:     "fixture",
		Cwd:       "/tmp/source",
		Status:    acpclient.SessionStatusCompleted,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	var out bytes.Buffer
	err := executeACPClientSessionExport(store, "s-raw", acpClientArchiveOptions{
		Output:      filepath.Join(t.TempDir(), "raw.json"),
		HistoryMode: "raw",
	}, false, &out)
	if !errors.Is(err, acpclient.ErrRawHistoryUnavailable) {
		t.Fatalf("raw export without sidecar error = %v, want ErrRawHistoryUnavailable", err)
	}

	events := []acpclient.EventLogLine{{
		Seq:       1,
		At:        now,
		Direction: acpclient.EventDirectionOutbound,
		Message:   json.RawMessage(`{"jsonrpc":"2.0","id":"prompt-1","method":"session/prompt","params":{"sessionId":"s-raw"}}`),
	}, {
		Seq:       2,
		At:        now.Add(time.Second),
		Direction: acpclient.EventDirectionInbound,
		Message:   json.RawMessage(`{"jsonrpc":"2.0","id":"prompt-1","result":{"stopReason":"end_turn"}}`),
	}}
	if err := store.AppendEventLog("s-raw", events); err != nil {
		t.Fatalf("AppendEventLog: %v", err)
	}

	rawPath := filepath.Join(t.TempDir(), "raw.json")
	var rawOut bytes.Buffer
	if err := executeACPClientSessionExport(store, "s-raw", acpClientArchiveOptions{
		Output:      rawPath,
		HistoryMode: "raw",
	}, true, &rawOut); err != nil {
		t.Fatalf("raw export with sidecar: %v", err)
	}
	var exportPayload struct {
		SessionID   string `json:"session_id"`
		Path        string `json:"path"`
		Status      string `json:"status"`
		HistoryMode string `json:"history_mode"`
	}
	if err := json.Unmarshal(rawOut.Bytes(), &exportPayload); err != nil {
		t.Fatalf("raw export json: %v\n%s", err, rawOut.String())
	}
	if exportPayload.SessionID != "s-raw" || exportPayload.Path != rawPath || exportPayload.HistoryMode != "raw" {
		t.Fatalf("raw export payload = %#v", exportPayload)
	}

	var rawArchive struct {
		History []json.RawMessage `json:"history"`
	}
	rawBytes, err := os.ReadFile(rawPath)
	if err != nil {
		t.Fatalf("read raw archive: %v", err)
	}
	if err := json.Unmarshal(rawBytes, &rawArchive); err != nil {
		t.Fatalf("raw archive json: %v\n%s", err, rawBytes)
	}
	if len(rawArchive.History) != 2 {
		t.Fatalf("raw history len = %d, want 2", len(rawArchive.History))
	}

	eventsPath := filepath.Join(t.TempDir(), "events.ndjson")
	var eventsOut bytes.Buffer
	if err := executeACPClientSessionEvents(store, "s-raw", acpClientArchiveOptions{Output: eventsPath}, true, &eventsOut); err != nil {
		t.Fatalf("events command: %v", err)
	}
	var eventsPayload struct {
		SessionID string                   `json:"session_id"`
		Path      string                   `json:"path"`
		Output    string                   `json:"output,omitempty"`
		Status    string                   `json:"status"`
		Events    []acpclient.EventLogLine `json:"events"`
	}
	if err := json.Unmarshal(eventsOut.Bytes(), &eventsPayload); err != nil {
		t.Fatalf("events json: %v\n%s", err, eventsOut.String())
	}
	if eventsPayload.SessionID != "s-raw" || eventsPayload.Status != "ok" || eventsPayload.Output != eventsPath || len(eventsPayload.Events) != 2 {
		t.Fatalf("events payload = %#v", eventsPayload)
	}
	if _, err := os.Stat(eventsPath); err != nil {
		t.Fatalf("stat copied events: %v", err)
	}
}

func TestExecuteACPClientExecPersistsResultEvents(t *testing.T) {
	store := acpclient.NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 2, 9, 20, 0, 0, time.UTC)
	runner := &fakeACPClientExecRunner{result: acpclient.Result{
		SessionID:  "exec-session",
		StopReason: acpsdk.StopReasonEndTurn,
		Text:       "done",
		Events: []acpclient.EventLogLine{{
			Seq:       1,
			At:        now,
			Direction: acpclient.EventDirectionOutbound,
			Message:   json.RawMessage(`{"jsonrpc":"2.0","id":"prompt-1","method":"session/prompt","params":{"sessionId":"exec-session"}}`),
		}},
	}}
	var out bytes.Buffer
	if err := executeACPClientExecWithStore(t.Context(), acpClientExecOptions{
		Agent:   "custom",
		Command: "/bin/fixture-agent",
		Cwd:     ".",
		Prompt:  "secret prompt",
		Timeout: time.Second,
	}, runner, store, &out); err != nil {
		t.Fatalf("executeACPClientExecWithStore: %v", err)
	}
	events, err := store.ReadEventLog("exec-session")
	if err != nil {
		t.Fatalf("ReadEventLog: %v", err)
	}
	if len(events) != 1 || events[0].Direction != acpclient.EventDirectionOutbound {
		t.Fatalf("events = %#v, want one outbound event", events)
	}
}

func TestExecuteACPClientCompareTextAndJSON(t *testing.T) {
	runner := &fakeCompareCommandRunner{
		results: map[string]acpclient.Result{
			"/bin/agent-a": {StopReason: acpsdk.StopReasonEndTurn, Text: "agent a final", Duration: 11 * time.Millisecond},
			"/bin/agent-b": {StopReason: acpsdk.StopReasonEndTurn, Text: "agent b final", Duration: 12 * time.Millisecond},
		},
	}
	var textOut bytes.Buffer
	if err := executeACPClientCompare(t.Context(), acpClientCompareOptions{
		Commands: []string{"/bin/agent-a", "/bin/agent-b"},
		Prompt:   "compare me",
		Cwd:      ".",
		Timeout:  time.Second,
	}, runner, &textOut); err != nil {
		t.Fatalf("executeACPClientCompare text: %v", err)
	}
	if got := textOut.String(); !strings.Contains(got, "AGENT") || !strings.Contains(got, "STATUS") ||
		!strings.Contains(got, "/bin/agent-a") || !strings.Contains(got, "agent a final") {
		t.Fatalf("text compare output = %q", got)
	}
	if got, want := strings.Join(runner.prompts, ","), "/bin/agent-a:compare me,/bin/agent-b:compare me"; got != want {
		t.Fatalf("runner prompts = %q, want %q", got, want)
	}

	promptFile := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(promptFile, []byte("from file"), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}
	var jsonOut bytes.Buffer
	if err := executeACPClientCompare(t.Context(), acpClientCompareOptions{
		Commands: []string{"/bin/agent-a", "/bin/agent-b"},
		File:     promptFile,
		Cwd:      ".",
		JSON:     true,
	}, runner, &jsonOut); err != nil {
		t.Fatalf("executeACPClientCompare json: %v", err)
	}
	var rows []acpclient.CompareRow
	if err := json.Unmarshal(jsonOut.Bytes(), &rows); err != nil {
		t.Fatalf("compare json: %v\n%s", err, jsonOut.String())
	}
	if len(rows) != 2 || rows[0].Status != "ok" || rows[0].Final != "agent a final" {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestExecuteACPClientCompareSaveBundle(t *testing.T) {
	runRoot := t.TempDir()
	runner := &fakeCompareCommandRunner{
		results: map[string]acpclient.Result{
			"/bin/agent-a": {
				StopReason: acpsdk.StopReasonEndTurn,
				Text:       "a final",
				Events: []acpclient.EventLogLine{{
					Seq:       1,
					At:        time.Date(2026, 7, 3, 9, 40, 0, 0, time.UTC),
					Direction: acpclient.EventDirectionOutbound,
					Message:   json.RawMessage(`{"jsonrpc":"2.0","id":"prompt-1","method":"session/prompt","params":{"sessionId":"a"}}`),
				}},
			},
			"/bin/agent-b": {StopReason: acpsdk.StopReasonEndTurn, Text: "b final"},
		},
	}
	var out bytes.Buffer
	err := executeACPClientCompare(t.Context(), acpClientCompareOptions{
		Commands: []string{"/bin/agent-a", "/bin/agent-b"},
		Cwd:      ".",
		Prompt:   "bundle prompt",
		JSON:     true,
		Save:     true,
		RunID:    "fixed-run",
		RunRoot:  runRoot,
	}, runner, &out)
	if err != nil {
		t.Fatalf("executeACPClientCompare: %v", err)
	}
	var payload struct {
		RunID  string                 `json:"run_id"`
		RunDir string                 `json:"run_dir"`
		Status string                 `json:"status"`
		Rows   []acpclient.CompareRow `json:"rows"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("compare save json: %v\n%s", err, out.String())
	}
	if payload.RunID != "fixed-run" || payload.Status != "completed" || len(payload.Rows) != 2 {
		t.Fatalf("payload = %#v", payload)
	}
	if _, err := os.Stat(filepath.Join(payload.RunDir, "compare.json")); err != nil {
		t.Fatalf("stat compare.json: %v", err)
	}
	eventsFiles, err := filepath.Glob(filepath.Join(payload.RunDir, "agents", "*", "events.ndjson"))
	if err != nil {
		t.Fatalf("glob events: %v", err)
	}
	if len(eventsFiles) != 2 {
		t.Fatalf("events files = %#v, want per-agent event files", eventsFiles)
	}
}

func TestExecuteACPClientCompareUsesTrustedProfile(t *testing.T) {
	setupDefaultACPProfile(t, "fixture", true)
	runner := &fakeCompareCommandRunner{
		results: map[string]acpclient.Result{
			"/tmp/fixture-acp": {StopReason: acpsdk.StopReasonEndTurn, Text: "profile final"},
			"/bin/agent-b":     {StopReason: acpsdk.StopReasonEndTurn, Text: "b final"},
		},
	}
	if err := executeACPClientCompare(t.Context(), acpClientCompareOptions{
		Agents:   []string{"fixture"},
		Commands: []string{"/bin/agent-b"},
		Prompt:   "compare profile",
	}, runner, io.Discard); err != nil {
		t.Fatalf("executeACPClientCompare: %v", err)
	}
	if len(runner.prompts) == 0 || !strings.HasPrefix(runner.prompts[0], "/tmp/fixture-acp:") {
		t.Fatalf("runner prompts = %#v", runner.prompts)
	}
}

func TestExecuteACPClientCompareTextIncludesErrorDetails(t *testing.T) {
	runner := &fakeCompareCommandRunner{
		errs: map[string]error{
			"/bin/agent-b": errors.New("agent b failed"),
		},
	}
	var out bytes.Buffer
	if err := executeACPClientCompare(t.Context(), acpClientCompareOptions{
		Commands: []string{"/bin/agent-a", "/bin/agent-b"},
		Prompt:   "compare me",
		Cwd:      ".",
	}, runner, &out); err != nil {
		t.Fatalf("executeACPClientCompare: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "ERROR") || !strings.Contains(got, "agent b failed") {
		t.Fatalf("compare output missing error detail:\n%s", got)
	}
}

func TestExecuteACPClientFlowRunTextAndJSON(t *testing.T) {
	flowPath := filepath.Join(t.TempDir(), "flow.json")
	if err := os.WriteFile(flowPath, []byte(`{
		"format_version": 1,
		"start_at": "package",
		"nodes": [{"id": "package", "type": "compute", "value": {"ok": true}}]
	}`), 0o600); err != nil {
		t.Fatalf("write flow: %v", err)
	}
	runRoot := t.TempDir()
	var textOut bytes.Buffer
	if err := executeACPClientFlowRun(t.Context(), acpClientFlowOptions{
		Path:      flowPath,
		InputJSON: `{"task":"x"}`,
		RunID:     "run-text",
		RunRoot:   runRoot,
		Cwd:       ".",
	}, &textOut); err != nil {
		t.Fatalf("executeACPClientFlowRun text: %v", err)
	}
	if got := textOut.String(); !strings.Contains(got, "flow run-text completed") {
		t.Fatalf("flow text output = %q", got)
	}
	if _, err := os.Stat(filepath.Join(runRoot, "run-text", "state.json")); err != nil {
		t.Fatalf("state file missing: %v", err)
	}

	var jsonOut bytes.Buffer
	if err := executeACPClientFlowRun(t.Context(), acpClientFlowOptions{
		Path:      flowPath,
		InputJSON: `{"task":"x"}`,
		RunID:     "run-json",
		RunRoot:   runRoot,
		Cwd:       ".",
		JSON:      true,
	}, &jsonOut); err != nil {
		t.Fatalf("executeACPClientFlowRun json: %v", err)
	}
	var result acpclient.FlowRunResult
	if err := json.Unmarshal(jsonOut.Bytes(), &result); err != nil {
		t.Fatalf("flow json: %v\n%s", err, jsonOut.String())
	}
	if result.RunID != "run-json" || result.Status != acpclient.FlowRunStatusCompleted || len(result.Outputs) != 1 {
		t.Fatalf("flow result = %#v", result)
	}
}

func TestExecuteACPClientFlowReplayTextAndJSON(t *testing.T) {
	runDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runDir, "trace.ndjson"), []byte("{}\n{}\n"), 0o600); err != nil {
		t.Fatalf("write trace: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(runDir, "projections"), 0o700); err != nil {
		t.Fatalf("mkdir projections: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "projections", "steps.json"), []byte(`[{"node_id":"a"},{"node_id":"b"}]`), 0o600); err != nil {
		t.Fatalf("write steps: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(runDir, "sessions", "main"), 0o700); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "sessions", "main", "events.ndjson"), nil, 0o600); err != nil {
		t.Fatalf("write events: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "manifest.json"), []byte(`{
		"schema":"acpx.flow-run-bundle.v1",
		"run_id":"run-replay",
		"status":"completed",
		"trace":"trace.ndjson",
		"projections":{"steps":"projections/steps.json"},
		"sessions":[{"handle":"main","events":"sessions/main/events.ndjson"}]
	}`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	var textOut bytes.Buffer
	if err := executeACPClientFlowReplay(acpClientFlowOptions{Path: runDir}, &textOut); err != nil {
		t.Fatalf("executeACPClientFlowReplay text: %v", err)
	}
	if got := textOut.String(); !strings.Contains(got, "flow run-replay completed") ||
		!strings.Contains(got, "steps: 2") || !strings.Contains(got, "trace events: 2") ||
		!strings.Contains(got, "sessions: 1") {
		t.Fatalf("flow replay text = %q", got)
	}
	var jsonOut bytes.Buffer
	if err := executeACPClientFlowReplay(acpClientFlowOptions{Path: runDir, JSON: true}, &jsonOut); err != nil {
		t.Fatalf("executeACPClientFlowReplay json: %v", err)
	}
	var summary acpclient.FlowReplaySummary
	if err := json.Unmarshal(jsonOut.Bytes(), &summary); err != nil {
		t.Fatalf("flow replay json: %v\n%s", err, jsonOut.String())
	}
	if summary.RunID != "run-replay" || summary.Status != acpclient.FlowRunStatusCompleted ||
		summary.ManifestPath != "manifest.json" || summary.StepCount != 2 || summary.TraceCount != 2 || summary.SessionCount != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestExecuteACPClientFlowRunRequiresAllowForAction(t *testing.T) {
	flowPath := filepath.Join(t.TempDir(), "flow.json")
	if err := os.WriteFile(flowPath, []byte(`{
		"format_version": 1,
		"start_at": "prepare",
		"nodes": [{"id": "prepare", "type": "action", "command": "ratchet"}]
	}`), 0o600); err != nil {
		t.Fatalf("write flow: %v", err)
	}

	var out bytes.Buffer
	err := executeACPClientFlowRun(t.Context(), acpClientFlowOptions{
		Path: flowPath,
		Cwd:  ".",
	}, &out)
	if err == nil || !strings.Contains(err.Error(), "flow requires permission shell") {
		t.Fatalf("executeACPClientFlowRun error = %v, want missing shell permission", err)
	}
}

func TestExecuteACPClientFlowRunActionJSONAndHumanSummary(t *testing.T) {
	helper, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	flowPath := filepath.Join(t.TempDir(), "flow.json")
	flow := map[string]any{
		"format_version": 1,
		"start_at":       "prepare",
		"nodes": []map[string]any{{
			"id":      "prepare",
			"type":    "action",
			"command": helper,
			"args":    []string{"-test.run=TestACPClientFlowActionHelperProcess", "--"},
			"env":     map[string]string{"RATCHET_FLOW_ACTION_HELPER": "1"},
		}},
	}
	b, err := json.Marshal(flow)
	if err != nil {
		t.Fatalf("marshal flow: %v", err)
	}
	if err := os.WriteFile(flowPath, b, 0o600); err != nil {
		t.Fatalf("write flow: %v", err)
	}

	var jsonOut bytes.Buffer
	if err := executeACPClientFlowRun(t.Context(), acpClientFlowOptions{
		Path:               flowPath,
		Cwd:                ".",
		AllowedPermissions: []string{"shell"},
		JSON:               true,
	}, &jsonOut); err != nil {
		t.Fatalf("executeACPClientFlowRun json: %v", err)
	}
	var result acpclient.FlowRunResult
	if err := json.Unmarshal(jsonOut.Bytes(), &result); err != nil {
		t.Fatalf("flow json: %v\n%s", err, jsonOut.String())
	}
	var actionOutput struct {
		Stdout string `json:"stdout"`
		Stderr string `json:"stderr"`
	}
	if err := json.Unmarshal(result.Outputs["prepare"], &actionOutput); err != nil {
		t.Fatalf("action output json: %v\n%s", err, result.Outputs["prepare"])
	}
	if !strings.Contains(actionOutput.Stdout, "action stdout") || !strings.Contains(actionOutput.Stderr, "action stderr") {
		t.Fatalf("action output = %#v", actionOutput)
	}

	var textOut bytes.Buffer
	if err := executeACPClientFlowRun(t.Context(), acpClientFlowOptions{
		Path:               flowPath,
		Cwd:                ".",
		AllowedPermissions: []string{"shell"},
		RunID:              "run-human-action",
	}, &textOut); err != nil {
		t.Fatalf("executeACPClientFlowRun text: %v", err)
	}
	if got := textOut.String(); strings.Contains(got, "action stdout") || !strings.Contains(got, "flow run-human-action completed") {
		t.Fatalf("human flow output = %q", got)
	}
}

func TestACPClientFlowActionHelperProcess(t *testing.T) {
	if os.Getenv("RATCHET_FLOW_ACTION_HELPER") != "1" {
		return
	}
	fmt.Fprint(os.Stdout, "action stdout")
	fmt.Fprint(os.Stderr, "action stderr")
	os.Exit(0)
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

type fakeCompareCommandRunner struct {
	results map[string]acpclient.Result
	errs    map[string]error
	prompts []string
}

func setupDefaultACPProfile(t *testing.T, name string, trusted bool) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	store, err := acpclient.NewDefaultProfileStore()
	if err != nil {
		t.Fatalf("NewDefaultProfileStore: %v", err)
	}
	if err := store.Add(acpclient.Profile{
		Name:    name,
		Spec:    acpclient.AgentSpec{Name: name, Command: "/tmp/" + name + "-acp", Args: []string{"--stdio"}},
		Trusted: trusted,
	}); err != nil {
		t.Fatalf("Add profile: %v", err)
	}
}

func (r *fakeCompareCommandRunner) RunPrompt(_ context.Context, spec acpclient.AgentSpec, _ acpclient.RunOptions, prompt string) (acpclient.Result, error) {
	r.prompts = append(r.prompts, spec.Command+":"+prompt)
	if r.errs != nil {
		if err := r.errs[spec.Command]; err != nil {
			return acpclient.Result{}, err
		}
	}
	if r.results != nil {
		if result, ok := r.results[spec.Command]; ok {
			return result, nil
		}
	}
	return acpclient.Result{}, nil
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
