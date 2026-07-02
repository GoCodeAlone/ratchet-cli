# Harness Emulation

This document records the supported local harness modes and the current parity
target for ratchet-cli as an agent harness. It is intentionally credential-free:
the smoke path uses temp home directories and the built-in mock provider where
possible.

## Command Modes

| Mode | Command | Backing path | Status | Smoke evidence |
|---|---|---|---|---|
| TUI | `ratchet` | daemon gRPC + Bubble Tea UI | Supported | Covered by daemon/session tests; full TUI remains manual. |
| one-shot | `ratchet -p "prompt"` | daemon session + default provider | Supported when provider configured | CLI binary smoke covers command dispatch; mock provider roundtrip covers daemon path. |
| daemon | `ratchet daemon status` | pid/socket state under `~/.ratchet` | Supported | `TestHarnessSmokeVersionHelpAndDaemonStatus`. |
| session lineage | `ratchet sessions history`, `ratchet sessions clone`, `ratchet sessions fork`, `ratchet sessions tree`, `ratchet sessions browse`, `ratchet sessions summary`, `ratchet sessions compactions` | daemon gRPC session history/clone/fork/tree/summary/compaction APIs plus Bubble Tea session tree browser | Supported for separate fork/clone sessions, branch summaries, persisted compaction records, archive session links, and Pi-style in-place branch navigation through `ctrl+b`, `/tree`, and `sessions browse` | `TestSessionLineageHistoryCloneForkTreeRPC`; `TestCompactionRecordRPC`; `TestHandleSessionsHistoryCloneForkTree`; `TestAppCtrlBOpensSessionTreeBrowser`; `TestParseTreeRequestsSessionTreeNavigation`; `TestHandleSessionsBrowseRunsInjectedBrowser`. |
| ACP | `ratchet acp` | ACP stdio JSON-RPC agent wrapping daemon service | Supported for initialize/new/load/prompt/cancel/model/mode | `TestACPStdioPromptSmoke`; `TestHarnessSmokeInitializeNewAndLoadSession`; `TestParityNewSessionIDCanBeLoaded`. |
| ACP client | `ratchet acp client exec --command <agent> "prompt"` | typed `acp-go-sdk` client over child-process stdio plus local JSON state under XDG state | Supported for one-shot exec, persisted session metadata, sessions list/show/status, multi-prompt FIFO `--no-wait` queue, explicit queue inspection/drain, cooperative cancel requests, ratchet-cli archive v1 export/import, serial compare, and JSON v1 ACP/compute flows | `TestACPClientExecBinarySmoke`; `TestDrainQueueAgainstFixtureProcessReusesSession`; `TestClientRunPromptAgainstFixtureProcess`; `TestSessionStoreLoadsMissingFileAndPersistsRecords`. |
| MCP | `ratchet mcp blackboard` / `ratchet mcp daemon` | stdio JSON-RPC blackboard or daemon server | Supported for standalone blackboard plus daemon session/project/blackboard/team status tools | `TestHarnessSmokeJSONRPCInitializeToolsListAndCall`; `TestDaemonMCPToolCallsUseDaemonClient`. |
| team | `ratchet team start "task"` | daemon team manager / mesh executor | Supported when provider configured | Existing team and mesh tests cover service behavior. |

## Temp Home Mock Provider Smoke

Use a throwaway home to avoid touching a real `~/.ratchet`:

```sh
tmp_home="$(mktemp -d)"
HOME="$tmp_home" ratchet version
HOME="$tmp_home" ratchet help
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

The ACP prompt smoke uses `acp-go-sdk` client and agent-side connections over
stdio-style pipes, a real ratchet daemon service, and the built-in mock provider:

```sh
go test ./internal/acp -run TestACPStdioPromptSmoke -count=1
```

## Competitor parity

The dated source-backed matrix lives in
[competitor-parity.md](competitor-parity.md). The snapshot was refreshed on
2026-07-02 from current Zed, ACP, Pi, Codex, Claude Code, Hermes, OpenClaw, and
ACPX sources. ratchet-cli now supports Windows release artifacts, ACP prompt stdio
smoke, headless ACP client exec/session/status/cancel primitives with
multi-prompt FIFO queue/watch/drain,
daemon-backed MCP blackboard/session/project/team status/message tools, runtime
trust slash commands, session lineage
history/clone/fork/tree commands, branch summaries, compaction records with
archive session links, Pi-style in-place branch navigation, and opt-in redacted
retro evidence, ACP client session archive export/import, serial compare, and
JSON v1 ACP/compute flows. The v0.20.0 release keeps Windows amd64/arm64 zip
artifacts in the GoReleaser output while adding ACP client archive, compare,
and flow commands. The policy boundaries are tracked in
[docs/policy-matrix.md](policy-matrix.md): runtime trust rules, persistent
trust grants, permission prompts, and explicit ACP client watch/drain are
supported, while daemon background drain, broad extension hooks, ACPX TypeScript
flow runtime compatibility, and local-first channel gateways remain deferred.

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
| import/export archives | Supported | `ratchet acp client sessions export <id> --output <archive.json>` writes ratchet-cli archive v1 JSON with ACPX-shaped metadata; `sessions import <archive.json> --session <id>` imports a copy. Binary smoke proves export/import through the built CLI and fixture ACP agent. Archives may contain prompt/response content and are not raw ACPX JSON-RPC event logs. |
| compare commands | Supported | `ratchet acp client compare --command <agent-a> --command <agent-b> "prompt"` runs agents serially and emits table or JSON rows. Binary smoke proves compare through the built CLI and fixture ACP agent. |
| flow commands | Supported | `ratchet acp client flow run flow.json --input-json '{"task":"x"}' --command <agent>` runs JSON v1 flows with `acp` and `compute` nodes, template prompts, shared ACP session handles, JSON output, and persisted run bundles. ACPX TypeScript flow runtime compatibility is deferred. |

### ACP client examples

```sh
ratchet acp client sessions export work --output work.archive.json
ratchet acp client sessions import work.archive.json --session work-copy

ratchet acp client compare --command ./agent-a --command ./agent-b "Review this patch"

ratchet acp client flow run flow.json \
  --input-json '{"task":"review release notes"}' \
  --command ./agent \
  --json

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

Scriptable equivalents are available through `ratchet trust list`,
`ratchet trust grants`, `ratchet trust allow|deny`,
`ratchet trust persist`, `ratchet trust revoke`, and `ratchet trust reset`.
The broader Policy Matrix lives in [docs/policy-matrix.md](policy-matrix.md),
including the explicit watch/drain boundary, sensitive local policy metadata
warning, and deferred background drain and extension hooks boundaries.
