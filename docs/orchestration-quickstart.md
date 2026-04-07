# Orchestration Quickstart

Run a 3-agent team: Ollama orchestrator + Claude Code (implementation) + Copilot CLI (review).

## Prerequisites

- **Ollama** installed with `qwen3:8b` pulled: `ollama pull qwen3:8b`
- **Claude Code** authenticated: `claude` runs without prompting for auth
- **GitHub Copilot CLI** authenticated: `gh copilot` runs without prompting

## 1. Add Providers

```bash
ratchet provider setup ollama
ratchet provider setup claude-code
ratchet provider setup copilot-cli
```

Verify: `ratchet provider list` — all three should show `ready`.

## 2. Run the Team

```bash
ratchet team start --config orchestrate "Build a Go function that validates email addresses using regex, with unit tests"
```

The team config is at `internal/mesh/teams/orchestrate.yaml`. Ratchet will:
1. Launch the Ollama orchestrator with Blackboard + messaging tools
2. Open PTY sessions for Claude Code and Copilot CLI
3. Inject an MCP server (`ratchet-blackboard`) into Claude Code's `.claude/mcp.json`
4. Start the transcript logger; logs go to `~/.ratchet/transcripts/<team-id>.log`

The command prints a `<team-id>` on start.

## 3. Watch Progress

```bash
ratchet team status <team-id>
```

Shows agent states, current BB sections, and iteration counts. Refresh manually or pipe to `watch`:

```bash
watch -n 2 ratchet team status <team-id>
```

## 4. View Transcript

```bash
cat ~/.ratchet/transcripts/<team-id>.log
```

Lines are prefixed with relative timestamps:
- `[00:12.3] BB WRITE plan/design by orchestrator` — Blackboard write
- `[00:15.1] MSG orchestrator → claude_code (task)` — directed message
- `[00:45.7] TEAM orchestrate COMPLETED` — team done

## 5. Verify MCP in Claude Code Terminal

Inside the Claude Code PTY session, run:

```
/mcp
```

`ratchet-blackboard` should appear in the server list. If not, see Troubleshooting below.

## 6. Custom Team Config

Copy and modify `internal/mesh/teams/orchestrate.yaml`:

```yaml
name: my-team
timeout: 10m
max_review_rounds: 1
agents:
  - name: orchestrator
    role: orchestrator
    provider: ollama
    model: llama3.2:3b       # swap model
    max_iterations: 20
    tools: [blackboard_read, blackboard_write, blackboard_list, send_message]
    system_prompt: |
      You are the orchestrator...

  - name: claude_code
    role: implementation
    provider: claude_code
    max_iterations: 3
    tools: [blackboard_read, blackboard_write]

  - name: copilot
    role: review
    provider: copilot_cli
    max_iterations: 3
    tools: [blackboard_read, blackboard_write]
```

Run it with:

```bash
ratchet team start --config ./my-team.yaml "your task here"
```

## Troubleshooting

**Provider not found**
```
error: provider "ollama" not registered
```
Run `ratchet provider setup ollama` and ensure Ollama is running (`ollama serve`).

**MCP not loading in Claude Code**
- Check `~/.claude/mcp.json` — `ratchet-blackboard` entry should be present while the team runs.
- Confirm `ratchet` is on `$PATH`: `which ratchet`.
- Run `/mcp` inside the Claude Code session to reload.

**Team timeout**
The default timeout is 15 minutes (`orchestrate.yaml`). Increase it for complex tasks:
```yaml
timeout: 30m
```
Or pass `--timeout 30m` to `ratchet team start`.

**Copilot not responding**
Copilot CLI uses prompt injection (no MCP). If responses are empty, check `gh copilot` auth:
```bash
gh auth status
gh copilot explain "test"
```
