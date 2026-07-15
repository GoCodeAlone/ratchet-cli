# Ratchet CLI Provider Operation Contract Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Make the existing provider `APPLIED` state truthful and reachable, preserve retryable finalization, and correct duplicate provider-type diagnostics.

**Architecture:** Keep SQLite as the operation authority and project each durable state one-to-one onto the existing protobuf enum. A query still retries finalization; failure returns metadata-only `APPLIED`, while a later success returns `COMMITTED`. The existing catalog, CLI, TUI, secret provider, and release pipeline are reused.

**Tech Stack:** Go 1.26, SQLite, gRPC/protobuf, Workflow `secrets.Provider`/Redactor, existing CLI daemon smoke harness, GoReleaser/Homebrew.

**Base branch:** master

---

## Scope Manifest

**PR Count:** 1
**Tasks:** 4
**Estimated Lines of Change:** ~260

**Out of scope:**
- New protobuf states, RPCs, database columns, migrations, or retry settings.
- Changes to secret-provider cancellation or custody.
- New provider SDKs, models, UI controls, or Windows daemon IPC claims.
- Returning raw finalization errors or credential-bearing data.

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|------|-------|-------|--------|
| 1 | fix(provider): expose applied operation state | Task 1, Task 2, Task 3, Task 4 | feat/provider-operation-contract |

**Status:** Draft

## Tasks

### Task 1: Correct Duplicate Canonical Type Diagnostics

**Files:**
- Modify: `internal/provider/catalog_test.go`
- Modify: `internal/provider/catalog.go`

**Steps:**
1. RED: change the duplicate-type table case to require the full error
   `duplicate provider type "anthropic"` (using the first catalog entry's
   actual type) rather than a substring.
2. Run:
   `go test ./internal/provider -run TestValidateCatalogRejectsInvalidEntriesAndRuntimeGaps -count=1`.
   Expected: FAIL because the current error appends
   `(already owned by "anthropic")`.
3. GREEN: return only `fmt.Errorf("duplicate provider type %q", entry.Type)`;
   retain owner details for distinct alias/name collisions.
4. Rerun the focused command. Expected: PASS.
5. Commit `fix(provider): clarify duplicate type diagnostics`.

Rollback: revert the focused catalog commit; catalog contents and runtime
provider behavior are unchanged.

### Task 2: Expose Applied State And Preserve Retry

**Files:**
- Modify: `internal/daemon/provider_operations_test.go`
- Modify: `internal/daemon/provider_operations.go`

**Steps:**
1. RED: add `TestProviderOperationStatePBMapsApplied` requiring internal
   `applied` → protobuf `APPLIED`.
2. RED: add `TestGetProviderOperationFinalizationFailureRemainsApplied`:
   seed a provider pointer and matching `applied` operation with non-secret
   result; force `secrets.Provider.Get` to fail; require `APPLIED`,
   `failure=UNSPECIFIED`, complete result, no sentinel/raw error, and SQL state
   still `applied`. Clear the failure, query again, and require `COMMITTED` plus
   SQL state `committed`.
3. Run:
   `go test ./internal/daemon -run 'ProviderOperationStatePBMapsApplied|GetProviderOperationFinalizationFailureRemainsApplied' -count=1`.
   Expected: FAIL because mapping and fallback both produce `PENDING`.
4. GREEN: split the mapping switch so `providerOperationApplied` maps to
   `PROVIDER_OPERATION_STATE_APPLIED`; remove the failed-finalization rewrite
   from `get`. Do not expose the internal error or alter journal state.
5. Rerun focused tests, then
   `go test ./internal/daemon -run 'ProviderOperation|ProviderMutationOrdering' -count=1`.
   Expected: PASS.
6. Commit `fix(provider): expose applied operation state`.

Rollback: revert the mapping/fallback commit. Existing persisted rows remain
schema-compatible and again project as `PENDING` until finalization.

### Task 3: Prove The Built Command Boundary And Document The Lifecycle

**Files:**
- Modify: `cmd/ratchet/harness_smoke_unix_test.go`
- Modify: `cmd/ratchet/harness_docs_test.go`
- Modify: `README.md`

**Steps:**
1. RED: add a lightweight `TestHarnessSmokeProviderAppliedState` using an
   isolated HOME, built ratchet binary, production daemon, SQLite DB, and file
   secret provider. Seed an `applied` operation whose secret is unavailable;
   run `ratchet provider operation <id> --json`; require `APPLIED`, complete
   result, unspecified failure, no credential/raw error, and SQL `applied`.
   Restore the secret, rerun the command, and require `COMMITTED` plus SQL
   `committed`. Keep it separate from the TLS provider smoke and skip only under
   `-race`, matching existing real-binary isolation.
2. RED: extend `TestHarnessDocsDescribeUnifiedProviderSetup` to require README
   wording for `PENDING`, `APPLIED`, `COMMITTED`, `FAILED`, the operation query
   command, and retry/recovery meaning.
3. Run:
   `go test ./cmd/ratchet -run 'HarnessSmokeProviderAppliedState|HarnessDocsDescribeUnifiedProviderSetup' -count=1 -timeout=10m`.
   Expected: FAIL because the smoke and lifecycle documentation do not exist.
4. GREEN: implement the smoke fixture using existing binary/daemon helpers.
   Update the README provider section: `PENDING` means not yet durably applied;
   `APPLIED` means provider state changed but finalization remains retryable;
   `COMMITTED`/`FAILED` are terminal; rerun the operation query for recovery.
5. Rerun the focused command, then
   `go test ./cmd/ratchet -run 'HarnessSmoke|HarnessDocsDescribeUnifiedProviderSetup' -count=1 -timeout=12m`.
   Expected: PASS with no sentinel in command output.
6. Commit `test(provider): prove applied status recovery`.

Rollback: revert smoke/docs changes with the daemon projection commit. Prior
binaries and persisted rows remain compatible.

### Task 4: Verify, Review, Merge, Release, And Close

**Files:**
- Modify only files named in Tasks 1-3 plus approved design/plan/ADR artifacts.

**Steps:**
1. Verify lock: run the autodev `plan-scope-check.sh --verify-lock` helper for
   this plan. Expected: PASS, 4 tasks/1 PR.
2. Run focused commands from Tasks 1-3. Expected: PASS.
3. Run `go test ./...`. Expected: all packages PASS.
4. Run the merge-gating selector exactly:
   `go test -race -coverprofile=coverage.out -covermode=atomic -skip '^TestACPClientExecBinarySmoke$' ./...`.
   Expected: PASS; the real-binary applied smoke is skipped under race.
5. Run `go vet ./...` and
   `golangci-lint run --new-from-rev=origin/master`. Expected: exit 0.
6. Run `go test ./internal/releaseguard -count=1`, `goreleaser check`, and
   `scripts/check-release-artifacts.sh --manifest-only <snapshot-dist>` via the
   repository's snapshot command. Expected: release guards PASS and six
   platform archives remain declared.
7. Build Windows command binaries:
   `GOOS=windows GOARCH=amd64 go build -o <tmp>/ratchet-amd64.exe ./cmd/ratchet`
   and arm64 equivalent. Expected: both exit 0. Native Windows CI remains the
   runtime gate; no Windows daemon claim is made.
8. Run `git diff --check`, machine-path scan, and inspect `git diff
   origin/master...HEAD`. Request code review; resolve every actionable thread
   and rerun affected/full gates.
9. Push `feat/provider-operation-contract`; open one PR with ADR/design/plan
   links and Step 1e activation evidence. Monitor all required checks and review
   threads until settled green, then admin squash-merge and delete the branch.
10. Confirm merge-commit CI green. Tag the exact merge commit with the next patch
    version; monitor Release to success. Verify public non-draft release,
    checksums, macOS/Linux/Windows amd64+arm64 assets, Formula+Cask version and
    hashes, direct archive `ratchet --version`, and a time-bounded installed
    Homebrew `ratchet --version` plus `ratchet provider operation --help`.
11. Write the post-merge retro, backfeed durable guidance only if warranted,
    complete the scope lock with verification evidence, and merge any closeout
    docs through a green PR. Release that merge as the next patch version.

Rollback: before merge, revert feature commits. After release, restore the prior
archive/Homebrew version and revert the merge; no schema or data downgrade is
required.

## Requirement Trace

| Design requirement | Plan task |
|---|---|
| Exact duplicate canonical-type diagnostic | Task 1 |
| Reachable one-to-one `APPLIED` projection | Task 2 |
| Failed finalization stays applied and retries | Task 2, Task 3 |
| Metadata-only result/no secret or raw error | Task 2, Task 3 |
| Built CLI → daemon → SQLite/file-secret proof | Task 3 |
| Human lifecycle documentation | Task 3 |
| Shared CLI/TUI compatibility | Task 2 focused/full existing tests, Task 4 |
| Windows/build/release/rollback gates | Task 4 |
