# Team Lifecycle & Multi-Team Project Orchestration — Design

**Date:** 2026-04-07
**Repo:** ratchet-cli
**Goal:** Dynamic team composition from CLI flags, multi-team projects with cross-team communication, human-in-the-loop with autoresponder, project/task tracker, and full session lifecycle (attach/detach/rejoin/kill).

## 1. CLI Composition — No YAML Required

```bash
# Multiple --agent flags (name:provider[:model])
ratchet team start --agent lead:ollama:qwen3:8b --agent coder:claude_code "task"

# Single --agents flag (comma-separated)
ratchet team start --agents lead:ollama,coder:claude_code,reviewer:copilot "task"

# First agent is orchestrator by default; override:
ratchet team start --orchestrator lead --agent lead:ollama --agent coder:claude_code "task"

# Name the team
ratchet team start --name my-team --agent lead:ollama --agent coder:claude_code "task"

# Explicit BB mode
ratchet team start --bb shared --agent lead:ollama --agent coder:claude_code "task"
```

## 2. Config Convention

**Default paths (searched in order):**
- `.ratchet/teams/` (project-level, in working directory)
- `~/.ratchet/teams/` (user-level)

**Formats:** YAML (`.yaml`/`.yml`) and JSON (`.json`) both supported, detected by extension.

**Multi-team config file:**
```yaml
project: email-service
teams:
  - name: design
    agents:
      - name: architect
        provider: ollama
        model: qwen3:8b
        role: orchestrator
      - name: researcher
        provider: claude_code
    blackboard: shared

  - name: dev
    agents:
      - name: lead
        provider: ollama
        model: qwen3:8b
        role: orchestrator
      - name: coder
        provider: claude_code
      - name: reviewer
        provider: copilot
    blackboard: shared

  - name: qa
    agents:
      - name: qa-lead
        provider: ollama
        role: orchestrator
      - name: tester
        provider: copilot
    blackboard: shared

  - name: oversight
    agents:
      - name: director
        provider: ollama
        model: qwen3:8b
        role: orchestrator
    blackboard: orchestrator
```

**Save/load:**
```bash
# Save a team composition
ratchet team save my-team --agent lead:ollama:qwen3:8b --agent coder:claude_code
# Writes to .ratchet/teams/my-team.yaml

# Save to explicit path
ratchet team save --output ~/teams/review-team.yaml --agent ...

# Load by name (searches .ratchet/teams/ then ~/.ratchet/teams/)
ratchet team start --config my-team "task"

# Load by path
ratchet team start --config ./custom-team.yaml "task"
```

## 3. Project Registry

Multiple projects with multiple teams, all concurrent in the same daemon.

```bash
# Start by project name (searches config dirs for matching project: field)
ratchet project start email-service

# Start with explicit config
ratchet project start --config .ratchet/teams/full-stack.yaml

# Start with no args (uses only config if unambiguous)
ratchet project start

# List all
ratchet project list
ratchet team list
ratchet team list --project email-service

# Manage
ratchet project pause email-service
ratchet project resume email-service
ratchet project kill email-service
```

Projects own the shared Blackboard, task tracker, and cross-team config. Ad-hoc teams (no config file) get an implicit anonymous project.

## 4. Team Identity & Multi-Team Support

Auto-generated short ID (e.g., `t-3a7f`) plus optional user-assigned name.

```bash
ratchet team list                      # all active/completed teams
ratchet team rename t-3a7f email-dev   # assign a name
ratchet team status t-3a7f             # agents, BB state, recent activity
ratchet team status email-dev          # works with name too
```

When only one team is active, team ID is implicit for modification commands. When multiple are active, ID/name is required.

## 5. Dynamic Add/Remove (mid-session)

```bash
# From CLI
ratchet team add t-3a7f debugger:claude_code
ratchet team remove t-3a7f reviewer
ratchet team replace t-3a7f coder --provider copilot

# From TUI (inside attached session)
/team add debugger:claude_code
/team remove reviewer
/team list
```

On add: new node registered with Router, BB `team/members` updated, orchestrator notified.
On remove: agent context cancelled, unregistered from Router, BB updated, orchestrator notified.
New agents get current BB state injected on first prompt.

## 6. Session Attach/Detach

```bash
ratchet team attach t-3a7f            # observe mode (default)

# Inside attached session:
/join                                  # become participant node "user"
/observe                               # back to read-only
/steer "focus on error handling"       # directive to orchestrator
@coder "use table-driven tests"        # direct message to specific agent
/team add helper:claude_code           # dynamic modification
Ctrl+D or /detach                      # detach — team keeps running

# Rejoin later
ratchet team attach t-3a7f

# List/kill
ratchet team list
ratchet team kill t-3a7f
```

**Observe mode:** Live activity stream (BB writes, messages, agent output). Can modify team and send directives. Agents don't see you.

**Participant mode (`/join`):** You become mesh node "user". Agents can `send_message` to "user". Your messages go to orchestrator by default, or `@agent_name` for direct. `/observe` exits.

**On reconnect:** Show status indicators — idle time, pending human messages, last activity.

## 7. Human-in-the-Loop

**Message queue:** When a team needs human input and no human is attached, messages queue in BB `pending_human/<team>` section. The requesting agent blocks (same pattern as `permissionGate`).

**On attach:**
```
⚠ 2 messages waiting:
  [12:03] architect: "REST or gRPC for the API?"
  [12:05] reviewer: "Security issue in auth.go:45. Approve fix?"
```

User responds inline. Response unblocks the waiting agent.

**Autoresponder** (`.ratchet/autorespond.yaml`):
```yaml
rules:
  - match: approval
    action: approve

  - match: "which.*approach"
    action: reply
    message: "Use the simpler approach unless there's a clear performance reason."

  - match: "*"
    action: queue        # default: wait for human
```

```bash
ratchet team pending                   # show all queued messages
ratchet team respond t-3a7f            # interactive respond
```

## 8. Idle Notifications

When a team has no BB activity for 5 minutes:
- Daemon logs warning
- OS notification: macOS (`osascript`), Linux (`notify-send`), Windows (`powershell New-BurntToastNotification`)
- On reconnect: `⚠ Team t-3a7f idle for 12m (last: BB write artifacts/code by coder)`

No auto-kill. Teams run until done or explicitly killed.

## 9. Blackboard Modes & Cross-Team Communication

Configurable per team via `blackboard:` field in config or `--bb` flag:

| Mode | Behavior |
|---|---|
| `shared` (default) | Project-level BB. Team writes to `<team>/` namespace, reads all namespaces. |
| `isolated` | Team-private BB. No cross-team visibility. |
| `orchestrator` | Own isolated BB + read-only view of all other teams' BBs. |
| `bridge:<t1>,<t2>` | Shared BB between named teams only. |

**Cross-team handoff protocol:** Teams write to `handoffs/<from>-to-<to>` sections. Receiving team's orchestrator watches for new entries.

**Directive protocol:** Oversight team writes to `directives/<team>`. Team orchestrators check their directive section each iteration.

**Example flow (design → dev → QA → oversight):**
```
oversight → directives/design: "Design email validator API"
design reads directive → designs → handoffs/design-to-dev: {spec}
dev reads handoff → implements → handoffs/dev-to-qa: {code, tests}
qa reads handoff → tests → handoffs/qa-to-oversight: {approved} OR handoffs/qa-to-dev: {bugs}
oversight reads result → next directive or completion
```

## 10. Project & Task Tracker

**Hybrid:** SQLite for persistence, BB notifications for real-time awareness, dedicated tools for structured queries.

**New mesh tools:**
- `project_status` — `{project?}` → summary (name, teams, completion %)
- `task_create` — `{title, project, assigned_team, priority}` → task ID
- `task_claim` — `{task_id, agent_name}` → claimed (fails if already claimed)
- `task_update` — `{task_id, status, notes}` → updated
- `task_list` — `{project?, team?, status?, limit?}` → compact list (default limit 10)
- `task_get` — `{task_id}` → full detail

**Context management:** `task_list` returns compact one-line summaries (ID + title + status + assignee). Agents use `task_get` only when they need full detail. Prevents context swamping.

**BB notifications:** Task create/claim/complete writes a one-line entry to `notifications/<team>`. Rotated at 20 entries to prevent unbounded growth.

**Schema:**
```sql
CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    config_path TEXT,
    status TEXT DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    project_id TEXT REFERENCES projects(id),
    title TEXT NOT NULL,
    description TEXT,
    assigned_team TEXT,
    claimed_by TEXT,
    status TEXT DEFAULT 'pending',
    priority INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## 11. Remove Builtin `orchestrate` Config

Delete the hardcoded `orchestrate` preset. Keep `code-gen` as the only builtin reference example. Users build teams via flags or saved configs.

## Files to Create/Modify

| File | Action |
|---|---|
| `cmd/ratchet/cmd_team.go` | **Modify** — --agent/--agents/--name/--bb flags, add/remove/replace/attach/detach/list/kill/save/rename |
| `cmd/ratchet/cmd_project.go` | **New** — project start/list/pause/resume/kill |
| `internal/daemon/teams.go` | **Modify** — dynamic add/remove, team registry with IDs/names, attach/detach |
| `internal/daemon/projects.go` | **New** — project registry, multi-team lifecycle |
| `internal/daemon/human_gate.go` | **New** — human message queue, autoresponder |
| `internal/daemon/notifications.go` | **New** — OS-native notifications (macOS/Linux/Windows) |
| `internal/proto/ratchet.proto` | **Modify** — new RPCs for teams, projects, human gate |
| `internal/mesh/mesh.go` | **Modify** — AddNode/RemoveNode on running team, roster BB writes |
| `internal/mesh/config.go` | **Modify** — remove orchestrate builtin, add JSON support, multi-team configs |
| `internal/mesh/project_bb.go` | **New** — project-level BB with namespace isolation and cross-team modes |
| `internal/mesh/tracker.go` | **New** — task tracker (SQLite + BB notifications) |
| `internal/mesh/tracker_tools.go` | **New** — project_status, task_create/claim/update/list/get tools |
| `internal/tui/team_view.go` | **Modify** — /team commands, observe/join, @agent, pending messages |
| `internal/mesh/teams/orchestrate.yaml` | **Delete** |

## Execution Order

```
Phase 1: Foundation
  - Config convention + multi-team config parsing
  - Project registry
  - Remove orchestrate builtin
  - --agent/--agents CLI flags

Phase 2: Lifecycle
  - Team ID/naming
  - Dynamic add/remove
  - Attach/detach with observe/join modes

Phase 3: Human-in-the-Loop
  - Message queue + pause gate
  - Autoresponder
  - OS notifications

Phase 4: Cross-Team
  - BB modes (shared/isolated/orchestrator/bridge)
  - Handoff + directive protocols
  - Project-level BB with namespacing

Phase 5: Tracker
  - SQLite schema + tracker
  - Tracker tools (project_status, task_*)
  - BB notification rotation
```
