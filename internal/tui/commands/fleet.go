package commands

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// fleetCmd starts fleet execution for a plan.
func fleetCmd(args []string, c *client.Client) *Result {
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

	// Fire-and-forget: start fleet async and return immediately.
	// Status updates are streamed back via ChatEvent.FleetStatus.
	go func() {
		_, _ = c.StartFleet(context.Background(), &pb.StartFleetReq{
			PlanId:     planID,
			MaxWorkers: maxWorkers,
		})
	}()

	return &Result{Lines: []string{
		fmt.Sprintf("Starting fleet for plan %q (max workers: %d)...", planID, maxWorkers),
		"Fleet status updates will appear in the chat stream.",
	}}
}
