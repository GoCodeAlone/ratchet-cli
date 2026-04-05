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
	engine := newTestEngine(t)
	engine.Hooks = hc

	// Construct a Service with the same cron wiring as NewService, so OnCronTick
	// fires through the real service callback (engine.Hooks.Run) on each tick.
	svc := &Service{
		engine:   engine,
		sessions: NewSessionManager(engine.DB),
		permGate: newPermissionGate(),
		tokens:   NewTokenTracker(),
	}
	svc.cron = NewCronScheduler(engine.DB, func(sessionID, command string) {
		go func() {
			tickCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if engine.Hooks != nil {
				_ = engine.Hooks.Run(hooks.OnCronTick, map[string]string{
					"session_id": sessionID,
					"command":    command,
				})
			}
			ns := &noopSendServer{ctx: tickCtx}
			_ = svc.handleChat(tickCtx, sessionID, command, ns)
		}()
	})
	ctx := context.Background()
	if err := svc.cron.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	job, err := svc.cron.Create(ctx, "sess-hook-cron", "100ms", "ping")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = svc.cron.Stop(ctx, job.ID) }()

	waitFor(t, func() bool { return fired(hooks.OnCronTick) }, 3*time.Second, "OnCronTick from service cron wiring")
}

func TestHooks_OnTokenLimit(t *testing.T) {
	hc, fired := hookRecorder(t, hooks.OnTokenLimit)
	engine := newTestEngine(t)
	engine.Hooks = hc

	sessionID := "sess-token-limit"
	_, _ = engine.DB.Exec(
		`INSERT INTO sessions (id, name, status, provider, working_dir, model) VALUES (?, ?, 'active', 'default', '', '')`,
		sessionID, sessionID,
	)

	svc := &Service{
		engine:   engine,
		sessions: NewSessionManager(engine.DB),
		permGate: newPermissionGate(),
		tokens:   NewTokenTracker(),
	}

	// Pre-load enough tokens to exceed the 90% threshold (200000 * 0.9 = 180000).
	svc.tokens.AddTokens(sessionID, 180001, 0)

	stream := &captureStream{ctx: context.Background()}
	_ = svc.handleChat(context.Background(), sessionID, "hello", stream)

	waitFor(t, func() bool { return fired(hooks.OnTokenLimit) }, 2*time.Second, "OnTokenLimit hook")
}

