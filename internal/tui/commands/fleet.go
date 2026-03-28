package commands

import (
	"context"
	"fmt"
	"math"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// fleetCmd starts fleet execution for a plan.
func fleetCmd(args []string, sessionID string, c *client.Client) *Result {
	if len(args) == 0 {
		return &Result{Lines: []string{"Usage: /fleet <plan-id> [max-workers]"}}
	}
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	planID := args[0]
	maxWorkers := int32(0) // 0 = no limit (use all steps)
	if len(args) > 1 {
		n, err := strconv.Atoi(args[1])
		if err == nil && n > 0 && n <= math.MaxInt32 {
			maxWorkers = int32(n)
		}
	}

	req := &pb.StartFleetReq{
		SessionId:  sessionID,
		PlanId:     planID,
		MaxWorkers: maxWorkers,
	}
	return &Result{
		Lines: []string{
			fmt.Sprintf("Starting fleet for plan %q (max workers: %d)...", planID, maxWorkers),
			"Fleet status updates will appear in the chat stream.",
		},
		Cmd: func() tea.Msg {
			if _, err := c.StartFleet(context.Background(), req); err != nil {
				return CommandErrorMsg{Op: "StartFleet", Err: err}
			}
			return nil
		},
	}
}
