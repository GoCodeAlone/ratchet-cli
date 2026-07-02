package acpclient

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
)

func TestLoadFlowDefinitionValidatesSchemaAndEdges(t *testing.T) {
	path := writeFlowDefinitionFixture(t, `{
		"format_version": 1,
		"name": "media summary",
		"start_at": "summarize",
		"nodes": [
			{"id": "summarize", "type": "acp", "prompt": "Summarize {{.Input.title}}", "session": "main"},
			{"id": "package", "type": "compute", "value": {"kind": "done"}}
		],
		"edges": [{"from": "summarize", "to": "package"}]
	}`)

	def, err := LoadFlowDefinition(path)
	if err != nil {
		t.Fatalf("LoadFlowDefinition: %v", err)
	}
	if def.FormatVersion != 1 || def.StartAt != "summarize" || len(def.Nodes) != 2 || len(def.Edges) != 1 {
		t.Fatalf("definition = %#v", def)
	}
}

func TestLoadFlowDefinitionRejectsInvalidGraphs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "missing format", raw: `{"start_at":"a","nodes":[{"id":"a","type":"compute","value":{}}]}`},
		{name: "missing start", raw: `{"format_version":1,"nodes":[{"id":"a","type":"compute","value":{}}]}`},
		{name: "duplicate node", raw: `{"format_version":1,"start_at":"a","nodes":[{"id":"a","type":"compute","value":{}},{"id":"a","type":"compute","value":{}}]}`},
		{name: "unknown type", raw: `{"format_version":1,"start_at":"a","nodes":[{"id":"a","type":"shell","value":{}}]}`},
		{name: "missing edge endpoint", raw: `{"format_version":1,"start_at":"a","nodes":[{"id":"a","type":"compute","value":{}}],"edges":[{"from":"a","to":"b"}]}`},
		{name: "cycle", raw: `{"format_version":1,"start_at":"a","nodes":[{"id":"a","type":"compute","value":{}},{"id":"b","type":"compute","value":{}}],"edges":[{"from":"a","to":"b"},{"from":"b","to":"a"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFlowDefinition(writeFlowDefinitionFixture(t, tt.raw))
			if !errors.Is(err, ErrInvalidFlowDefinition) {
				t.Fatalf("LoadFlowDefinition error = %v, want ErrInvalidFlowDefinition", err)
			}
		})
	}
}

func TestFlowRunStoreWritesBundleFiles(t *testing.T) {
	root := t.TempDir()
	def := FlowDefinition{FormatVersion: 1, StartAt: "a", Nodes: []FlowNode{{ID: "a", Type: FlowNodeTypeCompute, Value: json.RawMessage(`{"ok":true}`)}}}
	input := map[string]any{"task": "x"}
	state := FlowRunState{
		RunID:  "run-fixed",
		Status: FlowRunStatusCompleted,
		Steps:  []FlowStepRecord{{NodeID: "a", Status: FlowStepStatusCompleted, Output: json.RawMessage(`{"ok":true}`)}},
	}

	store, err := NewFlowRunStore(root, "run-fixed")
	if err != nil {
		t.Fatalf("NewFlowRunStore: %v", err)
	}
	if err := store.WriteDefinition(def); err != nil {
		t.Fatalf("WriteDefinition: %v", err)
	}
	if err := store.WriteInput(input); err != nil {
		t.Fatalf("WriteInput: %v", err)
	}
	if err := store.WriteState(state); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	if err := store.WriteStep("a", json.RawMessage(`{"ok":true}`)); err != nil {
		t.Fatalf("WriteStep: %v", err)
	}
	for _, rel := range []string{"flow.json", "input.json", "state.json", filepath.Join("steps", "a.json")} {
		if _, err := os.Stat(filepath.Join(root, "run-fixed", rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
}

func TestRunFlowRendersACPNodeAndStoresComputeOutput(t *testing.T) {
	root := t.TempDir()
	runner := &fakeFlowPromptRunner{sessionID: "acp-main"}
	def := FlowDefinition{
		FormatVersion: 1,
		StartAt:       "ask",
		Nodes: []FlowNode{
			{ID: "ask", Type: FlowNodeTypeACP, Prompt: "Summarize {{.Input.title}}", Session: "main", Command: "fixture"},
			{ID: "package", Type: FlowNodeTypeCompute, Value: json.RawMessage(`{"kind":"done"}`)},
		},
		Edges: []FlowEdge{{From: "ask", To: "package"}},
	}

	result, err := RunFlow(t.Context(), def, map[string]any{"title": "Video"}, FlowRunOptions{
		RunID:   "run-flow",
		RunRoot: root,
		StartRunner: func(_ context.Context, _ AgentSpec, _ RunOptions, existingID string) (FlowPromptRunner, func() error, error) {
			if existingID != "" {
				t.Fatalf("existingID = %q, want empty", existingID)
			}
			return runner, func() error { runner.closed = true; return nil }, nil
		},
	})
	if err != nil {
		t.Fatalf("RunFlow: %v", err)
	}
	if result.Status != FlowRunStatusCompleted || result.RunID != "run-flow" {
		t.Fatalf("result = %#v", result)
	}
	if got, want := strings.Join(runner.prompts, ","), "Summarize Video"; got != want {
		t.Fatalf("prompts = %q, want %q", got, want)
	}
	if !runner.closed {
		t.Fatal("runner close was not called")
	}
	if string(result.Outputs["package"]) != `{"kind":"done"}` {
		t.Fatalf("package output = %s", result.Outputs["package"])
	}
	if _, err := os.Stat(filepath.Join(root, "run-flow", "state.json")); err != nil {
		t.Fatalf("state file missing: %v", err)
	}
}

func TestRunFlowReusesACPRunnerForSharedSessionHandle(t *testing.T) {
	runner := &fakeFlowPromptRunner{sessionID: "acp-shared"}
	starts := 0
	def := FlowDefinition{
		FormatVersion: 1,
		StartAt:       "first",
		Nodes: []FlowNode{
			{ID: "first", Type: FlowNodeTypeACP, Prompt: "first", Session: "shared", Command: "fixture"},
			{ID: "second", Type: FlowNodeTypeACP, Prompt: "second {{.Outputs.first.text}}", Session: "shared", Command: "fixture"},
		},
		Edges: []FlowEdge{{From: "first", To: "second"}},
	}

	result, err := RunFlow(t.Context(), def, map[string]any{}, FlowRunOptions{
		StartRunner: func(context.Context, AgentSpec, RunOptions, string) (FlowPromptRunner, func() error, error) {
			starts++
			return runner, func() error { return nil }, nil
		},
	})
	if err != nil {
		t.Fatalf("RunFlow: %v", err)
	}
	if starts != 1 {
		t.Fatalf("starts = %d, want 1", starts)
	}
	if got, want := strings.Join(runner.prompts, ","), "first,second flow: first"; got != want {
		t.Fatalf("prompts = %q, want %q", got, want)
	}
	if result.Status != FlowRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunFlowPersistsFailedState(t *testing.T) {
	root := t.TempDir()
	def := FlowDefinition{
		FormatVersion: 1,
		StartAt:       "fail",
		Nodes:         []FlowNode{{ID: "fail", Type: FlowNodeTypeACP, Prompt: "fail", Command: "fixture"}},
	}
	_, err := RunFlow(t.Context(), def, map[string]any{}, FlowRunOptions{
		RunID:   "run-failed",
		RunRoot: root,
		StartRunner: func(context.Context, AgentSpec, RunOptions, string) (FlowPromptRunner, func() error, error) {
			return &fakeFlowPromptRunner{err: errors.New("agent failed")}, func() error { return nil }, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "agent failed") {
		t.Fatalf("RunFlow error = %v, want agent failed", err)
	}
	var state FlowRunState
	b, readErr := os.ReadFile(filepath.Join(root, "run-failed", "state.json"))
	if readErr != nil {
		t.Fatalf("read state: %v", readErr)
	}
	if err := json.Unmarshal(b, &state); err != nil {
		t.Fatalf("state json: %v\n%s", err, b)
	}
	if state.Status != FlowRunStatusFailed || len(state.Steps) != 1 || state.Steps[0].Status != FlowStepStatusFailed {
		t.Fatalf("state = %#v", state)
	}
}

type fakeFlowPromptRunner struct {
	sessionID acpsdk.SessionId
	prompts   []string
	closed    bool
	err       error
}

func (r *fakeFlowPromptRunner) SessionID() acpsdk.SessionId {
	return r.sessionID
}

func (r *fakeFlowPromptRunner) Prompt(_ context.Context, prompt string) (Result, error) {
	r.prompts = append(r.prompts, prompt)
	if r.err != nil {
		return Result{}, r.err
	}
	return Result{
		SessionID:  r.sessionID,
		StopReason: acpsdk.StopReasonEndTurn,
		Text:       "flow: " + prompt,
	}, nil
}

func writeFlowDefinitionFixture(t *testing.T, raw string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flow.json")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write flow fixture: %v", err)
	}
	return path
}
