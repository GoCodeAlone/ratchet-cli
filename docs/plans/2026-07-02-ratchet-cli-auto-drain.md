# ratchet-cli Auto-Drain Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Add an explicit opt-in `ratchet acp client watch` worker that repeatedly drains queued ACP client prompts under the policy matrix boundaries.

**Architecture:** The worker is a foreground CLI loop that polls local ACP client session state and delegates execution to the existing `internal/acpclient.DrainQueue` path. It does not persist launch argv, start a daemon scheduler, add a policy engine, or print prompt bodies in watch summaries.

**Tech Stack:** Go 1.26.4, standard library CLI parsing, existing `internal/acpclient` store/drain types, existing fixture ACP agent for binary smoke.

**Base branch:** master

---

## Scope Manifest

**PR Count:** 1
**Tasks:** 6
**Estimated Lines of Change:** ~450

**Out of scope:**
- Hidden daemon/background service scheduling.
- Persisted launch profiles or argv reconstruction from stored command fingerprints.
- Multi-session global scheduler.
- Extension hooks or mutation-capable lifecycle interception.
- New trust, sandbox, path, network, or secrets policy engine.
- ACPX raw event-log or TypeScript runtime compatibility.
- Release tag, Homebrew publish, or registry changes.

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|------|-------|-------|--------|
| 1 | `feat: add explicit ACP client watch drain` | Task 1, Task 2, Task 3, Task 4, Task 5, Task 6 | `feat/ratchet-cli-auto-drain-policy` |

**Status:** Draft

## Integration Matrix

| Integration | Classification | Proof |
|---|---|---|
| Existing ACP child-process client | runtime-integrated | Task 3 builds `ratchet` and the fixture ACP agent, queues prompts through the CLI, runs `ratchet acp client watch`, and verifies status through the built CLI. |
| Local ACP client state store | runtime-integrated | Task 1 and Task 2 unit tests exercise `internal/acpclient.Store` with temp state files; Task 3 repeats through CLI commands. |
| Policy/harness docs | config-only | Task 4 updates public docs and `cmd/ratchet/harness_docs_test.go` guards required text. |
| Windows artifacts | config-only | Task 6 runs `GOOS=windows GOARCH=amd64 go build` and `GOOS=windows GOARCH=arm64 go build` to prove compile portability. |

### Task 1: Add Internal Watch Loop Tests

**Files:**
- Create: `internal/acpclient/watch_test.go`
- Modify: none

**Step 1: Write the failing tests**

Add tests for:
- `WatchQueue` drains pending prompts in cycles by calling the existing drain runner.
- `StopWhenEmpty` reports one idle cycle and exits without starting an agent.
- `MaxCycles` stops after the configured number of cycles.
- an existing owner lock returns `ErrDrainBusy` through the watch path.
- cycle summaries do not include queued prompt text.

Use a fake `DrainPromptRunner` in the same style as `internal/acpclient/drain_test.go`. Use a fake sleep function that records durations and returns immediately so tests are deterministic.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/acpclient -run WatchQueue -count=1`

Expected: FAIL with `undefined: WatchQueue` or missing `WatchOptions`/`WatchCycle`.

**Step 3: Commit failing tests**

```bash
git add internal/acpclient/watch_test.go
git commit -m "test: cover acp client watch loop"
```

**Rollback:** revert this test-only commit.

### Task 2: Implement Internal Watch Loop

**Files:**
- Create: `internal/acpclient/watch.go`
- Modify: `internal/acpclient/store.go` only if a small exported queue-count helper is needed
- Test: `internal/acpclient/watch_test.go`

**Step 1: Implement minimal code**

Add:
- `WatchOptions` with `Interval`, `MaxPerCycle`, `MaxCycles`, `StopWhenEmpty`, `Now`, `Sleep`, and `StartRunner`.
- `WatchCycle` with aggregate fields: `SessionID`, `ACPSessionID`, `Cycle`, `PendingBefore`, `Processed`, `Completed`, `Failed`, `Canceled`, `Remaining`, `Idle`, `StartedAt`, `CompletedAt`.
- `WatchResult` with aggregate totals and final remaining count.
- `WatchQueue(ctx, store, spec, runOpts, sessionID, opts, onCycle)` that validates inputs, polls pending counts, calls `DrainQueue` with `MaxPerCycle`, emits aggregate cycle data, sleeps via `Sleep` or a context-aware default, and returns on drain error, max cycles, stop-when-empty, or context cancellation.

Do not store command argv or reconstruct launch options from `SessionRecord.CommandFingerprint`.

**Step 2: Run tests to verify they pass**

Run: `go test ./internal/acpclient -run 'WatchQueue|DrainQueue' -count=1`

Expected: PASS.

**Step 3: Regression invariant proof**

Temporarily remove the `DrainQueue` call or force `WatchQueue` to skip pending prompts, then run:

`go test ./internal/acpclient -run WatchQueueDrainsPendingPrompts -count=1`

Expected: FAIL because queued prompts remain pending or the fake runner is not called. Restore the implementation and rerun the same command; expected PASS.

**Step 4: Commit implementation**

```bash
gofmt -w internal/acpclient/watch.go internal/acpclient/watch_test.go internal/acpclient/store.go
git add internal/acpclient/watch.go internal/acpclient/watch_test.go internal/acpclient/store.go
git commit -m "feat: add acp client watch loop"
```

**Rollback:** revert commit; no state migration is involved.

### Task 3: Add CLI and Binary Watch Tests

**Files:**
- Modify: `cmd/ratchet/cmd_acp_client_test.go`
- Modify: `cmd/ratchet/acp_client_binary_test.go`

**Step 1: Write failing tests**

Add parser/executor tests for:
- `parseACPClient` accepts `watch <session-id> --command ./agent --interval 100ms --max-per-cycle 2 --max-cycles 1 --stop-when-empty --json`.
- invalid `--interval <= 0`, `--max-per-cycle <= 0`, and `--max-cycles <= 0` are rejected.
- `executeACPClientWatch` resolves explicit agent command and produces aggregate human output without prompt bodies.
- JSON output is newline-delimited or per-cycle JSON and includes aggregate fields only.

Add a binary smoke test that:
1. builds the ratchet test binary and fixture ACP agent through existing helpers;
2. queues two prompts with `ratchet acp client exec --no-wait --session <id>`;
3. runs `ratchet acp client watch <id> --command <fixture> --stop-when-empty --max-per-cycle 2 --max-cycles 2`;
4. runs `ratchet acp client status <id>`;
5. asserts the built CLI reports completed queue counts and no prompt bodies in watch output.

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ratchet -run 'ACPClient.*Watch|ParseACPClient.*Watch' -count=1`

Expected: FAIL with unknown `watch` command or undefined executor.

**Step 3: Commit failing tests**

```bash
gofmt -w cmd/ratchet/cmd_acp_client_test.go cmd/ratchet/acp_client_binary_test.go
git add cmd/ratchet/cmd_acp_client_test.go cmd/ratchet/acp_client_binary_test.go
git commit -m "test: cover acp client watch command"
```

**Rollback:** revert this test-only commit.

### Task 4: Implement CLI Watch Command

**Files:**
- Modify: `cmd/ratchet/cmd_acp_client.go`
- Modify: `cmd/ratchet/cmd_acp_client_test.go` only if test fixtures need small compile adjustments
- Modify: `cmd/ratchet/acp_client_binary_test.go` only if test helpers need small compile adjustments

**Step 1: Implement CLI command**

Add:
- `acpClientCommandWatch` and `acpClientWatchOptions`.
- `parseACPClientWatch` matching the design flags.
- `executeACPClientWatch(ctx, store, id, opts, startRunner, writer)` that normalizes `cwd`, resolves the explicit agent spec, calls `acpclient.WatchQueue`, and prints aggregate cycle summaries.
- update `printACPClientUsage` to list `watch`.

Use `signal.NotifyContext` only in the top-level command path if needed; tests should call the executor with explicit contexts.

**Step 2: Run tests to verify they pass**

Run: `go test ./cmd/ratchet -run 'ACPClient.*Watch|ParseACPClient.*Watch|ACPClientExecBinarySmoke|HarnessEmulationDocs' -count=1`

Expected: PASS.

**Step 3: Regression invariant proof**

Temporarily remove the `watch` case from the parser, then run:

`go test ./cmd/ratchet -run 'ParseACPClient.*Watch' -count=1`

Expected: FAIL with unknown command. Restore the parser and rerun; expected PASS.

**Step 4: Commit CLI work**

```bash
gofmt -w cmd/ratchet/cmd_acp_client.go cmd/ratchet/cmd_acp_client_test.go cmd/ratchet/acp_client_binary_test.go
git add cmd/ratchet/cmd_acp_client.go cmd/ratchet/cmd_acp_client_test.go cmd/ratchet/acp_client_binary_test.go
git commit -m "feat: add acp client watch command"
```

**Rollback:** revert commit; existing `exec`, `queue`, `drain`, `status`, and `cancel` commands remain unchanged.

### Task 5: Update Public Docs and Policy Matrix

**Files:**
- Modify: `README.md`
- Modify: `docs/harness-emulation.md`
- Modify: `docs/policy-matrix.md`
- Modify: `docs/competitor-parity.md`
- Modify: `cmd/ratchet/harness_docs_test.go`

**Step 1: Write failing docs guard update**

Update `TestHarnessEmulationDocsCoverPolicyMatrixLayers` to require `Explicit watch/drain only` and `ratchet acp client watch`.

Run: `go test ./cmd/ratchet -run HarnessEmulationDocs -count=1`

Expected: FAIL until docs are updated.

**Step 2: Update docs**

Document:
- `ratchet acp client watch <session-id> --command ./agent --stop-when-empty`;
- watch is explicit foreground auto-drain, not a hidden daemon scheduler;
- queue contents remain sensitive local policy metadata;
- daemon/scheduled background drain and extension hooks remain follow-up work.

**Step 3: Run docs verification**

Run:

```bash
go test ./cmd/ratchet -run HarnessEmulationDocs -count=1
rg -n "Explicit watch/drain only|ratchet acp client watch|foreground|background drain|sensitive local policy metadata" README.md docs/harness-emulation.md docs/competitor-parity.md docs/policy-matrix.md
```

Expected: PASS and all required phrases found.

**Step 4: Commit docs**

```bash
git add README.md docs/harness-emulation.md docs/policy-matrix.md docs/competitor-parity.md cmd/ratchet/harness_docs_test.go
git commit -m "docs: document acp client watch policy"
```

**Rollback:** revert docs/test commit.

### Task 6: Full Verification, PR, and Closeout

**Files:**
- Modify only if final fixes are required by verification.

**Step 1: Scope and formatting checks**

Run:

```bash
bash <autodev-plugin>/tests/plan-scope-check.sh --verify-lock docs/plans/2026-07-02-ratchet-cli-auto-drain.md
git diff --check
```

Expected: PASS.

**Step 2: Go test, vet, lint, and Windows builds**

Run:

```bash
go test ./internal/acpclient ./cmd/ratchet -run 'WatchQueue|DrainQueue|ACPClient.*Watch|ParseACPClient.*Watch|ACPClientExecBinarySmoke|HarnessEmulationDocs' -count=1
go test ./... -count=1 -p=1
go vet ./...
golangci-lint run --new-from-rev=origin/master
GOOS=windows GOARCH=amd64 go build ./cmd/ratchet
GOOS=windows GOARCH=arm64 go build ./cmd/ratchet
```

Expected: all exit 0.

**Step 3: Runtime CLI proof**

Run a representative built-binary invocation through the smoke test path:

`go test ./cmd/ratchet -run 'ACPClient.*Watch.*Binary|ACPClientExecBinarySmoke' -count=1 -v`

Expected: PASS and output shows built CLI/fixture execution.

**Step 4: Create PR and monitor**

Push `feat/ratchet-cli-auto-drain-policy`, create a PR, add Copilot reviewer, and monitor checks/review comments until green. If GitHub checks are delayed, keep the local verification evidence from Steps 1-3 and continue monitoring rather than blocking.

**Step 5: Admin merge and close lock**

After PR checks are green, admin squash merge. Then run:

```bash
bash <autodev-plugin>/hooks/scope-lock-complete docs/plans/2026-07-02-ratchet-cli-auto-drain.md --evidence "<PR/check/local verification summary>"
```

Create a closeout PR only if scope-lock completion or a retro changes tracked files after the feature PR merges.

**Rollback:** revert the merge commit or the feature commits; no persistent schema/data migration exists.
