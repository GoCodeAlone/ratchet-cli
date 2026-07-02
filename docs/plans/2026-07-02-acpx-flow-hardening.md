# ACPX Flow Hardening Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Add JSON v1 ACP client flow action nodes, per-node cwd, and explicit permission preflight.

**Architecture:** Extend the existing Go JSON flow runtime in `internal/acpclient`; do not embed ACPX's TypeScript runtime. Flow execution remains sequential DAG traversal with persisted run bundles, now supporting runtime-owned `action` steps and implicit permission checks before any node runs.

**Tech Stack:** Go 1.26.4, standard library `os/exec`, existing `internal/acpclient` flow runner/store, existing `cmd/ratchet` parser and binary smoke fixtures.

**Base branch:** master

---

## Scope Manifest

**PR Count:** 1
**Tasks:** 6
**Estimated Lines of Change:** ~650

**Out of scope:**
- ACPX TypeScript `.flow.ts` runtime compatibility.
- Raw ACPX JSON-RPC event-log archive compatibility.
- Flow branching/decision DSL and replay viewer.
- Daemon background drain.
- Broad extension hooks/profile distributions/self-evolution.
- Full sandbox/path/network enforcement.
- Release tag/Homebrew publish.

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|------|-------|-------|--------|
| 1 | `feat: harden acp client flows` | Task 1, Task 2, Task 3, Task 4, Task 5, Task 6 | `feat/acpx-flow-hardening` |

**Status:** Draft

## Integration Matrix

| Integration | Classification | Proof |
|---|---|---|
| Existing ACP client flow runtime | runtime-integrated | Task 2 unit tests exercise `RunFlow` with compute, action, and ACP nodes through injected runners. |
| CLI flow command | runtime-integrated | Task 4 parser/executor tests and binary smoke run the built `ratchet acp client flow run` command. |
| Local process action runner | runtime-integrated | Task 2 tests inject a fake runner; Task 4 binary smoke executes a real local command through the built CLI. |
| ACP child-process fixture agent | runtime-integrated | Task 4 binary smoke runs action + ACP nodes through the fixture ACP agent. |
| ACPX TypeScript runtime | deferred | Explicitly out of scope; docs retain deferred compatibility language. |
| Hermes profile/self-evolution surfaces | deferred | External source signal only; no runtime integration in this slice. |

### Task 1: Add Internal Flow Hardening Tests

**Files:**
- Modify: `internal/acpclient/flow_test.go`

**Step 1: Write failing tests**

Add tests for:
- `FlowDefinition.Validate` accepts `action` nodes with `command` and optional `args`, `cwd`, `env`, `input`.
- validation rejects action nodes without `command`.
- `RunFlow` fails before executing any node when an action node exists and `FlowRunOptions.AllowedPermissions` lacks `shell`.
- `RunFlow` fails before executing any node when `node.cwd` resolves outside base cwd and `outside-cwd` is not allowed.
- `RunFlow` succeeds with a fake action runner when `shell` is allowed; output includes `exit_code`, `stdout`, `stderr`, `duration_ms`, `cwd`.
- action non-zero exit marks state failed and persists the failed step.
- action output is truncated and records truncation flags.

Use a fake `ActionRunner` so this task stays internal-logic only.

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/acpclient -run 'Flow|Action' -count=1
```

Expected: FAIL with undefined `FlowNodeTypeAction`, `AllowedPermissions`, or `ActionRunner`.

**Step 3: Commit failing tests**

```bash
git add internal/acpclient/flow_test.go
git commit -m "test: cover acp flow action nodes"
```

Rollback: revert this test-only commit.

### Task 2: Implement Flow Action Runtime

**Files:**
- Modify: `internal/acpclient/flow.go`
- Modify: `internal/acpclient/flow_runner.go`
- Modify: `internal/acpclient/flow_store.go` only if output metadata storage helpers need adjustment
- Test: `internal/acpclient/flow_test.go`

**Step 1: Implement minimal runtime**

Add:
- `FlowNodeTypeAction = "action"`.
- `FlowDefinition.Requires []string`.
- `FlowNode.Cwd string`, `FlowNode.Env map[string]string`, `FlowNode.Input json.RawMessage`.
- `FlowRunOptions.AllowedPermissions []string`, `ActionRunner ActionRunner`, `ActionOutputLimit int`.
- `ActionRunner` interface and default `exec.CommandContext` runner.
- preflight helper that unions `def.Requires`, implicit `shell` for action nodes, and implicit `outside-cwd` for cwd escape.
- cwd resolver that keeps relative node cwd under flow base cwd unless `outside-cwd` is allowed.
- action output JSON with `exit_code`, `stdout`, `stderr`, `stdout_truncated`, `stderr_truncated`, `duration_ms`, `cwd`.
- failure handling that persists failed state before returning.

Keep `node.input` static JSON stdin only; do not add template/select expansion.

**Step 2: Run tests**

Run:

```bash
go test ./internal/acpclient -run 'Flow|Action' -count=1
```

Expected: PASS.

**Step 3: Regression invariant proof**

Temporarily remove implicit `shell` from preflight, then run:

```bash
go test ./internal/acpclient -run 'RunFlow.*Permission|Action' -count=1
```

Expected: FAIL because action runner executes without `shell` permission or missing permission is not reported. Restore code and rerun; expected PASS.

**Step 4: Commit implementation**

```bash
gofmt -w internal/acpclient/flow.go internal/acpclient/flow_runner.go internal/acpclient/flow_store.go internal/acpclient/flow_test.go
git add internal/acpclient/flow.go internal/acpclient/flow_runner.go internal/acpclient/flow_store.go internal/acpclient/flow_test.go
git commit -m "feat: add acp flow action runtime"
```

Rollback: revert commit; existing JSON flow bundles remain readable because new fields are optional.

### Task 3: Add CLI Flow Permission Tests

**Files:**
- Modify: `cmd/ratchet/cmd_acp_client_test.go`
- Modify: `cmd/ratchet/acp_client_binary_test.go`

**Step 1: Write failing parser/executor tests**

Add tests for:
- `parseACPClient` accepts `flow run flow.json --allow shell --allow outside-cwd --json`.
- parser rejects empty `--allow`.
- `executeACPClientFlowRun` passes allowed permissions into `RunFlow` and returns the missing permission error when not allowed.
- JSON output includes action outputs and does not print action stdout/stderr in human summary.

Use temp flow JSON files and an injectable action runner only if needed for executor tests; keep fixture ACP agent use in binary smoke.

**Step 2: Write failing binary smoke expectation**

Extend `TestACPClientExecBinarySmoke` flow section or add a focused smoke that:
1. writes a JSON flow with an `action` node that runs a built test binary with simple args and an `acp` node that consumes action output;
2. runs built `ratchet acp client flow run ... --allow shell --command <fixture> --json`;
3. asserts output status completed, action output exists, ACP output uses action output, and bundle files exist.

Use `ratchetBin` or the existing fixture ACP binary as the action command so the smoke path is portable; do not rely on `sh -c`, `cmd /C`, `echo`, or other platform shell syntax.

**Step 3: Run tests to verify they fail**

Run:

```bash
go test ./cmd/ratchet -run 'ACPClient.*Flow|ParseACPClientFlow' -count=1 -timeout=6m
```

Expected: FAIL with unknown `--allow` or missing action support.

**Step 4: Commit failing tests**

```bash
gofmt -w cmd/ratchet/cmd_acp_client_test.go cmd/ratchet/acp_client_binary_test.go
git add cmd/ratchet/cmd_acp_client_test.go cmd/ratchet/acp_client_binary_test.go
git commit -m "test: cover acp flow action CLI"
```

Rollback: revert this test-only commit.

### Task 4: Implement CLI Flow Flags and Smoke Path

**Files:**
- Modify: `cmd/ratchet/cmd_acp_client.go`
- Modify: `cmd/ratchet/cmd_acp_client_test.go` only for fixture compile fixes
- Modify: `cmd/ratchet/acp_client_binary_test.go` only for fixture compile fixes

**Step 1: Implement CLI plumbing**

Add:
- repeated `--allow <permission>` to `parseACPClientFlowRun`;
- `AllowedPermissions []string` in `acpClientFlowOptions`;
- validation that trims permissions and rejects empty values;
- pass permissions to `acpclient.RunFlow`.

Human output remains summary-only:

```text
flow <run-id> <status>
run dir: <path>
```

Do not print action stdout/stderr unless `--json` is requested.

**Step 2: Run command tests**

Run:

```bash
go test ./cmd/ratchet -run 'ACPClient.*Flow|ParseACPClientFlow' -count=1 -timeout=6m
```

Expected: PASS.

**Step 3: Runtime CLI proof**

Run:

```bash
go test ./cmd/ratchet -run 'ACPClientExecBinarySmoke|ACPClient.*Flow' -count=1 -v -timeout=8m
```

Expected: PASS and verbose output shows built CLI/fixture execution.

**Step 4: Commit implementation**

```bash
gofmt -w cmd/ratchet/cmd_acp_client.go cmd/ratchet/cmd_acp_client_test.go cmd/ratchet/acp_client_binary_test.go
git add cmd/ratchet/cmd_acp_client.go cmd/ratchet/cmd_acp_client_test.go cmd/ratchet/acp_client_binary_test.go
git commit -m "feat: wire acp flow action CLI"
```

Rollback: revert commit; existing `flow run` flags remain available from previous commits after revert.

### Task 5: Update Public Docs and Policy Matrix

**Files:**
- Modify: `README.md`
- Modify: `docs/harness-emulation.md`
- Modify: `docs/competitor-parity.md`
- Modify: `docs/policy-matrix.md`
- Modify: `cmd/ratchet/harness_docs_test.go`

**Step 1: Write failing docs guard update**

Update docs guard to require:
- `action nodes`
- `--allow shell`
- `ACPX TypeScript flow runtime compatibility remains deferred`
- `sensitive local command output`

Run:

```bash
go test ./cmd/ratchet -run HarnessEmulationDocs -count=1
```

Expected: FAIL until docs are updated.

**Step 2: Update docs**

Document:
- JSON v1 flows support `acp`, `compute`, and `action`;
- action nodes require `--allow shell`;
- outside cwd requires `--allow outside-cwd`;
- action stdout/stderr in run bundles is sensitive local metadata;
- ACPX `.flow.ts` runtime and replay viewer remain deferred.

**Step 3: Run docs verification**

Run:

```bash
go test ./cmd/ratchet -run HarnessEmulationDocs -count=1
rg -n "action nodes|--allow shell|outside-cwd|ACPX TypeScript flow runtime compatibility remains deferred|sensitive local command output" README.md docs/harness-emulation.md docs/competitor-parity.md docs/policy-matrix.md
```

Expected: PASS and required phrases found.

**Step 4: Commit docs**

```bash
git add README.md docs/harness-emulation.md docs/competitor-parity.md docs/policy-matrix.md cmd/ratchet/harness_docs_test.go
git commit -m "docs: document acp flow actions"
```

Rollback: revert docs/test commit.

### Task 6: Full Verification, PR, and Monitoring

**Files:**
- Modify only if verification finds required fixes.

**Step 1: Scope and formatting checks**

Run:

```bash
bash <autodev-plugin>/tests/plan-scope-check.sh --verify-lock docs/plans/2026-07-02-acpx-flow-hardening.md
git diff --check
rg -n "/Users/|/home/|/var/folders" docs/plans/2026-07-02-acpx-flow-hardening*.md README.md docs/harness-emulation.md docs/competitor-parity.md docs/policy-matrix.md || true
```

Expected: scope-lock PASS after lock, no whitespace errors, no machine-local paths.

**Step 2: Go tests, vet, lint, Windows builds**

Run:

```bash
go test ./internal/acpclient ./cmd/ratchet -run 'Flow|Action|ACPClient.*Flow|ParseACPClientFlow|ACPClientExecBinarySmoke|HarnessEmulationDocs' -count=1 -timeout=10m
go test ./... -count=1 -p=1 -timeout=20m
go vet ./...
golangci-lint run --new-from-rev=origin/master
GOOS=windows GOARCH=amd64 go build ./cmd/ratchet
GOOS=windows GOARCH=arm64 go build ./cmd/ratchet
```

Expected: all exit 0.

**Step 3: Runtime launch validation**

Run the verbose binary smoke:

```bash
go test ./cmd/ratchet -run 'ACPClientExecBinarySmoke|ACPClient.*Flow' -count=1 -v -timeout=10m
```

Expected: PASS; built CLI runs JSON flow with action + ACP fixture path.

**Step 4: PR and monitor**

Push branch, create PR, add Copilot reviewer, monitor CI/reviews until green. If checks are delayed, keep local verification evidence and continue monitoring rather than blocking.

**Step 5: Merge and closeout**

Admin squash merge after checks green and no unresolved review threads. Then run:

```bash
bash <autodev-plugin>/hooks/scope-lock-complete docs/plans/2026-07-02-acpx-flow-hardening.md --evidence "<PR/check/local verification summary>"
```

Create a closeout PR only if scope-lock completion or retro changes tracked files after the feature PR merges.

Rollback: revert merged PR; no schema migration or external resource change exists.
