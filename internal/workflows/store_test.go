package workflows

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreInstallListShowRunStopResumeWorkflow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflows.json")
	file := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(file, []byte(`{
		"name":"daily-plan",
		"description":"prepare a daily plan",
		"nodes":[{"id":"start","type":"prompt","prompt":"make a plan"}],
		"edges":[]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Load(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}

	def, err := store.InstallFile(file)
	if err != nil {
		t.Fatalf("install workflow: %v", err)
	}
	if def.Name != "daily-plan" || def.Source == "" {
		t.Fatalf("definition = %#v", def)
	}
	list := store.List()
	if len(list) != 1 || list[0].Name != def.Name {
		t.Fatalf("list = %#v", list)
	}
	if _, ok := store.Get("daily-plan"); !ok {
		t.Fatal("workflow not found after install")
	}

	run, err := store.Run("daily-plan")
	if err != nil {
		t.Fatalf("run workflow: %v", err)
	}
	if run.ID == "" || run.WorkflowName != "daily-plan" || run.Status != RunStatusRunning {
		t.Fatalf("run = %#v", run)
	}
	if err := store.Stop(run.ID); err != nil {
		t.Fatalf("stop run: %v", err)
	}
	stopped, ok := store.GetRun(run.ID)
	if !ok || stopped.Status != RunStatusStopped {
		t.Fatalf("stopped run = %#v, ok=%v", stopped, ok)
	}
	resumed, err := store.Resume(run.ID)
	if err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if resumed.Status != RunStatusRunning || resumed.ParentRunID != run.ID {
		t.Fatalf("resumed run = %#v", resumed)
	}
}

func TestStorePersistsWorkflowRunRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflows.json")
	store, err := Load(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if _, err := store.Install(Definition{
		Name:  "review",
		Nodes: []Node{{ID: "start", Type: "prompt", Prompt: "review"}},
	}); err != nil {
		t.Fatalf("install: %v", err)
	}
	run, err := store.Run("review")
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	got, ok := reloaded.GetRun(run.ID)
	if !ok || got.Status != RunStatusRunning {
		t.Fatalf("run after reload = %#v, ok=%v", got, ok)
	}
}

func TestStoreRejectsExecutableWorkflowNodes(t *testing.T) {
	store, err := Load(filepath.Join(t.TempDir(), "workflows.json"))
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	for _, nodeType := range []string{"shell", "command", "javascript", "js"} {
		if _, err := store.Install(Definition{Name: "bad-" + nodeType, Nodes: []Node{{ID: "n", Type: nodeType}}}); err == nil {
			t.Fatalf("expected %s node to be rejected", nodeType)
		}
	}
}

func TestStoreNormalizesEdgeEndpoints(t *testing.T) {
	store, err := Load(filepath.Join(t.TempDir(), "workflows.json"))
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if _, err := store.Install(Definition{
		Name: "trimmed-edges",
		Nodes: []Node{
			{ID: "start", Type: "prompt", Prompt: "start"},
			{ID: "next", Type: "prompt", Prompt: "next"},
		},
		Edges: []Edge{{From: "start ", To: " next"}},
	}); err != nil {
		t.Fatalf("install workflow with padded edge endpoints: %v", err)
	}
}
