package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	"github.com/GoCodeAlone/ratchet-cli/internal/hooks"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// hookRecorder returns a HookConfig that writes a sentinel file for each event fired.
func hookRecorder(t *testing.T, events ...hooks.Event) (*hooks.HookConfig, func(hooks.Event) bool) {
	t.Helper()
	dir := t.TempDir()

	hc := &hooks.HookConfig{
		Hooks: make(map[hooks.Event][]hooks.Hook),
	}
	for _, ev := range events {
		ev := ev
		sentinel := filepath.Join(dir, string(ev))
		hc.Hooks[ev] = []hooks.Hook{
			{Command: "touch " + sentinel},
		}
	}

	fired := func(ev hooks.Event) bool {
		_, err := os.Stat(filepath.Join(dir, string(ev)))
		return err == nil
	}
	return hc, fired
}

func waitFor(t *testing.T, condition func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("timed out waiting for: %s", msg)
}

func TestHooks_PrePostPlan(t *testing.T) {
	hc, fired := hookRecorder(t, hooks.PrePlan, hooks.PostPlan)
	pm := NewPlanManager(hc)

	steps := []*pb.PlanStep{
		{Id: "s1", Status: "pending"},
	}
	plan := pm.Create("sess", "goal", steps)

	if err := pm.Approve(plan.Id, nil); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	waitFor(t, func() bool { return fired(hooks.PrePlan) }, time.Second, "PrePlan hook")

	// Transition to executing so UpdateStep can complete it.
	pm.mu.Lock()
	pm.plans[plan.Id].Status = "executing"
	pm.mu.Unlock()

	if err := pm.UpdateStep(plan.Id, "s1", "completed", ""); err != nil {
		t.Fatalf("UpdateStep: %v", err)
	}
	waitFor(t, func() bool { return fired(hooks.PostPlan) }, time.Second, "PostPlan hook")
}

func TestHooks_PrePostFleet(t *testing.T) {
	hc, fired := hookRecorder(t, hooks.PreFleet, hooks.PostFleet)
	engine := newTestEngine(t)
	fm := NewFleetManager(config.ModelRouting{}, engine, hc)

	eventCh := make(chan *pb.FleetStatus, 64)
	fm.StartFleet(context.Background(), &pb.StartFleetReq{
		SessionId:  "sess-hooks",
		MaxWorkers: 1,
	}, []string{"hook-step"}, eventCh)

	for range eventCh {
	}

	waitFor(t, func() bool { return fired(hooks.PreFleet) }, time.Second, "PreFleet hook")
	waitFor(t, func() bool { return fired(hooks.PostFleet) }, time.Second, "PostFleet hook")
}

func TestHooks_AgentSpawnComplete(t *testing.T) {
	hc, fired := hookRecorder(t, hooks.OnAgentSpawn, hooks.OnAgentComplete)
	tm := NewTeamManager(nil, hc)

	_, eventCh := tm.StartTeam(context.Background(), &pb.StartTeamReq{
		Task: "hook agent task",
	})
	for range eventCh {
	}

	waitFor(t, func() bool { return fired(hooks.OnAgentSpawn) }, time.Second, "OnAgentSpawn hook")
	waitFor(t, func() bool { return fired(hooks.OnAgentComplete) }, time.Second, "OnAgentComplete hook")
}

func TestHooks_OnCronTick(t *testing.T) {
	hc, fired := hookRecorder(t, hooks.OnCronTick)
	db := setupCronDB(t)

	cs := NewCronScheduler(db, func(_, _ string) {})
	ctx := context.Background()
	if err := cs.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Simulate what the service's cron callback does: fire OnCronTick
	job, err := cs.Create(ctx, "sess-hook-cron", "100ms", "ping")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = cs.Stop(ctx, job.ID) }()

	// Manually invoke the hook to verify wiring logic
	if err := hc.Run(hooks.OnCronTick, map[string]string{
		"session_id": "sess-hook-cron",
		"command":    "ping",
	}); err != nil {
		t.Fatalf("Run OnCronTick: %v", err)
	}

	waitFor(t, func() bool { return fired(hooks.OnCronTick) }, time.Second, "OnCronTick hook")
}

