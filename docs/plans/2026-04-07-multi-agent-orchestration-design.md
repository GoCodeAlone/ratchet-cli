# Multi-Agent Orchestration with Blackboard — Design

**Date:** 2026-04-07
**Repos:** ratchet-cli + workflow-plugin-agent
**Goal:** Local LLM orchestrator coordinates Claude Code and Copilot via Blackboard, with MCP integration for direct tool access and full transcript logging.

## Overview

A 3-agent team where:
- **Orchestrator** (Ollama local LLM) — designs, delegates, reviews. Has full Blackboard + send_message tools.
- **Claude Code** (interactive PTY) — implements. Gets Blackboard state via prompt injection AND native MCP tools.
- **Copilot CLI** (interactive PTY) — reviews. Gets Blackboard state via prompt injection AND native MCP tools (`~/.copilot/mcp-config.json`).

All messaging is directed (explicit `to` field). No broadcast. Full transcript logged.

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                     ratchet daemon (mesh)                     │
│                                                              │
│  ┌──────────────┐    Blackboard     ┌──────────────────────┐ │
│  │  Orchestrator │◄──(sections)───►│   Transcript Logger  │ │
│  │  (Ollama LLM) │   plan/code/    │   watches all BB     │ │
│  │               │   reviews/      │   writes + messages  │ │
│  │  Native tools:│   status/       └──────────────────────┘ │
│  │  bb_read/write│   artifacts                              │
│  │  bb_list      │                                          │
│  │  send_message │                                          │
│  └───────┬───────┘                                          │
│          │ send_message(to: "claude_code" | "copilot")      │
│          ▼                                                  │
│  ┌──────────────────┐       ┌──────────────────┐           │
│  │  BB Bridge        │       │  BB Bridge        │           │
│  │  (claude_code)    │       │  (copilot)        │           │
│  │                   │       │                   │           │
│  │  Inbound: inject  │       │  Inbound: inject  │           │
│  │  BB state + task  │       │  BB state + task  │           │
│  │  into PTY prompt  │       │  into PTY prompt  │           │
│  │                   │       │                   │           │
│  │  Outbound: parse  │       │  Outbound: parse  │           │
│  │  response, write  │       │  response, write  │           │
│  │  to BB section    │       │  to BB section    │           │
│  └────────┬──────────┘       └────────┬──────────┘           │
│           │ PTY session               │ PTY session          │
│           ▼                           ▼                      │
│  ┌──────────────────┐       ┌──────────────────┐           │
│  │  Claude Code      │       │  Copilot CLI      │           │
│  │  (interactive)    │       │  (interactive)    │           │
│  │                   │       │                   │           │
│  │  + MCP server:    │       │  (prompt injection │           │
│  │  ratchet-bb with  │       │   only — no MCP)  │           │
│  │  bb_read/write/   │       │                   │           │
│  │  bb_list tools    │       │                   │           │
│  └──────────────────┘       └──────────────────┘           │
└──────────────────────────────────────────────────────────────┘
```

## Component Details

### 1. BB Bridge (`internal/mesh/bb_bridge.go`)

Sits between the mesh Router and PTY providers. Translates mesh messages into rich prompts for PTY agents.

**Inbound (orchestrator → PTY agent):**
- Receives `send_message` via Router inbox
- Reads relevant Blackboard sections referenced in the message
- Formats structured prompt:
  ```
  [TEAM CONTEXT]
  You are "claude_code" (implementation role) in a 3-agent team.
  The orchestrator is directing you. Other team member: "copilot" (review).

  [BLACKBOARD — plan]
  design: "Build a URL shortener with..."

  [TASK FROM orchestrator]
  Implement the URL shortener based on the design above.
  When done, end your response with [RESULT: <one-line summary>].
  ```
- Sends via PTY provider's `Stream()` method

**Outbound (PTY agent → Blackboard):**
- Captures full response text
- Writes to `artifacts/<agent_name>/<auto_key>` with response content
- Detects `[RESULT: ...]` marker → writes to `status/<agent_name>` = "done"
- Sends result summary back to orchestrator via Router

### 2. Blackboard MCP Server (`internal/mcp/bb_mcp.go` + `cmd/ratchet/cmd_mcp.go`)

**Command:** `ratchet mcp blackboard --team-id <id>`

Runs as a stdio MCP server (JSON-RPC). Claude Code launches it as a child process via MCP config.

**Tools exposed:**
- `bb_read` — `{section, key}` → entry value
- `bb_write` — `{section, key, value}` → confirmation
- `bb_list` — `{section?}` → list sections or keys

**Connection:** The MCP server connects to the ratchet daemon's Unix socket to access the team's Blackboard instance.

### 3. MCP Config Management

**On team start:**

For Claude Code (`.claude/mcp.json`):
1. Read existing file (backup if present)
2. Merge `ratchet-blackboard` entry:
   ```json
   {
     "mcpServers": {
       "ratchet-blackboard": {
         "command": "ratchet",
         "args": ["mcp", "blackboard", "--team-id", "<team-id>"],
         "env": {}
       }
     }
   }
   ```
3. Write merged config

For Copilot CLI (`~/.copilot/mcp-config.json`):
1. Read existing file (backup if present)
2. Merge entry using Copilot's config format:
   ```json
   {
     "servers": {
       "ratchet-blackboard": {
         "command": "ratchet",
         "args": ["mcp", "blackboard", "--team-id", "<team-id>"]
       }
     }
   }
   ```
3. Write merged config

**On team complete:**
1. Remove `ratchet-blackboard` entry from both config files
2. Restore originals (or delete if they didn't exist before)

**Verification:**
- Claude Code: `/mcp` → shows `ratchet-blackboard` with bb_read, bb_write, bb_list
- Copilot: `/mcp` → shows `ratchet-blackboard` with same tools

### 4. Transcript Logger (`internal/mesh/transcript.go`)

Watches all Blackboard writes (via `bb.Watch()`) and Router messages.

**Format:**
```
[00:00.0] TEAM orchestrate STARTED — task: "Build email validator"
[00:01.2] BB WRITE plan/design by orchestrator rev=1
          | Design: Go function using regex...
[00:01.5] MSG orchestrator → claude_code (task)
          | Implement the email validator...
[00:15.3] BB WRITE artifacts/claude_code/email.go by claude_code rev=5
          | package main...
[00:15.4] MSG claude_code → orchestrator (result)
          | [RESULT: implemented email.go with tests]
[00:16.0] MSG orchestrator → copilot (task)
          | Review this code for correctness...
[00:25.1] BB WRITE reviews/copilot/round1 by copilot rev=8
          | Code is clean, one suggestion...
[00:26.0] BB WRITE artifacts/final by orchestrator rev=10
          | Final: approved
[00:26.1] TEAM orchestrate COMPLETED — 26.1s, 3 agents, 10 BB writes
```

**Storage:** `~/.ratchet/transcripts/<team-id>.log`

**Live view:** Streamed to TUI via team event channel.

### 5. Team Config (`internal/mesh/teams/orchestrate.yaml`)

```yaml
name: orchestrate
timeout: 15m
max_review_rounds: 2
agents:
  - name: orchestrator
    role: orchestrator
    provider: ollama
    model: qwen3:8b
    max_iterations: 30
    tools:
      - blackboard_read
      - blackboard_write
      - blackboard_list
      - send_message
    system_prompt: |
      You are the orchestrator of a 3-agent team. Your team members are:
      - "claude_code": An AI coding assistant. Strong at implementation.
      - "copilot": An AI coding assistant. Strong at code review.

      Your workflow:
      1. Analyze the task and design the approach
      2. Write your design to blackboard section "plan" key "design"
      3. Send implementation task to "claude_code" via send_message
      4. Wait — read blackboard "artifacts" to check for claude_code's output
      5. Send review task to "copilot" via send_message with the code
      6. Read blackboard "reviews" for copilot's feedback
      7. If changes needed, send feedback to "claude_code"
      8. When satisfied, write summary to blackboard "artifacts" key "final"
      9. Write "done" to blackboard "status" key "orchestrator"

      Rules:
      - Always specify "to" in send_message. Never broadcast.
      - Read blackboard before sending new tasks (check agent status).
      - Keep messages focused and specific.

  - name: claude_code
    role: implementation
    provider: claude_code
    max_iterations: 5
    tools:
      - blackboard_read
      - blackboard_write

  - name: copilot
    role: review
    provider: copilot_cli
    max_iterations: 5
    tools:
      - blackboard_read
      - blackboard_write
```

### 6. LocalNode Changes

`LocalNode.Run()` needs to detect when a node's provider is a PTY provider and use the BB Bridge instead of direct `executor.Execute`:

```go
if isPTYProvider(node.provider) {
    return runWithBBBridge(ctx, node, task, bb, inbox, outbox)
}
// existing executor.Execute path for native LLM providers
```

The BB Bridge acts as the agent loop for PTY nodes — it receives messages from inbox, formats prompts, sends to PTY, parses responses, writes to BB.

### 7. Test Scenario

**Task:** "Build a Go function that validates email addresses using regex, with unit tests."

**Expected flow:**
1. Orchestrator designs approach → BB `plan/design`
2. Orchestrator → claude_code: "Implement this design"
3. Claude Code writes code → BB `artifacts/claude_code/email.go`
4. Orchestrator → copilot: "Review this code"
5. Copilot writes review → BB `reviews/copilot/round1`
6. Orchestrator synthesizes → BB `artifacts/final`

**Manual test after:**
```bash
# Run the orchestration team
ratchet team start --config orchestrate \
  "Build a Go function that validates email addresses with regex and unit tests"

# Watch live
ratchet team status <team-id>

# View transcript
cat ~/.ratchet/transcripts/<team-id>.log
```

## Files to Create/Modify

| File | Repo | Action |
|---|---|---|
| `internal/mesh/bb_bridge.go` | ratchet-cli | **New** — BB Bridge for PTY agents |
| `internal/mesh/transcript.go` | ratchet-cli | **New** — Transcript logger |
| `internal/mesh/teams/orchestrate.yaml` | ratchet-cli | **New** — Team config |
| `internal/mcp/bb_mcp.go` | ratchet-cli | **New** — BB MCP server |
| `cmd/ratchet/cmd_mcp.go` | ratchet-cli | **Modify** — Add `ratchet mcp blackboard` |
| `internal/mesh/local_node.go` | ratchet-cli | **Modify** — PTY provider detection + bridge |
| `internal/mesh/config.go` | ratchet-cli | **Modify** — Register `orchestrate` builtin |
| `internal/mesh/bb_bridge_test.go` | ratchet-cli | **New** — Unit tests |
| `internal/mesh/transcript_test.go` | ratchet-cli | **New** — Unit tests |
| `internal/mesh/orchestration_test.go` | ratchet-cli | **New** — Integration test |

## Execution Order

```
1. Transcript logger (no deps)
2. BB Bridge (needs transcript)
3. BB MCP server + ratchet mcp command
4. MCP config management (write/restore .claude/mcp.json)
5. LocalNode PTY detection + bridge wiring
6. orchestrate team config
7. Integration test
8. Manual test instructions
```
