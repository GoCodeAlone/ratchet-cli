package mesh

import (
	"testing"
)

func TestProjectBBSharedMode(t *testing.T) {
	pbb := NewProjectBlackboard()
	teamA := pbb.TeamBB("design", "shared")
	_ = pbb.TeamBB("dev", "shared")

	// Team A writes to its namespace.
	teamA.Write("design/spec", "api", "REST API spec", "architect")

	// Team B can read team A's writes via project BB.
	if e, ok := pbb.Root().Read("design/spec", "api"); !ok {
		t.Error("team B cannot read team A's write via root")
	} else if e.Author != "architect" {
		t.Errorf("got author %q, want %q", e.Author, "architect")
	}
}

func TestProjectBBIsolatedMode(t *testing.T) {
	pbb := NewProjectBlackboard()
	teamA := pbb.TeamBB("design", "isolated")
	teamB := pbb.TeamBB("dev", "isolated")

	teamA.Write("data", "key1", "val1", "agent1")
	teamB.Write("data", "key1", "val2", "agent2")

	// Each team sees only its own data.
	if e, ok := teamA.Read("data", "key1"); !ok || e.Value != "val1" {
		t.Error("teamA should see its own value")
	}
	if e, ok := teamB.Read("data", "key1"); !ok || e.Value != "val2" {
		t.Error("teamB should see its own value")
	}
}

func TestProjectBBOrchestratorMode(t *testing.T) {
	pbb := NewProjectBlackboard()
	_ = pbb.TeamBB("dev", "shared")
	orchBB := pbb.TeamBB("oversight", "orchestrator")

	// Dev team writes.
	pbb.Root().Write("dev/code", "main.go", "package main", "coder")

	// Orchestrator can read all via root (read-only view).
	if _, ok := pbb.Root().Read("dev/code", "main.go"); !ok {
		t.Error("orchestrator cannot read dev's BB")
	}

	// Orchestrator has its own writable BB too.
	orchBB.Write("directives", "dev", "focus on tests", "director")
	if e, ok := orchBB.Read("directives", "dev"); !ok || e.Value != "focus on tests" {
		t.Error("orchestrator BB write failed")
	}
}
