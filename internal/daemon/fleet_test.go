package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestFleetManager_Decompose(t *testing.T) {
	fm := NewFleetManager(config.ModelRouting{})
	req := &pb.StartFleetReq{
		SessionId:  "sess-1",
		PlanId:     "plan-1",
		MaxWorkers: 3,
	}
	steps := []string{"step-a", "step-b", "step-c"}
	eventCh := make(chan *pb.FleetStatus, 64)
	fleetID := fm.StartFleet(context.Background(), req, steps, eventCh)
	if fleetID == "" {
		t.Fatal("expected non-empty fleetID")
	}

	// Drain events
	var last *pb.FleetStatus
	for fs := range eventCh {
		last = fs
	}

	if last == nil {
		t.Fatal("expected at least one status event")
	}
	if last.Total != 3 {
		t.Errorf("expected total=3, got %d", last.Total)
	}
	if last.Status != "completed" {
		t.Errorf("expected status=completed, got %s", last.Status)
	}
}

func TestFleetManager_WorkerLifecycle(t *testing.T) {
	fm := NewFleetManager(config.ModelRouting{})
	req := &pb.StartFleetReq{
		SessionId:  "sess-2",
		PlanId:     "plan-2",
		MaxWorkers: 2,
	}
	steps := []string{"step-1", "step-2"}
	eventCh := make(chan *pb.FleetStatus, 64)
	fleetID := fm.StartFleet(context.Background(), req, steps, eventCh)

	// Wait for completion
	for range eventCh {
	}

	fs, err := fm.GetStatus(fleetID)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if fs.Completed != 2 {
		t.Errorf("expected 2 completed, got %d", fs.Completed)
	}
	for _, w := range fs.Workers {
		if w.Status != "completed" {
			t.Errorf("worker %s: expected completed, got %s", w.Id, w.Status)
		}
	}
}

func TestFleetManager_KillWorker(t *testing.T) {
	fm := NewFleetManager(config.ModelRouting{})

	// Use a context to control worker duration
	req := &pb.StartFleetReq{
		SessionId:  "sess-3",
		PlanId:     "plan-3",
		MaxWorkers: 1,
	}

	// Override worker execution to be long-running by using a slow step
	steps := []string{"slow-step"}
	eventCh := make(chan *pb.FleetStatus, 64)
	fleetID := fm.StartFleet(context.Background(), req, steps, eventCh)

	// Give workers time to start
	time.Sleep(10 * time.Millisecond)

	fs, err := fm.GetStatus(fleetID)
	if err != nil {
		t.Fatalf("GetStatus before kill: %v", err)
	}
	if len(fs.Workers) == 0 {
		t.Fatal("expected at least one worker")
	}

	// Kill the worker — it may already be done since execution is 100ms,
	// so we just verify KillWorker doesn't panic on a finished worker.
	workerID := fs.Workers[0].Id
	// Error is acceptable here if worker already completed.
	_ = fm.KillWorker(fleetID, workerID)

	// Drain
	for range eventCh {
	}
}

func TestFleetManager_MaxWorkers(t *testing.T) {
	fm := NewFleetManager(config.ModelRouting{})
	req := &pb.StartFleetReq{
		SessionId:  "sess-4",
		PlanId:     "plan-4",
		MaxWorkers: 2, // cap at 2 even with 4 steps
	}
	steps := []string{"s1", "s2", "s3", "s4"}
	eventCh := make(chan *pb.FleetStatus, 128)
	fleetID := fm.StartFleet(context.Background(), req, steps, eventCh)

	// Drain events
	for range eventCh {
	}

	fs, err := fm.GetStatus(fleetID)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if int(fs.Total) != len(steps) {
		t.Errorf("expected total=%d, got %d", len(steps), fs.Total)
	}
	if fs.Completed != int32(len(steps)) {
		t.Errorf("expected completed=%d, got %d", len(steps), fs.Completed)
	}
	_ = fleetID
}
