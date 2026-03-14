package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

func makePlan(id, goal string, stepIDs ...string) *pb.Plan {
	steps := make([]*pb.PlanStep, len(stepIDs))
	for i, sid := range stepIDs {
		steps[i] = &pb.PlanStep{Id: sid, Description: "step " + sid, Status: "pending"}
	}
	return &pb.Plan{Id: id, Goal: goal, Steps: steps, Status: "proposed"}
}

func planKey(ch rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: ch, Text: string(ch)}
}

func TestPlanView_InitialState(t *testing.T) {
	v := NewPlanView()
	if v.Active() {
		t.Error("new PlanView should not be active")
	}
}

func TestPlanView_SetPlan(t *testing.T) {
	plan := makePlan("p1", "build something", "s1", "s2", "s3")
	v := NewPlanView().SetPlan(plan)

	if !v.Active() {
		t.Error("PlanView should be active after SetPlan")
	}
	if v.cursor != 0 {
		t.Errorf("cursor: got %d want 0", v.cursor)
	}
	if len(v.skipped) != 0 {
		t.Error("skipped map should be empty after SetPlan")
	}
}

func TestPlanView_Navigation(t *testing.T) {
	plan := makePlan("p1", "goal", "s1", "s2", "s3")
	v := NewPlanView().SetPlan(plan)

	// Navigate down with 'j'
	v, _ = v.Update(planKey('j'))
	if v.cursor != 1 {
		t.Errorf("after j: cursor=%d want 1", v.cursor)
	}
	v, _ = v.Update(planKey('j'))
	if v.cursor != 2 {
		t.Errorf("after jj: cursor=%d want 2", v.cursor)
	}
	// Can't go past last
	v, _ = v.Update(planKey('j'))
	if v.cursor != 2 {
		t.Errorf("after jjj (past end): cursor=%d want 2", v.cursor)
	}

	// Navigate up with 'k'
	v, _ = v.Update(planKey('k'))
	if v.cursor != 1 {
		t.Errorf("after k: cursor=%d want 1", v.cursor)
	}

	// Navigate with arrow keys
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if v.cursor != 2 {
		t.Errorf("after down arrow: cursor=%d want 2", v.cursor)
	}
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if v.cursor != 1 {
		t.Errorf("after up arrow: cursor=%d want 1", v.cursor)
	}
}

func TestPlanView_ToggleSkip(t *testing.T) {
	plan := makePlan("p1", "goal", "s1", "s2")
	v := NewPlanView().SetPlan(plan)

	// Toggle skip on s1 (cursor=0)
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !v.skipped["s1"] {
		t.Error("s1 should be skipped after space")
	}

	// Toggle off
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if v.skipped["s1"] {
		t.Error("s1 should be un-skipped after second space")
	}
}

func TestPlanView_Approve(t *testing.T) {
	plan := makePlan("p1", "goal", "s1", "s2", "s3")
	v := NewPlanView().SetPlan(plan)

	// Skip s2 then approve
	v, _ = v.Update(planKey('j')) // move to s2
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeySpace})

	var approveMsg PlanApproveMsg
	var gotMsg bool
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		msg := cmd()
		if am, ok := msg.(PlanApproveMsg); ok {
			approveMsg = am
			gotMsg = true
		}
	}

	if !gotMsg {
		t.Fatal("expected PlanApproveMsg from Enter key")
	}
	if approveMsg.PlanID != "p1" {
		t.Errorf("PlanID: got %q want p1", approveMsg.PlanID)
	}
	if len(approveMsg.SkipSteps) != 1 || approveMsg.SkipSteps[0] != "s2" {
		t.Errorf("SkipSteps: got %v want [s2]", approveMsg.SkipSteps)
	}
	if v.Active() {
		t.Error("PlanView should be inactive after approval")
	}
}

func TestPlanView_ApproveNoSkips(t *testing.T) {
	plan := makePlan("p1", "goal", "s1", "s2")
	v := NewPlanView().SetPlan(plan)

	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from Enter key")
	}
	msg := cmd()
	am, ok := msg.(PlanApproveMsg)
	if !ok {
		t.Fatalf("expected PlanApproveMsg, got %T", msg)
	}
	if len(am.SkipSteps) != 0 {
		t.Errorf("expected no skip steps, got %v", am.SkipSteps)
	}
	if v.Active() {
		t.Error("PlanView should be inactive after approval")
	}
}

func TestPlanView_Reject(t *testing.T) {
	plan := makePlan("p1", "goal", "s1")
	v := NewPlanView().SetPlan(plan)

	var rejectMsg PlanRejectMsg
	var gotMsg bool
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		msg := cmd()
		if rm, ok := msg.(PlanRejectMsg); ok {
			rejectMsg = rm
			gotMsg = true
		}
	}

	if !gotMsg {
		t.Fatal("expected PlanRejectMsg from Esc key")
	}
	if rejectMsg.PlanID != "p1" {
		t.Errorf("PlanID: got %q want p1", rejectMsg.PlanID)
	}
	if v.Active() {
		t.Error("PlanView should be inactive after rejection")
	}
}

func TestPlanView_InactiveIgnoresKeys(t *testing.T) {
	v := NewPlanView() // no plan set

	_, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Error("inactive PlanView should not emit commands")
	}
}

func TestPlanView_SetPlanResetsCursor(t *testing.T) {
	plan1 := makePlan("p1", "goal1", "s1", "s2", "s3")
	v := NewPlanView().SetPlan(plan1)

	// Move cursor and toggle skip
	v, _ = v.Update(planKey('j'))
	v, _ = v.Update(planKey('j'))
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeySpace})

	// Set new plan — cursor and skips should reset
	plan2 := makePlan("p2", "goal2", "a", "b")
	v = v.SetPlan(plan2)

	if v.cursor != 0 {
		t.Errorf("cursor should reset to 0, got %d", v.cursor)
	}
	if len(v.skipped) != 0 {
		t.Errorf("skipped should be empty, got %v", v.skipped)
	}
	if v.plan.Id != "p2" {
		t.Errorf("plan should be updated to p2, got %s", v.plan.Id)
	}
}

func TestPlanView_Render(t *testing.T) {
	plan := makePlan("p1", "build the feature", "s1", "s2", "s3")
	v := NewPlanView().SetPlan(plan)

	out := v.View(theme.Dark())

	if out == "" {
		t.Fatal("expected non-empty View output")
	}
	if !strings.Contains(out, "build the feature") {
		t.Errorf("expected goal 'build the feature' in output, got:\n%s", out)
	}
	for _, sid := range []string{"s1", "s2", "s3"} {
		if !strings.Contains(out, "step "+sid) {
			t.Errorf("expected step description 'step %s' in output, got:\n%s", sid, out)
		}
	}
}

func TestPlanView_StepStatusUpdate(t *testing.T) {
	plan := &pb.Plan{
		Id:   "p-status",
		Goal: "check status indicators",
		Steps: []*pb.PlanStep{
			{Id: "s1", Description: "completed step", Status: "completed"},
			{Id: "s2", Description: "failed step", Status: "failed"},
			{Id: "s3", Description: "in progress step", Status: "in_progress"},
			{Id: "s4", Description: "pending step", Status: "pending"},
		},
		Status: "executing",
	}
	v := NewPlanView().SetPlan(plan)

	out := v.View(theme.Dark())

	if !strings.Contains(out, "✓") {
		t.Errorf("expected ✓ for completed step in output:\n%s", out)
	}
	if !strings.Contains(out, "✗") {
		t.Errorf("expected ✗ for failed step in output:\n%s", out)
	}
	if !strings.Contains(out, "⟳") {
		t.Errorf("expected ⟳ for in_progress step in output:\n%s", out)
	}
}
