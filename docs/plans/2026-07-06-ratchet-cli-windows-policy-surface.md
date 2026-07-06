# Ratchet CLI Windows and Policy Surface Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Prove non-interactive Windows `ratchet.exe` startup in CI and expose the existing policy matrix through `ratchet policy matrix`.

**Architecture:** Add a Windows-hosted CI job guarded by releaseguard tests. Add a read-only `cmd/ratchet` policy command backed by a small static table that mirrors `docs/policy-matrix.md`.

**Tech Stack:** Go, GitHub Actions, existing releaseguard YAML tests.

**Base branch:** master

---

## Scope Manifest

**PR Count:** 1
**Tasks:** 4
**Estimated Lines of Change:** ~350

**Out of scope:**
- No new policy evaluator or enforcement engine.
- No managed hooks, extension SDK, background drain, credentialed agent CI, Windows installer, or full packaged TUI runtime claim.

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|------|-------|-------|--------|
| 1 | Prove Windows startup and expose policy matrix | Task 1, Task 2, Task 3, Task 4 | feat/windows-policy-surface |

**Status:** Locked 2026-07-06T03:59:49Z

## Task 1: Guard Windows Command Startup CI

**Files:**
- Modify: `internal/releaseguard/workflow_test.go`

**Steps:**
1. Write failing releaseguard assertions for a `windows-release-smoke` CI job on `windows-2025`.
2. Require the job to build `./cmd/ratchet` to `$RUNNER_TEMP/.../ratchet.exe`.
3. Require the job to run `ratchet.exe --version` and `ratchet.exe help`.
4. Run `go test ./internal/releaseguard -run Windows -count=1`.

**Expected:** FAIL because the new CI job does not exist yet.

**Rollback:** Revert this task commit; no runtime state.

## Task 2: Add Windows Command Startup CI

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `internal/releaseguard/workflow_test.go`

**Steps:**
1. Add `windows-release-smoke` on `windows-2025`.
2. Use existing checkout, Go setup, private module env, and Git rewrite patterns.
3. Build native `ratchet.exe` under `$env:RUNNER_TEMP`.
4. Run `& $exe --version` and `& $exe help`.
5. Run `go test ./internal/releaseguard -run Windows -count=1`.

**Expected:** PASS and workflow test proves the real command binary launch path.

**Rollback:** Revert the workflow job and guard update; existing CI/release jobs remain.

## Task 3: Add Read-Only Policy Matrix Command

**Files:**
- Create: `cmd/ratchet/cmd_policy.go`
- Create/modify tests under: `cmd/ratchet/`
- Modify: `cmd/ratchet/main.go`

**Steps:**
1. Write failing tests for `ratchet policy matrix` text output containing supported, partial, explicit-operator, and deferred layers.
2. Write failing tests for `ratchet policy matrix --json` returning valid JSON rows without reading local policy state.
3. Implement a small static row table and command handler.
4. Wire the `policy` command into top-level dispatch and help.
5. Run `go test ./cmd/ratchet -run Policy -count=1`.

**Expected:** PASS and command exits without daemon/provider/network access.

**Rollback:** Revert command files and dispatch/help changes; no persisted state.

## Task 4: Reconcile Docs and Verification

**Files:**
- Modify: `README.md`
- Modify: `RATCHET.md`
- Modify: `docs/harness-emulation.md`
- Modify: `docs/competitor-parity.md`
- Modify: `docs/policy-matrix.md`
- Modify: `cmd/ratchet/harness_docs_test.go`

**Steps:**
1. Update docs to describe Windows command startup proof precisely while keeping full packaged TUI/installer runtime deferred.
2. Document `ratchet policy matrix [--json]` as a convenience view of `docs/policy-matrix.md`.
3. Update docs guard tests for the new wording.
4. Run `go test ./cmd/ratchet ./internal/releaseguard -count=1`.
5. Build and launch locally: `go build -o /tmp/ratchet-policy-surface ./cmd/ratchet`; run `--version`, `help`, `policy matrix`, and `policy matrix --json`.
6. Cross-build Windows: `GOOS=windows GOARCH=amd64 go build -o /tmp/ratchet-policy-surface.exe ./cmd/ratchet`.
7. Run `git diff --check`.

**Expected:** all commands exit 0; JSON parses; no stale docs claim packaged release `ratchet.exe` runtime remains wholly unproven.

**Rollback:** Revert docs/tests/command changes; remove temporary local binaries.
