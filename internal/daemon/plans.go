package daemon

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// PlanManager holds all plans keyed by plan ID.
type PlanManager struct {
	mu          sync.RWMutex
	plans       map[string]*pb.Plan
	completedAt map[string]time.Time
	stop        chan struct{}
}

func NewPlanManager() *PlanManager {
	pm := &PlanManager{
		plans:       make(map[string]*pb.Plan),
		completedAt: make(map[string]time.Time),
		stop:        make(chan struct{}),
	}
	go pm.cleanupLoop()
	return pm
}

// cleanupLoop periodically removes plans in terminal states older than 30 minutes.
func (pm *PlanManager) cleanupLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("planManager cleanupLoop: panic: %v", r)
		}
	}()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-pm.stop:
			return
		case <-ticker.C:
			pm.purgeTerminal(30 * time.Minute)
		}
	}
}

// purgeTerminal removes plans in terminal states (approved, rejected) older than ttl.
func (pm *PlanManager) purgeTerminal(ttl time.Duration) {
	now := time.Now()
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for id, ts := range pm.completedAt {
		if now.Sub(ts) > ttl {
			delete(pm.plans, id)
			delete(pm.completedAt, id)
		}
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
	pm.completedAt[planID] = time.Now()
	return nil
}

// Reject marks a plan as rejected with optional feedback.
// Returns an error if the plan is not found or is already in a terminal state
// (approved, executing, completed, or rejected).
func (pm *PlanManager) Reject(planID, feedback string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	plan, ok := pm.plans[planID]
	if !ok {
		return fmt.Errorf("plan %q not found", planID)
	}
	switch plan.Status {
	case "approved", "executing", "completed", "rejected":
		return fmt.Errorf("plan %q cannot be rejected (current status: %s)", planID, plan.Status)
	}
	plan.Status = "rejected"
	plan.Feedback = feedback
	pm.completedAt[planID] = time.Now()
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

