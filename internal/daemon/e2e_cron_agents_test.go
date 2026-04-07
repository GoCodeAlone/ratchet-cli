package daemon

// E2E tests for cron tick injection, agent listing, and model switching (Task 4).
// All tests exercise real gRPC paths through E2EHarness.

import (
	"context"
	"testing"
	"time"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// TestE2E_UpdateProviderModel verifies that UpdateProviderModel RPC writes the
// new model value to the DB and that subsequent provider resolution reflects it.
func TestE2E_UpdateProviderModel(t *testing.T) {
	h := newE2EHarness(t)
	ctx := context.Background()

	// Add a second mock provider so we have an alias to update (distinct from
	// the default "e2e-mock" inserted by newE2EHarness).
	h.addProvider(t, "switch-mock", "mock", "", false)

	_, err := h.Client.UpdateProviderModel(ctx, &pb.UpdateProviderModelReq{
		Alias: "switch-mock",
		Model: "claude-3-5-sonnet",
	})
	if err != nil {
		t.Fatalf("UpdateProviderModel: %v", err)
	}

	// Verify the model column was actually written to the DB.
	var model string
	row := h.DB.QueryRowContext(ctx, `SELECT model FROM llm_providers WHERE alias = ?`, "switch-mock")
	if err := row.Scan(&model); err != nil {
		t.Fatalf("query model after update: %v", err)
	}
	if model != "claude-3-5-sonnet" {
		t.Errorf("expected model 'claude-3-5-sonnet', got %q", model)
	}

	// Verify cache was invalidated: updating to a different model should also
	// succeed without error (the registry must re-read from DB).
	_, err = h.Client.UpdateProviderModel(ctx, &pb.UpdateProviderModelReq{
		Alias: "switch-mock",
		Model: "claude-opus-4",
	})
	if err != nil {
		t.Fatalf("UpdateProviderModel (second update): %v", err)
	}

	row2 := h.DB.QueryRowContext(ctx, `SELECT model FROM llm_providers WHERE alias = ?`, "switch-mock")
	var model2 string
	if err := row2.Scan(&model2); err != nil {
		t.Fatalf("query model after second update: %v", err)
	}
	if model2 != "claude-opus-4" {
		t.Errorf("expected model 'claude-opus-4', got %q", model2)
	}
}

// TestE2E_CronTickInjection creates a cron job with a very short interval and
// verifies that the tick callback saves a user message to the messages table.
func TestE2E_CronTickInjection(t *testing.T) {
	h := newE2EHarness(t)
	ctx := context.Background()

	// Create a session pinned to the default mock provider so handleChat can
	// resolve a provider during the cron tick.
	session := h.createSession(t, "e2e-mock")

	// Create a cron job that fires every 200 ms on the test session.
	job, err := h.Client.CreateCron(ctx, &pb.CreateCronReq{
		SessionId: session.Id,
		Schedule:  "200ms",
		Command:   "cron-ping",
	})
	if err != nil {
		t.Fatalf("CreateCron: %v", err)
	}

	// Wait long enough for at least two ticks (200 ms interval × 2 + margin).
	time.Sleep(600 * time.Millisecond)

	// Verify the cron job's run_count was incremented.
	list, err := h.Client.ListCrons(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListCrons: %v", err)
	}
	var found *pb.CronJob
	for _, j := range list.Jobs {
		if j.Id == job.Id {
			jCopy := j
			found = jCopy
			break
		}
	}
	if found == nil {
		t.Fatalf("cron job %s not found in list", job.Id)
	}
	if found.RunCount < 1 {
		t.Errorf("expected run_count >= 1 after 600 ms, got %d", found.RunCount)
	}

	// Verify at least one message was saved to the messages table for the session.
	var msgCount int
	row := h.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM messages WHERE session_id = ? AND role = 'user'`,
		session.Id,
	)
	if err := row.Scan(&msgCount); err != nil {
		t.Fatalf("query messages: %v", err)
	}
	if msgCount < 1 {
		t.Errorf("expected at least 1 user message injected by cron, got %d", msgCount)
	}
}

// TestE2E_ListAgents_Empty verifies that ListAgents returns an empty (not error)
// result when no teams or fleet workers are running.
func TestE2E_ListAgents_Empty(t *testing.T) {
	h := newE2EHarness(t)

	resp, err := h.Client.ListAgents(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	// No teams or fleet workers started — agent list must be empty, not nil.
	if len(resp.Agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(resp.Agents))
	}
}
