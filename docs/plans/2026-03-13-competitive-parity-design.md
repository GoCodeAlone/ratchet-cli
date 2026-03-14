# ratchet-cli Competitive Parity Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Upgrade ratchet-cli to workflow v0.3.40 and achieve competitive parity with GitHub Copilot CLI and Claude Code — adding plan mode, fleet mode, team mode, context compression, code review agent, cron scheduling, enhanced hooks, bundled MCP, per-agent model routing, session actors, a unified job control panel, comprehensive tests, and interactive QA validation.

**Architecture:** The daemon (`internal/daemon/`) gains new RPCs and state management for plans, fleets, teams, cron jobs, and actors. The TUI (`internal/tui/`) adds new pages and components for plan display, job control, and fleet/team views. The workflow engine (v0.3.40) provides `step.parallel` for fleet fan-out, actors for session state, and `step.graphql`/`step.json_parse`/`step.secret_fetch` for richer tool capabilities. All new features are testable in isolation and validated by QA agents.

**Tech Stack:** Go 1.26, Bubbletea v2, gRPC, protobuf, SQLite (modernc.org), workflow v0.3.40, goakt v4 (actors)

---

## Phase 1: Engine Upgrade (v0.3.30 → v0.3.40)

### Task 1: Upgrade workflow dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1:** Update workflow version in go.mod
```bash
cd /Users/jon/workspace/ratchet-cli
go get github.com/GoCodeAlone/workflow@v0.3.40
go mod tidy
```

**Step 2:** Fix any breaking imports from the interfaces refactor. Search for `module.PipelineContext`, `module.StepResult`, `module.PipelineStep` — these may need to become `interfaces.PipelineContext` etc. Check:
```bash
grep -rn "module\.PipelineContext\|module\.StepResult\|module\.PipelineStep" --include="*.go"
```
Fix any hits by updating import paths. If ratchet-cli doesn't directly reference these types (likely — it uses ratchetplugin which abstracts them), no changes needed.

**Step 3:** Build and test
```bash
go build ./...
go test ./... -count=1
```

**Step 4:** Commit
```bash
git add go.mod go.sum
git commit -m "chore: upgrade workflow v0.3.30 → v0.3.40"
```

### Task 2: Upgrade ratchet and workflow-plugin-agent dependencies

**Files:**
- Modify: `go.mod`

**Step 1:** Check if ratchet and workflow-plugin-agent have new releases compatible with workflow v0.3.40:
```bash
cd /Users/jon/workspace/ratchet && git tag --sort=-v:refname | head -3
cd /Users/jon/workspace/workflow-plugin-agent && git tag --sort=-v:refname | head -3
```

Update to latest compatible versions:
```bash
cd /Users/jon/workspace/ratchet-cli
go get github.com/GoCodeAlone/ratchet@latest
go get github.com/GoCodeAlone/workflow-plugin-agent@latest
go mod tidy
```

**Step 2:** Build and test
```bash
go build ./... && go test ./... -count=1
```

**Step 3:** Commit
```bash
git add go.mod go.sum
git commit -m "chore: upgrade ratchet and workflow-plugin-agent to latest"
```

---

## Phase 2: Plan Mode

### Task 3: Add proto messages and RPCs for plan mode

**Files:**
- Modify: `internal/proto/ratchet.proto`

**Step 1:** Add plan-related messages after the existing TeamStatus message (~L220):
```protobuf
// Plan mode
message PlanStep {
  string id = 1;
  string description = 2;
  string status = 3;  // "pending", "in_progress", "completed", "failed", "skipped"
  repeated string files = 4;
  string error = 5;
}

message Plan {
  string id = 1;
  string session_id = 2;
  string goal = 3;
  repeated PlanStep steps = 4;
  string status = 5;  // "proposed", "approved", "executing", "completed", "rejected"
  string created_at = 6;
}

message ApprovePlanReq {
  string session_id = 1;
  string plan_id = 2;
  repeated string skip_steps = 3;  // step IDs to skip
}

message RejectPlanReq {
  string session_id = 1;
  string plan_id = 2;
  string feedback = 3;
}
```

Add `plan_proposed` to the `ChatEvent` oneof:
```protobuf
Plan plan_proposed = 10;
PlanStep plan_step_update = 11;
```

Add RPCs to the service:
```protobuf
// Plan mode
rpc ApprovePlan(ApprovePlanReq) returns (stream ChatEvent);
rpc RejectPlan(RejectPlanReq) returns (Empty);
```

**Step 2:** Regenerate proto:
```bash
cd /Users/jon/workspace/ratchet-cli
protoc --go_out=. --go-grpc_out=. internal/proto/ratchet.proto
```

**Step 3:** Build to verify generation:
```bash
go build ./...
```

**Step 4:** Commit
```bash
git add internal/proto/
git commit -m "proto: add plan mode messages and RPCs"
```

### Task 4: Implement plan mode in daemon

**Files:**
- Create: `internal/daemon/plans.go`
- Modify: `internal/daemon/service.go`

**Step 1:** Create `internal/daemon/plans.go`:
```go
package daemon

import (
    "sync"
    "time"
    "github.com/google/uuid"
    pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

type PlanManager struct {
    mu    sync.RWMutex
    plans map[string]*pb.Plan // planID -> Plan
}

func NewPlanManager() *PlanManager {
    return &PlanManager{plans: make(map[string]*pb.Plan)}
}

func (pm *PlanManager) Create(sessionID, goal string, steps []*pb.PlanStep) *pb.Plan {
    pm.mu.Lock()
    defer pm.mu.Unlock()
    plan := &pb.Plan{
        Id:        uuid.New().String(),
        SessionId: sessionID,
        Goal:      goal,
        Steps:     steps,
        Status:    "proposed",
        CreatedAt: time.Now().UTC().Format(time.RFC3339),
    }
    pm.plans[plan.Id] = plan
    return plan
}

func (pm *PlanManager) Get(planID string) *pb.Plan {
    pm.mu.RLock()
    defer pm.mu.RUnlock()
    return pm.plans[planID]
}

func (pm *PlanManager) ForSession(sessionID string) *pb.Plan {
    pm.mu.RLock()
    defer pm.mu.RUnlock()
    for _, p := range pm.plans {
        if p.SessionId == sessionID && (p.Status == "proposed" || p.Status == "executing") {
            return p
        }
    }
    return nil
}

func (pm *PlanManager) Approve(planID string, skipSteps []string) error {
    pm.mu.Lock()
    defer pm.mu.Unlock()
    p, ok := pm.plans[planID]
    if !ok {
        return fmt.Errorf("plan %q not found", planID)
    }
    skip := make(map[string]bool, len(skipSteps))
    for _, s := range skipSteps {
        skip[s] = true
    }
    for _, step := range p.Steps {
        if skip[step.Id] {
            step.Status = "skipped"
        }
    }
    p.Status = "approved"
    return nil
}

func (pm *PlanManager) Reject(planID string) error {
    pm.mu.Lock()
    defer pm.mu.Unlock()
    p, ok := pm.plans[planID]
    if !ok {
        return fmt.Errorf("plan %q not found", planID)
    }
    p.Status = "rejected"
    return nil
}

func (pm *PlanManager) UpdateStep(planID, stepID, status, errMsg string) {
    pm.mu.Lock()
    defer pm.mu.Unlock()
    p, ok := pm.plans[planID]
    if !ok {
        return
    }
    for _, step := range p.Steps {
        if step.Id == stepID {
            step.Status = status
            step.Error = errMsg
            break
        }
    }
    // Check if all steps done
    allDone := true
    for _, step := range p.Steps {
        if step.Status != "completed" && step.Status != "skipped" && step.Status != "failed" {
            allDone = false
            break
        }
    }
    if allDone {
        p.Status = "completed"
    }
}
```

**Step 2:** Add `plans *PlanManager` field to `Service` struct in `service.go`. Initialize in `NewService`.

**Step 3:** Implement `ApprovePlan` and `RejectPlan` RPCs in `service.go`:
- `ApprovePlan`: calls `pm.Approve()`, then starts executing the plan steps sequentially, streaming `plan_step_update` events for each step
- `RejectPlan`: calls `pm.Reject()`, publishes feedback to the session

**Step 4:** Write tests in `internal/daemon/plans_test.go`:
```go
func TestPlanManager_CreateAndGet(t *testing.T) { ... }
func TestPlanManager_Approve(t *testing.T) { ... }
func TestPlanManager_Reject(t *testing.T) { ... }
func TestPlanManager_UpdateStep(t *testing.T) { ... }
func TestPlanManager_ForSession(t *testing.T) { ... }
```

**Step 5:** Run tests:
```bash
go test ./internal/daemon/ -run TestPlanManager -v
```

**Step 6:** Commit
```bash
git add internal/daemon/plans.go internal/daemon/plans_test.go internal/daemon/service.go
git commit -m "feat: implement plan mode in daemon (PlanManager + RPCs)"
```

### Task 5: Add plan mode TUI components

**Files:**
- Create: `internal/tui/components/plan.go`
- Modify: `internal/tui/pages/chat.go`
- Modify: `internal/tui/commands/commands.go`

**Step 1:** Create `internal/tui/components/plan.go` — a `PlanView` component that renders a plan as a numbered task list with status indicators (✓/✗/⟳/○), approve/reject keybinds.

**Step 2:** Add `/plan` and `/approve` and `/reject` slash commands in `commands.go`.

**Step 3:** In `chat.go`, handle `Plan` messages from the ChatEvent stream — display the PlanView, handle approve/reject key events.

**Step 4:** Write component test `internal/tui/components/plan_test.go`.

**Step 5:** Commit
```bash
git commit -m "feat: add plan mode TUI components and slash commands"
```

---

## Phase 3: Fleet Mode (Parallel Agent Decomposition)

### Task 6: Add fleet proto messages and RPCs

**Files:**
- Modify: `internal/proto/ratchet.proto`

Add:
```protobuf
message StartFleetReq {
  string session_id = 1;
  string plan_id = 2;  // decompose this plan into fleet workers
  int32 max_workers = 3;
}

message FleetWorker {
  string id = 1;
  string name = 2;
  string step_id = 3;  // which plan step this worker handles
  string status = 4;   // "pending", "running", "completed", "failed"
  string model = 5;
  string provider = 6;
  string error = 7;
}

message FleetStatus {
  string fleet_id = 1;
  string session_id = 2;
  repeated FleetWorker workers = 3;
  string status = 4;  // "running", "completed", "failed"
  int32 completed = 5;
  int32 total = 6;
}
```

Add `fleet_status` to ChatEvent oneof. Add RPCs:
```protobuf
rpc StartFleet(StartFleetReq) returns (stream ChatEvent);
rpc GetFleetStatus(FleetStatusReq) returns (FleetStatus);
rpc KillFleetWorker(KillFleetWorkerReq) returns (Empty);
```

**Step 2:** Regenerate proto, build, commit.

### Task 7: Implement fleet orchestration in daemon

**Files:**
- Create: `internal/daemon/fleet.go`
- Create: `internal/daemon/fleet_test.go`
- Modify: `internal/daemon/service.go`

Fleet mode:
1. Takes an approved plan
2. Identifies independent steps (no blockedBy dependencies)
3. Spawns N worker goroutines (capped at `max_workers`)
4. Each worker creates a sub-session, executes its step, reports back
5. Lead goroutine collects results, streams FleetStatus updates
6. Uses the workflow engine's `step.parallel` pattern internally

**Tests:** TestFleetManager_Decompose, TestFleetManager_WorkerLifecycle, TestFleetManager_KillWorker

### Task 8: Add fleet TUI components

**Files:**
- Create: `internal/tui/components/fleet.go`
- Modify: `internal/tui/pages/chat.go`
- Modify: `internal/tui/commands/commands.go`

Fleet panel shows: worker name, assigned step, status (spinner/checkmark/X), model used, elapsed time. Add `/fleet` slash command.

---

## Phase 4: Team Mode (Named Agent Messaging)

### Task 9: Implement team mode RPCs (currently unimplemented)

**Files:**
- Create: `internal/daemon/teams.go`
- Create: `internal/daemon/teams_test.go`
- Modify: `internal/daemon/service.go`

The proto already has `StartTeam`, `GetTeamStatus`, `AgentSpawned`, `AgentMessage` — but the RPCs return `Unimplemented`. Implement:
- `StartTeam`: creates a team with named agents, each with role/model/provider/tools
- `GetTeamStatus`: returns all agents and their statuses
- Agent message routing: agents send messages to each other via `MessageSendTool`
- Team view streams `AgentSpawned` and `AgentMessage` events

**Tests:** TestTeamManager_Create, TestTeamManager_AgentLifecycle, TestTeamManager_DirectMessage

### Task 10: Wire up team TUI page

**Files:**
- Modify: `internal/tui/pages/team.go`

The `TeamModel` struct exists but likely shows placeholder content. Wire it to the daemon's team RPCs:
- Show agent cards with name, role, model, status
- Show message flow between agents
- Allow killing individual agents

---

## Phase 5: Context Auto-Compression

### Task 11: Add token counting and compression

**Files:**
- Create: `internal/daemon/compression.go`
- Create: `internal/daemon/compression_test.go`
- Modify: `internal/daemon/service.go` (SendMessage handler)

**Implementation:**
1. Track token counts per session (input + output) in a `TokenTracker` struct
2. After each message exchange, check if total tokens exceed threshold (configurable, default 90% of model's context window)
3. When threshold hit, summarize older messages using a fast model call (Haiku-equivalent)
4. Replace old messages with summary, preserving: system prompt, last N messages (default 10), active tool results
5. Stream a `context_compressed` event to the TUI

Add `/compact` slash command for manual compression.

**Tests:** TestTokenTracker_ThresholdDetection, TestCompression_SummarizeMessages, TestCompression_PreservesRecent

### Task 12: Add config for compression settings

**Files:**
- Modify: `internal/config/config.go`

Add to Config:
```go
Context ContextConfig `yaml:"context"`
```
```go
type ContextConfig struct {
    CompressionThreshold float64 `yaml:"compression_threshold"` // 0.0-1.0, default 0.9
    PreserveMessages     int     `yaml:"preserve_messages"`     // default 10
    CompressionModel     string  `yaml:"compression_model"`     // default "haiku"
}
```

---

## Phase 6: Code Review Agent

### Task 13: Add built-in code-reviewer agent definition

**Files:**
- Create: `internal/agent/builtins/code-reviewer.yaml`
- Modify: `internal/agent/definitions.go`

Embed a built-in agent definition:
```yaml
name: code-reviewer
role: Reviews code changes for quality, security, and correctness
model: sonnet  # good balance of speed and quality
tools:
  - CodeReviewTool
  - CodeDiffReviewTool
  - CodeComplexityTool
  - FileReadTool
  - GitDiffTool
  - GitLogStatsTool
max_iterations: 5
system_prompt: |
  You are a code reviewer. Analyze diffs and files for:
  - Security vulnerabilities (injection, auth bypass, etc.)
  - Logic errors and edge cases
  - Code style and naming conventions
  - Test coverage gaps
  Output structured review with Critical/Important/Minor categories.
```

Add `/review` slash command that invokes this agent on the current git diff or a specified file/PR.

**Tests:** TestBuiltinAgents_CodeReviewerLoads, TestReviewCommand_Parse

---

## Phase 7: Cron/Loop Scheduling

### Task 14: Add cron job proto messages and RPCs

**Files:**
- Modify: `internal/proto/ratchet.proto`

```protobuf
message CronJob {
  string id = 1;
  string session_id = 2;
  string schedule = 3;     // cron expression or duration (e.g., "5m", "*/10 * * * *")
  string command = 4;      // slash command or prompt to execute
  string status = 5;       // "active", "paused", "stopped"
  string last_run = 6;
  string next_run = 7;
  int32 run_count = 8;
}

message CreateCronReq {
  string session_id = 1;
  string schedule = 2;
  string command = 3;
}

message CronJobList { repeated CronJob jobs = 1; }
message PauseCronReq { string job_id = 1; }
message StopCronReq { string job_id = 1; }
```

RPCs:
```protobuf
rpc CreateCron(CreateCronReq) returns (CronJob);
rpc ListCrons(Empty) returns (CronJobList);
rpc PauseCron(PauseCronReq) returns (Empty);
rpc ResumeCron(PauseCronReq) returns (Empty);
rpc StopCron(StopCronReq) returns (Empty);
```

### Task 15: Implement cron scheduler in daemon

**Files:**
- Create: `internal/daemon/cron.go`
- Create: `internal/daemon/cron_test.go`

**Implementation:**
- `CronScheduler` struct with goroutine per job, `time.Ticker` for intervals, `robfig/cron/v3` for cron expressions
- Jobs persist to SQLite `cron_jobs` table (survive daemon restarts)
- On tick, inject the command as a message into the session's chat stream
- Support pause/resume/stop lifecycle

Add `/loop <interval> <command>` and `/cron <expr> <command>` slash commands, plus `/cron list`, `/cron stop <id>`, `/cron pause <id>`.

**Tests:** TestCronScheduler_CreateAndTick, TestCronScheduler_Pause, TestCronScheduler_PersistRestart

---

## Phase 8: Enhanced Hooks

### Task 16: Add new lifecycle hook events

**Files:**
- Modify: `internal/hooks/hooks.go`

Add new events:
```go
PrePlan             Event = "pre-plan"
PostPlan            Event = "post-plan"
PreFleet            Event = "pre-fleet"
PostFleet           Event = "post-fleet"
OnTokenLimit        Event = "on-token-limit"
OnAgentSpawn        Event = "on-agent-spawn"
OnAgentComplete     Event = "on-agent-complete"
OnCronTick          Event = "on-cron-tick"
```

Add template data keys: `"plan_id"`, `"fleet_id"`, `"agent_name"`, `"agent_role"`, `"cron_id"`, `"tokens_used"`, `"tokens_limit"`.

**Tests:** TestHookConfig_NewEvents, TestHookConfig_TemplateExpansion_NewKeys

---

## Phase 9: Bundled MCP Discovery

### Task 17: Auto-discover and register CLI tool MCP servers

**Files:**
- Create: `internal/mcp/discovery.go`
- Create: `internal/mcp/discovery_test.go`
- Modify: `internal/daemon/engine.go`

**Implementation:**
1. On daemon start, check if `gh` CLI is available (`exec.LookPath("gh")`). If so, register GitHub tools (issues, PRs, repos) via the existing `RegisterMCP` mechanism in ratchetplugin.
2. Check for `docker` CLI → register container tools.
3. Check for `kubectl` → register k8s tools (supplement existing K8s tools).
4. Store discovery results in config so subsequent starts skip re-discovery.
5. Add `/mcp list`, `/mcp enable <name>`, `/mcp disable <name>` slash commands.

**Tests:** TestMCPDiscovery_GHFound, TestMCPDiscovery_NoCLIs, TestMCPDiscovery_DockerFound

---

## Phase 10: Per-Agent Model Routing

### Task 18: Add model routing to agent definitions and fleet workers

**Files:**
- Modify: `internal/agent/definitions.go`
- Modify: `internal/daemon/fleet.go`
- Modify: `internal/config/config.go`

**Implementation:**
1. `AgentDefinition` already has `Provider` and `Model` fields — ensure they're respected when creating sub-sessions for agents.
2. Fleet workers use per-step model assignment: simple steps get fast/cheap models (Haiku), complex steps get capable models (Opus/Sonnet).
3. Add `ModelRouting` config for auto-classification:
```go
type ModelRouting struct {
    SimpleTaskModel  string `yaml:"simple_task_model"`  // default: provider's cheapest
    ComplexTaskModel string `yaml:"complex_task_model"` // default: provider's most capable
    ReviewModel      string `yaml:"review_model"`       // default: mid-tier
}
```
4. `/cost` command shows per-agent token usage breakdown.

**Tests:** TestModelRouting_SimpleTask, TestModelRouting_ComplexTask, TestModelRouting_CostBreakdown

---

## Phase 11: Session Actors (Engine Integration)

### Task 19: Wire actor system into daemon

**Files:**
- Modify: `internal/daemon/engine.go`
- Create: `internal/daemon/actors.go`
- Create: `internal/daemon/actors_test.go`

**Implementation:**
1. Add actor system initialization in `NewEngineContext` — create an `actor.system` and `actor.pool` for sessions.
2. Each session gets a persistent actor (identity = session ID) that maintains conversation state.
3. Approval workflows use `step.actor_ask` — blocks until user responds via the TUI permission prompt.
4. Actor mailboxes backed by SQLite for persistence across daemon restarts.

**Tests:** TestActorSystem_SessionActor, TestActorSystem_ApprovalFlow, TestActorSystem_Persistence

---

## Phase 12: Unified Job Control Panel

### Task 20: Add job control proto messages

**Files:**
- Modify: `internal/proto/ratchet.proto`

```protobuf
message Job {
  string id = 1;
  string type = 2;      // "session", "fleet_worker", "team_agent", "cron", "tool_exec"
  string name = 3;
  string status = 4;    // "running", "paused", "completed", "failed", "pending"
  string session_id = 5;
  string started_at = 6;
  string elapsed = 7;
  map<string, string> metadata = 8;  // type-specific details
}

message JobList { repeated Job jobs = 1; }
message PauseJobReq { string job_id = 1; }
message KillJobReq { string job_id = 1; }
```

RPCs:
```protobuf
rpc ListJobs(Empty) returns (JobList);
rpc PauseJob(PauseJobReq) returns (Empty);
rpc ResumeJob(PauseJobReq) returns (Empty);
rpc KillJob(KillJobReq) returns (Empty);
```

### Task 21: Implement job registry in daemon

**Files:**
- Create: `internal/daemon/jobs.go`
- Create: `internal/daemon/jobs_test.go`
- Modify: `internal/daemon/service.go`

**Implementation:**
`JobRegistry` aggregates all active work across the daemon:
- Sessions from `SessionManager`
- Fleet workers from `FleetManager`
- Team agents from `TeamManager`
- Cron jobs from `CronScheduler`
- Active tool executions (tracked via hook on `on-tool-call`)

Each source registers a `JobProvider` interface:
```go
type JobProvider interface {
    ActiveJobs() []*pb.Job
    PauseJob(id string) error
    KillJob(id string) error
}
```

`ListJobs` aggregates from all providers. `PauseJob`/`KillJob` route to the correct provider by job type.

**Tests:** TestJobRegistry_Aggregate, TestJobRegistry_KillSession, TestJobRegistry_KillFleetWorker, TestJobRegistry_PauseCron

### Task 22: Add job control TUI panel

**Files:**
- Create: `internal/tui/components/jobpanel.go`
- Create: `internal/tui/components/jobpanel_test.go`
- Modify: `internal/tui/app.go`

**Implementation:**
- `Ctrl+J` toggles the job panel (similar to `Ctrl+S` for sidebar)
- Panel shows a table: Type | Name | Status | Elapsed | Actions
- Arrow keys navigate, `p` to pause, `k` to kill, `Enter` to focus
- `/jobs` slash command also opens the panel
- Job panel auto-refreshes every 2 seconds via daemon `ListJobs` polling

**Tests:** TestJobPanel_Render, TestJobPanel_Navigation, TestJobPanel_PauseAction, TestJobPanel_KillAction

---

## Phase 13: Comprehensive Testing

### Task 23: Integration tests for all new RPCs

**Files:**
- Create: `internal/daemon/integration_plan_test.go`
- Create: `internal/daemon/integration_fleet_test.go`
- Create: `internal/daemon/integration_team_test.go`
- Create: `internal/daemon/integration_cron_test.go`
- Create: `internal/daemon/integration_jobs_test.go`

Each file tests the full gRPC flow: create daemon → call RPC → verify response → verify side effects.

**Test patterns:**
- Plan: Create session → send message that triggers plan → verify plan proposed → approve → verify execution → verify completion
- Fleet: Create plan → start fleet → verify workers spawned → verify parallel execution → verify results merged
- Team: Start team → verify agents spawned → send direct message → verify delivery → kill agent → verify cleanup
- Cron: Create cron job → wait for tick → verify command executed → pause → verify no ticks → resume → stop
- Jobs: Start multiple work types → ListJobs → verify all visible → kill one → verify removed

### Task 24: TUI component tests for all new components

**Files:**
- Create: `internal/tui/components/plan_test.go`
- Create: `internal/tui/components/fleet_test.go`
- Create: `internal/tui/components/jobpanel_test.go`

Test rendering, key handling, state transitions for each new component.

### Task 25: Provider tests (currently zero test coverage)

**Files:**
- Create: `internal/provider/models_test.go`
- Create: `internal/provider/auth_test.go`

Test `ListModels` for each provider type. Test auth flows with mock HTTP servers.

---

## Phase 14: Interactive QA Validation

### Task 26: QA agent validates plan mode end-to-end

**QA Process:**
1. Start ratchet daemon
2. Create a session
3. Send a message that should trigger plan mode (e.g., "refactor the auth module into separate files")
4. Verify plan is proposed with numbered steps
5. Approve the plan
6. Verify steps execute in order with status updates
7. Verify completion
8. Test rejection flow: send message → plan proposed → reject with feedback → verify feedback delivered
9. Test `/plan` command
10. Verify `/approve` and `/reject` slash commands work

### Task 27: QA agent validates fleet mode end-to-end

**QA Process:**
1. Create a plan with independent steps
2. Start fleet with max_workers=3
3. Verify workers spawn in parallel
4. Verify independent steps run concurrently
5. Verify results merged correctly
6. Test killing a fleet worker mid-execution
7. Verify fleet status shows correct counts

### Task 28: QA agent validates team mode end-to-end

**QA Process:**
1. Start a team with 2 agents (implementer + reviewer)
2. Verify both agents spawn
3. Send a task to implementer
4. Verify implementer processes and messages reviewer
5. Verify reviewer receives message
6. Test killing an agent
7. Verify team status updates

### Task 29: QA agent validates job control panel

**QA Process:**
1. Start multiple work items: a session, a cron job, a fleet
2. Open job panel with Ctrl+J
3. Verify all items visible with correct types and statuses
4. Pause a cron job → verify status changes to "paused"
5. Kill a fleet worker → verify removed from panel
6. Kill a session → verify removed
7. Close panel with Ctrl+J again

### Task 30: QA agent validates cron/loop scheduling

**QA Process:**
1. Create a cron job: `/loop 5s /sessions` (list sessions every 5 seconds)
2. Wait 15 seconds
3. Verify at least 2-3 ticks executed
4. Pause the job: `/cron pause <id>`
5. Wait 10 seconds → verify no new ticks
6. Resume: `/cron resume <id>`
7. Verify ticks resume
8. Stop: `/cron stop <id>`
9. Verify job removed from `/cron list`

### Task 31: QA agent validates context compression

**QA Process:**
1. Create a session
2. Send many messages to fill context (programmatic — send 50+ short messages)
3. Verify token count increases (visible in status bar)
4. When threshold hit, verify compression triggers automatically
5. Verify older messages replaced with summary
6. Verify recent messages preserved
7. Test `/compact` manual compression

### Task 32: QA agent validates code review agent

**QA Process:**
1. Create a test repo with some code changes (use `git init` + `git add` + `git commit` in a temp dir)
2. Make a change and stage it
3. Run `/review`
4. Verify code-reviewer agent spawns
5. Verify structured output with Critical/Important/Minor sections
6. Verify file:line references are accurate

---

## Execution Order

```
Phase 1 (Tasks 1-2)  ──→  Phase 2 (Tasks 3-5)
                      ├──→ Phase 3 (Tasks 6-8)
                      ├──→ Phase 4 (Tasks 9-10)
                      ├──→ Phase 5 (Tasks 11-12)
                      ├──→ Phase 6 (Task 13)
                      ├──→ Phase 7 (Tasks 14-15)
                      ├──→ Phase 8 (Task 16)
                      ├──→ Phase 9 (Task 17)
                      └──→ Phase 10 (Task 18)
Phase 11 (Task 19)        # depends on Phase 1
Phase 12 (Tasks 20-22)    # depends on Phases 2-7,11
Phase 13 (Tasks 23-25)    # depends on all implementation phases
Phase 14 (Tasks 26-32)    # depends on Phase 13
```

**Parallel groups (after Phase 1):**
- Group A: Phases 2, 5, 6, 8 (plan mode, compression, code review, hooks)
- Group B: Phases 3, 4 (fleet + team — related but independent)
- Group C: Phases 7, 9, 10 (cron, MCP, model routing)
- Group D: Phase 11 (actors — needs engine upgrade)
- Group E: Phase 12 (job control — needs all above)
- Group F: Phase 13 (testing — needs all implementation)
- Group G: Phase 14 (QA — needs all tests passing)
