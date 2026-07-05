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
ratchet acp client watch work --command ./agent --stop-when-empty
                            # Explicit foreground auto-drain for queued ACP prompts
ratchet acp client sessions list
                            # List persisted ACP client sessions
ratchet acp client sessions show ID
                            # Show persisted ACP client session metadata
ratchet acp client sessions export ID --output session.archive.json
                            # Export a portable ratchet-cli archive v1 JSON file
ratchet acp client sessions export ID --history raw --output session.acpx.json
                            # Export raw ACPX-compatible JSON-RPC history when available
ratchet acp client sessions import session.archive.json --session imported
                            # Import an archive as a new local ACP client session
ratchet acp client sessions events ID --output events.ndjson
                            # Inspect or copy raw ACPX event logs for a session
ratchet acp client compare --save --command ./agent --command ./other-agent "prompt"
                            # Run one prompt serially and persist a compare bundle
ratchet acp client flow run flow.json --input-json '{"task":"x"}' --command ./agent
                            # Run a JSON v1 ACP/compute flow
ratchet acp client flow replay RUN_DIR --json
                            # Summarize a persisted flow replay bundle without execution
ratchet acp client profiles list
                            # List local ACP launch profiles and plugin templates
ratchet acp client profiles add local --command ./agent --trust
                            # Save a reviewed reusable ACP launch profile
ratchet acp client profiles verify local --json
                            # Verify a trusted ACP profile without printing prompt/response text
ratchet acp client status ID
                            # Show ACP client session status
ratchet acp client cancel ID
                            # Cancel an active or queued ACP client prompt
ratchet daemon status       # Check daemon
ratchet blackboard write coordination status ready
                            # Share local coordination state through the daemon
ratchet blackboard read coordination status
                            # Read coordination state from another terminal
ratchet blackboard export coordination --jsonl
                            # Export local notification-event records for messaging plugins
ratchet blackboard export coordination --workflow-messaging --jsonl
                            # Add Workflow step.messaging_send handoff metadata
ratchet retro analyze --evidence ~/.ratchet/retro/evidence.jsonl --session ID
                            # Analyze local retro evidence without mutating config
ratchet provider list       # List providers
ratchet team start "task"   # Start agent team
ratchet hooks list --cwd .  # Review lifecycle hooks before trusting them
ratchet plugin marketplace add local ./marketplace.json
                            # Add a plugin marketplace catalog
ratchet plugin install agent-tools@local
                            # Install a plugin from a reviewed marketplace
ratchet plugin disable agent-tools
                            # Keep a plugin installed but skip loading it
ratchet plugin reload       # Reload installed plugin capabilities in the daemon
ratchet skill list          # List user, project, and plugin skills
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

Retro analysis is reporting-only. Use
`ratchet retro analyze --evidence <evidence.jsonl> [--session ID] [--json]` to
load local evidence, summarize findings, and emit local-action or upstream-PR
instructions according to `retro.*` config. The command does not edit config or
open PRs.

Lifecycle hooks are reviewable local command hooks. User hooks in
`~/.ratchet/hooks.yaml` remain trusted by default for compatibility; project
hooks in `.ratchet/hooks.yaml` and plugin-contributed hooks are listed but
skipped until their descriptor hash is trusted. Use
`ratchet hooks list --cwd .` to review event, source, status, hash, and a
truncated command preview, then `ratchet hooks trust <hash>` to enable that
exact descriptor. `ratchet hooks disable <hash>` overrides trust, and changed
project or plugin hook commands produce a new hash that must be reviewed again.
The daemon fires hooks at session start/end, prompt submit, command start/end,
tool pre/post/failure, permission request/denial, compaction, stop/failure,
token-limit, cron, plan, fleet, and team-agent lifecycle points. Hook template
data prefers IDs, paths, counts, and hashes; raw prompt text is not passed to
hooks by default. This hook trust model is local and hash-based; managed hooks
remain deferred.

Installed plugin skills are available through `ratchet skill list` and
`ratchet skill show <name>`. Plugin skills are also exposed to chat turns
through a compact index, and full skill text is injected only when explicitly
referenced by name, such as `$autodev:using-autodev` or `/plugin:skill`. Use
`ratchet plugin marketplace add|list|update|remove` to manage reviewed catalog
sources, `ratchet plugin install <name>@<marketplace>` to install a catalog
entry, `ratchet plugin update <name|--all>` to refresh installed plugins, and
`ratchet plugin enable|disable <name>` to control loader participation without
deleting files. Use `ratchet plugin reload` after installing, updating,
enabling, or disabling plugins to refresh daemon skills, hooks, commands,
tools, MCP declarations, ACP profiles, and plugin daemons without restarting
ratchet. Marketplace catalogs are metadata, not trust: project/plugin hooks
still require hash review before execution. Plugin autoupdate policy, managed
hooks, visible routines, and dynamic workflow run primitives remain deferred to
the runtime extension lifecycle plan.

ACP launch profiles are reviewed launch specs for explicit foreground ACP
client commands. Use `ratchet acp client profiles list`, `add`, `install`,
`trust`, and `remove` to manage local profile copies. Profiles store command,
args, cwd, and env key names only, never secret values. Built-in ACP agents win
over profile names, and profile names cannot shadow built-ins. Trusted profiles
can be used with `--agent <name>` for `exec`, `drain`, `watch`, `compare`, and
`flow run`; untrusted profiles are listed but refused at execution time.
Plugin-distributed profile templates can be installed into the local profile
store, then reviewed or trusted like local profiles. Use
`ratchet acp client profiles verify <name> [--json]` as a credential-free CI
contract check for trusted profiles; it reports session id, stop reason,
command fingerprint, and response byte count without printing prompt text,
response text, or env values. The TypeScript extension SDK remains deferred.

See [docs/policy-matrix.md](docs/policy-matrix.md) for the Policy Matrix
covering static config trust rules, runtime trust rules, persistent trust
grants, permission prompts, explicit ACP client watch/drain, partial
sandbox/path/network controls, hook trust, plugin marketplace lifecycle,
plugin reload, ACP launch profiles, retro evidence, and deferred daemon
background drain, plugin autoupdate, routines, dynamic workflows, and extension
SDK work.

The ACP client queue persists prompt text under the user's XDG state directory.
Do not use `--no-wait` for prompts that should not be written to local disk.
`ratchet acp client watch` is an explicit foreground worker: it drains queued
prompts only while the operator-started command is running and still requires an
explicit `--command` or `--agent` launch target. It is not a hidden daemon
background drain.
ACP client archives are explicit JSON exports and can contain prompt text,
responses, summaries, and queue history. Treat exported archives as sensitive
conversation data.

The daemon blackboard is a same-device coordination surface for separate
terminal sessions. Use `ratchet blackboard write coordination status ready` and
`ratchet blackboard read coordination status` to pass short status, plan, or
handoff notes through the running daemon; add `--json` for scripts. The
blackboard is daemon-scoped volatile state, not durable storage across daemon
restart. Treat values as sensitive local coordination data because they can
contain prompts, file paths, or task context. Use
`ratchet blackboard export [section] --jsonl` to emit local notification-event
records with a `messaging.text` projection, or add `--workflow-messaging` to
include `workflow-plugin-messaging-core` handoff metadata for
`step.messaging_send` with `channel` supplied by the downstream Workflow
pipeline. External delivery belongs in the existing
`workflow-plugin-messaging-core`, `workflow-plugin-slack`,
`workflow-plugin-discord`, and `workflow-plugin-teams` plugin family, not in
ratchet-cli direct adapters or credential flags.

The v0.25.0 release line adds raw ACPX event-log import/export, `sessions
events`, saved compare bundles, and flow replay bundles on top of reviewable
hook trust controls, ACP launch profiles, and Windows amd64/arm64 zip artifacts
alongside Linux and macOS archives.

ACP client archive v1 JSON remains the backward-compatible summary format by
default. Use `sessions export --history summary|raw|both` for raw
ACPX-compatible JSON-RPC history, summary history, or both. Raw ACPX event logs,
compare bundles, flow replay bundles, prompts, responses, and action outputs
are sensitive local conversation artifacts. JSON v1 flows support `acp`,
`compute`, and `action` nodes, template prompts, shared session handles, and
persisted run bundles; `flow replay` reads ratchet bundles and upstream-shaped
ACPX durable bundles through the shared `workflow-plugin-acpx` runtime
without contacting agents or executing actions. New `flow run` bundles write
ACPX-shaped `manifest.json`, `flow.json`, `trace.ndjson`, projections, session
records, and artifacts while preserving the JSON v1 input flow format. Action
nodes require `--allow shell`, and node working directories outside the flow
base require `--allow outside-cwd`. Action stdout/stderr in run bundles is
sensitive local command output. Ratchet does not execute `.flow.ts` files or
embed a TypeScript ACPX runtime.

## ACP Client Examples

```sh
# Export and import an ACP client session archive.
ratchet acp client sessions export work --output work.archive.json
ratchet acp client sessions export work --history raw --output work.acpx.json
ratchet acp client sessions events work --output work.events.ndjson
ratchet acp client sessions import work.archive.json --session work-copy

# Compare two external ACP agents with one prompt and save a bundle.
ratchet acp client compare \
  --save \
  --command ./agent-a \
  --command ./agent-b \
  "Summarize the current project risks"

# Run a JSON v1 ACP/compute/action flow.
cat > flow.json <<'JSON'
{
  "format_version": 1,
  "start_at": "prepare",
  "nodes": [
    {
      "id": "prepare",
      "type": "action",
      "command": "ratchet",
      "args": ["version"]
    },
    {
      "id": "draft",
      "type": "acp",
      "prompt": "Draft a brief answer for {{ .Input.topic }} after {{ .Outputs.prepare.stdout }}",
      "session": "shared"
    },
    {
      "id": "result",
      "type": "compute",
      "select": "draft"
    }
  ],
  "edges": [{"from": "prepare", "to": "draft"}, {"from": "draft", "to": "result"}]
}
JSON
ratchet acp client flow run flow.json \
  --input-json '{"topic":"release readiness"}' \
  --command ./agent \
  --allow shell \
  --json
ratchet acp client flow replay .ratchet/acp-client/flows/RUN_ID --json

# Explicitly drain queued prompts while this foreground command is running.
ratchet acp client watch work \
  --command ./agent \
  --stop-when-empty \
  --max-per-cycle 2
```

## Harness Modes

| Mode | Command | Credential-free smoke |
|---|---|---|
| TUI | `ratchet` | Starts a daemon-backed interactive session. |
| One-shot | `ratchet -p "prompt"` | Uses the configured default provider. |
| Daemon | `HOME="$(mktemp -d)" ratchet daemon status` | Runs credential-free when pointed at a temp home. |
| Blackboard | `ratchet blackboard write coordination status ready` / `ratchet blackboard read coordination status` / `ratchet blackboard export [section] --jsonl` / `ratchet blackboard export [section] --workflow-messaging --jsonl` | Shares daemon-scoped volatile local coordination data across separate terminal invocations and exports local notification-event records plus Workflow `step.messaging_send` handoff metadata for Workflow messaging plugins; `TestHarnessSmokeBlackboardCLI` proves built CLI write/read/list through the daemon. |
| ACP | `ratchet acp` | Exposes the agent over ACP stdio JSON-RPC; prompt smoke is covered by `TestACPStdioPromptSmoke`. |
| ACP client | `ratchet acp client exec --command ./agent "prompt"` | Drives an external ACP agent over stdio; binary smoke covers exec, persisted sessions, FIFO `--no-wait` queue, queue inspection, explicit watch/drain, status, cancel, archive export/import with raw ACPX event logs, saved compare bundles, Go-native ACPX flow replay bundles, and trusted ACP launch profiles. |
| MCP | `ratchet mcp blackboard` / `ratchet mcp daemon` | Exposes standalone blackboard or daemon-backed session/project/blackboard/team MCP tools over stdio, including active-team `team_message`. |
| Team | `ratchet team start "task"` | Uses daemon team orchestration with configured providers. |

TUI binary evidence is split by boundary. The release-shaped startup smoke
builds the untagged `ratchet` binary, starts it against a temp home/workdir,
reaches the onboarding/provider setup boundary, and shuts the background daemon
down by RPC; release-shaped startup smoke is not full TUI PTY proof.
`ratchet-tui-smoke` is build-tagged test-only and is used for Unix PTY binary
smoke of slash commands, shortcuts, trust state, session tree, and job panel
flows. Windows ConPTY binary smoke drives the same test-only smoke binary
through a ConPTY-backed TUI startup, mocked chat turn, slash help, and clean
exit. GoReleaser snapshot release-check, draft release asset postcheck, tap
preflight, generated-cask publish, and tap postcheck gates verify release
artifacts and the Homebrew cask path before the GitHub release is made public.
Windows cross-build/package archive inspection is release artifact proof;
packaged release `ratchet.exe` runtime remains deferred.

See [docs/harness-emulation.md](docs/harness-emulation.md) for credential-free
mock provider recipes, and [docs/competitor-parity.md](docs/competitor-parity.md)
for the dated source-backed parity matrix. Policy boundaries for trust,
permissions, queue drain, hooks, and sandbox follow-ups are in
[docs/policy-matrix.md](docs/policy-matrix.md).
