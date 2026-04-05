package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestFleet_RealExecution(t *testing.T) {
	engine := newTestEngine(t)
	fm := NewFleetManager(config.ModelRouting{}, engine, nil)

	eventCh := make(chan *pb.FleetStatus, 64)
	fleetID := fm.StartFleet(context.Background(), &pb.StartFleetReq{
		SessionId:  "sess-real",
		PlanId:     "plan-real",
		MaxWorkers: 2,
	}, []string{"step-a", "step-b"}, eventCh)

	for range eventCh {
	}

	fs, err := fm.GetStatus(fleetID)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if fs.Status != "completed" {
		t.Errorf("expected completed, got %s", fs.Status)
	}
	if int(fs.Completed) != 2 {
		t.Errorf("expected 2 completed workers, got %d", fs.Completed)
	}
	for _, w := range fs.Workers {
		if w.Status != "completed" {
			t.Errorf("worker %s: expected completed, got %s", w.Id, w.Status)
		}
	}
}

func TestFleet_WorkerFailure(t *testing.T) {
	// nil engine causes executeWorker to return error, marking worker failed
	fm := NewFleetManager(config.ModelRouting{}, nil, nil)

	eventCh := make(chan *pb.FleetStatus, 64)
	fleetID := fm.StartFleet(context.Background(), &pb.StartFleetReq{
		SessionId:  "sess-fail",
		MaxWorkers: 1,
	}, []string{"step-fail"}, eventCh)

	for range eventCh {
	}

	fs, err := fm.GetStatus(fleetID)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if len(fs.Workers) == 0 {
		t.Fatal("expected at least one worker")
	}
	if fs.Workers[0].Status != "failed" {
		t.Errorf("expected worker failed, got %s", fs.Workers[0].Status)
	}
}

func TestFleet_PlanDecomposition(t *testing.T) {
	engine := newTestEngine(t)
	fm := NewFleetManager(config.ModelRouting{}, engine, nil)

	steps := []string{"s1", "s2", "s3"}
	eventCh := make(chan *pb.FleetStatus, 64)
	fleetID := fm.StartFleet(context.Background(), &pb.StartFleetReq{
		SessionId:  "sess-decomp",
		MaxWorkers: 3,
	}, steps, eventCh)

	for range eventCh {
	}

	fs, err := fm.GetStatus(fleetID)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if int(fs.Total) != len(steps) {
		t.Errorf("expected total=%d, got %d", len(steps), fs.Total)
	}
	if int(fs.Completed) != len(steps) {
		t.Errorf("expected completed=%d, got %d", len(steps), fs.Completed)
	}
	_ = fleetID
}

func TestFleet_KillWorker(t *testing.T) {
	engine := newTestEngine(t)
	fm := NewFleetManager(config.ModelRouting{}, engine, nil)

	eventCh := make(chan *pb.FleetStatus, 64)
	fleetID := fm.StartFleet(context.Background(), &pb.StartFleetReq{
		SessionId:  "sess-kill",
		MaxWorkers: 1,
	}, []string{"step-kill"}, eventCh)

	// Give workers time to start
	time.Sleep(10 * time.Millisecond)

	fs, err := fm.GetStatus(fleetID)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if len(fs.Workers) == 0 {
		t.Fatal("expected at least one worker")
	}
	// Worker may already be done; KillWorker should not panic either way.
	_ = fm.KillWorker(fleetID, fs.Workers[0].Id)

	// Drain
	for range eventCh {
	}
}
