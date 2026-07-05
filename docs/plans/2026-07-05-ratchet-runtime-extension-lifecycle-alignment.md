# Runtime Extension Lifecycle Alignment

**Verdict:** PASS

| requirement | coverage |
|---|---|
| Support skills, plugins, and hooks so autodev can function | T3-T5 first PR injects explicit plugin skills, reloads plugins, and wires hook callsites. |
| Marketplace support and marketplace updating | T1-T2 add catalog registry and update commands. |
| Plugin updating and hook updating | T2 updates installed plugin copies; T3 reloads updated hooks into daemon runtime while preserving hook hash trust. |
| Dynamic reloading | T3 daemon reload primitive. |
| Optional autoupdating per marketplace/plugin | T1-T2 registry includes marketplace and plugin autoupdate flags. |
| Mimic Claude/Codex hook breadth | Design adds session, prompt, tool, permission, compact, stop/failure, agent/workflow, notification/config/file events. |
| Dynamic workflows | T7 lays workflow definitions/runs over existing sessions/fleet/team primitives. |
| Scheduled tasks/routines | T6 adds visible persisted routines and due-run checks. |
| Messaging bridge | T8 places schema/projection in messaging-core and keeps transport in Workflow messaging plugins. |

## Scope Lock

Locked first PR: daemon reload, plugin skills in CLI/prompt, hook parity callsites, docs/tests.

Deferred by design: marketplace registry/update, routines/workflows, and messaging-core bridge. They remain required follow-up PRs, not optional backlog.

