# Ratchet Blackboard + Notify Design

**Status:** Approved by user preauthorization, 2026-07-04
**Guidance:** workspace `docs/design-guidance.md`; ratchet-cli README and harness docs
**User ask:** identify next ratchet-cli work and start it; prioritize multi-agent/session communication across same-device terminals; consider Discord/Slack/etc via `github.com/nikoksr/notify` as a Workflow plugin rather than bespoke per-service integrations.

## Global Design Guidance

| guidance | design response |
|---|---|
| Workflow platform substrate; reuse over rebuild | Local coordination uses ratchet's existing daemon blackboard. External notifications are deferred to a reusable Workflow plugin, not ratchet-owned Slack/Discord adapters. |
| Go primary; stdlib-first; deps justified | PR1 adds no new dependency. Notify dependency belongs in a later plugin repo where the fanout surface is reusable. |
| wfctl/plugin ecosystem for new reusable capability | Notify bridge is recorded as `workflow-plugin-notify` follow-up; ratchet consumes it later through Workflow/plugin boundaries. |
| Secrets never logged | Blackboard CLI output only prints values the operator explicitly wrote; docs mark blackboard content sensitive. Notify credentials are future plugin secrets, not ratchet CLI flags in PR1. |
| Multi-component validation | PR1 proves real CLI → daemon gRPC → shared blackboard. Future Notify plugin must prove plugin host → Notify service adapter with stubbed external transports. |

## Context

- Existing ratchet capability:
  - daemon owns `mesh.Blackboard`
  - gRPC: `BlackboardRead`, `BlackboardWrite`, `BlackboardList`
  - MCP: `ratchet mcp daemon` exposes `bb_read`, `bb_write`, `bb_list`
  - no direct operator CLI for terminal-to-terminal blackboard use
- Existing portfolio capability:
  - `workflow-plugin-messaging-core`, `workflow-plugin-discord`, `workflow-plugin-slack`
  - no generic Notify-backed fanout plugin
- Notify source check: `github.com/nikoksr/notify` is a Go notification library with many service adapters. Its README warns about non-critical reliability and anti-spam responsibility.

## Approaches

| option | shape | trade-off | decision |
|---|---|---|---|
| A | Add `ratchet blackboard read/write/list` over existing daemon RPC | Fastest useful same-device coordination; no persistence or external deps | Chosen PR1 |
| B | Build `workflow-plugin-notify` first | Reusable outbound channel foundation, but does not solve same-device agent coordination by itself | Follow-up PR/repo |
| C | Add direct Slack/Discord to ratchet-cli | Immediate chat-app output, but duplicates existing plugins and hardcodes service deps into ratchet | Rejected |

## Design

- Add top-level `ratchet blackboard` CLI:
  - `list [section]` → sections or entries
  - `read <section> <key>` → value + metadata, nonzero if missing
  - `write <section> <key> <value...>` → stores joined value with `--author` defaulting from `$USER` or `ratchet-cli`
  - `--json` on all commands for script/agent consumption
- Keep data in the existing daemon blackboard for this slice:
  - same device, shared daemon, separate terminal instances
  - no daemon schema migration
  - no hidden background worker
- Document usage and sensitivity:
  - blackboard values can contain prompt/task context
  - users should not write secrets unless they accept local daemon exposure
- Record follow-up:
  - `workflow-plugin-notify` using `github.com/nikoksr/notify`
  - later ratchet bridge can send selected blackboard/team events to Workflow notify step, with credentials held by plugin config/secrets

## Security Review

| area | handling |
|---|---|
| Auth/authz | Same local daemon trust boundary as existing CLI/TUI/MCP commands. No remote listener added. |
| Secrets/PII | CLI does not redact explicit blackboard values because it is a read/write tool; docs warn values are local coordination data and may be sensitive. |
| Abuse/spam | No external delivery in PR1. Notify plugin follow-up must include rate/recipient controls and explicit non-critical-delivery docs. |
| Dependency trust | PR1 adds no dependency. Notify dependency reviewed in future plugin design. |

## Infrastructure Impact

- PR1: none. No cloud resources, no migrations, no network exposure beyond existing daemon socket.
- Future Notify plugin: new repo/release/registry manifest; external service credentials; CI should use stubs/fakes unless live env explicitly approved.

## Multi-Component Validation

| proof | command | expected |
|---|---|---|
| CLI parser/output | `go test ./cmd/ratchet -run TestHandleBlackboard -count=1` | read/write/list output and validation pass |
| Real daemon boundary | `go test ./cmd/ratchet -run TestHarnessSmokeBlackboardCLI -count=1` | built CLI writes through daemon, reads same value from a second invocation |
| Existing daemon RPC | `go test ./internal/daemon -run TestBlackboardRPCReadWriteList -count=1` | unchanged |

## Assumptions

| id | assumption | challenge | fallback |
|---|---|---|---|
| A1 | Existing daemon blackboard is acceptable as volatile session coordination | Users may expect restart persistence | Document volatile scope; persistence can be a later daemon storage task if demanded. |
| A2 | Top-level `blackboard` is clearer than burying under `mcp` or `team` | More top-level commands add clutter | Add concise help and keep command count minimal. |
| A3 | Notify belongs in a plugin, not ratchet-cli | Plugin repo work delays Slack/Discord delivery | Local blackboard is useful immediately; plugin avoids per-service duplication. |

## Self-Challenge

1. Laziest solution: document `ratchet mcp daemon` tools only. Rejected because terminal sessions need scriptable CLI without MCP client config.
2. Fragile assumption: volatile blackboard may surprise users after daemon restart. Mitigation: docs state local/daemon-scoped; no persistence claim.
3. YAGNI risk: `watch`/subscriptions. Deferred; first slice is CRUD only.

## Rollback

- Revert the `ratchet blackboard` command and docs. No state migration or external resource cleanup.

## Non-Goals

- No Discord/Slack/Notify integration in PR1.
- No daemon background scheduling or hidden delivery.
- No blackboard persistence across daemon restart.
- No remote multi-device mesh transport.
