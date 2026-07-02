package acpclient

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
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

func writeFlowDefinitionFixture(t *testing.T, raw string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flow.json")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write flow fixture: %v", err)
	}
	return path
}
