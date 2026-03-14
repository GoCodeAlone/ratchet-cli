package daemon

import (
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func makePlanSteps(ids ...string) []*pb.PlanStep {
	steps := make([]*pb.PlanStep, len(ids))
	for i, id := range ids {
		steps[i] = &pb.PlanStep{Id: id, Description: "step " + id, Status: "pending"}
	}
	return steps
}

func TestPlanManager_CreateAndGet(t *testing.T) {
	pm := NewPlanManager()
	plan := pm.Create("sess1", "my goal", makePlanSteps("s1", "s2"))

	if plan.Id == "" {
		t.Fatal("expected non-empty plan ID")
	}
	if plan.SessionId != "sess1" {
		t.Errorf("session ID: got %q want %q", plan.SessionId, "sess1")
	}
	if plan.Goal != "my goal" {
		t.Errorf("goal: got %q want %q", plan.Goal, "my goal")
	}
	if plan.Status != "proposed" {
		t.Errorf("status: got %q want %q", plan.Status, "proposed")
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(plan.Steps))
	}

	got := pm.Get(plan.Id)
	if got == nil {
		t.Fatal("Get returned nil for existing plan")
	}
	if got.Id != plan.Id {
		t.Errorf("Get ID mismatch: got %q want %q", got.Id, plan.Id)
	}

	if pm.Get("nonexistent") != nil {
		t.Error("Get should return nil for nonexistent plan")
	}
}

func TestPlanManager_Approve(t *testing.T) {
	pm := NewPlanManager()
	plan := pm.Create("sess1", "goal", makePlanSteps("s1", "s2", "s3"))

	// Approve skipping s2
	if err := pm.Approve(plan.Id, []string{"s2"}); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	got := pm.Get(plan.Id)
	if got.Status != "approved" {
		t.Errorf("plan status: got %q want approved", got.Status)
	}
	for _, step := range got.Steps {
		if step.Id == "s2" {
			if step.Status != "skipped" {
				t.Errorf("step s2 status: got %q want skipped", step.Status)
			}
		} else {
			if step.Status != "pending" {
				t.Errorf("step %s status: got %q want pending", step.Id, step.Status)
			}
		}
	}

	// Approve already-approved plan should fail
	if err := pm.Approve(plan.Id, nil); err == nil {
		t.Error("expected error approving non-proposed plan")
	}

	// Approve nonexistent plan
	if err := pm.Approve("bad-id", nil); err == nil {
		t.Error("expected error approving nonexistent plan")
	}
}

func TestPlanManager_Reject(t *testing.T) {
	pm := NewPlanManager()
	plan := pm.Create("sess1", "goal", makePlanSteps("s1"))

	if err := pm.Reject(plan.Id, "needs more detail"); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	got := pm.Get(plan.Id)
	if got.Status != "rejected" {
		t.Error("expected plan status rejected")
	}
	if got.Feedback != "needs more detail" {
		t.Errorf("feedback: got %q want %q", got.Feedback, "needs more detail")
	}

	// Reject already-rejected plan should fail (state guard)
	if err := pm.Reject(plan.Id, "again"); err == nil {
		t.Error("expected error rejecting already-rejected plan")
	}

	// Reject approved plan should fail
	plan2 := pm.Create("sess1", "goal2", makePlanSteps("s1"))
	_ = pm.Approve(plan2.Id, nil)
	if err := pm.Reject(plan2.Id, ""); err == nil {
		t.Error("expected error rejecting approved plan")
	}

	// Reject nonexistent
	if err := pm.Reject("bad-id", ""); err == nil {
		t.Error("expected error rejecting nonexistent plan")
	}
}

func TestPlanManager_UpdateStep(t *testing.T) {
	pm := NewPlanManager()
	plan := pm.Create("sess1", "goal", makePlanSteps("s1", "s2", "s3"))

	// Mark plan as executing manually (simulate approval flow)
	plan.Status = "executing"

	// Update s1 to completed
	if err := pm.UpdateStep(plan.Id, "s1", "completed", ""); err != nil {
		t.Fatalf("UpdateStep s1: %v", err)
	}
	// Plan should not be completed yet
	if pm.Get(plan.Id).Status != "executing" {
		t.Error("plan should still be executing")
	}

	// Update s2 to completed
	if err := pm.UpdateStep(plan.Id, "s2", "completed", ""); err != nil {
		t.Fatalf("UpdateStep s2: %v", err)
	}

	// Update s3 to failed — all terminal now, plan should auto-complete
	if err := pm.UpdateStep(plan.Id, "s3", "failed", "some error"); err != nil {
		t.Fatalf("UpdateStep s3: %v", err)
	}
	if pm.Get(plan.Id).Status != "completed" {
		t.Error("expected plan to auto-complete when all steps terminal")
	}

	// Check error is stored
	for _, step := range pm.Get(plan.Id).Steps {
		if step.Id == "s3" {
			if step.Error != "some error" {
				t.Errorf("step error: got %q want %q", step.Error, "some error")
			}
		}
	}

	// Nonexistent plan
	if err := pm.UpdateStep("bad-id", "s1", "completed", ""); err == nil {
		t.Error("expected error updating step in nonexistent plan")
	}

	// Nonexistent step
	plan2 := pm.Create("sess2", "goal2", makePlanSteps("a"))
	if err := pm.UpdateStep(plan2.Id, "bad-step", "completed", ""); err == nil {
		t.Error("expected error updating nonexistent step")
	}
}

func TestPlanManager_ForSession(t *testing.T) {
	pm := NewPlanManager()
	pm.Create("sess1", "goal A", makePlanSteps("s1"))
	pm.Create("sess1", "goal B", makePlanSteps("s2"))
	pm.Create("sess2", "goal C", makePlanSteps("s3"))

	plans := pm.ForSession("sess1")
	if len(plans) != 2 {
		t.Errorf("ForSession sess1: got %d want 2", len(plans))
	}
	plans2 := pm.ForSession("sess2")
	if len(plans2) != 1 {
		t.Errorf("ForSession sess2: got %d want 1", len(plans2))
	}
	plans3 := pm.ForSession("sess3")
	if len(plans3) != 0 {
		t.Errorf("ForSession sess3: got %d want 0", len(plans3))
	}
}

func TestPlanManager_UpdateStep_SkipDoesNotBlock(t *testing.T) {
	pm := NewPlanManager()
	plan := pm.Create("sess1", "goal", makePlanSteps("s1", "s2"))
	plan.Status = "executing"

	// Skip s2 first (approve fails when already executing; status set manually below)
	_ = pm.Approve(plan.Id, nil)
	// Reset to executing with s2 skipped
	for _, step := range plan.Steps {
		if step.Id == "s2" {
			step.Status = "skipped"
		}
	}
	plan.Status = "executing"

	// Complete s1 → all non-skipped steps done → plan completes
	if err := pm.UpdateStep(plan.Id, "s1", "completed", ""); err != nil {
		t.Fatalf("UpdateStep: %v", err)
	}
	if pm.Get(plan.Id).Status != "completed" {
		t.Error("expected plan completed when only non-skipped step is done")
	}
}
