package daemon

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// PlanManager holds all plans keyed by plan ID.
type PlanManager struct {
	mu    sync.RWMutex
	plans map[string]*pb.Plan
}

func NewPlanManager() *PlanManager {
	return &PlanManager{
		plans: make(map[string]*pb.Plan),
	}
}

// Create stores a new plan and returns it.
func (pm *PlanManager) Create(sessionID, goal string, steps []*pb.PlanStep) *pb.Plan {
	plan := &pb.Plan{
		Id:        uuid.New().String(),
		SessionId: sessionID,
		Goal:      goal,
		Steps:     steps,
		Status:    "proposed",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	pm.mu.Lock()
	pm.plans[plan.Id] = plan
	pm.mu.Unlock()
	return plan
}

// Get returns a plan by ID, or nil if not found.
func (pm *PlanManager) Get(planID string) *pb.Plan {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.plans[planID]
}

// ForSession returns all plans for the given session ID.
func (pm *PlanManager) ForSession(sessionID string) []*pb.Plan {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	var out []*pb.Plan
	for _, p := range pm.plans {
		if p.SessionId == sessionID {
			out = append(out, p)
		}
	}
	return out
}

// Approve marks a plan as approved, optionally skipping specific step IDs.
// Returns an error if the plan is not found or not in proposed status.
func (pm *PlanManager) Approve(planID string, skipSteps []string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	plan, ok := pm.plans[planID]
	if !ok {
		return fmt.Errorf("plan %q not found", planID)
	}
	if plan.Status != "proposed" {
		return fmt.Errorf("plan %q is not in proposed status (current: %s)", planID, plan.Status)
	}
	skip := make(map[string]bool, len(skipSteps))
	for _, s := range skipSteps {
		skip[s] = true
	}
	for _, step := range plan.Steps {
		if skip[step.Id] {
			step.Status = "skipped"
		}
	}
	plan.Status = "approved"
	return nil
}

// Reject marks a plan as rejected.
func (pm *PlanManager) Reject(planID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	plan, ok := pm.plans[planID]
	if !ok {
		return fmt.Errorf("plan %q not found", planID)
	}
	plan.Status = "rejected"
	return nil
}

// UpdateStep updates the status (and optional error) of a step within a plan.
// If all non-skipped steps are completed or failed, the plan is marked completed.
func (pm *PlanManager) UpdateStep(planID, stepID, stepStatus, errMsg string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	plan, ok := pm.plans[planID]
	if !ok {
		return fmt.Errorf("plan %q not found", planID)
	}
	found := false
	for _, step := range plan.Steps {
		if step.Id == stepID {
			step.Status = stepStatus
			step.Error = errMsg
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("step %q not found in plan %q", stepID, planID)
	}
	// Auto-complete plan when all non-skipped steps are terminal
	allDone := true
	for _, step := range plan.Steps {
		if step.Status == "skipped" {
			continue
		}
		if step.Status != "completed" && step.Status != "failed" {
			allDone = false
			break
		}
	}
	if allDone && plan.Status == "executing" {
		plan.Status = "completed"
	}
	return nil
}

// ApprovePlan implements the ApprovePlan RPC.
func (s *Service) ApprovePlan(req *pb.ApprovePlanReq, stream pb.RatchetDaemon_ApprovePlanServer) error {
	if err := s.plans.Approve(req.PlanId, req.SkipSteps); err != nil {
		return status.Errorf(codes.InvalidArgument, "approve plan: %v", err)
	}
	plan := s.plans.Get(req.PlanId)
	if plan == nil {
		return status.Error(codes.NotFound, "plan not found after approval")
	}
	// Send the approved plan back as a plan_proposed event so the client can refresh
	return stream.Send(&pb.ChatEvent{
		Event: &pb.ChatEvent_PlanProposed{
			PlanProposed: plan,
		},
	})
}

// RejectPlan implements the RejectPlan RPC.
func (s *Service) RejectPlan(ctx context.Context, req *pb.RejectPlanReq) (*pb.Empty, error) {
	if err := s.plans.Reject(req.PlanId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "reject plan: %v", err)
	}
	return &pb.Empty{}, nil
}
