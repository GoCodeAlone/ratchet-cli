# ACPX Archives Compare Flow Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Add ACPX-compatible raw event-log archive import/export, persisted compare bundles, replay-grade JSON v1 flow bundles, docs, Windows proof, and a versioned release.

**Architecture:** Extend existing `internal/acpclient` archive/compare/flow code with additive event-log sidecars and bundle writers. Continue using `github.com/coder/acp-go-sdk` for live ACP; preserve/import raw JSON-RPC histories but do not tap or reimplement stdio transport framing.

**Tech Stack:** Go 1.26.4, `acp-go-sdk v0.6.3`, standard-library JSON/NDJSON/filesystem APIs, existing fixture ACP agent, existing `cmd/ratchet` parser and binary smoke harness.

**Base branch:** master

---

## Scope Manifest

**PR Count:** 4
**Tasks:** 10
**Estimated Lines of Change:** ~1800

**Out of scope:**
- ACPX TypeScript `.flow.ts` execution.
- Exact stdio byte capture around `acp-go-sdk`.
- Daemon background drain or unattended queue execution.
- Managed hooks or broad TypeScript extension SDK.
- Credentialed third-party agent CI.
- Pi JSONL branch-tree interoperability.
- Local-first gateway/channels.
- Full sandbox/path/network enforcement.

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|------|-------|-------|--------|
| 1 | `feat: add acpx raw archive event logs` | Task 1, Task 2, Task 3 | `feat/acpx-raw-archive-events` |
| 2 | `feat: persist acp compare bundles` | Task 4, Task 5 | `feat/acp-compare-bundles` |
| 3 | `feat: add acp flow replay bundles` | Task 6, Task 7, Task 8 | `feat/acp-flow-replay-bundles` |
| 4 | `docs: release acpx archive flow parity` | Task 9, Task 10 | `docs/acpx-archive-flow-release` |

**Status:** Draft

## Integration Matrix

| Integration | Classification | Proof |
|---|---|---|
| `acp-go-sdk` live client | runtime-integrated | PR1 client tests run fixture ACP agent through SDK and assert event envelopes from prompt/update/response. |
| ACPX archive v1 raw `history` | runtime-integrated | PR1 fixture imports `exported_by:"acpx"` archive with raw JSON-RPC messages and re-exports raw history. |
| Ratchet summary archive v1 | runtime-integrated | PR1 keeps existing archive tests green and adds summary/default regression. |
| Compare CLI | runtime-integrated | PR2 binary smoke runs two fixture agents with `--save`, then reads `compare.json` and event files. |
| Flow CLI/runtime | runtime-integrated | PR3 binary smoke runs action+ACP flow, then `flow replay --json` validates manifest/projections/trace without launching agents. |
| Docs/policy/parity | config-only | PR4 docs guard covers public docs; no runtime integration. |
| GoReleaser/Homebrew | runtime-integrated | PR4 release workflow and Homebrew tap version check after tag. |
| ACPX TypeScript runtime | deferred | Explicit non-goal; docs keep separate follow-up. |

### Task 1: Event Log And Raw Archive Tests

**Files:**
- Modify: `internal/acpclient/archive_test.go`
- Modify: `internal/acpclient/client_test.go`
- Create: `internal/acpclient/eventlog_test.go`

**Step 1: Write failing unit tests**

Add tests for:
- `ValidateJSONRPCMessage` accepts request, notification, response, and rejects missing `jsonrpc`, empty method, response with both `result` and `error`, invalid error shape.
- event sidecar writer appends `{seq,at,direction,message}` NDJSON with `0600` mode and safe escaped session filename.
- event sidecar reader rejects invalid JSON-RPC `message`.
- importing an ACPX-shaped archive fixture with `exported_by:"acpx"` and raw `history` preserves sidecar events.
- exporting `--history raw` from that imported record round-trips the raw JSON-RPC `history`.
- exporting `--history raw` without a sidecar returns a typed raw-history-unavailable error.
- existing summary export/import remains default.
- `Client.RunPrompt` against in-process/fixture agent returns events containing outbound prompt, inbound `session/update`, and inbound response envelopes.

**Step 2: Run red tests**

Run:

```bash
go test ./internal/acpclient -run 'EventLog|Archive|ClientRunPrompt.*Event' -count=1
```

Expected: FAIL with undefined event-log helpers/options.

**Step 3: Commit failing tests**

```bash
git add internal/acpclient/archive_test.go internal/acpclient/client_test.go internal/acpclient/eventlog_test.go
git commit -m "test: cover acpx raw archive events"
```

Rollback: revert test-only commit.

### Task 2: Event Log And Archive Implementation

**Files:**
- Create: `internal/acpclient/eventlog.go`
- Modify: `internal/acpclient/archive.go`
- Modify: `internal/acpclient/client.go`
- Modify: `internal/acpclient/callbacks.go`
- Modify: `internal/acpclient/spec.go`
- Modify: `internal/acpclient/store.go`
- Test: `internal/acpclient/*_test.go`

**Step 1: Implement event primitives**

Add:
- `type JSONRPCMessage json.RawMessage`
- `type EventDirection string` with `outbound`, `inbound`
- `type EventLogLine struct { Seq int; At time.Time; Direction EventDirection; Message json.RawMessage }`
- JSON-RPC validator matching ACPX `isAcpJsonRpcMessage`.
- sidecar helpers on `Store`: event root, safe event path, append/read/copy metadata.
- `ErrRawHistoryUnavailable`.

**Step 2: Implement client event capture**

Add:
- `Result.Events []EventLogLine`.
- `Callbacks` event buffer and `LastEvents`/`Snapshot` support.
- prompt event builder around `SessionRunner.Prompt`: outbound prompt request, inbound normalized `session/update`, inbound prompt response/error.
- synthetic local request IDs scoped to sidecar logs; docs/tests must not call them wire IDs.

**Step 3: Implement archive raw/summary modes**

Add:
- `ArchiveHistoryModeSummary`, `ArchiveHistoryModeRaw`, `ArchiveHistoryModeBoth`.
- custom import that detects raw JSON-RPC `history` vs ratchet summary events.
- `summary_history` additive field for `both`.
- `ExportOptions.HistoryMode`, `Store`/sidecar dependency.

**Step 4: Run green tests**

Run:

```bash
go test ./internal/acpclient -run 'EventLog|Archive|ClientRunPrompt.*Event|ClientRunPrompt' -count=1
```

Expected: PASS.

**Step 5: Regression invariant proof**

Temporarily allow raw export without sidecar, run:

```bash
go test ./internal/acpclient -run 'Export.*Raw|Archive' -count=1
```

Expected: FAIL. Restore fail-closed behavior and rerun; expected PASS.

**Step 6: Commit implementation**

```bash
gofmt -w internal/acpclient/eventlog.go internal/acpclient/archive.go internal/acpclient/client.go internal/acpclient/callbacks.go internal/acpclient/spec.go internal/acpclient/store.go internal/acpclient/*_test.go
git add internal/acpclient
git commit -m "feat: add acpx raw archive event logs"
```

Rollback: revert commit; local sidecar files become inert and summary archives remain supported.

### Task 3: Archive CLI And Binary Smoke

**Files:**
- Modify: `cmd/ratchet/cmd_acp_client.go`
- Modify: `cmd/ratchet/cmd_acp_client_test.go`
- Modify: `cmd/ratchet/acp_client_binary_test.go`
- Modify: `README.md` only for command examples if needed by smoke docs

**Step 1: Write failing CLI tests**

Add parser/executor tests for:
- `sessions export <id> --output a.json --history raw|summary|both`.
- invalid `--history` is rejected.
- `sessions events <id> [--json] [--output events.ndjson]`.
- raw export without sidecar surfaces raw-history-unavailable.
- import ACPX fixture through CLI writes sidecar event metadata.
- live `exec` with a store writes `Result.Events` to the session sidecar before `--history raw` export.

**Step 2: Extend binary smoke**

Add a credential-free built-binary path:
1. run fixture ACP prompt;
2. export summary archive default;
3. export raw archive with `--history raw`;
4. inspect `sessions events <id> --json`;
5. import an ACPX-shaped raw fixture and re-export raw;
6. assert raw `history` entries are JSON-RPC messages.

Expected: no prompt bodies printed in human event summary.

**Step 3: Run red tests**

Run:

```bash
go test ./cmd/ratchet -run 'ACPClient.*Archive|ACPClient.*Events|ParseACPClient.*Sessions' -count=1 -timeout=8m
```

Expected: FAIL with missing flags/subcommand.

**Step 4: Implement CLI**

Add:
- `HistoryMode string` in archive options.
- parser for `--history`.
- `sessions events` subcommand.
- JSON output with `session_id`, `path`, `events`, `status`.
- command-level event persistence helper used by `executeACPClientExecWithStore` and queue drain/watch completion paths when a `Result` contains events.
- no raw event payloads in human summaries.

**Step 5: Verify**

Run:

```bash
go test ./cmd/ratchet -run 'ACPClient.*Archive|ACPClient.*Events|ParseACPClient.*Sessions' -count=1 -timeout=8m
go test ./internal/acpclient ./cmd/ratchet -run 'EventLog|Archive|ACPClient.*Archive|ACPClient.*Events' -count=1 -timeout=10m
```

Expected: PASS.

**Step 6: Commit**

```bash
gofmt -w cmd/ratchet/cmd_acp_client.go cmd/ratchet/cmd_acp_client_test.go cmd/ratchet/acp_client_binary_test.go
git add cmd/ratchet README.md
git commit -m "feat: expose acp archive event logs"
```

Rollback: revert PR1 branch; summary archive CLI remains available from previous release.

### Task 4: Compare Bundle Tests

**Files:**
- Modify: `internal/acpclient/compare_test.go`
- Create: `internal/acpclient/compare_store_test.go`
- Modify: `cmd/ratchet/cmd_acp_client_test.go`
- Modify: `cmd/ratchet/acp_client_binary_test.go`

**Step 1: Write failing tests**

Add tests for:
- saved compare JSON uses wrapper shape `{run_id, run_dir, status, rows}`; unsaved compare JSON remains row array for backwards compatibility.
- `CompareRunStore` writes `compare.json` and per-agent `events.ndjson` with safe agent path segments.
- compare save copies `Result.Events` from fake runner rows.
- parser accepts `compare --save --run-id fixed --run-root <dir>`.
- binary smoke runs two fixture agents with `--save` and verifies bundle files.

**Step 2: Run red tests**

Run:

```bash
go test ./internal/acpclient ./cmd/ratchet -run 'Compare' -count=1 -timeout=8m
```

Expected: FAIL with undefined compare store/save flags.

**Step 3: Commit failing tests**

```bash
gofmt -w internal/acpclient/compare_test.go internal/acpclient/compare_store_test.go cmd/ratchet/cmd_acp_client_test.go cmd/ratchet/acp_client_binary_test.go
git add internal/acpclient/compare_test.go internal/acpclient/compare_store_test.go cmd/ratchet/cmd_acp_client_test.go cmd/ratchet/acp_client_binary_test.go
git commit -m "test: cover acp compare bundles"
```

Rollback: revert test-only commit.

### Task 5: Compare Bundle Implementation

**Files:**
- Create: `internal/acpclient/compare_store.go`
- Modify: `internal/acpclient/compare.go`
- Modify: `cmd/ratchet/cmd_acp_client.go`
- Test: files from Task 4

**Step 1: Implement compare store/runtime**

Add:
- `CompareRunResult {RunID, RunDir, Status, Rows}` wrapper used only when `Save` is true.
- `CompareOptions.RunID`, `RunRoot`, `Save`.
- `CompareRunStore` with `compare.json`, `agents/<safe-agent>/events.ndjson`.
- stable status: `completed` when all rows non-error, `completed_with_errors` when any row error.

**Step 2: Implement CLI flags**

Add:
- `--save`
- `--run-id`
- `--run-root`
- JSON output is the wrapper when saved and the existing row array when not saved.
- human output appends `run dir: <path>` after table when saved.

**Step 3: Verify**

Run:

```bash
go test ./internal/acpclient ./cmd/ratchet -run 'Compare|ACPClientExecBinarySmoke' -count=1 -timeout=10m
```

Expected: PASS.

**Step 4: Commit**

```bash
gofmt -w internal/acpclient/compare.go internal/acpclient/compare_store.go internal/acpclient/compare*_test.go cmd/ratchet/cmd_acp_client.go cmd/ratchet/*acp_client*_test.go
git add internal/acpclient cmd/ratchet
git commit -m "feat: persist acp compare bundles"
```

Rollback: revert PR2; compare table/JSON rows from prior release remain supported.

### Task 6: Flow Replay Bundle Tests

**Files:**
- Modify: `internal/acpclient/flow_test.go`
- Create: `internal/acpclient/flow_replay_test.go`
- Modify: `cmd/ratchet/cmd_acp_client_test.go`
- Modify: `cmd/ratchet/acp_client_binary_test.go`

**Step 1: Write failing tests**

Add tests for:
- `FlowRunStore` writes `manifest.json`, `trace.ndjson`, `projections/run.json`, `projections/live.json`, `projections/steps.json`.
- action node writes stdout/stderr/output artifacts by sha256 path.
- ACP node writes prompt artifact and session event bundle when runner satisfies `interface{ LastEvents() []EventLogLine }`.
- trace seq starts at 1 and increments.
- replay loader rejects manifest paths outside run dir.
- CLI parses `flow replay <run-dir> [--json]`.
- binary smoke runs action+ACP flow then `flow replay --json`, asserting status and counts.

**Step 2: Run red tests**

Run:

```bash
go test ./internal/acpclient ./cmd/ratchet -run 'Flow.*Replay|FlowRunStore|ACPClient.*Flow' -count=1 -timeout=8m
```

Expected: FAIL with missing manifest/replay APIs.

**Step 3: Commit failing tests**

```bash
gofmt -w internal/acpclient/flow_test.go internal/acpclient/flow_replay_test.go cmd/ratchet/cmd_acp_client_test.go cmd/ratchet/acp_client_binary_test.go
git add internal/acpclient/flow*_test.go cmd/ratchet/cmd_acp_client_test.go cmd/ratchet/acp_client_binary_test.go
git commit -m "test: cover acp flow replay bundles"
```

Rollback: revert test-only commit.

### Task 7: Flow Replay Bundle Implementation

**Files:**
- Modify: `internal/acpclient/flow.go`
- Modify: `internal/acpclient/flow_runner.go`
- Modify: `internal/acpclient/flow_store.go`
- Create: `internal/acpclient/flow_replay.go`
- Modify: `cmd/ratchet/cmd_acp_client.go`
- Test: files from Task 6

**Step 1: Implement bundle writer**

Add:
- manifest schema `acpx.flow-run-bundle.v1`.
- trace event struct and append-only writer.
- projection writers.
- content-addressed artifact writer.
- optional flow-session event bundle through a helper that type-asserts `interface{ LastEvents() []EventLogLine }`; do not add `LastEvents` to `FlowPromptRunner`.
- `FlowReplaySummary` loader with path containment validation.

**Step 2: Implement CLI replay**

Add:
- `ratchet acp client flow replay <run-dir> [--json]`.
- human summary: `flow <run-id> <status>`, `steps: <n>`, `trace events: <n>`, `sessions: <n>`.
- JSON summary with same counts and manifest path.

**Step 3: Verify**

Run:

```bash
go test ./internal/acpclient ./cmd/ratchet -run 'Flow.*Replay|FlowRunStore|ACPClient.*Flow' -count=1 -timeout=10m
```

Expected: PASS.

**Step 4: Commit**

```bash
gofmt -w internal/acpclient/flow*.go cmd/ratchet/cmd_acp_client.go internal/acpclient/flow*_test.go cmd/ratchet/*acp_client*_test.go
git add internal/acpclient cmd/ratchet
git commit -m "feat: add acp flow replay bundles"
```

Rollback: revert PR3; existing `state.json`/`steps/*.json` files are still written by previous release.

### Task 8: Cross-Surface Verification Hardening

**Files:**
- Modify: `cmd/ratchet/acp_client_binary_test.go` only if smoke consolidation is needed
- Modify: `internal/acpclient/*_test.go` only for discovered edge regressions

**Step 1: Add cross-surface regression guard**

Add or consolidate non-doc regression assertions that prove archive, compare,
and flow replay can run in the same binary smoke environment without leaking raw
payloads in human summaries. Do not update `HarnessEmulationDocs` in this task;
Task 9 owns docs and docs-guard expectations.

**Step 2: Run focused suite**

Run:

```bash
go test ./internal/acpclient ./cmd/ratchet -run 'EventLog|Archive|Compare|Flow|HarnessEmulationDocs|ACPClientExecBinarySmoke' -count=1 -timeout=12m
```

Expected: PASS. `HarnessEmulationDocs` remains at the previous docs contract until Task 9.

**Step 3: Full local gate**

Run:

```bash
go test ./... -count=1 -p=1 -timeout=20m
go vet ./...
golangci-lint run --new-from-rev=origin/master
GOOS=windows GOARCH=amd64 go build ./cmd/ratchet
rm -f ratchet.exe
GOOS=windows GOARCH=arm64 go build ./cmd/ratchet
rm -f ratchet.exe
git diff --check
```

Expected: all PASS; no `ratchet.exe` remains.

**Step 4: Commit**

```bash
gofmt -w cmd/ratchet/acp_client_binary_test.go internal/acpclient/*_test.go
git add cmd/ratchet internal/acpclient
git commit -m "test: harden acp replay parity coverage"
```

Rollback: revert PR3 guard commit with implementation commit if guard reveals incompatible behavior.

### Task 9: Public Docs And Parity State

**Files:**
- Modify: `README.md`
- Modify: `docs/harness-emulation.md`
- Modify: `docs/competitor-parity.md`
- Modify: `docs/policy-matrix.md`
- Modify: `docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow-design.md`
- Test: `cmd/ratchet/harness_docs_test.go`

**Step 1: Update docs**

Document:
- `sessions export --history raw|summary|both`
- `sessions events`
- imported ACPX raw archive round-trip
- compare `--save` bundles
- flow replay bundles and `flow replay`
- sensitive artifact warning
- TypeScript `.flow.ts` runtime still deferred
- credentialed third-party CI still deferred
- source snapshot still `ACPX@1d882575e34e18621e59229f0e711723cef223ae`, `ACP@a90d7e3a7a77bad4d9af35bbb08962daa0167453`

Extend `cmd/ratchet/harness_docs_test.go` expectations for:
- `ratchet acp client sessions events`
- `--history raw`
- `compare --save`
- `flow replay`
- `raw ACPX event logs`
- `ACPX TypeScript flow runtime compatibility remains deferred`

**Step 2: Run docs guard**

Run:

```bash
go test ./cmd/ratchet -run HarnessEmulationDocs -count=1
```

Expected: PASS.

**Step 3: Hygiene**

Run:

```bash
rg -n "raw ACPX JSON-RPC event-log archive compatibility remains deferred|raw JSON-RPC event-log archive compatibility remains intentionally deferred" README.md docs || true
pattern="$(printf '/%s/|/%s/|/var/%s' Users home folders)"
rg -n "$pattern" docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow*.md README.md docs/harness-emulation.md docs/competitor-parity.md docs/policy-matrix.md || true
git diff --check
```

Expected: no stale raw-event deferral matches, no machine paths, diff check clean.

**Step 4: Commit**

```bash
git add README.md docs/harness-emulation.md docs/competitor-parity.md docs/policy-matrix.md docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow-design.md cmd/ratchet/harness_docs_test.go
git commit -m "docs: document acpx replay parity"
```

Rollback: revert docs PR; code features remain available but parity docs return to previous release state.

### Task 10: Release, Retro, And Workspace State

**Files:**
- Modify: `docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow.md`
- Delete after completion: `docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow.md.scope-lock`
- Create: `docs/retros/2026-07-02-ratchet-cli-acpx-archives-compare-flow-retro.md`
- Modify workspace repo after ratchet merge/release: `docs/PROJECTS.md`, `docs/FOLLOWUPS.md`, `.autodev/state/phase-progress.jsonl`

**Step 1: Release gate on merged main**

After PRs 1-3 and PR4 docs merge to `master`, create a release worktree at `origin/master` and run:

```bash
go test ./... -count=1 -p=1 -timeout=20m
go vet ./...
golangci-lint run --new-from-rev=origin/master
GOOS=windows GOARCH=amd64 go build ./cmd/ratchet
rm -f ratchet.exe
GOOS=windows GOARCH=arm64 go build ./cmd/ratchet
rm -f ratchet.exe
goreleaser check
git diff --check
```

Expected: all PASS.

**Step 2: Tag release**

If no `v0.25.0` tag exists:

```bash
git tag -a v0.25.0 -m "release v0.25.0"
git push origin v0.25.0
```

Expected: release workflow succeeds; assets include Windows amd64/arm64 zips; Homebrew tap cask version is `0.25.0`.

**Step 3: Close lock and retro**

Run from the ratchet closeout worktree with the absolute helper path:

```bash
bash "<autodev-plugin-dir>/hooks/scope-lock-complete" docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow.md --evidence "<PRs merged, local gates, release workflow, assets, Homebrew>"
```

Create retro covering:
- design review D1-D3 effectiveness;
- CI/review issues;
- whether raw/summary archive distinction was clear enough;
- Windows gate results;
- deferred TypeScript runtime/credentialed CI follow-ups.

**Step 4: Verify closeout docs**

Run:

```bash
go test ./cmd/ratchet -run HarnessEmulationDocs -count=1
pattern="$(printf '/%s/|/%s/|/var/%s' Users home folders)"
rg -n "$pattern" docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow*.md docs/retros/2026-07-02-ratchet-cli-acpx-archives-compare-flow-retro.md || true
git diff --check
```

Expected: PASS/no machine paths/clean diff check.

**Step 5: Commit closeout PR**

```bash
git add docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow.md docs/retros/2026-07-02-ratchet-cli-acpx-archives-compare-flow-retro.md
git add -u docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow.md.scope-lock
git commit -m "docs: close acpx replay parity plan"
```

Open/admin-merge closeout PR after checks/review pass.

**Step 6: Workspace state**

In `<workspace-root>`, update committed state:
- `.autodev/state/phase-progress.jsonl` rows for each PR/release phase.
- `docs/PROJECTS.md` ratchet-cli next/state.
- `docs/FOLLOWUPS.md` remaining follow-ups: ACPX TypeScript runtime, credentialed third-party agent CI, daemon background drain, managed hooks/SDK, channel gateway.

Verify:

```bash
jq -c . .autodev/state/phase-progress.jsonl >/dev/null
git diff --check
```

Commit, push, open/admin-merge workspace PR, then fast-forward workspace `main`.

Rollback: if release fails before public assets publish, delete local tag and retry after fix; if public release publishes with a defect, ship `v0.25.1` rather than rewriting public history.
