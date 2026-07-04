# ratchet

Interactive AI agent CLI with multi-provider support, multi-agent orchestration, and a rich terminal UI.

## Features

- **Auto-daemon pattern**: Single daemon process serves multiple terminal TUI clients
- **Multi-provider**: Anthropic, OpenAI, Google Gemini, Ollama support
- **Multi-agent**: Orchestrate teams of agents with role definitions
- **Workflow engine**: Built on the GoCodeAlone/workflow engine with 54 ratchetplugin tools
- **Plugin support**: Load external workflow plugins at runtime
- **Harness protocols**: ACP stdio agent mode plus MCP blackboard and daemon-backed session/project tools
- **Optional retros**: Disabled-by-default retro analyzer for local action suggestions and upstream PR instructions

## Usage

```sh
ratchet                     # Launch interactive TUI
ratchet "fix the bug"       # Implicit chat mode
ratchet chat "prompt"       # Explicit chat mode
ratchet sessions            # Manage sessions
ratchet daemon status       # Check daemon
ratchet provider list       # List providers
ratchet acp                 # Run as an ACP stdio agent
ratchet mcp daemon          # Run daemon-backed MCP tools
ratchet config show         # Show configuration, including retro settings
```

## Install

```sh
go install github.com/GoCodeAlone/ratchet-cli/cmd/ratchet@latest
```

GitHub Releases also publish Linux and macOS tar.gz archives plus Windows zip
archives for amd64 and arm64. Windows installer packages are not published yet.
Release safety is covered by the GoReleaser snapshot release-check, draft
release asset postcheck, tap preflight, and tap postcheck gates. Windows
executable runtime remains deferred pending approved runner changes.

Harness evidence boundaries live in [docs/harness-emulation.md](docs/harness-emulation.md);
policy and deferred automation boundaries live in
[docs/policy-matrix.md](docs/policy-matrix.md).
