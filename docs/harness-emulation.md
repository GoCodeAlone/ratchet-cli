# Harness Emulation

This document records the supported local harness modes and the current parity
target for ratchet-cli as an agent harness. It is intentionally credential-free:
the smoke path uses temp home directories and the built-in mock provider where
possible.

## Command Modes

| Mode | Command | Backing path | Status | Smoke evidence |
|---|---|---|---|---|
| TUI | `ratchet` | daemon gRPC + Bubble Tea UI | Supported | Release-shaped startup smoke builds untagged `ratchet`, reaches onboarding/provider setup, and shuts the daemon down by RPC; Unix PTY binary smoke drives the build-tagged test-only TUI binary through slash commands and shortcuts. |
| one-shot | `ratchet -p "prompt"` | daemon session + default provider | Supported when provider configured | CLI binary smoke covers command dispatch; mock provider roundtrip covers daemon path. |
| doctor | `ratchet doctor [--json]` | local executable/config/state path inspection plus daemon status files | Supported credential-free | `TestRunDoctorJSON`; `TestHarnessSmokeVersionHelpAndDaemonStatus`. |
| daemon | `ratchet daemon status` | pid/socket state under `~/.ratchet` | Supported | `TestHarnessSmokeVersionHelpAndDaemonStatus`. |
| blackboard | `ratchet blackboard write coordination status ready` / `ratchet blackboard read coordination status` / `ratchet blackboard export [section] --jsonl` / `ratchet blackboard export [section] --workflow-messaging --jsonl` | daemon gRPC `BlackboardWrite`/`BlackboardRead`/`BlackboardList` | Supported for same-device, daemon-scoped volatile local coordination data across separate terminal invocations, local notification-event export, and Workflow `step.messaging_send` handoff metadata | `TestHarnessSmokeBlackboardCLI`; blackboard export command tests. |
| session lineage | `ratchet sessions history`, `ratchet sessions clone`, `ratchet sessions fork`, `ratchet sessions tree`, `ratchet sessions browse`, `ratchet sessions summary`, `ratchet sessions compactions`, `ratchet sessions export` | daemon gRPC session history/clone/fork/tree/summary/compaction/export APIs plus Bubble Tea session tree browser | Supported for separate fork/clone sessions, branch summaries, persisted compaction records, archive session links, daemon session export bundles, JSONL session/message/compaction exports, and Pi-style in-place branch navigation through `ctrl+b`, `/tree`, and `sessions browse` | `TestSessionLineageHistoryCloneForkTreeRPC`; `TestCompactionRecordRPC`; `TestHandleSessionsHistoryCloneForkTree`; `TestHandleSessionsExportWritesSensitiveBundle`; `TestHandleSessionsExportWritesJSONLRecords`; `TestAppCtrlBOpensSessionTreeBrowser`; `TestParseTreeRequestsSessionTreeNavigation`; `TestHandleSessionsBrowseRunsInjectedBrowser`. |
| ACP | `ratchet acp` / `ratchet acp config zed` | ACP stdio JSON-RPC agent wrapping daemon service plus Zed settings writer | Supported for initialize/new/load/prompt/cancel/model/mode and custom Zed ACP agent config | `TestACPStdioPromptSmoke`; `TestHarnessSmokeInitializeNewAndLoadSession`; `TestParityNewSessionIDCanBeLoaded`; `TestWriteZedACPConfig`; `TestRunACPConfigZedWritesSettings`. |
| ACP client | `ratchet acp client exec --command <agent> "prompt"` | typed `acp-go-sdk` client over child-process stdio plus local JSON state under XDG state | Supported for one-shot exec, persisted session metadata, sessions list/show/status, multi-prompt FIFO `--no-wait` queue, explicit queue inspection/drain, cooperative cancel requests, ratchet-cli archive v1 export/import with raw ACPX event logs, `sessions events`, saved compare bundles, Go-native ACPX flow replay bundles, `flow replay`, trusted ACP launch profiles, and `ratchet acp client profiles verify` redacted profile checks | `TestACPClientExecBinarySmoke`; `TestDrainQueueAgainstFixtureProcessReusesSession`; `TestClientRunPromptAgainstFixtureProcess`; `TestSessionStoreLoadsMissingFileAndPersistsRecords`; profile command, archive, compare, and flow replay tests. |
| MCP | `ratchet mcp blackboard` / `ratchet mcp daemon` / `ratchet mcp config zed` | stdio JSON-RPC blackboard or daemon server plus config writers | Supported for standalone blackboard plus daemon session/project/blackboard/team status tools and Zed/Claude/Copilot/generic config entries | `TestHarnessSmokeJSONRPCInitializeToolsListAndCall`; `TestDaemonMCPToolCallsUseDaemonClient`; `TestWriteZedMCPConfig`; `TestHandleMCPConfigZedWritesSettings`. |
| team | `ratchet team start "task"` | daemon team manager / mesh executor | Supported when provider configured | Existing team and mesh tests cover service behavior. |

## Temp Home Mock Provider Smoke

Use a throwaway home to avoid touching a real `~/.ratchet`:

```sh
tmp_home="$(mktemp -d)"
HOME="$tmp_home" ratchet version
HOME="$tmp_home" ratchet help
HOME="$tmp_home" ratchet doctor --json
HOME="$tmp_home" ratchet daemon status
```

For a credential-free daemon chat path, use the test harness rather than a paid
provider. The in-process harness registers an `e2e-mock` provider and exercises
the same gRPC service methods as the CLI daemon:

```sh
go test ./internal/daemon -run TestHarnessSmokeMockProviderSessionRoundTrip -count=1
```

The CLI binary smoke is also hermetic and uses a temp home:

```sh
go test ./cmd/ratchet -run TestHarnessSmokeVersionHelpAndDaemonStatus -count=1
```

The TUI binary evidence has explicit boundaries:

- release-shaped startup smoke is not full TUI PTY proof; it proves untagged
  `ratchet` startup, temp home/workdir containment, onboarding/provider setup
  reachability, daemon socket permissions, and RPC shutdown cleanup;
- `ratchet-tui-smoke` is build-tagged test-only and must not be packaged in
  public release artifacts;
- Unix PTY binary smoke drives `ratchet-tui-smoke` through command rows marked
  `pty-proven` in `internal/tui/commands/testdata/command_surface_spec.json`;
- Windows ConPTY binary smoke drives the test-only `ratchet-tui-smoke` binary
  through TUI startup, mocked chat, slash help, and clean exit on a hosted
  Windows runner;
- Windows cross-build/package archive inspection remains release artifact
  proof for the packaged Windows archives;
- Windows command binary startup smoke builds and runs native `ratchet.exe`
  `--version` and `help` on a hosted Windows runner;
- GoReleaser snapshot release-check, draft release asset postcheck, tap
  preflight, generated-cask publish, and tap postcheck gates verify release
  archives and Homebrew cask updates before the GitHub release is made public;
- full packaged release `ratchet.exe` TUI/installer runtime remains deferred.

```sh
go test ./cmd/ratchet -run 'StartupSmoke|VersionHelpAndDaemonStatus' -count=1
go test ./internal/tui -run TestTUIBinarySmoke -count=1 -timeout=8m
```

The ACP prompt smoke uses `acp-go-sdk` client and agent-side connections over
stdio-style pipes, a real ratchet daemon service, and the built-in mock provider:

```sh
go test ./internal/acp -run TestACPStdioPromptSmoke -count=1
```

## Competitor parity

The dated source-backed matrix lives in
[competitor-parity.md](competitor-parity.md). The snapshot was refreshed on
2026-07-02 from current Zed, ACP, Pi, Codex, Claude Code, Hermes, OpenClaw, and
ACPX sources, with Zed ACP/MCP hosted docs checked again on 2026-07-06.
ratchet-cli now supports Windows release artifacts, ACP prompt stdio
smoke, reviewable hook trust controls, headless ACP client
exec/session/status/cancel primitives with
multi-prompt FIFO queue/watch/drain,
daemon-backed MCP blackboard/session/project/team status/message tools,
same-device `ratchet blackboard` read/write/list for sensitive local
coordination data, runtime
trust slash commands, session lineage
history/clone/fork/tree commands, branch summaries, compaction records with
archive session links, Pi-style in-place branch navigation, and opt-in redacted
retro evidence, ACP client session archive export/import with raw ACPX event
logs, saved compare bundles, and Go-native ACPX flow replay bundles. ACP
launch profiles let reviewed local or
plugin-distributed launch specs feed `--agent` for explicit foreground ACP
client commands; built-ins win over profile names and untrusted profiles are
refused at execution time. `ratchet acp client profiles verify <name>` gives CI
a redacted trusted-profile proof without printing prompt or response text, and
`ratchet acp client profiles verify --all --json` reports all trusted local
profile checks plus skipped untrusted profiles without printing prompts,
responses, or env values. JSON
v1 action nodes run local commands only with
`--allow shell`; node cwd escapes require `--allow outside-cwd`, and run bundles
may contain sensitive local command output. `flow replay` is read-only and does
not contact agents or execute actions. The daemon blackboard is daemon-scoped
volatile state and should not be used as durable storage. Use
`ratchet blackboard export [section] --jsonl` to emit local notification-event
records with `messaging.text` for downstream Workflow messaging plugins; add
`--workflow-messaging` when the handoff should include
`workflow-plugin-messaging-core` `step.messaging_send` metadata.
`workflow-plugin-messaging-core` owns `ParseNotificationEvents` and
`ProjectNotificationEventToMessagingSend` so Workflow-side pipelines can parse
notification-event JSON/JSONL exports and supply the target `channel`. Outbound
Discord, Slack, Teams, email, webhook, or other service delivery stays in the
existing messaging-core and channel plugins rather than built into ratchet-cli.
`ratchet provider setup list` and `ratchet provider setup guide <provider>` give
humans and automation a provider onboarding path before the TUI is usable.
`ratchet acp config zed` and `ratchet mcp config zed` merge ratchet into Zed's
custom agent and MCP settings without writing provider secrets.
`ratchet sessions export <id> --format jsonl --output <path>` writes
line-oriented `export`, `session`, `message`, and `compaction` records with
schema `ratchet.session-jsonl.v1`. The file is written with user-only
permissions and is a sensitive local conversation artifact. External JSONL
import remains deferred.
The
v0.25.0 release line keeps Windows
amd64/arm64 zip artifacts in the GoReleaser output while adding raw event
archives, compare artifacts, and replay-grade flow bundles. The policy
boundaries are tracked in
[docs/policy-matrix.md](policy-matrix.md): runtime trust rules, persistent
trust grants, permission prompts, hook trust, ACP launch profiles, and explicit
ACP client watch/drain are supported, while daemon background drain, managed
hooks remain deferred, TypeScript extension SDK remains deferred, ACPX
TypeScript flow runtime compatibility, and local-first channel gateways remain
deferred.

## ACP Matrix

| ACP capability | Status | Evidence |
|---|---|---|
| initialize | Supported | `RatchetAgent.Initialize`; `TestHarnessSmokeInitializeNewAndLoadSession`. |
| new session | Supported | `RatchetAgent.NewSession`; service-backed test. |
| load session | Supported | `NewSession` returns the ratchet session ID as the ACP ID; `TestParityNewSessionIDCanBeLoaded`. |
| prompt | Supported | `TestACPStdioPromptSmoke` negotiates initialize/new-session/prompt over stdio-style ACP connections and receives agent message updates from daemon `SendMessageChan`. |
| cancel | Supported | `RatchetAgent.Cancel`. |
| plan updates | Partial | Chat event conversion supports plan proposed/step update events. |
| session model | Supported | `SetSessionModel` updates the ratchet session model; `TestParitySetSessionModelUpdatesSession`. |
| session mode | Supported in-memory | `SetSessionMode` validates known sessions and records the ACP mode for the agent process; daemon-wide persistence is deferred. |
| Zed custom agent config | Supported | `ratchet acp config zed [.zed/settings.json]`; `TestWriteZedACPConfig`; `TestRunACPConfigZedWritesSettings`. |
| session list/resume/close/delete | Deferred | `acp-go-sdk v0.6.3` exposes no agent methods for these schema-v2 lifecycle operations. |
| HTTP/SSE MCP via ACP | Deferred | Agent capabilities intentionally do not advertise HTTP/SSE MCP support. |

## ACP Client Matrix

ACP client `--no-wait` queues persist prompt text under the user's XDG state
directory. Use direct `exec` instead when prompts should not be written to local
disk.

| ACP client capability | Status | Evidence |
|---|---|---|
| external process stdio | Supported | `TestClientRunPromptAgainstFixtureProcess`; `TestACPClientExecBinarySmoke`. |
| one-shot prompt | Supported | `ratchet acp client exec`; human and JSON output tests. |
| session metadata | Supported | XDG state JSON store; `TestSessionStoreLoadsMissingFileAndPersistsRecords`. |
| sessions list/show/status | Supported | `ratchet acp client sessions list`, `ratchet acp client sessions show <id>`, and `ratchet acp client status <id>`; command tests cover empty, one-session, and invalid-id cases. |
| no-wait FIFO queue | Supported | `ratchet acp client exec --no-wait --session <id>` appends prompt text to a local FIFO queue under XDG state; use `ratchet acp client queue <id>` to inspect it. |
| drain FIFO queue | Supported | `ratchet acp client drain <id> --command <agent> --max <n>` drains pending prompts through one ACP session; binary smoke verifies two queued prompts complete on the same fixture session. |
| watch FIFO queue | Supported as explicit foreground worker | `ratchet acp client watch <id> --command <agent> --stop-when-empty` polls the local FIFO queue and delegates each cycle to the same drain path. It runs only while the operator-started foreground command is active; daemon background drain remains deferred. Binary smoke verifies queued prompts complete without printing prompt bodies in watch output. |
| cancel | Supported as cooperative request | `ratchet acp client cancel <id>` marks pending queued prompts canceled or writes a cancel-request file for active owners; active clients poll and send ACP cancel. |
| import/export archives | Supported | `ratchet acp client sessions export <id> --history summary|raw|both --output <archive.json>` writes summary archives, raw ACPX-compatible JSON-RPC history, or both; `sessions import <archive.json> --session <id>` imports ratchet summary archives and `exported_by:"acpx"` raw history archives. `ratchet acp client sessions events <id>` reports or copies raw ACPX event logs. Raw export fails when no sidecar is available instead of inventing wire history. Archives and event logs may contain prompt/response content and are sensitive local conversation data. |
| compare commands | Supported | `ratchet acp client compare --save --command <agent-a> --command <agent-b> "prompt"` runs agents serially, emits table or JSON rows, and persists `compare.json` plus per-agent `events.ndjson` files when `--save` is set. Binary smoke proves compare through the built CLI and fixture ACP agent. |
| flow commands | Supported | `ratchet acp client flow run flow.json --input-json '{"task":"x"}' --command <agent> --allow shell` runs JSON v1 flows with `acp`, `compute`, `action`, and `checkpoint` nodes, template prompts, shared ACP session handles, JSON output, and Go-native ACPX durable replay bundles. `ratchet acp client flow replay <run-dir> --json` validates and summarizes both legacy ratchet bundles and upstream-shaped ACPX durable bundles through the shared `workflow-plugin-acpx` runtime, including `manifest.json`, `flow.json`, `trace.ndjson`, projections, artifacts, and session event links without contacting agents or executing actions. Action nodes require `--allow shell`; cwd escapes require `--allow outside-cwd`; action stdout/stderr is sensitive local command output. Ratchet does not execute `.flow.ts` files or embed a TypeScript ACPX runtime. |
| ACP launch profiles | Supported with local trust | `ratchet acp client profiles list`, `add`, `install`, `trust`, and `remove` manage reviewed launch specs under ratchet state. Profiles store command metadata and env key names only. Built-in ACP agents win over profile names, profile names cannot shadow built-ins, and only trusted profiles resolve through `--agent` for `exec`, `drain`, `watch`, `compare`, and `flow run`. Plugin `acpProfiles` templates are copied locally before use. |
| ACP profile verify | Supported | `ratchet acp client profiles verify <name> [--json]` resolves a trusted profile, runs a small ACP prompt, and emits redacted metadata: session id, stop reason, command fingerprint, and response byte count. `ratchet acp client profiles verify --all --json` verifies trusted profiles and reports untrusted profiles as skipped. Neither form prints prompt text, response text, or env values. |

### ACP client examples

```sh
ratchet acp client profiles list
ratchet acp client profiles add local-agent --command ./agent --arg --stdio --trust
ratchet acp client profiles verify --all --json
ratchet acp client exec --agent local-agent "Review this patch"

ratchet acp client sessions export work --output work.archive.json
ratchet acp client sessions export work --history raw --output work.acpx.json
ratchet acp client sessions events work --output work.events.ndjson
ratchet acp client sessions import work.archive.json --session work-copy

ratchet acp client compare --save --command ./agent-a --command ./agent-b "Review this patch"

ratchet acp client flow run flow.json \
  --input-json '{"task":"review release notes"}' \
  --command ./agent \
  --allow shell \
  --json
ratchet acp client flow replay .ratchet/acp-client/flows/RUN_ID --json

ratchet acp client watch work \
  --command ./agent \
  --stop-when-empty \
  --max-per-cycle 2
```

## MCP Matrix

| MCP tool/config | Status | Evidence |
|---|---|---|
| initialize | Supported | `BBMCPServer.handleInitialize`; JSON-RPC smoke. |
| tools/list | Supported | Exposes blackboard tools in standalone mode and daemon blackboard/session/project/team tools in daemon mode. |
| `bb_write` | Supported | JSON-RPC smoke writes `smoke/status`; daemon mode calls unary `BlackboardWrite`. |
| `bb_read` | Supported | JSON-RPC smoke reads back `ok`; daemon mode calls unary `BlackboardRead`. |
| `bb_list` | Supported | Existing blackboard tests; daemon mode calls unary `BlackboardList`. |
| `session_list` | Supported | `ratchet mcp daemon`; `TestDaemonMCPToolCallsUseDaemonClient`. |
| `session_kill` | Supported | `ratchet mcp daemon`; calls daemon session kill through the client adapter. |
| `project_list` | Supported | `ratchet mcp daemon`; calls daemon project list through the client adapter. |
| `team_list` | Supported | `ratchet mcp daemon`; calls daemon team list through the client adapter. |
| `team_status` | Supported | `ratchet mcp daemon`; calls daemon team status through the client adapter. |
| `team_message` | Supported for active teams | `ratchet mcp daemon`; calls daemon `DirectMessage`, resolves team/agent by id or name, and appends an operator-originated message to the recipient. |
| Claude Code config | Supported | `WriteMCPConfig` tests. |
| Copilot config | Supported | `WriteCopilotMCPConfig` API and `ratchet mcp config copilot`. |
| generic MCP config | Supported | `WriteGenericMCPConfig` and `ratchet mcp config generic`. |
| Zed config | Supported | `WriteZedMCPConfig` and `ratchet mcp config zed [.zed/settings.json] [blackboard\|daemon]`. |
| daemon-backed blackboard | Supported | Unary daemon API added for MCP reads, writes, and lists; `TestBlackboardRPCReadWriteList`. |
| daemon-backed team tools | Supported | Team list/status/message are daemon-backed. Direct messages require an active running team; completed teams reject new messages. |

## Trust Controls

| TUI command | Status | Notes |
|---|---|---|
| `/mode <mode>` | Supported | Switches daemon trust mode at runtime for `conservative`, `permissive`, `locked`, `sandbox`, or `custom`. |
| `/trust list` | Supported | Shows daemon trust mode and effective rules from `workflow-plugin-agent/policy.TrustEngine`. |
| `/trust allow "pattern" [--scope scope]` | Supported | Adds a runtime allow rule. Scope defaults to `global`. |
| `/trust deny "pattern" [--scope scope]` | Supported | Adds a runtime deny rule. Scope defaults to `global`. |
| `/trust grants` | Supported | Shows persistent grants stored by `workflow-plugin-agent/policy.PermissionStore`. Treat output as sensitive local policy metadata. |
| `/trust persist allow "pattern" [--scope scope]` | Supported | Adds a durable allow grant. Scope defaults to `global`. |
| `/trust persist deny "pattern" [--scope scope]` | Supported | Adds a durable deny grant. Scope defaults to `global`; deny grants preserve deny-wins semantics. |
| `/trust revoke "pattern" [--scope scope]` | Supported | Revokes a durable grant. Missing grants are treated as already revoked. |
| `/trust reset` | Supported | Clears runtime slash-command rules and rebuilds from config defaults. It does not edit config files or delete persisted permission grants. |

## Hook Trust

Lifecycle hooks use local hash review for project and plugin command hooks.
User hooks in `~/.ratchet/hooks.yaml` remain trusted by default for
compatibility, while `.ratchet/hooks.yaml` project hooks and plugin hooks are
skipped until `ratchet hooks trust <hash>` records the descriptor shown by
`ratchet hooks list --cwd .`. `ratchet hooks disable <hash>` overrides trust,
and changed hook commands, events, glob filters, or source metadata produce a
new hash that must be reviewed again. Plugin hook/profile paths are contained to
the plugin root. The daemon fires session, prompt, command, tool, permission,
compaction, stop/failure, token-limit, cron, plan, fleet, and team-agent hook
events. Plugin skills are listed by `ratchet skill list`, explicit skill
mentions such as `$autodev:using-autodev` load full skill text into the next
chat turn, `ratchet plugin marketplace add|list|update|remove` manages reviewed
catalog sources, `ratchet plugin install <name>@<marketplace>` installs catalog
entries, `ratchet plugin enable|disable` controls loader participation, and
`ratchet plugin reload` refreshes installed plugin capabilities without a
daemon restart. `ratchet routines add|list|show|run|pause|resume|remove` and
`ratchet workflows install|list|show|run|stop|resume` persist visible local
definitions and run records without hidden workers or JavaScript/shell
execution. Managed hooks, plugin autoupdate, workflow source execution/triggers,
broader extension hooks, and the TypeScript extension SDK remain deferred to the
runtime extension lifecycle plan.

Scriptable equivalents are available through `ratchet trust list`,
`ratchet trust grants`, `ratchet trust allow|deny`,
`ratchet trust persist`, `ratchet trust revoke`, and `ratchet trust reset`.
The broader Policy Matrix lives in [docs/policy-matrix.md](policy-matrix.md),
including the explicit watch/drain boundary, sensitive local policy metadata
warning, hook trust, ACP launch profiles, and deferred background drain and
extension SDK boundaries. `ratchet policy matrix`,
`ratchet policy matrix --json`, and `ratchet policy matrix --status deferred`
expose a read-only CLI view of that matrix.
