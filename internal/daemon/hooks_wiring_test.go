package daemon

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
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
	tm := NewTeamManager(newTestEngine(t), hc)

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

func TestHooks_ChatTurnLifecycle(t *testing.T) {
	hc, fired := hookRecorder(t, hooks.UserPromptSubmit, hooks.PreCommand, hooks.Stop, hooks.PostCommand)
	h := newE2EHarness(t)
	h.Svc.engine.Hooks = hc
	session := h.createSession(t, "e2e-mock")

	stream, err := h.Client.SendMessage(context.Background(), &pb.SendMessageReq{
		SessionId: session.Id,
		Content:   "hello hooks",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	for {
		ev, err := stream.Recv()
		if err != nil {
			break
		}
		if _, ok := ev.Event.(*pb.ChatEvent_Complete); ok {
			break
		}
	}

	waitFor(t, func() bool { return fired(hooks.UserPromptSubmit) }, time.Second, "UserPromptSubmit hook")
	waitFor(t, func() bool { return fired(hooks.PreCommand) }, time.Second, "PreCommand hook")
	waitFor(t, func() bool { return fired(hooks.Stop) }, time.Second, "Stop hook")
	waitFor(t, func() bool { return fired(hooks.PostCommand) }, time.Second, "PostCommand hook")
}

func TestHooks_ProjectHookRequiresTrustForSessionWorkdir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workDir := t.TempDir()
	sentinel := filepath.Join(t.TempDir(), "project-fired")
	if err := os.MkdirAll(filepath.Join(workDir, ".ratchet"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".ratchet", "hooks.yaml"), []byte(`
hooks:
  post-command:
    - command: "touch `+sentinel+`"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	engine := newTestEngine(t)
	sessionID := "sess-project-hooks"
	if _, err := engine.DB.Exec(
		`INSERT INTO sessions (id, name, status, provider, working_dir, model) VALUES (?, ?, 'active', 'default', ?, '')`,
		sessionID, sessionID, workDir,
	); err != nil {
		t.Fatal(err)
	}

	if err := engine.RunHooks(context.Background(), hooks.PostCommand, map[string]string{"session_id": sessionID}); err != nil {
		t.Fatalf("RunHooks untrusted: %v", err)
	}
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("untrusted project hook fired")
	}

	store, err := hooks.LoadTrustStore(hooks.DefaultTrustStorePath())
	if err != nil {
		t.Fatalf("LoadTrustStore: %v", err)
	}
	cfg, err := hooks.LoadWithOptions(hooks.LoadOptions{WorkingDir: workDir, TrustStore: store})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}
	hash := cfg.Hooks[hooks.PostCommand][0].Hash
	if err := store.Trust(hash); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	if err := engine.RunHooks(context.Background(), hooks.PostCommand, map[string]string{"session_id": sessionID}); err != nil {
		t.Fatalf("RunHooks trusted: %v", err)
	}
	waitFor(t, func() bool {
		_, err := os.Stat(sentinel)
		return err == nil
	}, time.Second, "trusted project hook")
}

func TestHooks_ProjectHookSkippedWithoutWorkdir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workDir := t.TempDir()
	sentinel := filepath.Join(t.TempDir(), "project-fired")
	if err := os.MkdirAll(filepath.Join(workDir, ".ratchet"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".ratchet", "hooks.yaml"), []byte(`
hooks:
  on-cron-tick:
    - command: "touch `+sentinel+`"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := hooks.LoadTrustStore(hooks.DefaultTrustStorePath())
	if err != nil {
		t.Fatalf("LoadTrustStore: %v", err)
	}
	cfg, err := hooks.LoadWithOptions(hooks.LoadOptions{WorkingDir: workDir, TrustStore: store})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}
	if err := store.Trust(cfg.Hooks[hooks.OnCronTick][0].Hash); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	engine := newTestEngine(t)
	if err := engine.RunHooks(context.Background(), hooks.OnCronTick, map[string]string{"command": "ping"}); err != nil {
		t.Fatalf("RunHooks: %v", err)
	}
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("project hook fired without a workdir")
	}
}

func TestHooks_ChangedPluginHookRequiresRetrust(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := t.TempDir()
	firstSentinel := filepath.Join(dir, "first")
	secondSentinel := filepath.Join(dir, "second")
	engine := newTestEngine(t)
	pluginHook := hooks.Hook{
		Command:       "touch " + firstSentinel,
		Event:         hooks.PostCommand,
		SourceKind:    hooks.SourcePlugin,
		SourceID:      "plugin:test@1.0.0:hooks.yaml",
		SourcePath:    filepath.Join(dir, "hooks.yaml"),
		PluginName:    "test",
		PluginVersion: "1.0.0",
	}
	pluginHook.Hash = pluginHook.DescriptorHash()
	engine.Hooks = &hooks.HookConfig{Hooks: map[hooks.Event][]hooks.Hook{
		hooks.PostCommand: {pluginHook},
	}}

	store, err := hooks.LoadTrustStore(hooks.DefaultTrustStorePath())
	if err != nil {
		t.Fatalf("LoadTrustStore: %v", err)
	}
	if err := store.Trust(pluginHook.Hash); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	if err := engine.RunHooks(context.Background(), hooks.PostCommand, map[string]string{}); err != nil {
		t.Fatalf("RunHooks original: %v", err)
	}
	waitFor(t, func() bool {
		_, err := os.Stat(firstSentinel)
		return err == nil
	}, time.Second, "trusted plugin hook")

	engine.Hooks.Hooks[hooks.PostCommand][0].Command = "touch " + secondSentinel
	if err := engine.RunHooks(context.Background(), hooks.PostCommand, map[string]string{}); err != nil {
		t.Fatalf("RunHooks changed: %v", err)
	}
	if _, err := os.Stat(secondSentinel); err == nil {
		t.Fatal("changed plugin hook fired under stale trust")
	}
}

func TestHooks_RunHooksAndLogReportsTrustedHookFailure(t *testing.T) {
	engine := newTestEngine(t)
	engine.Hooks = &hooks.HookConfig{Hooks: map[hooks.Event][]hooks.Hook{
		hooks.PostCommand: {{Command: "exit 7"}},
	}}

	var buf bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(oldOutput) })

	runHooksAndLog(context.Background(), engine, hooks.PostCommand, map[string]string{}, "test hook")
	if !strings.Contains(buf.String(), "test hook") || !strings.Contains(buf.String(), "exit status 7") {
		t.Fatalf("log output missing hook failure details:\n%s", buf.String())
	}
}

func TestManagedHooksApplyPolicyAfterAllSources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workDir := t.TempDir()
	markers := managedHookMarkers(t)
	event := hooks.SessionStart

	daemonHooks := &hooks.HookConfig{Hooks: map[hooks.Event][]hooks.Hook{
		event: {managedHookMarker(markers.user), managedHookMarker(markers.plugin)},
	}}
	daemonHooks.Hooks[event][0].Event = event
	daemonHooks.Hooks[event][0].SourceKind = hooks.SourceUser
	daemonHooks.Hooks[event][0].SourceID = "user:hooks.yaml"
	daemonHooks.Hooks[event][0].Hash = daemonHooks.Hooks[event][0].DescriptorHash()
	daemonHooks.Hooks[event][1].Event = event
	daemonHooks.Hooks[event][1].SourceKind = hooks.SourcePlugin
	daemonHooks.Hooks[event][1].SourceID = "plugin:runtime@1.0.0:hooks.yaml"
	daemonHooks.Hooks[event][1].Hash = daemonHooks.Hooks[event][1].DescriptorHash()

	writeProjectHook(t, workDir, event, managedHookMarker(markers.project))
	store, err := hooks.LoadTrustStore(hooks.DefaultTrustStorePath())
	if err != nil {
		t.Fatalf("LoadTrustStore: %v", err)
	}
	if err := store.Trust(daemonHooks.Hooks[event][1].Hash); err != nil {
		t.Fatalf("trust plugin hook: %v", err)
	}
	projectConfig, err := hooks.LoadWithOptions(hooks.LoadOptions{
		WorkingDir: workDir,
		TrustStore: store,
		SkipUser:   true,
	})
	if err != nil {
		t.Fatalf("load project hook: %v", err)
	}
	if err := store.Trust(projectConfig.Hooks[event][0].Hash); err != nil {
		t.Fatalf("trust project hook: %v", err)
	}

	engine := newTestEngine(t)
	engine.Hooks = daemonHooks
	engine.ManagedHookAudit = hooks.NewHookAudit(managedHookAuditPath(t))
	data := map[string]string{"working_dir": workDir, "session_id": "managed-all-sources"}

	for _, test := range []struct {
		name       string
		mode       hooks.ManagedMode
		wantSource map[hooks.SourceKind]bool
	}{
		{
			name: "additive",
			mode: hooks.ManagedModeAdditive,
			wantSource: map[hooks.SourceKind]bool{
				hooks.SourceUser: true, hooks.SourcePlugin: true,
				hooks.SourceProject: true, hooks.SourceManaged: true,
			},
		},
		{
			name: "managed-only",
			mode: hooks.ManagedModeOnly,
			wantSource: map[hooks.SourceKind]bool{
				hooks.SourceManaged: true,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			removeManagedHookMarkers(t, markers)
			engine.ManagedHookPolicy = managedHookPolicy(test.mode, event, managedHookMarker(markers.managed), "injected-managed-hooks.yaml")

			effective, _ := engine.effectiveHooks(t.Context(), event, data)
			assertManagedHookDiagnostics(t, effective.Hooks[event], test.mode)
			if err := engine.RunHooks(t.Context(), event, data); err != nil {
				t.Fatalf("RunHooks: %v", err)
			}
			for source, marker := range markers.bySource() {
				assertManagedHookMarker(t, marker, test.wantSource[source])
			}
		})
	}
}

func TestManagedHooksAuditFailureLogIsPrivate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	marker := filepath.Join(t.TempDir(), "must-not-launch")
	const commandSecret = "MANAGED_COMMAND_SECRET"
	const promptSecret = "MANAGED_PROMPT_SECRET"
	const auditSecret = "MANAGED_AUDIT_SECRET"

	engine := newTestEngine(t)
	engine.ManagedHookPolicy = managedHookPolicy(
		hooks.ManagedModeOnly,
		hooks.SessionStart,
		managedHookMarkerWithPrefix(marker, commandSecret),
		"injected-managed-hooks.yaml",
	)
	appends := 0
	engine.ManagedHookAudit = daemonHookAuditWriterFunc(func(hooks.HookAuditRecord) error {
		appends++
		return errors.New(auditSecret)
	})

	var buf bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(oldOutput) })

	runHooksAndLog(t.Context(), engine, hooks.SessionStart, map[string]string{"prompt": promptSecret}, "managed runtime")
	if appends != 1 {
		t.Fatalf("audit appends = %d, want 1", appends)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("managed hook launched after audit failure: %v", err)
	}
	for _, secret := range []string{commandSecret, promptSecret, auditSecret} {
		if strings.Contains(buf.String(), secret) {
			t.Fatalf("daemon log contains private managed-hook data %q: %s", secret, buf.String())
		}
	}
}

func assertManagedHookDiagnostics(t *testing.T, hookList []hooks.Hook, mode hooks.ManagedMode) {
	t.Helper()
	if len(hookList) != 4 {
		t.Fatalf("effective hooks = %d, want all four sources: %#v", len(hookList), hookList)
	}
	seen := make(map[hooks.SourceKind]bool, 4)
	for _, hook := range hookList {
		seen[hook.SourceKind] = true
		wantSuppressed := mode == hooks.ManagedModeOnly && hook.SourceKind != hooks.SourceManaged
		if hook.Suppressed != wantSuppressed {
			t.Fatalf("source %q suppressed = %v, want %v", hook.SourceKind, hook.Suppressed, wantSuppressed)
		}
	}
	for _, source := range []hooks.SourceKind{hooks.SourceUser, hooks.SourcePlugin, hooks.SourceProject, hooks.SourceManaged} {
		if !seen[source] {
			t.Fatalf("effective diagnostics missing source %q: %#v", source, hookList)
		}
	}
}

type daemonHookAuditWriterFunc func(hooks.HookAuditRecord) error

func (fn daemonHookAuditWriterFunc) Append(record hooks.HookAuditRecord) error {
	return fn(record)
}
