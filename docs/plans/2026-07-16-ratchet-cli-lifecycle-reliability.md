# Ratchet CLI Lifecycle Reliability Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Make provider cleanup and ACP child-process lifecycle evidence causal, observable, bounded, and reliable under Linux and native Windows CI.

**Architecture:** Keep the daemon, SQLite, secret-provider, ACP SDK, and process ownership contracts. Return joined cleanup dispatch errors with rate-limited reporting, add shutdown/reconcile regressions, remove ACP's arbitrary post-send sleep, and isolate real-process tests from race coverage in dedicated Linux/Windows steps.

**Tech Stack:** Go 1.26.4, `database/sql`, `testing/synctest`, `exec.Cmd`, `github.com/coder/acp-go-sdk`, GitHub Actions, releaseguard.

**Base branch:** `master`

---

## Scope Manifest

**PR Count:** 2
**Tasks:** 6
**Estimated Lines of Change:** ~320

**Out of scope:**
- New RPCs, protobuf state, metrics/log frameworks, cleanup schema/retry changes, process-tree ownership, SDK replacement, or runner changes.
- GitHub Actions major-version and Docker dependency maintenance; those remain the next phase.
- Self-improving harness features; they follow reliability/dependency work.

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|---|---|---|---|
| 1 | `fix: harden lifecycle cleanup and process smoke` | Task 1, Task 2, Task 3, Task 4, Task 5 | `fix/reliability-followups` |
| 2 | `docs: close lifecycle reliability plan` | Task 6 | `docs/lifecycle-reliability-closeout` |

**Status:** Complete 2026-07-16T15:00:54Z

## Guidance Mapping

Source: `docs/design-guidance.md`

| Guidance | Task |
|---|---|
| bounded/inspectable background failure | Task 1 |
| shared daemon/service contract | Task 2 |
| real-process smoke outside race coverage | Task 3, Task 4 |
| native Windows runtime proof | Task 4, Task 5 |
| release only merge commits with archive/Homebrew/runtime proof | Task 5, Task 6 |

### Task 1: Provider Cleanup Completion And Diagnostics

**Files:**
- Modify: `internal/daemon/provider_cleanup.go`
- Modify: `internal/daemon/provider_cleanup_test.go`

**Step 1: Write RED tests**

- `TestProviderCleanupCandidateRowsPreservePrimaryAndCloseErrors`: fake rows return a scan/iteration sentinel and independent close sentinel; require `errors.Is` for both.
- `TestProviderCleanupDispatchReturnsQueryFailure`: construct a manager without starting its loop, set its internal context to `t.Context()`, close its database, invoke dispatch, and require a classified candidate-query error. The live context prevents `context.Canceled` from masking the database failure.
- `TestProviderCleanupErrorReporterSuppressesEquivalentFailures`: inject clock/log function; same error logs once inside one minute, logs after one minute, and nil resets suppression.
- Make `TestProviderCleanupDispatcherFairness` wait for both four provider deletes and cleanup-row count zero.

Run: `go test ./internal/daemon -run '^TestProviderCleanup(CandidateRows|DispatchReturnsQueryFailure|ErrorReporter|DispatcherFairness)' -count=1`

Expected: FAIL because candidate collection/reporter APIs and dispatch error return do not exist.

**Step 2: Implement minimal behavior**

- Add a minimal rows interface (`Next`, `Scan`, `Err`, `Close`) and `collectProviderCleanupCandidates` with a named error return and deferred `errors.Join(err, rows.Close())`.
- Change `dispatchCleanup() error`; wrap query/scan/iterate/close errors and return nil after scheduling.
- Add loop-local reporter state. Equivalent error strings log at most once/minute; nil resets. Log only `provider cleanup: dispatch: <error>` without candidate names, provider payloads, prompts, or secrets.
- Keep worker count, retry schedule, schema, and cleanup semantics unchanged.

**Step 3: Verify GREEN and stress**

```bash
go test ./internal/daemon -run '^TestProviderCleanup' -count=1
go test ./internal/daemon -run '^TestProviderCleanupDispatcherFairness$' -count=30 -timeout=2m
go test -race ./internal/daemon -run '^TestProviderCleanup' -count=10 -timeout=5m
```

Expected: PASS; four deletes, two poison attempts, max concurrency 1..2, zero cleanup rows.

**Step 4: Regression proof and commit**

Temporarily keep the new method signatures but discard the query error, drop the joined close error, and disable reporter suppression while retaining tests; focused selector must FAIL behaviorally rather than only failing to compile. Restore implementation; expect PASS. Record both outputs in PR #1.

```bash
git add internal/daemon/provider_cleanup.go internal/daemon/provider_cleanup_test.go
git commit -m "fix(daemon): surface cleanup failures"
```

### Task 2: Provider Operation Shutdown Regressions

**Files:**
- Modify: `internal/daemon/provider_operations_test.go`

**Step 1: Add service and reconcile coverage**

- In `TestProviderOperationRequestsAfterStopReturnUnavailable`, call `svc.GetProviderOperation(...)`, not the manager; expect `codes.Unavailable` after database close.
- Add `TestProviderOperationStopCancelsStartupReconciliation`: seed an applied operation/secret, block `provider.getHook` until context cancellation, start manager in a goroutine, wait for the hook, call `Stop`, then require `Start` matches `context.Canceled`, the error is sanitized/finalization-classified, and `Stop` joins.
- This is missing characterization coverage; no production change is expected.

**Step 2: Verify and commit**

```bash
go test ./internal/daemon -run '^TestProviderOperation(RequestsAfterStopReturnUnavailable|StopCancelsStartupReconciliation)$' -count=30 -timeout=3m
go test -race ./internal/daemon -run '^TestProviderOperation(RequestsAfterStopReturnUnavailable|StopCancelsStartupReconciliation)$' -count=10 -timeout=5m
git add internal/daemon/provider_operations_test.go
git commit -m "test(daemon): cover reconcile shutdown"
```

Expected: PASS; service calls are `Unavailable`; reconcile stop always cancels/joins without sensitive metadata.

### Task 3: ACP Cancellation And Real-Process Contract

**Files:**
- Modify: `internal/acpclient/client.go`
- Modify: `internal/acpclient/client_test.go`
- Modify: `internal/acpclient/fixture_test.go`
- Modify: `internal/acpclient/background_process_lock_test.go`
- Modify: `internal/acpclient/profiles_process_test.go`

**Step 1: Write RED cancellation timing test**

Add `TestClientCancellationNoPostSendDelay` with `testing/synctest`, closable in-process transport, and immediate cancellation. Fake elapsed time around the watcher must be zero. Current code advances 100 ms and must FAIL.

Keep exact-once peer handling in the in-process test. Rename the three real-process tests to begin `TestACPClientLifecycleBinarySmoke`; the cancellation case proves prompt return/process reap/idempotent close, not peer handling before forced termination.

Run: `go test ./internal/acpclient -run '^TestClientCancellationNoPostSendDelay$' -count=1`

Expected: FAIL with elapsed `100ms`, want `0s`.

**Step 2: Remove grace and stabilize process bounds**

- After ACP `Cancel` send completion, immediately join the send error with `terminateCancellation()`; retain bounded send context, transport closure/join, process kill/reap, and cause precedence.
- Add shared test constant `acpClientProcessSmokeTimeout = 30 * time.Second` plus channel/poll helper.
- Use it for named profile launch/start-return/child-exit and cancellation prompt/return waits. Keep 200 ms lock-blocked negative assertions.
- On a child-exit timeout, kill the process and drain its `Wait` result before failing the test so the failure path does not leak a child/zombie.

**Step 3: Verify, regression-proof, and commit**

```bash
go test ./internal/acpclient -run '^TestClientCancellation' -count=10 -timeout=5m
go test ./internal/acpclient -run '^TestACPClientLifecycleBinarySmoke' -count=20 -timeout=10m
go test -race ./internal/acpclient -skip '^TestACPClientLifecycleBinarySmoke' -run 'Cancellation|BackgroundProfile' -count=5 -timeout=10m
```

Expected: PASS; zero fake-time grace, exact-once in-process cancel, reaped child, and profile lock held through real start success/failure acknowledgment.

Restore only the 100 ms grace; timing test must FAIL. Remove it; expect PASS. Record both outputs.

```bash
git add internal/acpclient/client.go internal/acpclient/client_test.go internal/acpclient/fixture_test.go internal/acpclient/background_process_lock_test.go internal/acpclient/profiles_process_test.go
git commit -m "fix(acp): bound process lifecycle smoke"
```

### Task 4: Linux And Windows CI Smoke Isolation

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `internal/releaseguard/workflow_test.go`

**Step 1: Write RED structured workflow guard**

Add `TestCIIsolatesACPClientLifecycleBinarySmoke` using `loadWorkflow`, `requireJob`, and `requireRun`. Require:

- Linux `test` step `Run ACP client lifecycle smoke`: `go test ./internal/acpclient -run '^TestACPClientLifecycleBinarySmoke' -count=1 -timeout=5m`.
- Existing native Windows job has the same step/command.
- Race step contains `-skip '^(TestACPClientExecBinarySmoke|TestACPClientLifecycleBinarySmoke)'`.

Run: `go test ./internal/releaseguard -run '^TestCIIsolatesACPClientLifecycleBinarySmoke$' -count=1`

Expected: FAIL because steps/paired skip are absent.

**Step 2: Update CI without changing runners**

- Add Linux lifecycle smoke after existing command-binary smoke.
- Add the same selector after Windows ACP persistence tests.
- Replace the race skip with the paired anchored regex.
- Do not change `runs-on`, action versions, job dependencies, release workflow, or runner count.

**Step 3: Verify, regression-proof, and commit**

```bash
go test ./internal/releaseguard -run 'CIIsolatesACPClientLifecycleBinarySmoke|CIRequiresNativeWindowsBackgroundPersistence|WorkflowsAvoidStaleWindowsRunner' -count=1
go test ./internal/acpclient -run '^TestACPClientLifecycleBinarySmoke' -count=1 -timeout=5m
go test -race ./internal/acpclient -skip '^TestACPClientLifecycleBinarySmoke' -count=1 -timeout=15m
```

Expected: PASS; parser finds Linux/Windows steps and exact skip; host executes three process tests.

Remove either dedicated step while retaining guard; guard must FAIL. Restore; expect PASS. Record both outputs.

Rollback: revert commit, restore old race selector, rerun old race command plus one untagged process selector.

```bash
git add .github/workflows/ci.yml internal/releaseguard/workflow_test.go
git commit -m "ci: isolate ACP process lifecycle smoke"
```

### Task 5: PR 1 Verification, Merge, And Release

**Files:**
- Verify all PR #1 changes.

**Step 1: Local gates**

```bash
go test ./internal/daemon ./internal/acpclient ./internal/releaseguard -count=1 -timeout=15m
go test -race ./internal/daemon ./internal/acpclient ./internal/releaseguard -skip '^TestACPClientLifecycleBinarySmoke' -count=1 -timeout=20m
GOTMPDIR=$HOME/.rt GOCACHE=$HOME/.rc go test -p 1 ./... -count=1 -timeout=30m
go vet ./...
golangci-lint run --new-from-rev=origin/master
git diff --check
goreleaser check
```

Expected: exits 0; lint reports 0 issues.

**Step 2: Runtime and Windows build proof**

```bash
go build -o /tmp/ratchet-lifecycle ./cmd/ratchet
/tmp/ratchet-lifecycle --version
/tmp/ratchet-lifecycle provider setup list --json
GOOS=windows GOARCH=amd64 go build -o /tmp/ratchet-windows-amd64.exe ./cmd/ratchet
GOOS=windows GOARCH=arm64 go build -o /tmp/ratchet-windows-arm64.exe ./cmd/ratchet
```

Expected: host commands exit within 10 seconds; provider JSON has 22 entries; Windows outputs are non-empty PE files. Native Windows lifecycle smoke must pass on PR.

Rollback: do not merge on failure; revert responsible task commit and rerun focused/runtime proof.

**Step 3: Review, PR, monitor, merge**

- Invoke `autodev:requesting-code-review`; process findings with `autodev:receiving-code-review`.
- Confirm `gh >= 2.88` before/after PR creation. Push branch, open PR with design/plan, RED/GREEN/revert proof, security boundary, verification, and Scope Manifest.
- Add Copilot immediately and invoke `autodev:pr-monitoring` immediately.
- Wait for all required checks, native Windows, CodeQL, and threads; fix and continue monitoring.
- Admin squash-merge only when green/resolved; delete remote branch.

**Step 4: Release merge commit**

Tag next patch from merge commit; wait for release workflow. Verify seven public assets, six archive checksums, Formula/Cask versions/hashes, Homebrew install/upgrade, bounded installed `ratchet --version`, and provider catalog count 22.

Expected: public release; every package/runtime check passes; no draft remains.

### Task 6: Retro, Closeout PR, And Release

**Files:**
- Create: `docs/retros/2026-07-16-ratchet-cli-lifecycle-reliability-retro.md`
- Modify: `docs/plans/2026-07-16-ratchet-cli-lifecycle-reliability.md`
- Modify only for a durable new rule: `docs/design-guidance.md`

**Step 1: Evidence-based retro**

Invoke `autodev:post-merge-retrospective`. Correlate D1-D4, plan findings, RED/GREEN/revert proof, code review, PR/native-Windows checks, merge SHA, release run, archives/Homebrew/runtime, and misses. Do not claim unavailable activation evidence.

**Step 2: Close plan and publish PR #2**

Mark complete only with merge/release evidence. Verify manifest and `git diff --check`. Branch current `origin/master` as `docs/lifecycle-reliability-closeout`; review/Copilot/PR-monitoring; admin squash-merge only green/resolved.

**Step 3: Release PR #2 and continue**

Publish next patch from closeout merge; repeat seven-asset, checksum, Formula/Cask, Homebrew, bounded version, and 22-provider proofs. Reconcile workspace portfolio/projects/followups/phase JSONL through an isolated workspace PR without touching the dirty root branch. Continue automatically to Actions runtime maintenance and upstream Docker remediation, then self-improving harness work.
