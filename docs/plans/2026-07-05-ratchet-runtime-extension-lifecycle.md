# ratchet Runtime Extension Lifecycle Plan

**Status:** Scope locked, 2026-07-05
**Design:** `docs/plans/2026-07-05-ratchet-runtime-extension-lifecycle-design.md`

## Task List

| id | task | deliverable | validation |
|---|---|---|---|
| T1 | Marketplace registry core | `internal/plugins/marketplace` support for add/list/update/remove catalogs and plugin update metadata | focused package tests |
| T2 | Plugin CLI lifecycle | `ratchet plugin marketplace ...`, `plugin update`, `plugin enable/disable`, `plugin reload` | CLI parser/unit tests |
| T3 | Daemon reload | Engine reload method that stops old plugin daemons and refreshes plugin skills/agents/commands/tools/hooks/MCP/profiles | daemon unit test with fixture plugin |
| T4 | Skill runtime integration | merged plugin/global/project skill discovery, namespacing, `skill list/show`, selective prompt injection for explicit skill mentions | skills tests plus chat prompt helper test |
| T5 | Hook parity slice | add canonical hook events/aliases and wire prompt, tool, permission, compact, stop, and error callsites | daemon hook wiring tests |
| T6 | Routine primitive | persistent routine definitions and CLI `routines list|add|run|pause|resume|remove` using normal sessions | package/CLI tests |
| T7 | Workflow primitive | persistent workflow definitions/runs and CLI `workflows list|show|run|stop|resume` using declarative graph stub over existing sessions/fleet | package/CLI tests |
| T8 | Messaging bridge contract | add ratchet notification-event schema/projection helpers to `workflow-plugin-messaging-core` | package tests in messaging-core |
| T9 | Docs and policy | README, policy matrix, harness docs, and follow-up state updated | docs grep and focused tests |

## First PR Cut

Deliver T3-T5 plus the minimum CLI support needed for visibility:

- fix plugin daemon retention on `EngineContext`;
- add daemon `ReloadPlugins` primitive and CLI `ratchet plugin reload`;
- include plugin skills in `ratchet skill list/show`;
- inject full plugin skill content when a prompt explicitly names it, especially `$autodev:using-autodev`;
- wire hook events for prompt, tool, permission, compact, stop, and error surfaces.

This makes autodev-style plugins functional inside ratchet before expanding marketplace storage.

## Second PR Cut

Deliver T1-T2:

- marketplace registry, catalog parsing, catalog update, plugin update, enable/disable, autoupdate flags;
- install by `name@marketplace`;
- docs for marketplace lifecycle.

## Third PR Cut

Deliver T6-T7:

- visible routine state and manual/scheduled due-run primitive;
- declarative workflow run state over existing session/team/fleet orchestration;
- hook events for workflow/routine starts/stops/failures.

## Fourth PR Cut

Deliver T8 and cross-repo messaging bridge tests in `workflow-plugin-messaging-core`.

## Acceptance

- `go test ./cmd/ratchet ./internal/skills ./internal/plugins ./internal/hooks ./internal/daemon`
- `go build ./cmd/ratchet`
- docs explain install/update/reload/skills/hooks workflows without implying direct messaging credentials in ratchet-cli.

