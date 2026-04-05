# ratchet-cli Stub Replacement & Full Wiring Design

**Date:** 2026-04-05
**Goal:** Replace all stub/placeholder implementations with real functionality, wire all disconnected features, and add comprehensive tests.

## Dependency Upgrade

Upgrade `workflow-plugin-agent` from v0.4.1 → v0.5.9 (remove local replace directive). This provides `executor.Execute()` with `Inbox`, `OnEvent`, `ShouldStop` mesh support — the real agent execution loop.

## Stub 1: Fleet Worker Execution (`fleet.go:executeWorker`)

**Current:** `time.After(100ms)` placeholder.

**Replacement:**
- `FleetManager` gains an `engine *EngineContext` field (injected at construction)
- `executeWorker(ctx, w)` does:
  1. Resolve provider via `engine.ProviderRegistry.GetByAlias(ctx, w.Provider)` with fallback to default
  2. Build `executor.Config{Provider, ToolRegistry, MaxIterations: 25, SecretRedactor}`
  3. System prompt: `"You are fleet worker %s. Execute this task step."` + step description
  4. Call `executor.Execute(ctx, cfg, systemPrompt, w.StepId, w.Id)`
  5. Map `executor.Result.Status` → worker status; `Result.Error` → `w.Error`
- Worker model comes from existing `ModelForStep()` routing

**Stub 2: Fleet StartFleet Decomposition (`service.go:StartFleet`)**

**Current:** `steps := []string{req.PlanId}` — hardcoded single step.

**Replacement:**
- Load plan: `plan := s.plans.Get(req.PlanId)`
- Extract step descriptions: `for _, step := range plan.Steps { if step.Status != "skipped" { steps = append(steps, step.Description) } }`
- Pass descriptions (not IDs) as the task prompt for each worker
- Fallback to single-step if plan not found (backward compat)

## Stub 3: Team Agent Execution (`teams.go:run`)

**Current:** `time.Sleep(50ms)` + hardcoded "acknowledged" messages.

**Replacement:**
- `TeamManager` gains an `engine *EngineContext` field
- `run()` executes agents sequentially (orchestrator first, then workers):
  1. Orchestrator agent: `executor.Execute(ctx, cfg, orchestratorPrompt, task, orchID)` where orchestratorPrompt includes "decompose this task and provide instructions for workers"
  2. For each worker: `executor.Execute(ctx, cfg, workerPrompt, orchestratorResult + workerRole, workerID)` — orchestrator's output becomes worker context
  3. Stream real `AgentMessage` events as agents complete (content = `executor.Result.Content`)
  4. Mark agents completed/failed based on `executor.Result.Status`
- Provider resolved per-agent: `agent.provider` field → `GetByAlias()`, fallback to `req.OrchestratorProvider`, fallback to default

## Stub 4: Approval Actor (`actors.go:ApprovalActor.Receive`)

**Current:** Immediately responds `{Approved: false, Reason: "no TUI response within timeout"}`.

**Replacement:**
- New `ApprovalGate` struct in `daemon/approval_gate.go`:
  ```go
  type ApprovalGate struct {
      mu      sync.Mutex
      pending map[string]chan ApprovalResponse
  }
  func (g *ApprovalGate) Request(requestID string) <-chan ApprovalResponse
  func (g *ApprovalGate) Resolve(requestID string, approved bool, reason string) bool
  ```
- `ApprovalActor.Receive(ApprovalRequest)`:
  1. Send request to gate: `ch := gate.Request(msg.RequestID)`
  2. Wait on channel with timeout (30m default from config)
  3. Reply with result or timeout denial
- `ApprovalGate` is wired to `Service.permGate` — when TUI sends `RespondToPermission`, it calls `gate.Resolve()`
- Implements `executor.Approver` interface for integration with `executor.Execute()`

## Stub 5: Cron Tick Handler (`service.go:NewCronScheduler callback`)

**Current:** Empty `func(sessionID, command string) {}`.

**Replacement:**
```go
svc.cron = NewCronScheduler(engine.DB, func(sessionID, command string) {
    if strings.HasPrefix(command, "/") {
        // Parse and execute slash command (future: needs command registry)
        log.Printf("cron: slash command %q for session %s (not yet wired)", command, sessionID)
        return
    }
    // Inject as user message into session chat
    go func() {
        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
        defer cancel()
        if err := svc.handleChat(ctx, sessionID, command, nil); err != nil {
            log.Printf("cron tick: %v", err)
        }
    }()
})
```
- For non-streaming injection (no active TUI), `handleChat` with `nil` stream skips event sending — needs a `noopStream` adapter that discards events
- Fire `hooks.Run(OnCronTick, ...)` in the callback

## Wiring Gap 1: Code Reviewer Sub-Session

**Current:** `/review` injects diff as plain chat message. No dedicated agent.

**Replacement:**
- New `StartReview` RPC (or reuse `SendMessage` with sentinel like compact):
  ```go
  const reviewSentinel = "\x00review\x00"
  ```
- `handleChat` detects sentinel, calls `handleReview(ctx, sessionID, reviewDiff, stream)`
- `handleReview`:
  1. Load code-reviewer builtin agent definition
  2. Create a sub-session (temp, not persisted) or use current session
  3. Build `executor.Config` with reviewer's tools, model override (sonnet), max 5 iterations
  4. System prompt from `code-reviewer.yaml` + "Review this diff:" + diff content
  5. Call `executor.Execute()` and stream result tokens back via the chat event stream
- TUI `review.go` sends `reviewSentinel + diff` as the message content
- `chat.go` parses: if starts with reviewSentinel, split and route to `handleReview`

## Wiring Gap 2: Per-Model Context Limits

**Current:** `const defaultModelLimit = 200000` hardcoded in `chat.go`.

**Replacement:**
- Add `ModelContextLimits` to config:
  ```go
  type ContextConfig struct {
      // ... existing fields ...
      ModelLimits map[string]int `yaml:"model_limits"`
  }
  ```
- Default limits map in `DefaultConfig()`:
  ```go
  ModelLimits: map[string]int{
      "claude-opus-4-6":    1000000,
      "claude-sonnet-4-6":  200000,
      "claude-haiku-4-5":   200000,
      "gpt-4o":             128000,
      "gpt-4o-mini":        128000,
  }
  ```
- `chat.go` looks up: `modelLimit := contextCfg.ModelLimits[session.Model]` with fallback to 200000
- `ShouldCompress` uses the looked-up limit

## Wiring Gap 3: Hook Call Site Wiring

**Current:** 8 lifecycle events defined in `hooks.go` but not fired from managers.

**Replacement:** Managers need a `*hooks.HookConfig` field and fire hooks at the right points:
- `PlanManager.Approve()` → `hooks.Run(PrePlan, {"plan_id": planID})`
- `PlanManager.UpdateStep()` when plan completes → `hooks.Run(PostPlan, {"plan_id": planID})`
- `FleetManager.StartFleet()` → `hooks.Run(PreFleet, {"fleet_id": fleetID})`
- `FleetManager.runFleet()` after wg.Wait → `hooks.Run(PostFleet, {"fleet_id": fleetID})`
- `TeamManager.run()` per agent spawn → `hooks.Run(OnAgentSpawn, {"agent_name", "agent_role"})`
- `TeamManager.run()` per agent complete → `hooks.Run(OnAgentComplete, {"agent_name"})`
- `chat.go` when ShouldCompress triggers → `hooks.Run(OnTokenLimit, {"tokens_used", "tokens_limit"})`
- Cron tick handler → `hooks.Run(OnCronTick, {"cron_id", "session_id", "command"})`

## Testing Strategy

All tests use mock providers (no real LLM calls). Tests validate:
1. **Fleet integration** — mock provider, verify N workers execute, status streaming works, kill works
2. **Team integration** — mock provider, verify orchestrator+worker execute sequentially, messages routed
3. **Approval gate** — request approval, resolve from test goroutine, verify blocking + timeout
4. **Cron injection** — create cron with short interval, verify message appears in session history
5. **Review session** — send review sentinel, verify executor called with reviewer agent config
6. **Compression limits** — verify per-model lookup, verify fallback
7. **Hook wiring** — mock hook config, verify all 8 events fire from call sites
8. **CI pipeline** — ensure `go test -race ./...` passes, lint clean

## Files Changed

| File | Change |
|---|---|
| `go.mod` | workflow-plugin-agent v0.4.1 → v0.5.9 |
| `internal/daemon/fleet.go` | Add engine field, real executeWorker |
| `internal/daemon/teams.go` | Add engine field, real run() with executor |
| `internal/daemon/actors.go` | Wire ApprovalActor to ApprovalGate |
| `internal/daemon/approval_gate.go` | NEW — ApprovalGate struct implementing executor.Approver |
| `internal/daemon/service.go` | Fix StartFleet decomposition, real cron tick handler, noopStream |
| `internal/daemon/chat.go` | Review sentinel handling, per-model limits, hook firing |
| `internal/daemon/engine.go` | Pass hooks config to EngineContext |
| `internal/config/config.go` | Add ModelLimits to ContextConfig |
| `internal/daemon/fleet_integration_test.go` | NEW — fleet with mock provider |
| `internal/daemon/teams_integration_test.go` | NEW — team with mock provider |
| `internal/daemon/approval_gate_test.go` | NEW — approval gate unit + timeout tests |
| `internal/daemon/cron_integration_test.go` | NEW — cron tick injection |
| `internal/daemon/review_test.go` | NEW — review sentinel routing |
| `internal/daemon/compression_limits_test.go` | NEW — per-model limit lookup |
| `internal/daemon/hooks_wiring_test.go` | NEW — all 8 events fire |
