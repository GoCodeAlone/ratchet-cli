# ratchet

Ratchet is a terminal agent harness for running AI coding sessions locally. It
starts as an interactive TUI, keeps session state in a local daemon, and also
exposes scriptable commands for ACP agents, teams, hooks, trust policy,
blackboard coordination, plugin loading, routines, workflows, and retros.

Use it when you want a local-first agent CLI that can be driven by a human in a
terminal, by another harness over ACP/MCP, or by repeatable smoke tests without
checking credentials into CI.

## Install

Homebrew users should install the cask:

```sh
brew tap gocodealone/tap
brew install --cask gocodealone/tap/ratchet-cli
ratchet --version
```

The tap also keeps a Formula current so older `brew install ratchet-cli`
installations can upgrade without leaving a stale binary linked at
`/opt/homebrew/bin/ratchet`:

```sh
brew upgrade gocodealone/tap/ratchet-cli
ratchet --version
```

If you prefer to move from the Formula to the cask, uninstall the Formula first,
then install the cask.

You can also install from source:

```sh
go install github.com/GoCodeAlone/ratchet-cli/cmd/ratchet@latest
```

Release artifacts are available on
[GitHub Releases](https://github.com/GoCodeAlone/ratchet-cli/releases). They
include Linux and macOS tar.gz archives and Windows zip archives for amd64 and
arm64. Windows installer packages are not published yet.

## Quick Start

```sh
ratchet                       # Start the interactive TUI
ratchet --version             # Print the CLI version and exit
ratchet help                  # Show top-level commands
ratchet doctor                # Print local diagnostics without credentials
ratchet daemon status         # Check the local daemon
ratchet provider list         # List configured providers
ratchet "summarize this repo" # Start chat with an initial prompt
ratchet -p "write a test"     # One-shot prompt mode
```

Configure providers with `ratchet provider setup`. For ChatGPT subscription
access, sign in with OpenAI device-code auth:

```sh
ratchet provider setup openai-chatgpt
ratchet provider setup openai-chatgpt --from-codex ~/.codex/auth.json
```

On first interactive use, ratchet starts or connects to its local daemon and
opens the TUI. The daemon owns persisted sessions, team state, blackboard
entries, trust grants, plugin state, routines, and workflow records. Most data
is local to your home/XDG state directory.

## Common Workflows

### Interactive TUI

```sh
ratchet
```

Useful TUI controls:

- `ctrl+b` or `/tree`: open the session branch tree.
- `ctrl+s`: open the session sidebar; use arrow keys to choose a session,
  `Enter` to switch, and `d` to kill the highlighted session.
- `ctrl+t`: open team view.
- `ctrl+j`: open the jobs panel.
- `ctrl+c`: quit.
- `/model`: show configured providers and models, including actions for
  changing a model or adding another provider.
- `/mode <mode>`: switch trust mode (`conservative`, `permissive`, `locked`,
  `sandbox`, or `custom`).
- `/trust list`, `/trust allow`, `/trust deny`, `/trust grants`, and
  `/trust reset`: inspect and adjust daemon-backed trust state.
- `/exit`: quit.

Runtime trust commands do not edit config files. Persistent trust grants are
explicit and should be treated as sensitive local policy metadata because
patterns can reveal local paths, commands, or workflow conventions.

### Sessions

```sh
ratchet sessions
ratchet sessions history SESSION_ID
ratchet sessions clone SESSION_ID
ratchet sessions fork SESSION_ID --at MESSAGE_ID
ratchet sessions tree SESSION_ID
ratchet sessions browse SESSION_ID
```

Use the tree/browser commands when you want to branch an investigation without
losing visible history.

### ACP Agent And ACP Client

Ratchet can run as an ACP stdio agent:

```sh
ratchet acp
```

It can also drive another ACP agent as a client:

```sh
ratchet acp client exec --command ./agent "prompt"
ratchet acp client exec --command ./agent --session work --no-wait "queued prompt"
ratchet acp client queue work --json
ratchet acp client drain work --command ./agent --max 2
ratchet acp client watch work --command ./agent --stop-when-empty
ratchet acp client status work
ratchet acp client cancel work
```

The ACP client queue persists prompt text under the user's XDG state directory.
Do not use `--no-wait` for prompts that should not be written to local disk.
`ratchet acp client watch` is an explicit foreground worker: it drains queued
prompts only while the operator-started command is running and still requires an
explicit `--command` or `--agent` launch target. It is not a hidden daemon
background drain.

Reviewed ACP launch profiles make repeated client runs safer:

```sh
ratchet acp client profiles list
ratchet acp client profiles add local --command ./agent --trust
ratchet acp client profiles verify local --json
```

Profiles store command, args, cwd, and env key names only, never secret values.
Use `ratchet acp client profiles verify` as a credential-free CI contract check
for trusted profiles.

### Archives, Compare, And Flow Replay

ACP client archive v1 JSON is the backward-compatible summary format by
default. Raw ACPX event logs and replay bundles are available when you need
interoperability or audit evidence:

```sh
ratchet acp client sessions export work --output work.archive.json
ratchet acp client sessions export work --history raw --output work.acpx.json
ratchet acp client sessions events work --output work.events.ndjson
ratchet acp client sessions import work.archive.json --session work-copy
ratchet acp client compare --save --command ./agent-a --command ./agent-b "prompt"
ratchet acp client flow run flow.json --input-json '{"task":"x"}' --command ./agent
ratchet acp client flow replay .ratchet/acp-client/flows/RUN_ID --json
```

Use `sessions export --history summary|raw|both` for summary history, raw
ACPX-compatible JSON-RPC history, or both. ACP client archives, compare --save
bundles, flow replay bundles, prompts, responses, summaries, queue history, raw
ACPX event logs, and action outputs are sensitive local conversation artifacts.
JSON v1 flows support `acp`, `compute`, and action nodes. Action nodes require
`--allow shell`, and node working directories outside the flow base require
`--allow outside-cwd`. Action stdout/stderr in run bundles is sensitive local
command output. Ratchet does not execute `.flow.ts` files, and ACPX TypeScript
flow runtime compatibility remains deferred.

### Blackboard And Messaging Handoff

The daemon blackboard is a same-device coordination surface for separate
terminal sessions:

```sh
ratchet blackboard write coordination status ready
ratchet blackboard read coordination status
ratchet blackboard export [section] --jsonl
ratchet blackboard export coordination --workflow-messaging --jsonl
```

The blackboard is daemon-scoped volatile state, not durable storage across
daemon restart. Treat values as sensitive local coordination data because they
can contain prompts, file paths, or task context.

`--workflow-messaging` adds Workflow `step.messaging_send` handoff metadata.
External delivery belongs in the existing `workflow-plugin-messaging-core`,
`workflow-plugin-slack`, `workflow-plugin-discord`, and
`workflow-plugin-teams` plugin family, not in ratchet-cli direct adapters or
credential flags. `workflow-plugin-messaging-core` exposes
`ParseNotificationEvents` and `ProjectNotificationEventToMessagingSend` for
Workflow-side JSON/JSONL parsing and typed `step.messaging_send` input
projection.

### Hooks, Plugins, Routines, Workflows, And Retros

```sh
ratchet hooks list --cwd .
ratchet hooks trust HASH
ratchet plugin marketplace add local ./marketplace.json
ratchet plugin install agent-tools@local
ratchet plugin reload
ratchet routines add --schedule 15m --prompt "summarize status"
ratchet routines run ID
ratchet workflows install workflow.yaml
ratchet workflows run NAME
ratchet retro analyze --evidence ~/.ratchet/retro/evidence.jsonl --session ID
ratchet retro instructions --evidence ~/.ratchet/retro/evidence.jsonl --session ID --output instructions.md
```

Project and plugin hooks are skipped until their descriptor hash is trusted.
Changed hook commands produce a new hash. Hook template data prefers IDs, paths,
counts, and hashes; raw prompt text is not passed to hooks by default. This hook
trust model is local and hash-based; managed hooks remain deferred.

Plugin marketplaces are metadata, not trust. Installing or enabling a plugin can
add skills, hooks, commands, tools, MCP declarations, ACP launch profiles, and
plugin daemons, but project/plugin hooks still require hash review before
execution. The TypeScript extension SDK remains deferred.

Routines and workflows are visible local records. `ratchet routines run` records
a manual run for auditability and does not start hidden workers. `ratchet
workflows run` records workflow lifecycle state and rejects shell/JavaScript
executable node types instead of running source code.

Retro analysis is reporting-only. It reads local evidence, summarizes findings,
and can emit local-action or upstream-PR instructions according to config.
`ratchet retro instructions` writes those findings as a Markdown handoff for
review before any PR is opened; it does not edit config or open PRs.

## Harness Modes

| Mode | Command | Credential-free smoke |
|---|---|---|
| TUI | `ratchet` | Starts a daemon-backed interactive session. |
| one-shot | `ratchet -p "prompt"` | Uses the configured default provider. |
| doctor | `HOME="$(mktemp -d)" ratchet doctor --json` | Prints credential-free local install, path, and daemon diagnostics. |
| daemon | `HOME="$(mktemp -d)" ratchet daemon status` | Runs credential-free when pointed at a temp home. |
| blackboard | `ratchet blackboard write coordination status ready` / `ratchet blackboard read coordination status` / `ratchet blackboard export [section] --jsonl` / `ratchet blackboard export [section] --workflow-messaging --jsonl` | Shares daemon-scoped volatile local coordination data and exports local notification-event records plus Workflow messaging handoff metadata. |
| ACP | `ratchet acp` | Exposes ratchet over ACP stdio JSON-RPC. |
| ACP client | `ratchet acp client exec --command ./agent "prompt"` | Drives an external ACP agent over stdio, including persisted sessions, FIFO queue, explicit watch/drain, archive export/import, raw ACPX event logs, compare bundles, flow replay bundles, and ACP launch profiles. |
| MCP | `ratchet mcp blackboard` / `ratchet mcp daemon` | Exposes blackboard or daemon-backed session/project/blackboard/team MCP tools over stdio. |
| team | `ratchet team start "task"` | Uses daemon team orchestration with configured providers. |

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
mock provider recipes, [docs/competitor-parity.md](docs/competitor-parity.md)
for the dated source-backed parity matrix, and
[docs/policy-matrix.md](docs/policy-matrix.md) for Policy Matrix details on
static config trust rules, runtime trust rules, persistent trust grants,
permission prompts, ACP client queue/drain, hook trust, extension hooks,
sandbox/path/network controls, retro evidence, action nodes, background drain,
managed hooks, workflow source execution, and extension SDK work.

## Development

```sh
go test ./...
go test ./cmd/ratchet -run TestHarnessSmokeVersionHelpAndDaemonStatus -count=1
go test ./internal/tui -run TestTUIBinarySmoke -count=1 -timeout=10m
```

Release publishing is tag-driven. The release workflow builds Linux, macOS, and
Windows artifacts, validates snapshot artifacts, checks draft release assets,
publishes the generated Homebrew cask, runs tap postcheck, and only then makes
the GitHub release public.
