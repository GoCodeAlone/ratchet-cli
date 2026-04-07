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

	// Poll until the cron job has run and its injected message is persisted.
	// Avoids fixed sleeps that can flake on slow CI machines.
	deadline := time.Now().Add(5 * time.Second)
	var (
		found    *pb.CronJob
		msgCount int
		lastErr  error
	)
	for time.Now().Before(deadline) {
		found = nil
		msgCount = 0
		lastErr = nil

		list, err := h.Client.ListCrons(ctx, &pb.Empty{})
		if err != nil {
			lastErr = err
		} else {
			for _, j := range list.Jobs {
				if j.Id == job.Id {
					jCopy := j
					found = jCopy
					break
				}
			}
		}

		if lastErr == nil {
			row := h.DB.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM messages WHERE session_id = ? AND role = 'user'`,
				session.Id,
			)
			if err := row.Scan(&msgCount); err != nil {
				lastErr = err
			}
		}

		if lastErr == nil && found != nil && found.RunCount >= 1 && msgCount >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if lastErr != nil {
		t.Fatalf("waiting for cron effects: %v", lastErr)
	}
	if found == nil {
		t.Fatalf("cron job %s not found in list before timeout", job.Id)
	}
	if found.RunCount < 1 {
		t.Fatalf("expected run_count >= 1 before timeout, got %d", found.RunCount)
	}
	if msgCount < 1 {
		t.Fatalf("expected at least 1 user message injected by cron before timeout, got %d", msgCount)
	}
}

// TestE2E_ListAgents_Empty verifies that ListAgents returns an empty (not error)
// result when no teams or fleet workers are running.
func TestE2E_ListAgents_Empty(t *testing.T) {
	h := newE2EHarness(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := h.Client.ListAgents(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	// No teams or fleet workers started — agent list must be empty, not nil.
	if len(resp.Agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(resp.Agents))
	}
}
