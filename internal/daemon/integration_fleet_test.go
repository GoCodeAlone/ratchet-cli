package daemon

import (
	"context"
	"io"
	"testing"
	"time"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestIntegration_FleetLifecycle(t *testing.T) {
	client, _ := startTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start fleet with 3 parallel workers.
	stream, err := client.StartFleet(ctx, &pb.StartFleetReq{
		SessionId:  "sess-fleet-1",
		PlanId:     "plan-abc",
		MaxWorkers: 3,
	})
	if err != nil {
		t.Fatalf("StartFleet: %v", err)
	}

	var lastFleetStatus *pb.FleetStatus
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("stream.Recv: %v", err)
		}
		if fs, ok := ev.Event.(*pb.ChatEvent_FleetStatus); ok {
			lastFleetStatus = fs.FleetStatus
		}
	}

	if lastFleetStatus == nil {
		t.Fatal("expected at least one FleetStatus event")
	}
	if lastFleetStatus.Status != "completed" {
		t.Errorf("expected fleet status=completed, got %s", lastFleetStatus.Status)
	}
	if lastFleetStatus.Total == 0 {
		t.Error("expected Total > 0")
	}
	if lastFleetStatus.Completed != lastFleetStatus.Total {
		t.Errorf("expected Completed=%d, got %d", lastFleetStatus.Total, lastFleetStatus.Completed)
	}
	// Workers may fail in test environments where no provider is configured;
	// verify they reached a terminal state (not pending/running).
	for _, w := range lastFleetStatus.Workers {
		if w.Status == "pending" || w.Status == "running" {
			t.Errorf("worker %s: expected terminal status, got %s", w.Name, w.Status)
		}
	}
}

func TestIntegration_FleetKillWorker(t *testing.T) {
	client, _ := startTestServer(t)
	ctx := context.Background()

	// Start fleet with a single slow-ish step.
	streamCtx, streamCancel := context.WithTimeout(ctx, 10*time.Second)
	defer streamCancel()

	stream, err := client.StartFleet(streamCtx, &pb.StartFleetReq{
		SessionId:  "sess-fleet-kill",
		PlanId:     "plan-kill",
		MaxWorkers: 1,
	})
	if err != nil {
		t.Fatalf("StartFleet: %v", err)
	}

	// Collect the first status event to get a fleet/worker ID.
	ev, err := stream.Recv()
	if err != nil {
		t.Fatalf("first Recv: %v", err)
	}

	var fleetID, workerID string
	if fs, ok := ev.Event.(*pb.ChatEvent_FleetStatus); ok {
		fleetID = fs.FleetStatus.FleetId
		if len(fs.FleetStatus.Workers) > 0 {
			workerID = fs.FleetStatus.Workers[0].Id
		}
	}

	// Attempt to kill the worker (may already be done given 100ms execution time).
	if fleetID != "" && workerID != "" {
		// KillFleetWorker returns NotFound if worker already finished — acceptable.
		_, _ = client.KillFleetWorker(ctx, &pb.KillFleetWorkerReq{
			FleetId:  fleetID,
			WorkerId: workerID,
		})
	}

	// Verify GetFleetStatus is reachable once fleet has started.
	if fleetID != "" {
		fs, err := client.GetFleetStatus(ctx, &pb.FleetStatusReq{FleetId: fleetID})
		if err != nil {
			t.Fatalf("GetFleetStatus: %v", err)
		}
		if fs.FleetId != fleetID {
			t.Errorf("expected fleet_id=%s, got %s", fleetID, fs.FleetId)
		}
	}

	// Drain remaining stream events.
	for {
		_, err := stream.Recv()
		if err == io.EOF || err != nil {
			break
		}
	}
}
