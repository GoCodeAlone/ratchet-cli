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
| ACP | `ratchet acp` | ACP stdio JSON-RPC agent wrapping daemon service | Partial | `TestHarnessSmokeInitializeNewAndLoadSession`; prompt streaming needs a real ACP client connection. |
| MCP | `ratchet mcp blackboard` | stdio JSON-RPC blackboard server | Supported for standalone blackboard | `TestHarnessSmokeJSONRPCInitializeToolsListAndCall`. |
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

## Competitor parity

Source snapshots used for this matrix were captured during the June 30, 2026
design pass:

| Harness | Source snapshot | Useful capability | ratchet-cli status |
|---|---|---|---|
| Zed / ACP | `zed-industries/zed@df33d78`, `agent-client-protocol@a0186bd` | ACP session lifecycle, plan updates, tool metadata, sandbox approvals | Partial; PR3 expands ACP lifecycle/config parity. |
| Pi | `earendil-works/pi@dd87c02` | JSONL session tree, fork/compact, extension/skills/themes | Partial; sessions exist, fork/compact parity deferred. |
| Codex | `openai/codex@db887d0` | filesystem sandbox, exec policy, hooks, MCP contributors, daemon/cloud tasks | Partial; trust policy and hooks exist, broader sandbox/extension parity deferred. |
| Claude Code | official docs checked during design | MCP, hooks, permissions, subagents, memory, policy layers | Partial; MCP config helpers and agent/team concepts exist, policy-layer parity deferred. |
| OpenClaw | `openclaw@a841c278` | local-first gateway, isolated agents/workspaces/channel routing, ACPX plugin | Deferred; voice/mobile/canvas/channel gateway is out of scope for this phase. |

## ACP Matrix

| ACP capability | Status | Evidence |
|---|---|---|
| initialize | Supported | `RatchetAgent.Initialize`; `TestHarnessSmokeInitializeNewAndLoadSession`. |
| new session | Supported | `RatchetAgent.NewSession`; service-backed test. |
| load session | Partial | Loads ratchet session IDs; ACP ID resume mapping needs PR3. |
| prompt | Partial | Implemented through daemon `SendMessageChan`; full stdio client smoke deferred to PR3. |
| cancel | Supported | `RatchetAgent.Cancel`. |
| plan updates | Partial | Chat event conversion supports plan proposed/step update events. |
| session config/model/mode | Partial | Model/mode hooks exist; PR3 tightens truthful capability reporting. |

## MCP Matrix

| MCP tool/config | Status | Evidence |
|---|---|---|
| initialize | Supported | `BBMCPServer.handleInitialize`; JSON-RPC smoke. |
| tools/list | Supported | Exposes `bb_read`, `bb_write`, `bb_list`. |
| `bb_write` | Supported | JSON-RPC smoke writes `smoke/status`. |
| `bb_read` | Supported | JSON-RPC smoke reads back `ok`. |
| `bb_list` | Supported | Existing blackboard tests. |
| Claude Code config | Supported | `WriteMCPConfig` tests. |
| Copilot config | Supported | `WriteCopilotMCPConfig` API present; broader export UX deferred to PR3. |
| daemon-backed MCP tools | Deferred | PR3 adds daemon-backed session/project/team tools. |
