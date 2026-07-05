# ratchet Runtime Extension Lifecycle Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Make ratchet-cli a reloadable extension host with marketplace lifecycle management, plugin skills/hooks, visible routines/workflows, and a Workflow messaging bridge contract.

**Architecture:** Reuse the existing plugin loader, daemon reload, hook trust, skill discovery, cron/session/team primitives, and Workflow messaging plugin family. Ratchet owns local extension metadata and visible orchestration state; external notification delivery stays in Workflow plugins.

**Tech Stack:** Go, Cobra-style command handlers, ratchet daemon gRPC client/server surfaces, existing plugin manifest/installer packages, existing hooks/skills packages, workflow-plugin-messaging-core for cross-repo bridge contracts.

**Base branch:** master

---

## Scope Manifest

**PR Count:** 4
**Tasks:** 9
**Estimated Lines of Change:** ~1800

**Out of scope:**
- JavaScript or TypeScript workflow runtime execution.
- Hidden daemon background ACP drain or unattended queued prompt execution.
- Direct Slack, Discord, Teams, email, webhook, or provider credentials inside ratchet-cli.
- Managed enterprise hook policy, allowlists, or central administration.
- Full sandbox/path/network parity beyond existing policy-matrix rows.

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|------|-------|-------|--------|
| 1 | Runtime reload, plugin skills, and hook parity | Task 3, Task 4, Task 5 | feat/ratchet-runtime-reload |
| 2 | Marketplace registry and plugin lifecycle | Task 1, Task 2 | feat/ratchet-plugin-marketplace |
| 3 | Visible routines and workflows | Task 6, Task 7 | feat/ratchet-routines-workflows |
| 4 | Messaging bridge contract and closeout | Task 8, Task 9 | feat/ratchet-messaging-bridge |

**Status:** Locked 2026-07-05T05:45:22Z

## Current State

PR #97 already delivered Task 3, Task 4, and Task 5 on `master` as commit `34d592a`. Do not reimplement that slice. Continue with PR #2, then PR #3, then PR #4 unless the manifest is explicitly amended.

## Global Design Guidance

Source: workspace `AGENTS.md`, workspace `docs/PORTFOLIO.md`, workspace `docs/FOLLOWUPS.md`, ratchet-cli `docs/policy-matrix.md`, and ratchet-cli `docs/competitor-parity.md`.

| guidance | plan response |
|---|---|
| Reuse existing plugins/tools and avoid duplicate provider plumbing. | Marketplace work extends ratchet plugin metadata only; messaging delivery remains in existing Workflow messaging plugins. |
| Prefer Go-native reusable modules and existing repo patterns. | New lifecycle state lives under `internal/plugins`; routines/workflows reuse existing daemon/session/cron concepts. |
| Treat local prompts, commands, hooks, archives, and policy metadata as sensitive. | Tests and docs must assert no secret values are stored in profiles/marketplaces and no raw prompt text is passed to hooks by default. |
| Build for Windows. | New file path, atomic write, and command tests must avoid Unix-only assumptions; package tests run in normal Go CI including Windows. |

## Integration Matrix

| Integration | Classification | Proof |
|---|---|---|
| ratchet plugin registry/store | runtime-integrated | package tests and CLI tests read/write temp state and reload installed plugin metadata. |
| ratchet daemon plugin reload | runtime-integrated | existing PR #97 daemon reload tests. |
| ratchet routines/workflows | runtime-integrated | package and CLI tests create temp state, run manual routines/workflows, and assert persisted visible run records. |
| workflow-plugin-messaging-core | cross-repo runtime-integrated | PR #4 adds schema/projection tests in that repo and ratchet docs point to it. |
| Slack/Discord/Teams delivery | deferred | existing Workflow plugins own credentials/rate limits/redaction/delivery; ratchet does not post directly. |

## Task List

### Task 1: Marketplace Registry Core

**Files:**
- Create/modify: `internal/plugins/marketplace.go`
- Test: `internal/plugins/marketplace_test.go`

**Steps:**
1. Write failing tests for adding/listing/removing marketplace sources, loading catalog entries from local files, preserving `auto_update`, and rejecting malformed catalog entries.
2. Run `go test ./internal/plugins -run Marketplace -count=1`; expected RED on missing marketplace APIs.
3. Implement a small registry/store using JSON under ratchet state with atomic temp-file replacement and Windows-safe rename behavior already used elsewhere in the repo.
4. Add catalog structs with `name`, `description`, `version`, `source`, optional `sha256`, `relevance`, and `autoUpdate`.
5. Run `go test ./internal/plugins -run Marketplace -count=1`; expected PASS.
6. Rollback: revert Task 1 commit; no migrations or external resources.

### Task 2: Plugin CLI Lifecycle

**Files:**
- Modify: `cmd/ratchet/cmd_plugin.go`
- Modify as needed: `internal/plugins/installer.go`, `internal/plugins/loader.go`
- Test: `cmd/ratchet/cmd_plugin_test.go`, `internal/plugins/*_test.go`

**Steps:**
1. Write failing command tests for `plugin marketplace add/list/update/remove`, `plugin install name@marketplace`, `plugin update`, `plugin enable`, `plugin disable`, and help text.
2. Run `go test ./cmd/ratchet -run Plugin -count=1`; expected RED on missing subcommands/options.
3. Wire CLI commands to the marketplace registry and existing installer/loader paths.
4. Implement enabled/disabled metadata without deleting plugin files; disabled plugins must be skipped by loader tests.
5. Run `go test ./cmd/ratchet ./internal/plugins -run 'Plugin|Marketplace' -count=1`; expected PASS.
6. Rollback: revert Task 2 commit; user plugin directories remain untouched unless test temp dirs are used.

### Task 3: Daemon Reload

**Status:** Shipped in PR #97.

**Evidence:** `internal/daemon/plugin_reload_test.go`, `internal/daemon/service.go`, `internal/client/client.go`, and `cmd/ratchet/cmd_plugin.go` now cover `ratchet plugin reload` and daemon reload state.

### Task 4: Skill Runtime Integration

**Status:** Shipped in PR #97.

**Evidence:** `internal/skills/skills.go`, `internal/skills/skills_test.go`, and `cmd/ratchet/cmd_skill.go` now include plugin skills in list/show and explicit skill injection.

### Task 5: Hook Parity Slice

**Status:** Shipped in PR #97.

**Evidence:** `internal/hooks/hooks.go`, `internal/daemon/hooks_wiring_test.go`, and daemon hook callsites cover prompt, command, tool, permission, compact, stop/failure, cron, plan, fleet, and team-agent lifecycle points.

### Task 6: Routine Primitive

**Files:**
- Create: `internal/routines/store.go`
- Create: `internal/routines/store_test.go`
- Modify: `cmd/ratchet/main.go`
- Create: `cmd/ratchet/cmd_routines.go`
- Test: `cmd/ratchet/cmd_routines_test.go`

**Steps:**
1. Write failing tests for adding, listing, showing, pausing, resuming, removing, and manually running routine definitions in temp state.
2. Run `go test ./internal/routines ./cmd/ratchet -run Routine -count=1`; expected RED on missing package/commands.
3. Implement routine definitions with ID, schedule, prompt, cwd, provider, paused flag, created/updated timestamps, and last manual run metadata.
4. Implement CLI commands: `routines add`, `list`, `show`, `run`, `pause`, `resume`, `remove`. Manual run should create visible local run state and not start hidden background workers.
5. Run focused routine tests; expected PASS.
6. Rollback: revert Task 6 commit; no external resources.

### Task 7: Workflow Primitive

**Files:**
- Create: `internal/workflows/store.go`
- Create: `internal/workflows/store_test.go`
- Create: `cmd/ratchet/cmd_workflows.go`
- Modify: `cmd/ratchet/main.go`
- Test: `cmd/ratchet/cmd_workflows_test.go`

**Steps:**
1. Write failing tests for installing/listing/showing/running/stopping/resuming declarative workflow definitions from JSON/YAML-like graph files.
2. Run `go test ./internal/workflows ./cmd/ratchet -run Workflow -count=1`; expected RED.
3. Implement persisted workflow definitions and run records with bounded status transitions. The first slice may validate and record declarative graphs, but must not execute JavaScript or shell.
4. Wire CLI commands: `workflows list`, `show`, `run`, `stop`, `resume`.
5. Run workflow tests; expected PASS.
6. Rollback: revert Task 7 commit; no external resources.

### Task 8: Messaging Bridge Contract

**Files:**
- Cross-repo: `workflow-plugin-messaging-core`
- Ratchet docs only as needed: `README.md`, `docs/harness-emulation.md`, `docs/policy-matrix.md`

**Steps:**
1. In `workflow-plugin-messaging-core`, write failing tests for parsing ratchet notification-event JSON/JSONL and projecting `messaging.text` into `step.messaging_send` inputs.
2. Implement schema/projection helpers in messaging-core, not ratchet-cli.
3. Add docs proving ratchet exports local events only and channel/provider credentials stay in Workflow plugins.
4. Run messaging-core focused tests and ratchet docs tests.
5. Rollback: revert messaging-core and docs commits; no external resources.

### Task 9: Docs, Policy, and Closeout

**Files:**
- Modify: `README.md`
- Modify: `docs/harness-emulation.md`
- Modify: `docs/policy-matrix.md`
- Modify: workspace `docs/FOLLOWUPS.md` / `docs/PORTFOLIO.md` after release
- Create: `docs/retros/2026-07-05-ratchet-runtime-extension-lifecycle-retro.md`

**Steps:**
1. Update ratchet docs for marketplace lifecycle, plugin enable/disable/update/reload, routines, workflows, and messaging bridge boundaries.
2. Add or update docs guard tests so deferred background drain, direct messaging credentials, JavaScript workflow runtime, and managed hooks are not accidentally claimed.
3. Run `go test ./cmd/ratchet -run HarnessEmulationDocs -count=1`.
4. Run acceptance: `go test ./cmd/ratchet ./internal/skills ./internal/plugins ./internal/hooks ./internal/daemon ./internal/routines ./internal/workflows` and `go build ./cmd/ratchet`.
5. Release/version if the feature PRs merge cleanly, then update workspace portfolio/follow-up state.
6. Rollback: revert docs/metadata commits or publish a corrective release note if release metadata was already published.
