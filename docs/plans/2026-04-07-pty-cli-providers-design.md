# PTY CLI Providers — Design

**Date:** 2026-04-07
**Repos:** workflow-plugin-agent + ratchet-cli
**Goal:** Use PTY to drive AI provider CLI tools (Claude Code, Copilot, Codex, Gemini, Cursor) as provider.Provider backends, enabling ratchet to orchestrate across multiple providers using users' existing subscriptions.

## Architecture

A new PTY provider type in `workflow-plugin-agent/genkit/` that implements `provider.Provider` by driving CLI tools through a pseudo-terminal.

```
provider.Provider interface
    └─ ptyProvider
        ├─ Claude Code  (claude -p "...")
        ├─ Copilot CLI  (copilot -p "...")
        ├─ Codex CLI    (codex exec "...")
        ├─ Gemini CLI   (gemini -p "...")
        └─ Cursor Agent (agent -p "...")
```

Registered in ProviderRegistry as types: `claude_code`, `copilot_cli`, `codex_cli`, `gemini_cli`, `cursor_cli`.

## Provider Interface Implementation

### Chat() — Non-Interactive Mode
1. Run CLI with single-shot flag: `claude -p "message"`
2. Capture stdout as the response
3. Parse into `provider.Response{Content: stdout}`
4. Fast, stateless, no PTY needed

### Stream() — PTY Interactive Mode
1. Start CLI in a PTY: `pty.Start(exec.Command("claude"))`
2. Wait for prompt (detect REPL readiness via adapter)
3. Send message text + Enter
4. Read streamed output tokens until next prompt detected
5. Emit `StreamEvent{Type: "text", Text: chunk}` per chunk
6. Keep PTY alive for multi-turn

## CLI Tool Adapters

Each tool has different prompt formats, flags, and behavior:

| Tool | Type Name | Binary | Non-Interactive Flag | Install |
|---|---|---|---|---|
| Claude Code | `claude_code` | `claude` | `-p "msg"` | `curl -fsSL https://claude.ai/install.sh \| bash` |
| Copilot CLI | `copilot_cli` | `copilot` | `-p "msg"` | Platform installer |
| Codex CLI | `codex_cli` | `codex` | `exec "msg"` | `npm install -g @openai/codex` |
| Gemini CLI | `gemini_cli` | `gemini` | `-p "msg"` | `npm install -g @google/gemini-cli` |
| Cursor Agent | `cursor_cli` | `agent` | `-p "msg"` | `curl https://cursor.com/install -fsSL \| bash` |

Adapter interface:
```go
type CLIAdapter interface {
    Binary() string                     // binary name
    NonInteractiveArgs(msg string) []string  // args for -p mode
    DetectPrompt(output string) bool    // is the CLI ready for input?
    DetectResponseEnd(output string) bool // has the response finished?
    ParseResponse(raw string) string    // clean response text
}
```

## Provider Configuration

```go
type PTYProviderConfig struct {
    Tool     string // "claude_code", "copilot_cli", etc.
    Binary   string // override binary path
    WorkDir  string // working directory for CLI session
}
```

## Setup Commands

```
ratchet provider setup claude-code    # verify binary, health check, register
ratchet provider setup copilot-cli
ratchet provider setup codex-cli
ratchet provider setup gemini-cli
ratchet provider setup cursor-cli
```

Each: verify binary exists → run health check (`<tool> -p "say hi"`) → register as provider.

## Files

### workflow-plugin-agent
| File | Description |
|---|---|
| `genkit/pty_provider.go` | PTY provider implementing provider.Provider |
| `genkit/pty_adapters.go` | Per-tool CLIAdapter implementations |
| `genkit/pty_provider_test.go` | Unit tests with mock CLI binary |

### ratchet-cli
| File | Description |
|---|---|
| `cmd/ratchet/cmd_provider.go` | Setup commands for each CLI tool |
| `internal/tui/pty_cli_test.go` | Integration tests driving each real CLI |

## Testing Strategy

Each provider validated by actually running the CLI through PTY:
1. Verify binary exists on the machine
2. Send non-interactive request, verify response
3. Start interactive PTY session, multi-turn
4. Verify context maintained across turns
5. Test in team execution (agents use CLI provider as backend)

## Security Considerations

- PTY providers run CLI tools with the user's existing authentication
- No API keys stored — the CLI tools manage their own auth
- Working directory scoped per session (tools can only access files in workdir)
- Tool approval prompts from CLI tools are visible in PTY output
