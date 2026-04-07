package daemon

import (
	"context"
	"testing"
	"time"
)

func TestHumanGate(t *testing.T) {
	hg := NewHumanGate()

	// Queue a request.
	reqID := hg.Request("t-1234", "architect", "REST or gRPC for the API?")

	// Check pending.
	pending := hg.Pending("")
	if len(pending) != 1 {
		t.Fatalf("got %d pending, want 1", len(pending))
	}
	if pending[0].Question != "REST or gRPC for the API?" {
		t.Errorf("got question %q", pending[0].Question)
	}

	// Respond (simulates user).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		hg.Respond(reqID, "Use REST")
	}()

	response, err := hg.Wait(ctx, reqID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if response != "Use REST" {
		t.Errorf("got response %q, want %q", response, "Use REST")
	}

	// Pending should be empty now.
	pending = hg.Pending("")
	if len(pending) != 0 {
		t.Errorf("got %d pending, want 0", len(pending))
	}
}

func TestHumanGateFilterByTeam(t *testing.T) {
	hg := NewHumanGate()
	hg.Request("team-a", "agent1", "q1")
	hg.Request("team-b", "agent2", "q2")

	a := hg.Pending("team-a")
	if len(a) != 1 {
		t.Errorf("team-a: got %d pending, want 1", len(a))
	}
	all := hg.Pending("")
	if len(all) != 2 {
		t.Errorf("all: got %d pending, want 2", len(all))
	}
}
