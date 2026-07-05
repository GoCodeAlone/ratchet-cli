# ratchet-cli Workflow Messaging Export And Profile CI Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Add Workflow messaging export envelopes for daemon blackboard handoffs and a credential-free ACP launch profile verification command.

**Architecture:** Extend existing ratchet-cli command surfaces rather than adding provider adapters. Blackboard export adds a local Workflow projection over current records; profile verification reuses trusted ACP launch profiles and the existing ACP client process runner.

**Tech Stack:** Go 1.26.4, existing `internal/acpclient`, existing daemon blackboard client, `workflow-plugin-messaging-core` contract naming.

**Base branch:** master

---

## Scope Manifest

**PR Count:** 2
**Tasks:** 6
**Estimated Lines of Change:** ~420

**Out of scope:**
- Direct Slack, Discord, Teams, webhook, email, or provider delivery.
- Secret storage, provider credentials, or env value printing.
- Managed hooks, TypeScript extension SDK, daemon background scheduling, local-first gateway/channel routing, or ACPX `.flow.ts` execution.

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|------|-------|-------|--------|
| 1 | Add Workflow messaging blackboard export envelopes | Task 1, Task 2, Task 3 | feat/ratchet-blackboard-workflow-export |
| 2 | Add ACP profile verification command and fixture CI proof | Task 4, Task 5, Task 6 | feat/ratchet-profile-verify-ci |

**Status:** Complete 2026-07-05T04:42:58Z

## Declared Integration Matrix

| Item | Status | Proof |
|---|---|---|
| `workflow-plugin-messaging-core` | config-only | Ratchet emits `step.messaging_send` metadata and required config names only; downstream Workflow execution remains external. |
| ACP fixture agent | runtime-integrated | Built CLI verifies a trusted local fixture ACP profile. |
| Third-party ACP agents | deferred | Future credentialed CI can reuse `profiles verify`; no provider secrets in this plan. |

### Task 1: Add Workflow Messaging Export Tests

**Files:**
- Modify: `cmd/ratchet/cmd_blackboard_test.go`

**Step 1: Write failing parser/output tests**

Add tests for:
- `ratchet blackboard export coordination --workflow-messaging --json`
- `ratchet blackboard export coordination --workflow-messaging --jsonl`
- continued rejection of `--channel`, `--token`, `--webhook-url`, and `--provider`

Expected JSON record fields:
- `workflow.stepType == "step.messaging_send"`
- `workflow.pluginFamily == "workflow-plugin-messaging-core"`
- `workflow.input.text == "[coordination/status] ready"`
- `workflow.requiredConfig == ["channel"]`

**Step 2: Verify RED**

Run: `go test ./cmd/ratchet -run 'TestHandleBlackboardExportWorkflowMessaging|TestHandleBlackboardExportRejectsCredentialFlags' -count=1`

Expected: FAIL because `--workflow-messaging` is unknown or workflow fields are missing.

**Step 3: Commit tests**

Commit: `test: require workflow messaging blackboard export`

Rollback: revert this commit; no runtime behavior changes.

### Task 2: Implement Workflow Messaging Export

**Files:**
- Modify: `cmd/ratchet/cmd_blackboard.go`

**Step 1: Add the minimal production code**

Extend `blackboardExportOptions` with `workflowMessaging bool`, parse
`--workflow-messaging`, add a `Workflow` projection to `blackboardExportRecord`,
and populate it from the existing `Messaging.Text`.

**Step 2: Verify GREEN**

Run: `go test ./cmd/ratchet -run 'TestHandleBlackboardExportWorkflowMessaging|TestHandleBlackboardExportRejectsCredentialFlags' -count=1`

Expected: PASS.

**Step 3: Verify invariant**

Temporarily remove the `Workflow` population, rerun the Task 1 tests, and confirm
they fail on missing workflow metadata. Restore the fix and rerun to PASS.

**Step 4: Commit implementation**

Commit: `feat: add workflow messaging blackboard export`

Rollback: revert this commit; `ratchet blackboard export` returns to prior JSON shape.

### Task 3: Document And Verify PR1

**Files:**
- Modify: `README.md`
- Modify: `docs/harness-emulation.md`
- Modify: `docs/policy-matrix.md`
- Modify: `docs/competitor-parity.md`
- Modify: `cmd/ratchet/harness_docs_test.go`

**Step 1: Update docs tests**

Require public docs to mention `--workflow-messaging`, `step.messaging_send`,
and `workflow-plugin-messaging-core`.

**Step 2: Verify docs RED/GREEN**

Run the docs test before and after documentation edits:
`go test ./cmd/ratchet -run TestHarnessEmulationDocsCoverSupportedModesAndParity -count=1`

Expected: FAIL before docs edits, PASS after docs edits.

**Step 3: Run PR1 verification**

Run:
- `go test ./cmd/ratchet -run 'TestHandleBlackboardExport|TestHarnessEmulationDocsCoverSupportedModesAndParity' -count=1`
- `GOOS=windows GOARCH=amd64 go build ./cmd/ratchet`

Expected: tests PASS; Windows build exits 0.

**Step 4: Commit docs**

Commit: `docs: document workflow messaging blackboard export`

Rollback: revert this commit; feature remains available but public docs no longer claim it.

### Task 4: Add ACP Profile Verify Tests

**Files:**
- Modify: `cmd/ratchet/cmd_acp_client_test.go`

**Step 1: Write failing command tests**

Add tests for:
- parsing `ratchet acp client profiles verify fixture --prompt ping --timeout 5s --json`
- rejecting missing profile name
- executing verify with a trusted profile through a fake runner
- rejecting untrusted profiles through default registry behavior

Expected JSON metadata:
- `name`
- `status`
- `commandFingerprint`
- `acpSessionId`
- `stopReason`
- `textBytes`

**Step 2: Verify RED**

Run: `go test ./cmd/ratchet -run 'TestACPClientProfilesVerify|TestExecuteACPClientProfilesVerify' -count=1`

Expected: FAIL because `verify` is unknown.

**Step 3: Commit tests**

Commit: `test: require ACP profile verification command`

Rollback: revert this commit; no runtime behavior changes.

### Task 5: Implement ACP Profile Verify

**Files:**
- Modify: `cmd/ratchet/cmd_acp_client.go`

**Step 1: Add command parsing and execution**

Add profile subcommand `verify`. Reuse the existing trusted-profile registry
resolution path and `acpClientExecRunner` interface. Redact prompt/response
content from human and JSON output.

**Step 2: Verify GREEN**

Run: `go test ./cmd/ratchet -run 'TestACPClientProfilesVerify|TestExecuteACPClientProfilesVerify' -count=1`

Expected: PASS.

**Step 3: Verify invariant**

Temporarily bypass trusted-profile resolution or print response text, rerun the
new tests, and confirm they fail. Restore the fix and rerun to PASS.

**Step 4: Commit implementation**

Commit: `feat: add ACP profile verify command`

Rollback: revert this commit; ACP profiles return to list/add/install/trust/remove only.

### Task 6: Add Fixture Binary Smoke And Docs For Profile CI

**Files:**
- Modify: `cmd/ratchet/acp_client_binary_test.go`
- Modify: `README.md`
- Modify: `docs/harness-emulation.md`
- Modify: `docs/policy-matrix.md`
- Modify: `docs/competitor-parity.md`
- Modify: `cmd/ratchet/harness_docs_test.go`

**Step 1: Add built CLI fixture proof**

Extend the ACP client binary smoke to:
- build `internal/acpclient/testdata/fixture-agent`
- add a trusted profile pointing at the fixture
- run `ratchet acp client profiles verify fixture --json`
- assert status `ok`, stop reason `end_turn`, non-zero `textBytes`, and no prompt/response text in stdout

**Step 2: Update public docs**

Document `profiles verify` as a credential-free CI contract check and keep
credentialed third-party agent CI marked deferred unless secrets are explicitly
configured outside ratchet-cli.

**Step 3: Run PR2 verification**

Run:
- `go test ./cmd/ratchet -run 'TestACPClientProfiles|TestACPClientExecBinarySmoke|TestHarnessEmulationDocsCoverSupportedModesAndParity' -count=1`
- `go test ./internal/acpclient -run 'Profile|Registry|ClientRunPromptAgainstFixtureProcess' -count=1`
- `GOOS=windows GOARCH=amd64 go build ./cmd/ratchet`

Expected: tests PASS; Windows build exits 0.

**Step 4: Commit smoke/docs**

Commit: `test: verify ACP profiles through fixture agent`

Rollback: revert this commit; `profiles verify` remains but binary smoke/docs claims are removed.
