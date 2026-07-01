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
| session lineage | `ratchet sessions history`, `ratchet sessions clone`, `ratchet sessions fork`, `ratchet sessions tree`, `ratchet sessions summary`, `ratchet sessions compactions` | daemon gRPC session history/clone/fork/tree/summary/compaction APIs | Supported for separate fork/clone sessions, branch summaries, persisted compaction records, and archive session links | `TestSessionLineageHistoryCloneForkTreeRPC`; `TestCompactionRecordRPC`; `TestHandleSessionsHistoryCloneForkTree`. |
| ACP | `ratchet acp` | ACP stdio JSON-RPC agent wrapping daemon service | Supported for initialize/new/load/prompt/cancel/model/mode | `TestACPStdioPromptSmoke`; `TestHarnessSmokeInitializeNewAndLoadSession`; `TestParityNewSessionIDCanBeLoaded`. |
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
2026-07-01 from current Zed, ACP, Pi, Codex, Claude Code, OpenClaw, and ACPX
sources. ratchet-cli is now supported for Windows release artifacts, ACP prompt
stdio smoke, daemon-backed MCP blackboard/session/project/team status tools,
session lineage history/clone/fork/tree commands, branch summaries, compaction
records with archive session links, and opt-in redacted retro evidence.
Deferred rows remain Pi-style in-place tree navigation, broader policy layering, extension
hooks, full daemon direct team messaging, ACP client/orchestrator mode, and
local-first channel gateways.

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
| `team_message` | Deferred by daemon | MCP exposes the tool and surfaces daemon errors; daemon `DirectMessage` still returns unimplemented. |
| Claude Code config | Supported | `WriteMCPConfig` tests. |
| Copilot config | Supported | `WriteCopilotMCPConfig` API and `ratchet mcp config copilot`. |
| generic MCP config | Supported | `WriteGenericMCPConfig` and `ratchet mcp config generic`. |
| daemon-backed blackboard | Supported | Unary daemon API added for MCP reads, writes, and lists; `TestBlackboardRPCReadWriteList`. |
| daemon-backed team tools | Partial | Team list/status are daemon-backed; direct message remains daemon-deferred. |
