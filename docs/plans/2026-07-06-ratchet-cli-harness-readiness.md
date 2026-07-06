# Ratchet CLI Harness Readiness Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Add credential-free all-profile readiness checks, policy status filtering, and portable retro handoff bundles.

**Architecture:** Extend existing `cmd/ratchet` command handlers and reuse existing profile verification, policy matrix rows, and retro analysis/rendering code.

**Tech Stack:** Go, existing command tests, existing docs guard tests.

**Base branch:** master

---

## Scope Manifest

**PR Count:** 1
**Tasks:** 5
**Estimated Lines of Change:** ~650

**Out of scope:**
- No daemon background drain or scheduler.
- No managed hooks, extension SDK execution, or automatic PR creation.
- No credentialed third-party agent CI or provider secrets.
- No raw retro evidence copy into generated bundles.

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|------|-------|-------|--------|
| 1 | Add harness readiness utilities | Task 1, Task 2, Task 3, Task 4, Task 5 | feat/harness-readiness |

**Status:** Complete 2026-07-06T06:46:14Z

**Completion evidence:** PR #118 merged at `a67d4bb`; local `go test ./... -count=1` passed after review fixes; merge-commit CI, CodeQL, and release workflow passed; release `v0.30.14` published Linux, macOS, Windows, and checksum assets.

## Task 1: Plan Lock and Baseline

**Files:**
- Add: `docs/plans/2026-07-06-ratchet-cli-harness-readiness-design.md`
- Add: `docs/plans/2026-07-06-ratchet-cli-harness-readiness.md`
- Add: review/alignment docs

**Steps:**
1. Record design, implementation plan, adversarial review, and alignment.
2. Lock scope.
3. Run focused baseline tests.

**Expected:** PASS baseline on `./cmd/ratchet ./internal/acpclient ./internal/retro ./internal/doctor`.

**Rollback:** Delete new plan docs and scope lock.

## Task 2: Add All-Profile Verification

**Files:**
- Modify: `cmd/ratchet/cmd_acp_client.go`
- Modify: `cmd/ratchet/cmd_acp_client_test.go`

**Steps:**
1. Write failing parser tests for `profiles verify --all [--json]`.
2. Write failing execution tests proving trusted profiles are launched, untrusted profiles are skipped, output is redacted, and JSON reports per-profile status.
3. Implement all-profile verification by reusing existing profile store and verify runner paths.
4. Run `go test ./cmd/ratchet -run 'ACPClientProfilesVerify' -count=1`.

**Expected:** PASS and no prompt/response/env values are printed.

**Rollback:** Revert command/test changes; existing single-profile verify remains.

## Task 3: Add Policy Status Filtering

**Files:**
- Modify: `cmd/ratchet/cmd_policy.go`
- Modify: `cmd/ratchet/cmd_policy_test.go`

**Steps:**
1. Write failing tests for `ratchet policy matrix --status deferred` text and JSON.
2. Reject unknown statuses.
3. Implement row filtering and usage text.
4. Run `go test ./cmd/ratchet -run Policy -count=1`.

**Expected:** PASS and command remains read-only static metadata.

**Rollback:** Revert policy parser/filter changes.

## Task 4: Add Retro Bundle

**Files:**
- Modify: `cmd/ratchet/cmd_retro.go`
- Modify: `cmd/ratchet/cmd_retro_test.go`
- Modify or add internal retro tests if helper extraction is needed.

**Steps:**
1. Write failing tests for `ratchet retro bundle --evidence <file> --output <dir>`.
2. Assert bundle contains `analysis.json`, `instructions.md`, and `manifest.json`.
3. Assert raw evidence JSONL is not copied.
4. Implement bundle writing with user-only permissions and portable paths.
5. Run `go test ./cmd/ratchet ./internal/retro -run 'Retro|Evidence' -count=1`.

**Expected:** PASS and generated bundle is local handoff metadata only.

**Rollback:** Revert retro command/test changes; existing analyze/instructions remain.

## Task 5: Docs, Verification, PR, Release

**Files:**
- Modify: `README.md`
- Modify: `docs/harness-emulation.md`
- Modify: `docs/policy-matrix.md`
- Modify: `docs/retro-loop.md`
- Modify: `cmd/ratchet/harness_docs_test.go`

**Steps:**
1. Document all-profile verify, policy status filtering, and retro bundle boundaries.
2. Update docs guards for new command mentions and local-sensitive warnings.
3. Run focused tests, full `go test ./cmd/ratchet ./internal/acpclient ./internal/retro ./internal/doctor -count=1`, `git diff --check`.
4. Build and launch a local binary for `--version`, `policy matrix --status deferred --json`, and command help.
5. Cross-build Windows amd64.
6. Open PR, monitor CI, admin merge when green, tag/release, update workspace state.

**Expected:** all verification passes; release assets publish through existing release pipeline.

**Rollback:** Revert docs/tests/command changes; remove any temporary local binaries.
