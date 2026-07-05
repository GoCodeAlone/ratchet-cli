# Retro: Ratchet Runtime Extension Lifecycle

**Plan:** `docs/plans/2026-07-05-ratchet-runtime-extension-lifecycle.md`
**Scope:** 4 PR groups / 9 tasks
**Completed:** 2026-07-05

## Delivered

- PR #97: daemon plugin reload, plugin skills, hook parity.
- PR #102: plugin marketplace registry, install/update/enable/disable lifecycle.
- PR #103: visible routine and workflow definitions plus bounded run records.
- workflow-plugin-messaging-core #10: ratchet notification-event JSON/JSONL parser and `step.messaging_send` projection helpers.
- Closeout docs: marketplace, reload, routines, workflows, and messaging bridge boundaries.

## Gates That Worked

- Scope lock kept JavaScript workflow execution, hidden background workers, provider credentials, and managed hooks out of this lifecycle.
- Copilot review found real edge cases in marketplace validation, Windows-safe file replacement, UTF-8 truncation, and messaging bridge projection validation.
- Windows checks and local `GOOS=windows GOARCH=amd64 go build ./cmd/ratchet` caught platform-sensitive paths before merge.
- Docs guards preserved the boundaries around background drain, direct messaging credentials, JavaScript workflow runtime, ACPX TypeScript runtime compatibility, and managed hooks.

## Gate Misses

| Issue | Gate that missed | Fix |
|---|---|---|
| PR #102 and PR #103 needed post-review fixes for path/source parsing and store replacement semantics. | Initial implementation review did not explicitly include ambiguous-name and Windows rename failure paths. | Keep marketplace/source names delimiter-safe and use backup/restore replacement for stores where Windows overwrite semantics matter. |
| PR #103 broad `go test ./cmd/ratchet` still hit pre-existing interactive/provider hangs. | Acceptance command was too broad for the package's current test mix. | Use focused command/docs tests for feature PRs and track a separate cleanup for interactive/provider test isolation. |
| Cross-repo Task 8 could not be represented as a single GitHub PR in ratchet-cli. | Plan said PR4 but included a cross-repo implementation target. | Treat cross-repo plugin PRs as part of the locked phase evidence, then use ratchet-cli docs closeout for the local plan. |

## Follow-Ups

- Isolate or tag the hanging broad `cmd/ratchet` interactive/provider tests so `go test ./cmd/ratchet` is reliable for future acceptance runs.
- Consider a shared internal file replacement helper for JSON stores that need Windows-safe overwrite behavior.
- Decide whether visible routine definitions should later bind to daemon cron/session execution; keep that behind a new scope lock because hidden autonomy remains deferred.
- Decide whether workflow definitions should later invoke existing ACP/compute flow execution; keep shell/JavaScript source execution explicitly out of ratchet until separately designed.
