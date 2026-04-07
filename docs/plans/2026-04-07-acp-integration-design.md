# ACP Integration + PTY Refinement — Design

**Date:** 2026-04-07
**Repos:** ratchet-cli + workflow-plugin-agent
**Goal:** Expose ratchet as an ACP agent, use ACP to orchestrate other agents, integrate ACP registry for discovery, and get Copilot interactive PTY fully working.

## Part 1: Ratchet as ACP Agent

Implement `acp.Agent` from `github.com/coder/acp-go-sdk` so any ACP-compatible editor (JetBrains, Zed, Neovim, Kiro) can launch ratchet as its AI backend.

### New Command: `ratchet acp`

Runs ratchet as an ACP stdio agent (reads JSON-RPC from stdin, writes to stdout):

```bash
# Editor launches ratchet as an ACP agent
ratchet acp
```

### ACP ↔ Ratchet Mapping

| ACP Method | Ratchet Implementation |
|---|---|
| `initialize` | Return capabilities (streaming, tools, permissions) |
| `session/new` | `CreateSession` RPC |
| `session/load` | `AttachSession` RPC |
| `session/prompt` | `SendMessage` RPC → stream `ChatEvent` back as `session/update` |
| `session/cancel` | Cancel context on active stream |
| `session/set_mode` | Map to plan mode toggle |
| `session/set_model` | `UpdateProviderModel` RPC |
| `session/request_permission` | Map to ratchet's `PermissionRequest` event |
| `fs/read_text_file` | Delegate to tool registry `file_read` |
| `fs/write_text_file` | Delegate to tool registry `file_write` |
| `terminal/create` | Delegate to tool registry `bash` |

### Files
- `cmd/ratchet/cmd_acp.go` — `ratchet acp` command, stdio wiring
- `internal/acp/agent.go` — ACP agent implementation wrapping ratchet Service
- `internal/acp/convert.go` — ChatEvent ↔ ACP session/update conversion

## Part 2: ACP Client for Agent Orchestration

Implement `acp.Client` to launch and orchestrate other ACP-compliant agents as sub-agents in ratchet teams.

### How It Works

Instead of (or alongside) PTY providers, ratchet can launch any ACP agent as a provider:

```go
type ACPProvider struct {
    cmd      *exec.Cmd
    conn     *acp.ClientSideConnection
    session  string
}
```

`Chat()`: `session/new` → `session/prompt` → collect response
`Stream()`: `session/new` → `session/prompt` → stream `session/update` events

### Advantages over PTY
- Structured JSON-RPC messages (no screen scraping)
- Proper session management (resume, fork)
- Permission handling built into protocol
- Multimodal content blocks (text, image, audio)

### Provider Registration

Register as provider types:
- `acp:<agent-name>` (e.g., `acp:goose`, `acp:custom-agent`)

### Files
- `workflow-plugin-agent/genkit/acp_provider.go` — ACP client provider
- `workflow-plugin-agent/genkit/acp_provider_test.go` — tests

## Part 3: ACP Registry for Discovery

Use the ACP Registry to discover available agents at runtime.

### Discovery Flow
1. On daemon startup (or on demand), query local ACP registry
2. For each discovered agent, register as an available provider type
3. User can select discovered agents via `ratchet provider list`

### Files
- `internal/acp/registry.go` — ACP registry client
- `internal/daemon/engine.go` — Wire discovery into startup

## Part 4: Copilot Interactive PTY

Get Copilot's vt10x interactive PTY working through ratchet's Stream() path.

### Root Cause
Copilot's response extraction fails in ratchet because:
1. The `DetectResponseEnd` fires too early (system `●` lines match before actual response)
2. The `extractResponse` collects system lines instead of response content
3. The working directory differs between standalone test and ratchet daemon

### Fix
- Track screen state across calls (remember what was on screen before the message)
- Extract only NEW `●` lines that weren't present before
- Use screen diff instead of absolute content scanning

### Files
- `workflow-plugin-agent/genkit/pty_provider.go` — Screen diff response extraction
- `workflow-plugin-agent/genkit/pty_adapters.go` — Copilot adapter refinement

## Part 5: Comprehensive PTY Testing

Validate interactive PTY with complex scenarios for both Claude Code and Copilot.

### Test Scenarios
1. **Claude Code multi-turn**: Send 3 messages, verify context maintained
2. **Claude Code long response**: Ask for a multi-function Python module, verify complete code
3. **Claude Code team task**: Run a team with Claude Code as the provider
4. **Copilot simple**: Math query, verify clean response
5. **Copilot multi-turn**: Two related questions, verify context
6. **Copilot code generation**: Ask for a function, verify clean code output

### Files
- `internal/tui/pty_cli_integration_test.go` — Updated integration tests

## Execution Order

```
Part 4 (Copilot PTY fix) ──┐
Part 5 (PTY testing)      ──┤
Part 1 (ACP agent)        ──┼→ Release
Part 2 (ACP client)       ──┤
Part 3 (ACP registry)     ──┘
```

Parts 1-3 (ACP) and Parts 4-5 (PTY) can run in parallel.
