package acpclient

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	acpx "github.com/GoCodeAlone/acpx-go"
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
		{name: "unreachable node", raw: `{"format_version":1,"start_at":"a","nodes":[{"id":"a","type":"compute","value":{}},{"id":"b","type":"compute","value":{}}]}`},
		{name: "edge into start", raw: `{"format_version":1,"start_at":"a","nodes":[{"id":"a","type":"compute","value":{}},{"id":"b","type":"compute","value":{}}],"edges":[{"from":"b","to":"a"}]}`},
		{name: "ambiguous compute", raw: `{"format_version":1,"start_at":"a","nodes":[{"id":"a","type":"compute","value":{},"select":"b"}]}`},
		{name: "unsafe node id", raw: `{"format_version":1,"start_at":"../a","nodes":[{"id":"../a","type":"compute","value":{}}]}`},
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

func TestFlowExecutionOrderHonorsFanInDependencies(t *testing.T) {
	def := FlowDefinition{
		FormatVersion: 1,
		StartAt:       "a",
		Nodes: []FlowNode{
			{ID: "a", Type: FlowNodeTypeCompute, Value: json.RawMessage(`{}`)},
			{ID: "b", Type: FlowNodeTypeCompute, Value: json.RawMessage(`{}`)},
			{ID: "c", Type: FlowNodeTypeCompute, Value: json.RawMessage(`{}`)},
			{ID: "d", Type: FlowNodeTypeCompute, Value: json.RawMessage(`{}`)},
		},
		Edges: []FlowEdge{{From: "a", To: "b"}, {From: "a", To: "c"}, {From: "b", To: "d"}, {From: "c", To: "d"}},
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	order, err := flowExecutionOrder(def)
	if err != nil {
		t.Fatalf("flowExecutionOrder error = %v", err)
	}
	if got, want := strings.Join(order, ","), "a,b,c,d"; got != want {
		t.Fatalf("flowExecutionOrder = %q, want %q", got, want)
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

func TestRunFlowWritesReplayBundleFiles(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base")
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	at := time.Date(2026, 7, 3, 10, 20, 0, 0, time.UTC)
	events := []EventLogLine{{
		Seq:       1,
		At:        at,
		Direction: EventDirectionOutbound,
		Message:   json.RawMessage(`{"jsonrpc":"2.0","id":"prompt-1","method":"session/prompt","params":{"sessionId":"flow-session"}}`),
	}}
	runner := &fakeFlowPromptRunner{sessionID: "flow-session", events: events}
	action := &fakeActionRunner{result: ActionResult{
		ExitCode: 0,
		Stdout:   "prepared stdout",
		Stderr:   "prepared stderr",
		Duration: 25 * time.Millisecond,
	}}
	def := FlowDefinition{
		FormatVersion: 1,
		StartAt:       "prepare",
		Nodes: []FlowNode{
			{ID: "prepare", Type: FlowNodeTypeAction, Command: "ratchet"},
			{ID: "ask", Type: FlowNodeTypeACP, Session: "main", Prompt: "use {{.Outputs.prepare.stdout}}", Command: "fixture"},
		},
		Edges: []FlowEdge{{From: "prepare", To: "ask"}},
	}

	result, err := RunFlow(t.Context(), def, map[string]any{"task": "x"}, FlowRunOptions{
		RunID:              "run-replay",
		RunRoot:            root,
		Cwd:                base,
		AllowedPermissions: []string{"shell"},
		ActionRunner:       action,
		StartRunner: func(context.Context, AgentSpec, RunOptions, string) (FlowPromptRunner, func() error, error) {
			return runner, func() error { return nil }, nil
		},
	})
	if err != nil {
		t.Fatalf("RunFlow: %v", err)
	}
	runDir := result.RunDir
	for _, rel := range []string{
		"manifest.json",
		"trace.ndjson",
		filepath.Join("projections", "run.json"),
		filepath.Join("projections", "live.json"),
		filepath.Join("projections", "steps.json"),
		filepath.Join("sessions", "main", "events.ndjson"),
	} {
		if _, err := os.Stat(filepath.Join(runDir, rel)); err != nil {
			t.Fatalf("replay bundle missing %s: %v", rel, err)
		}
	}
	assertFlowArtifact(t, runDir, "prepared stdout")
	assertFlowArtifact(t, runDir, "prepared stderr")
	assertFlowArtifact(t, runDir, string(result.Outputs["prepare"]))
	bundle, err := acpx.LoadBundle(t.Context(), runDir)
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	if bundle.Manifest.Schema != "acpx.flow-run-bundle.v1" || bundle.Manifest.RunID != "run-replay" ||
		bundle.Manifest.Status != acpx.RunStatusComplete || bundle.Manifest.Paths.Trace != "trace.ndjson" ||
		len(bundle.Manifest.Sessions) != 1 || bundle.Manifest.Sessions[0].Handle != "main" ||
		bundle.Flow == nil || bundle.Flow.Nodes["prepare"].NodeType != acpx.NodeTypeAction {
		t.Fatalf("bundle = %#v", bundle)
	}
	traceBytes, err := os.ReadFile(filepath.Join(runDir, "trace.ndjson"))
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(traceBytes)), "\n")
	if len(lines) != 5 {
		t.Fatalf("trace lines = %d, want 5\n%s", len(lines), traceBytes)
	}
	for i, line := range lines {
		var event struct {
			Seq  int    `json:"seq"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("trace line %d json: %v\n%s", i, err, line)
		}
		if event.Seq != i+1 || event.Type == "" {
			t.Fatalf("trace event %d = %#v", i, event)
		}
	}
	summary, err := LoadFlowReplaySummary(runDir)
	if err != nil {
		t.Fatalf("LoadFlowReplaySummary: %v", err)
	}
	if summary.RunID != "run-replay" || summary.Status != FlowRunStatusCompleted ||
		summary.ManifestPath != "manifest.json" || summary.StepCount != 2 || summary.TraceCount != 5 || summary.SessionCount != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	var runProjection acpx.FlowRunState
	readFlowJSONFile(t, filepath.Join(runDir, "projections", "run.json"), &runProjection)
	if runProjection.RunID != "run-replay" || runProjection.Status != acpx.RunStatusComplete || len(runProjection.Results) != 2 {
		t.Fatalf("run projection = %#v", runProjection)
	}
}

func TestLoadFlowReplaySummaryRejectsManifestPathsOutsideRunDir(t *testing.T) {
	runDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runDir, "trace.ndjson"), nil, 0o600); err != nil {
		t.Fatalf("write trace: %v", err)
	}
	if err := writeJSONFileAtomic(filepath.Join(runDir, "manifest.json"), map[string]any{
		"schema": "acpx.flow-run-bundle.v1",
		"run_id": "escape",
		"status": FlowRunStatusCompleted,
		"trace":  "../trace.ndjson",
	}, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if _, err := LoadFlowReplaySummary(runDir); err == nil || !strings.Contains(err.Error(), "outside run dir") {
		t.Fatalf("LoadFlowReplaySummary error = %v, want outside run dir", err)
	}
}

func TestLoadFlowReplaySummaryReadsUpstreamACPXBundle(t *testing.T) {
	runDir := t.TempDir()
	writeMinimalUpstreamACPXBundle(t, runDir, "trace.ndjson")

	summary, err := LoadFlowReplaySummary(runDir)
	if err != nil {
		t.Fatalf("LoadFlowReplaySummary: %v", err)
	}
	if summary.RunID != "upstream-run" || summary.Status != FlowRunStatusCompleted ||
		summary.StepCount != 1 || summary.TraceCount != 3 || summary.SessionCount != 0 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestLoadFlowReplaySummaryUsesACPXRuntimeSummary(t *testing.T) {
	runDir := t.TempDir()
	writeMinimalUpstreamACPXBundle(t, runDir, "trace.ndjson")
	old := acpxReplaySummaryFunc
	t.Cleanup(func() { acpxReplaySummaryFunc = old })
	called := false
	acpxReplaySummaryFunc = func(ctx context.Context, root string) (FlowReplaySummary, error) {
		if ctx == nil {
			t.Fatal("summary context is nil")
		}
		called = true
		if root != runDir {
			t.Fatalf("runtime root = %q, want %q", root, runDir)
		}
		return FlowReplaySummary{
			RunID:        "runtime-run",
			Status:       FlowRunStatusCompleted,
			ManifestPath: "manifest.json",
			StepCount:    7,
			TraceCount:   11,
			SessionCount: 2,
		}, nil
	}

	summary, err := LoadFlowReplaySummary(runDir)
	if err != nil {
		t.Fatalf("LoadFlowReplaySummary: %v", err)
	}
	if !called {
		t.Fatal("ACPX runtime summary function was not called")
	}
	if summary.RunID != "runtime-run" || summary.StepCount != 7 || summary.TraceCount != 11 || summary.SessionCount != 2 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestLoadFlowReplaySummaryRejectsUpstreamEscapesBeforeACPXRuntime(t *testing.T) {
	runDir := t.TempDir()
	writeMinimalUpstreamACPXBundle(t, runDir, "../outside.ndjson")
	old := acpxReplaySummaryFunc
	t.Cleanup(func() { acpxReplaySummaryFunc = old })
	acpxReplaySummaryFunc = func(context.Context, string) (FlowReplaySummary, error) {
		t.Fatal("ACPX runtime summary function was called for an escaping manifest")
		return FlowReplaySummary{}, nil
	}

	if _, err := LoadFlowReplaySummary(runDir); err == nil || !strings.Contains(err.Error(), "outside run dir") {
		t.Fatalf("LoadFlowReplaySummary error = %v, want outside run dir", err)
	}
}

func TestLoadFlowReplaySummaryRejectsUpstreamManifestPathsOutsideRunDir(t *testing.T) {
	runDir := t.TempDir()
	writeMinimalUpstreamACPXBundle(t, runDir, "../outside.ndjson")
	if _, err := LoadFlowReplaySummary(runDir); err == nil || !strings.Contains(err.Error(), "outside run dir") {
		t.Fatalf("LoadFlowReplaySummary error = %v, want outside run dir", err)
	}
}

func TestLoadFlowReplaySummaryRejectsUpstreamSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	runDir := filepath.Join(root, "run")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatalf("mkdir run: %v", err)
	}
	writeMinimalUpstreamACPXBundle(t, runDir, "trace.ndjson")
	outside := filepath.Join(root, "outside.ndjson")
	if err := os.WriteFile(outside, []byte(upstreamACPXTraceFixture()), 0o600); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	if err := os.Remove(filepath.Join(runDir, "trace.ndjson")); err != nil {
		t.Fatalf("remove trace: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(runDir, "trace.ndjson")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := LoadFlowReplaySummary(runDir); err == nil || !strings.Contains(err.Error(), "outside run dir") {
		t.Fatalf("LoadFlowReplaySummary error = %v, want symlink outside run dir", err)
	}
}

func TestLoadFlowReplaySummaryRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	runDir := filepath.Join(root, "run")
	outside := filepath.Join(root, "outside.ndjson")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatalf("mkdir run: %v", err)
	}
	if err := os.WriteFile(outside, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(runDir, "trace.ndjson")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := writeJSONFileAtomic(filepath.Join(runDir, "manifest.json"), map[string]any{
		"schema": "acpx.flow-run-bundle.v1",
		"run_id": "escape",
		"status": FlowRunStatusCompleted,
		"trace":  "trace.ndjson",
	}, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if _, err := LoadFlowReplaySummary(runDir); err == nil || !strings.Contains(err.Error(), "outside run dir") {
		t.Fatalf("LoadFlowReplaySummary error = %v, want symlink outside run dir", err)
	}
}

func writeMinimalUpstreamACPXBundle(t *testing.T, runDir, tracePath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(runDir, "projections"), 0o700); err != nil {
		t.Fatalf("mkdir projections: %v", err)
	}
	if err := writeJSONFileAtomic(filepath.Join(runDir, "manifest.json"), map[string]any{
		"schema":      "acpx.flow-run-bundle.v1",
		"runId":       "upstream-run",
		"flowName":    "upstream-flow",
		"startedAt":   "2026-07-04T00:00:00Z",
		"finishedAt":  "2026-07-04T00:00:01Z",
		"status":      "completed",
		"traceSchema": "acpx.flow-trace-event.v1",
		"paths": map[string]string{
			"flow":            "flow.json",
			"trace":           tracePath,
			"runProjection":   "projections/run.json",
			"liveProjection":  "projections/live.json",
			"stepsProjection": "projections/steps.json",
			"sessionsDir":     "sessions",
			"artifactsDir":    "artifacts",
		},
		"sessions": []any{},
	}, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := writeJSONFileAtomic(filepath.Join(runDir, "flow.json"), map[string]any{
		"schema":  "acpx.flow-definition-snapshot.v1",
		"name":    "upstream-flow",
		"startAt": "only",
		"nodes": map[string]any{
			"only": map[string]any{"nodeType": "compute", "hasRun": true},
		},
		"edges": []any{},
	}, 0o600); err != nil {
		t.Fatalf("write flow: %v", err)
	}
	if tracePath == "trace.ndjson" {
		if err := os.WriteFile(filepath.Join(runDir, "trace.ndjson"), []byte(upstreamACPXTraceFixture()), 0o600); err != nil {
			t.Fatalf("write trace: %v", err)
		}
	}
}

func upstreamACPXTraceFixture() string {
	return strings.Join([]string{
		`{"seq":1,"at":"2026-07-04T00:00:00Z","scope":"run","type":"run_started","runId":"upstream-run","payload":{}}`,
		`{"seq":2,"at":"2026-07-04T00:00:00Z","scope":"node","type":"node_started","runId":"upstream-run","nodeId":"only","attemptId":"only#1","payload":{}}`,
		`{"seq":3,"at":"2026-07-04T00:00:01Z","scope":"node","type":"node_outcome","runId":"upstream-run","nodeId":"only","attemptId":"only#1","payload":{}}`,
	}, "\n") + "\n"
}

func TestFlowRunStoreRejectsUnsafeRunAndStepIDs(t *testing.T) {
	if _, err := NewFlowRunStore(t.TempDir(), "../escape"); err == nil {
		t.Fatal("NewFlowRunStore accepted unsafe run id")
	}
	store, err := NewFlowRunStore(t.TempDir(), "safe")
	if err != nil {
		t.Fatalf("NewFlowRunStore safe: %v", err)
	}
	if err := store.WriteStep("../escape", json.RawMessage(`{}`)); err == nil {
		t.Fatal("WriteStep accepted unsafe node id")
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

func TestRunFlowResolvesTrustedProfileAgent(t *testing.T) {
	root := t.TempDir()
	runner := &fakeFlowPromptRunner{sessionID: "acp-profile"}
	reg, err := DefaultRegistry().WithProfiles([]Profile{{
		Name:    "fixture-profile",
		Spec:    AgentSpec{Name: "fixture-profile", Command: "/tmp/fixture-acp", Args: []string{"--stdio"}},
		Trusted: true,
	}})
	if err != nil {
		t.Fatalf("WithProfiles: %v", err)
	}
	def := FlowDefinition{
		FormatVersion: 1,
		StartAt:       "ask",
		Nodes: []FlowNode{
			{ID: "ask", Type: FlowNodeTypeACP, Agent: "fixture-profile", Prompt: "Hello"},
		},
	}

	var gotSpec AgentSpec
	_, err = RunFlow(t.Context(), def, map[string]any{}, FlowRunOptions{
		RunID:    "run-profile",
		RunRoot:  root,
		Registry: reg,
		StartRunner: func(_ context.Context, spec AgentSpec, _ RunOptions, _ string) (FlowPromptRunner, func() error, error) {
			gotSpec = spec
			return runner, func() error { return nil }, nil
		},
	})
	if err != nil {
		t.Fatalf("RunFlow: %v", err)
	}
	if gotSpec.Command != "/tmp/fixture-acp" {
		t.Fatalf("flow spec = %#v", gotSpec)
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

func TestLoadFlowDefinitionAcceptsActionNodes(t *testing.T) {
	path := writeFlowDefinitionFixture(t, `{
		"format_version": 1,
		"start_at": "prepare",
		"nodes": [
			{
				"id": "prepare",
				"type": "action",
				"command": "ratchet",
				"args": ["version"],
				"cwd": "tools",
				"env": {"RATCHET_FLOW_TEST": "1"},
				"input": {"task": "inspect"}
			}
		]
	}`)

	def, err := LoadFlowDefinition(path)
	if err != nil {
		t.Fatalf("LoadFlowDefinition: %v", err)
	}
	node := def.Nodes[0]
	var input map[string]string
	if err := json.Unmarshal(node.Input, &input); err != nil {
		t.Fatalf("node input json: %v", err)
	}
	if node.Type != FlowNodeTypeAction || node.Command != "ratchet" || node.Cwd != "tools" ||
		node.Env["RATCHET_FLOW_TEST"] != "1" || input["task"] != "inspect" {
		t.Fatalf("action node = %#v", node)
	}
}

func TestLoadFlowDefinitionRejectsActionWithoutCommand(t *testing.T) {
	_, err := LoadFlowDefinition(writeFlowDefinitionFixture(t, `{
		"format_version": 1,
		"start_at": "prepare",
		"nodes": [{"id": "prepare", "type": "action"}]
	}`))
	if !errors.Is(err, ErrInvalidFlowDefinition) || !strings.Contains(err.Error(), "action node prepare command is required") {
		t.Fatalf("LoadFlowDefinition error = %v, want action command validation", err)
	}
}

func TestRunFlowActionRequiresShellPermissionBeforeAnyNodeRuns(t *testing.T) {
	runner := &fakeActionRunner{}
	starts := 0
	def := FlowDefinition{
		FormatVersion: 1,
		StartAt:       "prepare",
		Nodes: []FlowNode{
			{ID: "prepare", Type: FlowNodeTypeAction, Command: "ratchet", Args: []string{"version"}},
			{ID: "ask", Type: FlowNodeTypeACP, Prompt: "use {{.Outputs.prepare.stdout}}", Command: "fixture"},
		},
		Edges: []FlowEdge{{From: "prepare", To: "ask"}},
	}

	_, err := RunFlow(t.Context(), def, map[string]any{}, FlowRunOptions{
		ActionRunner: runner,
		StartRunner: func(context.Context, AgentSpec, RunOptions, string) (FlowPromptRunner, func() error, error) {
			starts++
			return &fakeFlowPromptRunner{}, nil, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "flow requires permission shell") {
		t.Fatalf("RunFlow error = %v, want missing shell permission", err)
	}
	if runner.calls != 0 || starts != 0 {
		t.Fatalf("preflight ran nodes: action calls=%d acp starts=%d", runner.calls, starts)
	}
}

func TestRunFlowOutsideCWDRequiresPermissionBeforeAnyNodeRuns(t *testing.T) {
	runner := &fakeActionRunner{}
	base := filepath.Join(t.TempDir(), "base")
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	def := FlowDefinition{
		FormatVersion: 1,
		StartAt:       "escape",
		Nodes: []FlowNode{{
			ID:      "escape",
			Type:    FlowNodeTypeAction,
			Command: "ratchet",
			Cwd:     "../outside",
		}},
	}

	_, err := RunFlow(t.Context(), def, map[string]any{}, FlowRunOptions{
		Cwd:                base,
		AllowedPermissions: []string{"shell"},
		ActionRunner:       runner,
	})
	if err == nil || !strings.Contains(err.Error(), "flow requires permission outside-cwd") {
		t.Fatalf("RunFlow error = %v, want missing outside-cwd permission", err)
	}
	if runner.calls != 0 {
		t.Fatalf("preflight ran action %d times", runner.calls)
	}
}

func TestRunFlowSymlinkedCWDOutsideBaseRequiresPermission(t *testing.T) {
	runner := &fakeActionRunner{}
	root := t.TempDir()
	base := filepath.Join(root, "base")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "tools")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	def := FlowDefinition{
		FormatVersion: 1,
		StartAt:       "escape",
		Nodes: []FlowNode{{
			ID:      "escape",
			Type:    FlowNodeTypeAction,
			Command: "ratchet",
			Cwd:     "tools",
		}},
	}

	_, err := RunFlow(t.Context(), def, map[string]any{}, FlowRunOptions{
		Cwd:                base,
		AllowedPermissions: []string{"shell"},
		ActionRunner:       runner,
	})
	if err == nil || !strings.Contains(err.Error(), "flow requires permission outside-cwd") {
		t.Fatalf("RunFlow error = %v, want missing outside-cwd permission", err)
	}
	if runner.calls != 0 {
		t.Fatalf("preflight ran action %d times", runner.calls)
	}
}

func TestRunFlowActionNodeStoresOutput(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base")
	if err := os.MkdirAll(filepath.Join(base, "tools"), 0o700); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	runner := &fakeActionRunner{
		result: ActionResult{
			ExitCode: 0,
			Stdout:   "prepared",
			Stderr:   "warning",
			Duration: 12 * time.Millisecond,
		},
	}
	def := FlowDefinition{
		FormatVersion: 1,
		StartAt:       "prepare",
		Nodes: []FlowNode{{
			ID:      "prepare",
			Type:    FlowNodeTypeAction,
			Command: "ratchet",
			Args:    []string{"version"},
			Cwd:     "tools",
			Env:     map[string]string{"RATCHET_FLOW_TEST": "1"},
			Input:   json.RawMessage(`{"task":"inspect"}`),
		}},
	}

	result, err := RunFlow(t.Context(), def, map[string]any{}, FlowRunOptions{
		RunID:              "run-action",
		RunRoot:            root,
		Cwd:                base,
		AllowedPermissions: []string{"shell"},
		ActionRunner:       runner,
	})
	if err != nil {
		t.Fatalf("RunFlow: %v", err)
	}
	if result.Status != FlowRunStatusCompleted || runner.calls != 1 {
		t.Fatalf("result=%#v action calls=%d", result, runner.calls)
	}
	if runner.last.Command != "ratchet" || strings.Join(runner.last.Args, ",") != "version" ||
		runner.last.Cwd != filepath.Join(base, "tools") || runner.last.Env["RATCHET_FLOW_TEST"] != "1" ||
		string(runner.last.Input) != `{"task":"inspect"}` {
		t.Fatalf("action opts = %#v", runner.last)
	}
	var output struct {
		ExitCode        int    `json:"exit_code"`
		Stdout          string `json:"stdout"`
		Stderr          string `json:"stderr"`
		StdoutTruncated bool   `json:"stdout_truncated"`
		StderrTruncated bool   `json:"stderr_truncated"`
		DurationMS      int64  `json:"duration_ms"`
		Cwd             string `json:"cwd"`
	}
	if err := json.Unmarshal(result.Outputs["prepare"], &output); err != nil {
		t.Fatalf("action output json: %v\n%s", err, result.Outputs["prepare"])
	}
	if output.ExitCode != 0 || output.Stdout != "prepared" || output.Stderr != "warning" ||
		output.StdoutTruncated || output.StderrTruncated || output.DurationMS != 12 || output.Cwd != filepath.Join(base, "tools") {
		t.Fatalf("action output = %#v", output)
	}
	if _, err := os.Stat(filepath.Join(root, "run-action", "steps", "prepare.json")); err != nil {
		t.Fatalf("step output missing: %v", err)
	}
}

func TestRunFlowActionNonZeroExitPersistsFailedState(t *testing.T) {
	root := t.TempDir()
	runner := &fakeActionRunner{result: ActionResult{ExitCode: 2, Stdout: "bad"}}
	def := FlowDefinition{
		FormatVersion: 1,
		StartAt:       "fail",
		Nodes:         []FlowNode{{ID: "fail", Type: FlowNodeTypeAction, Command: "ratchet"}},
	}

	_, err := RunFlow(t.Context(), def, map[string]any{}, FlowRunOptions{
		RunID:              "run-action-failed",
		RunRoot:            root,
		AllowedPermissions: []string{"shell"},
		ActionRunner:       runner,
	})
	if err == nil || !strings.Contains(err.Error(), "action fail exited with 2") {
		t.Fatalf("RunFlow error = %v, want non-zero action exit", err)
	}
	var state FlowRunState
	b, readErr := os.ReadFile(filepath.Join(root, "run-action-failed", "state.json"))
	if readErr != nil {
		t.Fatalf("read state: %v", readErr)
	}
	if err := json.Unmarshal(b, &state); err != nil {
		t.Fatalf("state json: %v\n%s", err, b)
	}
	if state.Status != FlowRunStatusFailed || len(state.Steps) != 1 ||
		state.Steps[0].Status != FlowStepStatusFailed || !strings.Contains(state.Steps[0].Error, "exited with 2") {
		t.Fatalf("state = %#v", state)
	}
}

func TestRunFlowActionStartFailurePersistsNonZeroExitCode(t *testing.T) {
	root := t.TempDir()
	def := FlowDefinition{
		FormatVersion: 1,
		StartAt:       "missing",
		Nodes:         []FlowNode{{ID: "missing", Type: FlowNodeTypeAction, Command: filepath.Join(t.TempDir(), "missing-command")}},
	}

	_, err := RunFlow(t.Context(), def, map[string]any{}, FlowRunOptions{
		RunID:              "run-action-start-failed",
		RunRoot:            root,
		AllowedPermissions: []string{"shell"},
	})
	if err == nil {
		t.Fatal("RunFlow succeeded, want start failure")
	}
	var state FlowRunState
	b, readErr := os.ReadFile(filepath.Join(root, "run-action-start-failed", "state.json"))
	if readErr != nil {
		t.Fatalf("read state: %v", readErr)
	}
	if err := json.Unmarshal(b, &state); err != nil {
		t.Fatalf("state json: %v\n%s", err, b)
	}
	var output struct {
		ExitCode int `json:"exit_code"`
	}
	if err := json.Unmarshal(state.Steps[0].Output, &output); err != nil {
		t.Fatalf("action output json: %v\n%s", err, state.Steps[0].Output)
	}
	if output.ExitCode != -1 {
		t.Fatalf("exit_code = %d, want -1", output.ExitCode)
	}
}

func TestRunFlowActionNonZeroExitWritesReplayFailedStep(t *testing.T) {
	root := t.TempDir()
	runner := &fakeActionRunner{result: ActionResult{ExitCode: 2, Stdout: "bad"}}
	def := FlowDefinition{
		FormatVersion: 1,
		StartAt:       "fail",
		Nodes:         []FlowNode{{ID: "fail", Type: FlowNodeTypeAction, Command: "ratchet"}},
	}

	_, err := RunFlow(t.Context(), def, map[string]any{}, FlowRunOptions{
		RunID:              "run-action-replay-failed",
		RunRoot:            root,
		AllowedPermissions: []string{"shell"},
		ActionRunner:       runner,
	})
	if err == nil || !strings.Contains(err.Error(), "action fail exited with 2") {
		t.Fatalf("RunFlow error = %v, want non-zero action exit", err)
	}
	runDir := filepath.Join(root, "run-action-replay-failed")
	for _, rel := range []string{
		filepath.Join("steps", "fail.json"),
		"trace.ndjson",
		"manifest.json",
		filepath.Join("projections", "steps.json"),
	} {
		if _, err := os.Stat(filepath.Join(runDir, rel)); err != nil {
			t.Fatalf("failed replay bundle missing %s: %v", rel, err)
		}
	}
	summary, err := LoadFlowReplaySummary(runDir)
	if err != nil {
		t.Fatalf("LoadFlowReplaySummary: %v", err)
	}
	if summary.Status != FlowRunStatusFailed || summary.StepCount != 1 || summary.TraceCount != 3 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestRunFlowACPFailureWritesNullReplayStep(t *testing.T) {
	root := t.TempDir()
	def := FlowDefinition{
		FormatVersion: 1,
		StartAt:       "fail",
		Nodes:         []FlowNode{{ID: "fail", Type: FlowNodeTypeACP, Prompt: "fail", Command: "fixture"}},
	}
	_, err := RunFlow(t.Context(), def, map[string]any{}, FlowRunOptions{
		RunID:   "run-acp-replay-failed",
		RunRoot: root,
		StartRunner: func(context.Context, AgentSpec, RunOptions, string) (FlowPromptRunner, func() error, error) {
			return &fakeFlowPromptRunner{err: errors.New("agent failed")}, func() error { return nil }, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "agent failed") {
		t.Fatalf("RunFlow error = %v, want agent failed", err)
	}
	stepBytes, err := os.ReadFile(filepath.Join(root, "run-acp-replay-failed", "steps", "fail.json"))
	if err != nil {
		t.Fatalf("read failed step: %v", err)
	}
	if strings.TrimSpace(string(stepBytes)) != "null" {
		t.Fatalf("failed ACP step = %s, want null", stepBytes)
	}
}

func TestRunFlowActionOutputIsTruncated(t *testing.T) {
	runner := &fakeActionRunner{
		result: ActionResult{
			ExitCode: 0,
			Stdout:   strings.Repeat("é", 8),
			Stderr:   strings.Repeat("w", 8),
		},
	}
	def := FlowDefinition{
		FormatVersion: 1,
		StartAt:       "truncate",
		Nodes:         []FlowNode{{ID: "truncate", Type: FlowNodeTypeAction, Command: "ratchet"}},
	}

	result, err := RunFlow(t.Context(), def, map[string]any{}, FlowRunOptions{
		AllowedPermissions: []string{"shell"},
		ActionRunner:       runner,
		ActionOutputLimit:  5,
	})
	if err != nil {
		t.Fatalf("RunFlow: %v", err)
	}
	var output struct {
		Stdout          string `json:"stdout"`
		Stderr          string `json:"stderr"`
		StdoutTruncated bool   `json:"stdout_truncated"`
		StderrTruncated bool   `json:"stderr_truncated"`
	}
	if err := json.Unmarshal(result.Outputs["truncate"], &output); err != nil {
		t.Fatalf("action output json: %v\n%s", err, result.Outputs["truncate"])
	}
	if output.Stdout != strings.Repeat("é", 5) || output.Stderr != "wwwww" ||
		!output.StdoutTruncated || !output.StderrTruncated {
		t.Fatalf("truncated output = %#v", output)
	}
}

type fakeFlowPromptRunner struct {
	sessionID acpsdk.SessionId
	prompts   []string
	closed    bool
	err       error
	events    []EventLogLine
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

func (r *fakeFlowPromptRunner) LastEvents() []EventLogLine {
	return r.events
}

type fakeActionRunner struct {
	calls  int
	last   ActionRunOptions
	result ActionResult
	err    error
}

func (r *fakeActionRunner) RunAction(_ context.Context, opts ActionRunOptions) (ActionResult, error) {
	r.calls++
	r.last = opts
	if r.err != nil {
		return ActionResult{}, r.err
	}
	return r.result, nil
}

func writeFlowDefinitionFixture(t *testing.T, raw string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flow.json")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write flow fixture: %v", err)
	}
	return path
}

func assertFlowArtifact(t *testing.T, runDir, content string) {
	t.Helper()
	sum := sha256.Sum256([]byte(content))
	path := filepath.Join(runDir, "artifacts", "sha256", fmt.Sprintf("%x", sum[:]))
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read artifact %s: %v", path, err)
	}
	if string(b) != content {
		t.Fatalf("artifact %s = %q, want %q", path, b, content)
	}
}

func readFlowJSONFile(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("decode %s: %v\n%s", path, err, b)
	}
}
