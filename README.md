# ratchet

Interactive AI agent CLI with multi-provider support, multi-agent orchestration, and a rich terminal UI.

## Install

```sh
go install github.com/GoCodeAlone/ratchet-cli/cmd/ratchet@latest
```

Or download a binary from [Releases](https://github.com/GoCodeAlone/ratchet-cli/releases).
Release artifacts include Linux and macOS tar.gz archives and Windows zip
archives for amd64 and arm64. Windows installer packages are not published yet.

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
ratchet sessions history ID # Show persisted message history
ratchet sessions clone ID   # Clone a session with full visible history
ratchet sessions fork ID --at MESSAGE_ID
                            # Fork a session through a specific message
ratchet sessions tree ID    # Show root/parent/fork lineage
ratchet daemon status       # Check daemon
ratchet provider list       # List providers
ratchet team start "task"   # Start agent team
```

## Harness Modes

| Mode | Command | Credential-free smoke |
|---|---|---|
| TUI | `ratchet` | Starts a daemon-backed interactive session. |
| One-shot | `ratchet -p "prompt"` | Uses the configured default provider. |
| Daemon | `HOME="$(mktemp -d)" ratchet daemon status` | Runs credential-free when pointed at a temp home. |
| ACP | `ratchet acp` | Exposes the agent over ACP stdio JSON-RPC; prompt smoke is covered by `TestACPStdioPromptSmoke`. |
| MCP | `ratchet mcp blackboard` / `ratchet mcp daemon` | Exposes standalone blackboard or daemon-backed session/project/blackboard/team MCP tools over stdio. |
| Team | `ratchet team start "task"` | Uses daemon team orchestration with configured providers. |

See [docs/harness-emulation.md](docs/harness-emulation.md) for credential-free
mock provider recipes, and [docs/competitor-parity.md](docs/competitor-parity.md)
for the dated source-backed parity matrix.
