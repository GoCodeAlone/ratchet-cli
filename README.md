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
                            # Append an ACP client prompt to a local FIFO queue
ratchet acp client queue work --json
                            # Inspect queued ACP client prompts
ratchet acp client drain work --command ./agent --max 2
                            # Drain queued prompts through one ACP session
ratchet acp client sessions list
                            # List persisted ACP client sessions
ratchet acp client sessions show ID
                            # Show persisted ACP client session metadata
ratchet acp client sessions export ID --output session.archive.json
                            # Export a portable ratchet-cli archive v1 JSON file
ratchet acp client sessions import session.archive.json --session imported
                            # Import an archive as a new local ACP client session
ratchet acp client compare --command ./agent --command ./other-agent "prompt"
                            # Run one prompt serially across multiple ACP agents
ratchet acp client flow run flow.json --input-json '{"task":"x"}' --command ./agent
                            # Run a JSON v1 ACP/compute flow
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

The TUI trust commands are daemon-backed at runtime: `/mode <mode>` switches
between `conservative`, `permissive`, `locked`, `sandbox`, and `custom`;
`/trust list` shows effective daemon rules; `/trust allow "pattern" [--scope scope]`
and `/trust deny "pattern" [--scope scope]` add runtime rules; `/trust reset`
clears runtime slash-command rules and rebuilds from config defaults. These
commands do not edit config files or delete persisted permission grants.

Persistent trust grants are explicit. Use
`ratchet trust persist allow|deny "pattern" [--scope scope]` or
`/trust persist allow|deny "pattern" [--scope scope]` to store durable grants
in the daemon's local state database through
`workflow-plugin-agent/policy.PermissionStore`. Use `ratchet trust grants` or
`/trust grants` to list them, and
`ratchet trust revoke "pattern" [--scope scope]` or
`/trust revoke "pattern" [--scope scope]` to remove one. Treat grant listings
as sensitive local policy metadata because patterns can reveal local paths,
commands, or workflow conventions.

The ACP client queue persists prompt text under the user's XDG state directory.
Do not use `--no-wait` for prompts that should not be written to local disk.
ACP client archives are explicit JSON exports and can contain prompt text,
responses, summaries, and queue history. Treat exported archives as sensitive
conversation data.

The v0.20.0 release adds ACP client archive import/export, serial compare, and
JSON v1 flow commands, and continues publishing Windows amd64/arm64 zip
artifacts alongside Linux and macOS archives.

ACP client archive v1 JSON is a ratchet-cli portable format with ACPX-shaped
metadata, not a raw ACPX JSON-RPC event log. JSON v1 flows support `acp` and
`compute` nodes, template prompts, shared session handles, and persisted run
bundles; ACPX TypeScript flow runtime compatibility remains deferred.

## ACP Client Examples

```sh
# Export and import an ACP client session archive.
ratchet acp client sessions export work --output work.archive.json
ratchet acp client sessions import work.archive.json --session work-copy

# Compare two external ACP agents with one prompt.
ratchet acp client compare \
  --command ./agent-a \
  --command ./agent-b \
  "Summarize the current project risks"

# Run a JSON v1 ACP/compute flow.
cat > flow.json <<'JSON'
{
  "format_version": 1,
  "start_at": "draft",
  "nodes": [
    {
      "id": "draft",
      "type": "acp",
      "prompt": "Draft a brief answer for {{ .Input.topic }}",
      "session": "shared"
    },
    {
      "id": "result",
      "type": "compute",
      "select": "draft"
    }
  ],
  "edges": [{"from": "draft", "to": "result"}]
}
JSON
ratchet acp client flow run flow.json \
  --input-json '{"topic":"release readiness"}' \
  --command ./agent \
  --json
```

## Harness Modes

| Mode | Command | Credential-free smoke |
|---|---|---|
| TUI | `ratchet` | Starts a daemon-backed interactive session. |
| One-shot | `ratchet -p "prompt"` | Uses the configured default provider. |
| Daemon | `HOME="$(mktemp -d)" ratchet daemon status` | Runs credential-free when pointed at a temp home. |
| ACP | `ratchet acp` | Exposes the agent over ACP stdio JSON-RPC; prompt smoke is covered by `TestACPStdioPromptSmoke`. |
| ACP client | `ratchet acp client exec --command ./agent "prompt"` | Drives an external ACP agent over stdio; binary smoke covers exec, persisted sessions, FIFO `--no-wait` queue, queue inspection, drain, status, cancel, archive export/import, serial compare, and JSON v1 flows. |
| MCP | `ratchet mcp blackboard` / `ratchet mcp daemon` | Exposes standalone blackboard or daemon-backed session/project/blackboard/team MCP tools over stdio, including active-team `team_message`. |
| Team | `ratchet team start "task"` | Uses daemon team orchestration with configured providers. |

See [docs/harness-emulation.md](docs/harness-emulation.md) for credential-free
mock provider recipes, and [docs/competitor-parity.md](docs/competitor-parity.md)
for the dated source-backed parity matrix.
