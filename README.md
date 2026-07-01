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
ratchet sessions browse ID  # Browse branch tree interactively
ratchet sessions summary ID "short label"
                            # Update the branch summary shown in lineage views
ratchet sessions compactions ID
                            # Show compaction records and archive sessions
ratchet acp                 # Run ratchet as an ACP stdio agent
ratchet acp client exec --command ./agent "prompt"
                            # Run one prompt against an external ACP agent
ratchet acp client exec --command ./agent --session work --no-wait "prompt"
                            # Queue one pending ACP client prompt locally
ratchet acp client sessions list
                            # List persisted ACP client sessions
ratchet acp client sessions show ID
                            # Show persisted ACP client session metadata
ratchet acp client status ID
                            # Show ACP client session status
ratchet acp client cancel ID
                            # Cancel an active or queued ACP client prompt
ratchet daemon status       # Check daemon
ratchet provider list       # List providers
ratchet team start "task"   # Start agent team
```

In the TUI, press `ctrl+b` or submit `/tree` to open the in-place session
branch browser. Use arrow keys or `j`/`k` to move, left/right or `h`/`l` to
collapse and expand, `Enter` to switch to a branch, `r` to refresh, and `Esc`
to return to chat. Switching through the tree or sidebar rebuilds chat for the
selected branch before new sends are accepted.

The v0.18.0 release includes the ACP client foundation and continues publishing
Windows amd64/arm64 zip artifacts alongside Linux and macOS archives.

## Harness Modes

| Mode | Command | Credential-free smoke |
|---|---|---|
| TUI | `ratchet` | Starts a daemon-backed interactive session. |
| One-shot | `ratchet -p "prompt"` | Uses the configured default provider. |
| Daemon | `HOME="$(mktemp -d)" ratchet daemon status` | Runs credential-free when pointed at a temp home. |
| ACP | `ratchet acp` | Exposes the agent over ACP stdio JSON-RPC; prompt smoke is covered by `TestACPStdioPromptSmoke`. |
| ACP client | `ratchet acp client exec --command ./agent "prompt"` | Drives an external ACP agent over stdio; binary smoke covers exec, persisted sessions, status, no-wait, and cancel. |
| MCP | `ratchet mcp blackboard` / `ratchet mcp daemon` | Exposes standalone blackboard or daemon-backed session/project/blackboard/team MCP tools over stdio. |
| Team | `ratchet team start "task"` | Uses daemon team orchestration with configured providers. |

See [docs/harness-emulation.md](docs/harness-emulation.md) for credential-free
mock provider recipes, and [docs/competitor-parity.md](docs/competitor-parity.md)
for the dated source-backed parity matrix.
