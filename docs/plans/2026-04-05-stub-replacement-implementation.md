# Stub Replacement Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Design:** `docs/plans/2026-04-05-stub-replacement-design.md`
**Branch:** `feat/stub-replacement`

---

## Task 1: Upgrade workflow-plugin-agent to v0.5.9

**Files:** `go.mod`, `go.sum`

1. Remove any `replace` directive for `workflow-plugin-agent`
2. `go get github.com/GoCodeAlone/workflow-plugin-agent@v0.5.9`
3. `go mod tidy`
4. `go build ./...` — fix any breaking API changes
5. `go test ./... -count=1` — ensure existing tests pass
6. Commit: `chore: upgrade workflow-plugin-agent v0.4.1 → v0.5.9`

---

## Task 2: Add ApprovalGate and per-model context limits config

**Files:**
- Create: `internal/daemon/approval_gate.go`
- Modify: `internal/config/config.go`

**Step 1:** Create `internal/daemon/approval_gate.go`:
```go
package daemon

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ApprovalGate manages pending approval requests with blocking semantics.
// It implements a channel-per-request pattern: callers block on Request()
// until Resolve() is called from the TUI or timeout elapses.
type ApprovalGate struct {
	mu      sync.Mutex
	pending map[string]chan ApprovalResponse
}

func NewApprovalGate() *ApprovalGate {
	return &ApprovalGate{
		pending: make(map[string]chan ApprovalResponse),
	}
}

// Request registers a pending approval and returns a channel that will
// receive exactly one response. The caller should select on this channel
// with a timeout.
func (g *ApprovalGate) Request(requestID string) <-chan ApprovalResponse {
	g.mu.Lock()
	defer g.mu.Unlock()
	ch := make(chan ApprovalResponse, 1)
	g.pending[requestID] = ch
	return ch
}

// Resolve delivers an approval decision for a pending request.
// Returns false if no pending request exists with that ID.
func (g *ApprovalGate) Resolve(requestID string, approved bool, reason string) bool {
	g.mu.Lock()
	ch, ok := g.pending[requestID]
	if ok {
		delete(g.pending, requestID)
	}
	g.mu.Unlock()
	if !ok {
		return false
	}
	ch <- ApprovalResponse{Approved: approved, Reason: reason}
	return true
}

// WaitForResolution blocks until the approval is resolved or ctx expires.
// This matches the executor.Approver interface pattern.
func (g *ApprovalGate) WaitForResolution(ctx context.Context, requestID string, timeout time.Duration) (bool, string, error) {
	ch := g.Request(requestID)
	select {
	case resp := <-ch:
		return resp.Approved, resp.Reason, nil
	case <-time.After(timeout):
		g.mu.Lock()
		delete(g.pending, requestID)
		g.mu.Unlock()
		return false, "approval timeout", nil
	case <-ctx.Done():
		g.mu.Lock()
		delete(g.pending, requestID)
		g.mu.Unlock()
		return false, "context cancelled", ctx.Err()
	}
}

// PendingCount returns the number of unresolved approval requests.
func (g *ApprovalGate) PendingCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.pending)
}
```

**Step 2:** Add `ModelLimits` to `ContextConfig` in `config.go`:
```go
type ContextConfig struct {
	CompressionThreshold float64        `yaml:"compression_threshold"`
	PreserveMessages     int            `yaml:"preserve_messages"`
	CompressionModel     string         `yaml:"compression_model"`
	ModelLimits          map[string]int `yaml:"model_limits"`
}
```
In `DefaultConfig()`, add:
```go
ModelLimits: map[string]int{
	"claude-opus-4-6":   1000000,
	"claude-sonnet-4-6": 200000,
	"claude-haiku-4-5":  200000,
	"gpt-4o":            128000,
	"gpt-4o-mini":       128000,
},
```

**Step 3:** Commit: `feat: add ApprovalGate and per-model context limits config`

---

## Task 3: Wire real fleet worker execution

**Files:**
- Modify: `internal/daemon/fleet.go`
- Modify: `internal/daemon/service.go`

**Step 1:** Add `engine *EngineContext` field to `FleetManager`:
```go
type FleetManager struct {
	mu      sync.RWMutex
	fleets  map[string]*fleetInstance
	routing config.ModelRouting
	engine  *EngineContext
	stop    chan struct{}
}
```
Update `NewFleetManager(routing, engine)` signature.

**Step 2:** Replace `executeWorker` with real implementation:
```go
func (fm *FleetManager) executeWorker(ctx context.Context, w *pb.FleetWorker) error {
	if fm.engine == nil || fm.engine.ProviderRegistry == nil {
		return fmt.Errorf("no engine context available")
	}

	// Resolve provider for this worker
	var prov provider.Provider
	var err error
	if w.Provider != "" {
		prov, err = fm.engine.ProviderRegistry.GetByAlias(ctx, w.Provider)
	} else {
		prov, err = fm.engine.ProviderRegistry.GetDefault(ctx)
	}
	if err != nil {
		return fmt.Errorf("resolve provider: %w", err)
	}

	cfg := executor.Config{
		Provider:      prov,
		ToolRegistry:  fm.engine.ToolRegistry,
		MaxIterations: 25,
	}
	if fm.engine.SecretGuard != nil {
		cfg.SecretRedactor = fm.engine.SecretGuard
	}

	systemPrompt := fmt.Sprintf("You are fleet worker %s. Execute the following task step thoroughly and report results.", w.Name)
	result, err := executor.Execute(ctx, cfg, systemPrompt, w.StepId, w.Id)
	if err != nil {
		return err
	}
	if result.Status == "failed" {
		return fmt.Errorf("executor: %s", result.Error)
	}
	return nil
}
```

**Step 3:** Fix `StartFleet` in `service.go` to decompose plans:
```go
func (s *Service) StartFleet(req *pb.StartFleetReq, stream pb.RatchetDaemon_StartFleetServer) error {
	var steps []string
	if plan := s.plans.Get(req.PlanId); plan != nil {
		for _, step := range plan.Steps {
			if step.Status != "skipped" {
				steps = append(steps, step.Description)
			}
		}
	}
	if len(steps) == 0 {
		steps = []string{req.PlanId}
		if req.PlanId == "" {
			steps = []string{"default-step"}
		}
	}
	// ... rest unchanged
```

**Step 4:** Update `NewService` to pass engine to FleetManager:
```go
svc.fleet = NewFleetManager(routing, engine)
```

**Step 5:** Build and test: `go build ./... && go test ./internal/daemon/ -v`

**Step 6:** Commit: `feat: wire real executor-based fleet worker execution`

---

## Task 4: Wire real team agent execution

**Files:**
- Modify: `internal/daemon/teams.go`
- Modify: `internal/daemon/service.go`

**Step 1:** Add `engine *EngineContext` field to `TeamManager`. Update `NewTeamManager(engine)`.

**Step 2:** Replace `run()` with real sequential agent execution:
```go
func (tm *TeamManager) run(ctx context.Context, ti *teamInstance, req *pb.StartTeamReq) {
	defer close(ti.eventCh)

	specs := []struct{ name, role, model, provider string }{
		{"orchestrator", "orchestrator", "", req.OrchestratorProvider},
		{"worker-1", "worker", "", ""},
	}

	// Spawn agents
	for _, spec := range specs {
		ag := &teamAgent{
			id: uuid.New().String(), name: spec.name, role: spec.role,
			model: spec.model, provider: spec.provider, status: "running",
		}
		ti.mu.Lock()
		ti.agents[ag.id] = ag
		ti.mu.Unlock()
		ti.eventCh <- &pb.TeamEvent{
			Event: &pb.TeamEvent_AgentSpawned{AgentSpawned: &pb.AgentSpawned{
				AgentId: ag.id, AgentName: ag.name, Role: ag.role,
			}},
		}
	}

	// Execute orchestrator
	orch := tm.agentByRole(ti, "orchestrator")
	if orch == nil {
		tm.markDone(ti, "failed")
		return
	}

	orchResult, err := tm.executeAgent(ctx, orch, req.Task, "")
	if err != nil {
		orch.mu.Lock()
		orch.status = "failed"
		orch.mu.Unlock()
		tm.markDone(ti, "failed")
		return
	}
	orch.mu.Lock()
	orch.status = "completed"
	orch.mu.Unlock()
	tm.routeMessage(ti, orch.name, "worker-1", orchResult)

	// Execute worker with orchestrator's output as context
	worker := tm.agentByRole(ti, "worker")
	if worker != nil {
		workerResult, err := tm.executeAgent(ctx, worker, req.Task, orchResult)
		if err != nil {
			worker.mu.Lock()
			worker.status = "failed"
			worker.mu.Unlock()
		} else {
			worker.mu.Lock()
			worker.status = "completed"
			worker.mu.Unlock()
			tm.routeMessage(ti, worker.name, orch.name, workerResult)
		}
	}

	ti.eventCh <- &pb.TeamEvent{
		Event: &pb.TeamEvent_Complete{Complete: &pb.SessionComplete{
			Summary: fmt.Sprintf("Team completed task: %s", req.Task),
		}},
	}
	tm.markDone(ti, "completed")
}

func (tm *TeamManager) executeAgent(ctx context.Context, ag *teamAgent, task, context string) (string, error) {
	if tm.engine == nil || tm.engine.ProviderRegistry == nil {
		return "", fmt.Errorf("no engine context")
	}

	var prov provider.Provider
	var err error
	if ag.provider != "" {
		prov, err = tm.engine.ProviderRegistry.GetByAlias(ctx, ag.provider)
	} else {
		prov, err = tm.engine.ProviderRegistry.GetDefault(ctx)
	}
	if err != nil {
		return "", fmt.Errorf("resolve provider for %s: %w", ag.name, err)
	}

	cfg := executor.Config{
		Provider:      prov,
		ToolRegistry:  tm.engine.ToolRegistry,
		MaxIterations: 15,
	}

	var systemPrompt string
	switch ag.role {
	case "orchestrator":
		systemPrompt = "You are a team orchestrator. Analyze the task, decompose it into subtasks, and provide clear instructions for workers."
	default:
		systemPrompt = fmt.Sprintf("You are team worker %s. Complete the assigned subtask thoroughly.", ag.name)
	}

	userMsg := task
	if context != "" {
		userMsg = fmt.Sprintf("Task: %s\n\nContext from orchestrator:\n%s", task, context)
	}

	ag.mu.Lock()
	ag.currentTask = task
	ag.mu.Unlock()

	result, err := executor.Execute(ctx, cfg, systemPrompt, userMsg, ag.id)
	if err != nil {
		return "", err
	}
	if result.Status == "failed" {
		return "", fmt.Errorf("agent %s failed: %s", ag.name, result.Error)
	}
	return result.Content, nil
}
```

**Step 3:** Update `NewService` to pass engine: `svc.teams = NewTeamManager(engine)`

**Step 4:** Build and test.

**Step 5:** Commit: `feat: wire real executor-based team agent execution`

---

## Task 5: Wire approval actor to TUI gate

**Files:**
- Modify: `internal/daemon/actors.go`
- Modify: `internal/daemon/service.go`

**Step 1:** Add `gate *ApprovalGate` field to `ApprovalActor`:
```go
type ApprovalActor struct {
	requestID string
	responded bool
	gate      *ApprovalGate
	timeout   time.Duration
}
```

**Step 2:** Replace `Receive` for `ApprovalRequest`:
```go
func (a *ApprovalActor) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case ApprovalRequest:
		if a.gate == nil {
			ctx.Response(ApprovalResponse{Approved: false, Reason: "no approval gate configured"})
			return
		}
		timeout := a.timeout
		if timeout == 0 {
			timeout = 30 * time.Minute
		}
		approved, reason, err := a.gate.WaitForResolution(ctx.Context(), a.requestID, timeout)
		if err != nil {
			ctx.Response(ApprovalResponse{Approved: false, Reason: "error: " + err.Error()})
			return
		}
		a.responded = true
		ctx.Response(ApprovalResponse{Approved: approved, Reason: reason})
	case ApprovalResponse:
		a.responded = true
		ctx.Response(msg)
	}
}
```

**Step 3:** Add `approvalGate *ApprovalGate` to `Service`, wire in `NewService`:
```go
svc.approvalGate = NewApprovalGate()
```

**Step 4:** Update `SpawnApproval` to accept and pass the gate:
```go
func (am *ActorManager) SpawnApproval(ctx context.Context, requestID string, gate *ApprovalGate, timeout time.Duration) (*actor.PID, error) {
	a := &ApprovalActor{requestID: requestID, gate: gate, timeout: timeout}
	// ...
}
```

**Step 5:** Wire `RespondToPermission` to also resolve via approval gate:
```go
func (s *Service) RespondToPermission(ctx context.Context, req *pb.PermissionResponse) (*pb.Empty, error) {
	if !s.permGate.Respond(req) {
		// Also try the approval gate for executor-driven approvals
		if !s.approvalGate.Resolve(req.RequestId, req.Allowed, req.Scope) {
			return nil, status.Error(codes.NotFound, "no pending permission request with that ID")
		}
	}
	return &pb.Empty{}, nil
}
```

**Step 6:** Commit: `feat: wire approval actor to TUI via ApprovalGate`

---

## Task 6: Wire cron tick handler and review sub-session

**Files:**
- Modify: `internal/daemon/service.go`
- Modify: `internal/daemon/chat.go`
- Modify: `internal/tui/commands/review.go`

**Step 1:** Implement cron tick handler in `NewService`:
```go
svc.cron = NewCronScheduler(engine.DB, func(sessionID, command string) {
	go func() {
		tickCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		// Use a noop stream since there's no active TUI listener for cron ticks
		ns := &noopSendServer{}
		if err := svc.handleChat(tickCtx, sessionID, command, ns); err != nil {
			log.Printf("cron tick session=%s command=%q: %v", sessionID, command, err)
		}
	}()
})
```

**Step 2:** Add `noopSendServer` that satisfies the stream interface but discards events:
```go
type noopSendServer struct {
	grpc.ServerStream
}
func (n *noopSendServer) Send(*pb.ChatEvent) error { return nil }
func (n *noopSendServer) Context() context.Context  { return context.Background() }
```

**Step 3:** Add review sentinel handling in `chat.go`:
```go
const reviewSentinel = "\x00review\x00"

func (s *Service) handleChat(ctx context.Context, sessionID, userMessage string, stream pb.RatchetDaemon_SendMessageServer) error {
	if userMessage == compactSentinel {
		return s.handleCompact(ctx, sessionID, stream)
	}
	if strings.HasPrefix(userMessage, reviewSentinel) {
		diff := strings.TrimPrefix(userMessage, reviewSentinel)
		return s.handleReview(ctx, sessionID, diff, stream)
	}
	// ... existing chat logic
}
```

**Step 4:** Implement `handleReview`:
```go
func (s *Service) handleReview(ctx context.Context, sessionID, diff string, stream pb.RatchetDaemon_SendMessageServer) error {
	// Load builtin code-reviewer agent definition
	builtins := agent.LoadBuiltins()
	var reviewerDef *agent.AgentDefinition
	for _, d := range builtins {
		if d.Name == "code-reviewer" {
			reviewerDef = &d
			break
		}
	}

	// Resolve provider (prefer reviewer model, fallback to session/default)
	var prov provider.Provider
	var err error
	if reviewerDef != nil && reviewerDef.Provider != "" {
		prov, err = s.engine.ProviderRegistry.GetByAlias(ctx, reviewerDef.Provider)
	}
	if prov == nil {
		prov, err = s.engine.ProviderRegistry.GetDefault(ctx)
	}
	if err != nil {
		return sendError(stream, "no provider for review: "+err.Error())
	}

	systemPrompt := "You are a code reviewer. Analyze diffs for security vulnerabilities, logic errors, code style issues, and test coverage gaps. Output structured review with Critical/Important/Minor categories."
	if reviewerDef != nil && reviewerDef.SystemPrompt != "" {
		systemPrompt = reviewerDef.SystemPrompt
	}

	cfg := executor.Config{
		Provider:      prov,
		ToolRegistry:  s.engine.ToolRegistry,
		MaxIterations: 5,
	}

	result, execErr := executor.Execute(ctx, cfg, systemPrompt, "Review this git diff:\n\n```diff\n"+diff+"\n```", "code-reviewer")

	if execErr != nil {
		return sendError(stream, "review failed: "+execErr.Error())
	}

	// Stream the review result as tokens
	if err := stream.Send(&pb.ChatEvent{
		Event: &pb.ChatEvent_Token{Token: &pb.TokenDelta{Content: result.Content}},
	}); err != nil {
		return err
	}

	// Save to history
	_ = s.saveMessage(ctx, sessionID, "assistant", result.Content, "", "")

	return stream.Send(&pb.ChatEvent{
		Event: &pb.ChatEvent_Complete{Complete: &pb.SessionComplete{Summary: "review complete"}},
	})
}
```

**Step 5:** Update `review.go` to send sentinel:
```go
return &Result{
	Lines:         lines,
	TriggerReview: true,
	ReviewDiff:    diff,
}
```
And in `chat.go` TUI page, where TriggerReview is handled, send `reviewSentinel + result.ReviewDiff` instead of the plain prompt.

**Step 6:** Commit: `feat: wire cron tick injection and code review sub-session`

---

## Task 7: Wire per-model context limits and hook call sites

**Files:**
- Modify: `internal/daemon/chat.go`
- Modify: `internal/daemon/fleet.go`
- Modify: `internal/daemon/teams.go`
- Modify: `internal/daemon/service.go`
- Modify: `internal/daemon/plans.go`

**Step 1:** In `chat.go`, replace hardcoded limit with per-model lookup:
```go
// Replace:
// const defaultModelLimit = 200000
// With:
modelLimit := 200000 // fallback
if session.Model != "" && contextCfg.ModelLimits != nil {
	if limit, ok := contextCfg.ModelLimits[session.Model]; ok {
		modelLimit = limit
	}
}
if s.tokens.ShouldCompress(sessionID, contextCfg.CompressionThreshold, modelLimit) {
```

**Step 2:** Add hooks config loading and wiring. Add `hooks *hooks.HookConfig` to `EngineContext`. Load in `NewEngineContext`:
```go
hc, _ := hooks.Load("")  // workingDir resolved per session
ec.Hooks = hc
```

**Step 3:** Fire hooks from managers. Add `hooks *hooks.HookConfig` field to `FleetManager`, `TeamManager`, `PlanManager`. Fire at call sites:
- `plans.go` `Approve()`: fire `PrePlan`
- `plans.go` `UpdateStep()` when plan completes: fire `PostPlan`
- `fleet.go` `StartFleet()`: fire `PreFleet`
- `fleet.go` `runFleet()` after wg.Wait: fire `PostFleet`
- `teams.go` `run()` per agent spawn: fire `OnAgentSpawn`
- `teams.go` `run()` per agent complete: fire `OnAgentComplete`
- `chat.go` when ShouldCompress triggers: fire `OnTokenLimit`
- Cron tick handler: fire `OnCronTick`

**Step 4:** Build and verify.

**Step 5:** Commit: `feat: wire per-model context limits and lifecycle hook call sites`

---

## Task 8: Write integration and unit tests

**Files:**
- Create: `internal/daemon/approval_gate_test.go`
- Create: `internal/daemon/fleet_integration_test.go`
- Create: `internal/daemon/teams_integration_test.go`
- Create: `internal/daemon/cron_integration_test.go`
- Create: `internal/daemon/review_test.go`
- Create: `internal/daemon/compression_limits_test.go`
- Create: `internal/daemon/hooks_wiring_test.go`

All tests use mock providers (no real LLM calls). See design doc for test specifications.

Key test patterns:
- Mock provider returns canned responses for executor.Execute
- In-memory SQLite for DB operations
- Short timeouts for approval gate tests
- Hook tests use a temp hooks.yaml and verify events fire

**Step 1:** Write all test files (details per design doc Section "Testing Strategy")

**Step 2:** Run: `go test -race ./internal/daemon/ -v -count=1`

**Step 3:** Run full suite: `go test -race ./... -count=1`

**Step 4:** Run lint: `golangci-lint run`

**Step 5:** Commit: `test: add integration tests for fleet, team, approval, cron, review, compression, hooks`

---

## Execution Order

```
Task 1 (dep upgrade) → Task 2 (approval gate + config)
                     → Task 3 (fleet workers) ─┐
                     → Task 4 (team agents)    ├→ Task 7 (limits + hooks) → Task 8 (tests)
                     → Task 5 (approval actor) ─┘
                     → Task 6 (cron + review)  ─┘
```

Tasks 3-6 can run in parallel after Tasks 1-2. Task 7 depends on 3-6. Task 8 depends on all.
