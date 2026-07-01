# ratchet

Interactive AI agent CLI with multi-provider support, multi-agent orchestration, and a rich terminal UI.

## Install

```sh
go install github.com/GoCodeAlone/ratchet-cli/cmd/ratchet@latest
```

Or download a binary from [Releases](https://github.com/GoCodeAlone/ratchet-cli/releases).

## Features

- **Auto-daemon pattern**: Single daemon process serves multiple terminal TUI clients via gRPC over Unix socket
- **Multi-provider**: Anthropic, OpenAI, Google Gemini, Ollama support
- **Multi-agent**: Orchestrate teams of agents with role definitions
- **Workflow engine**: Built on the GoCodeAlone/workflow engine
- **Universal instruction files**: Loads CLAUDE.md, AGENTS.md, .cursorrules, .windsurfrules, RATCHET.md
- **Rich TUI**: Bubbletea v2 with streaming tokens, tool call display, permission prompts

## Usage

```sh
ratchet                     # Launch interactive TUI
ratchet "fix the bug"       # Implicit chat mode
ratchet chat "prompt"       # Explicit chat mode
ratchet sessions            # Manage sessions
ratchet daemon status       # Check daemon
ratchet provider list       # List providers
ratchet team start "task"   # Start agent team
```

## Harness Modes

| Mode | Command | Credential-free smoke |
|---|---|---|
| TUI | `ratchet` | Starts a daemon-backed interactive session. |
| One-shot | `ratchet -p "prompt"` | Uses the configured default provider. |
| Daemon | `ratchet daemon status` | Runs in a temp home without credentials. |
| ACP | `ratchet acp` | Exposes the agent over ACP stdio JSON-RPC. |
| MCP | `ratchet mcp blackboard` | Exposes standalone blackboard MCP tools over stdio. |
| Team | `ratchet team start "task"` | Uses daemon team orchestration with configured providers. |

See [docs/harness-emulation.md](docs/harness-emulation.md) for credential-free
mock provider recipes and parity notes.
