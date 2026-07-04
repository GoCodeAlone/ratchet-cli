# Ratchet Blackboard + Notify Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Add a direct `ratchet blackboard` CLI for same-device multi-session coordination, and document the Notify-backed Workflow plugin as the next outbound integration layer.

**Architecture:** Reuse the existing daemon `BlackboardRead`, `BlackboardWrite`, and `BlackboardList` gRPC APIs. Keep the first PR local-only and dependency-free; external notifications remain a documented follow-up for a Workflow plugin built around `github.com/nikoksr/notify`.

**Tech Stack:** Go, existing ratchet daemon gRPC client, existing `mesh.Blackboard`, stdlib JSON.

**Base branch:** master

---

## Scope Manifest

**PR Count:** 1
**Tasks:** 4
**Estimated Lines of Change:** ~260

**Out of scope:**
- Discord, Slack, or Notify delivery implementation.
- Blackboard persistence across daemon restart.
- Background daemon notification scheduler.
- Remote multi-device mesh.

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|------|-------|-------|--------|
| 1 | feat: add daemon blackboard CLI | Task 1, Task 2, Task 3, Task 4 | feat/blackboard-cli |

**Status:** Draft

### Task 1: Blackboard CLI Parser And Output Tests

**Files:**
- Create: `cmd/ratchet/cmd_blackboard_test.go`
- Modify: `cmd/ratchet/main.go`
- Create: `cmd/ratchet/cmd_blackboard.go`

**Step 1: Write failing tests**

Add tests around a fake blackboard client and captured stdout/stderr:
- `TestHandleBlackboardWritePrintsRevision`
- `TestHandleBlackboardReadPrintsValue`
- `TestHandleBlackboardListPrintsSectionsAndEntries`
- `TestHandleBlackboardJSONOutput`
- `TestHandleBlackboardValidation`

**Step 2: Verify red**

Run: `go test ./cmd/ratchet -run TestHandleBlackboard -count=1`
Expected: FAIL because `handleBlackboard` is undefined or command is unknown.

**Step 3: Minimal implementation**

Implement:
- top-level dispatch case `blackboard`
- `handleBlackboard(args []string)`
- local interface:
  - `BlackboardRead(ctx, section, key string)`
  - `BlackboardWrite(ctx, section, key, value, author string)`
  - `BlackboardList(ctx, section string)`
- `--json` and `--author` parsing
- tabular/plain output using existing protobuf fields

**Step 4: Verify green**

Run: `go test ./cmd/ratchet -run TestHandleBlackboard -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/ratchet/main.go cmd/ratchet/cmd_blackboard.go cmd/ratchet/cmd_blackboard_test.go
git commit -m "feat: add blackboard cli"
```

Rollback: revert commit; no daemon data migration.

### Task 2: Real CLI-To-Daemon Smoke

**Files:**
- Create: `cmd/ratchet/blackboard_harness_test.go`

**Step 1: Write failing smoke test**

Add `TestHarnessSmokeBlackboardCLI` that:
- builds/uses the ratchet test binary via existing harness helper
- uses temp `HOME`
- runs `ratchet blackboard write coordination status ready --author test-agent`
- runs `ratchet blackboard read coordination status`
- runs `ratchet blackboard list coordination --json`
- shuts the daemon down through existing cleanup path

**Step 2: Verify red**

Run: `go test ./cmd/ratchet -run TestHarnessSmokeBlackboardCLI -count=1`
Expected: FAIL before Task 1 implementation or fail if command does not reach daemon.

**Step 3: Implement or adjust harness**

Wire test to existing harness helpers. Do not create new daemon lifecycle code unless current helpers cannot express the scenario.

**Step 4: Verify green**

Run: `go test ./cmd/ratchet -run TestHarnessSmokeBlackboardCLI -count=1`
Expected: PASS; output includes `ready`.

**Step 5: Commit**

```bash
git add cmd/ratchet/*blackboard* cmd/ratchet/harness_smoke*_test.go
git commit -m "test: prove blackboard cli through daemon"
```

Rollback: revert commit; no runtime data cleanup.

### Task 3: Public Help And Docs

**Files:**
- Modify: `README.md`
- Modify: `docs/harness-emulation.md`
- Modify: `cmd/ratchet/main.go`

**Step 1: Write failing docs/help test**

Extend help-surface or command docs tests to require:
- `ratchet blackboard write coordination status ready`
- `ratchet blackboard read coordination status`
- blackboard row in harness docs.

**Step 2: Verify red**

Run: `go test ./cmd/ratchet -run 'TestCLIHelpSlashSurfaceMatchesCommandSpec|TestHarnessEmulationDocsCoverSupportedModesAndParity|TestHarnessDocsDescribeTUIBinaryEvidenceBoundaries' -count=1`
Expected: FAIL until docs/help mention the command.

**Step 3: Update docs/help**

Document:
- same-device/separate-terminal usage
- daemon-scoped volatile storage
- sensitivity warning
- Notify plugin follow-up as outbound integration path, not implemented behavior

**Step 4: Verify green**

Run: `go test ./cmd/ratchet -run 'TestCLIHelpSlashSurfaceMatchesCommandSpec|TestHarnessEmulationDocsCoverSupportedModesAndParity|TestHarnessDocsDescribeTUIBinaryEvidenceBoundaries' -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add README.md docs/harness-emulation.md cmd/ratchet/main.go cmd/ratchet/*test.go
git commit -m "docs: document blackboard coordination"
```

Rollback: revert commit.

### Task 4: Closeout Verification And Follow-Up Recording

**Files:**
- Modify: `docs/retros/2026-07-04-ratchet-blackboard-cli-retro.md`

**Step 1: Add retro**

Record:
- why local blackboard landed before Notify
- proof commands and outcomes
- follow-up: design `workflow-plugin-notify` around Notify with stubbed external delivery and registry/release plan

**Step 2: Run verification**

Run:
- `go test ./cmd/ratchet -run 'TestHandleBlackboard|TestHarnessSmokeBlackboardCLI|TestCLIHelpSlashSurfaceMatchesCommandSpec|TestHarnessEmulationDocsCoverSupportedModesAndParity|TestHarnessDocsDescribeTUIBinaryEvidenceBoundaries' -count=1`
- `go test ./internal/daemon -run 'TestBlackboardRPCReadWriteList|TestDaemonMCPToolCallsUseDaemonClient|TestMeshStream_BlackboardSync' -count=1`
- `go test ./cmd/ratchet ./internal/daemon -count=1`
- `go vet ./...`
- `git diff --check`

Expected: all commands exit 0.

**Step 3: Commit**

```bash
git add docs/retros/2026-07-04-ratchet-blackboard-cli-retro.md
git commit -m "docs: close blackboard cli slice"
```

Rollback: revert commit.
