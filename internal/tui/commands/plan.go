package commands

import (
	"context"
	"fmt"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
)

// approvePlanCmd approves a proposed plan and starts execution.
func approvePlanCmd(planID string, skipSteps []string, c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	_, err := c.ApprovePlan(context.Background(), "", planID, skipSteps)
	if err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error approving plan: %v", err)}}
	}
	return &Result{Lines: []string{
		fmt.Sprintf("Plan %q approved — executing...", planID),
	}}
}

// rejectPlanCmd rejects a proposed plan with optional feedback.
func rejectPlanCmd(planID, feedback string, c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	if err := c.RejectPlan(context.Background(), "", planID, feedback); err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error rejecting plan: %v", err)}}
	}
	return &Result{Lines: []string{
		fmt.Sprintf("Plan %q rejected.", planID),
	}}
}
