# ratchet-cli Policy Matrix Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Ship a source-of-truth policy matrix for ratchet-cli permissions, sandboxing, trust, and per-agent scope, guarded by docs regression tests.

**Architecture:** Add `docs/policy-matrix.md` as the durable policy contract, update public docs to link it and mark v0.22.0 persistent trust behavior as shipped, then extend the existing docs regression test in `cmd/ratchet/harness_docs_test.go` so policy-layer drift fails CI. No new runtime policy engine, scheduler, sandbox, or hook SDK is added.

**Tech Stack:** Go tests, Markdown docs, existing `cmd/ratchet` docs-test pattern.

**Base branch:** `master`

---

## Scope Manifest

**PR Count:** 1
**Tasks:** 4
**Estimated Lines of Change:** ~260

**Out of scope:**
- Background or scheduled auto-drain.
- New trust matching semantics or policy evaluator.
- Config-file mutation.
- New sandbox/path/network enforcement.
- Broad runtime extension SDK.
- Credentialed third-party agent CI.
- Raw ACPX JSON-RPC event-log compatibility.
- ACPX TypeScript flow runtime compatibility.
- Local-first gateway or channel routing.

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|------|-------|-------|--------|
| 1 | Add ratchet-cli policy matrix docs and guards | Task 1, Task 2, Task 3, Task 4 | `feat/ratchet-cli-policy-matrix` |

**Status:** Draft

## Integration Matrix

| Integration | Type | Proof |
|---|---|---|
| Documentation policy matrix | runtime-integrated docs guard | `go test ./cmd/ratchet -run HarnessEmulationDocs -count=1` reads real docs from repo root and fails if required policy terms are missing. |
| Current competitor/source snapshot | config-only | `docs/competitor-parity.md` remains a source-backed snapshot; this PR updates local ratchet-cli status but does not refetch or vendor external code. |
| Runtime policy behavior | deferred | This PR documents existing behavior only. New behavior requires a later locked design. |

### Task 1: Add failing policy docs regression

**Files:**
- Modify: `cmd/ratchet/harness_docs_test.go`

**Step 1: Write failing test coverage**

Extend `TestHarnessEmulationDocsCoverSupportedModesAndParity` or add a sibling test that reads `../../docs/policy-matrix.md`, `../../docs/harness-emulation.md`, `../../docs/competitor-parity.md`, and `../../README.md`.

Required assertions:

- `docs/policy-matrix.md` exists.
- Matrix names these layers: `Static config trust rules`, `Runtime trust rules`, `Persistent trust grants`, `Permission prompts`, `ACP client queue/drain`, `Sandbox/path/network controls`, `Hooks/extensions`, `Retro/self-improvement`, `Per-agent/team scopes`.
- Matrix names these statuses: `Supported`, `Partial`, `Deferred`, `Explicit drain only`.
- Matrix includes `sensitive local policy metadata`.
- Public docs include `docs/policy-matrix.md`.
- Public docs mention `background drain` and `extension hooks` as deferred.

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/ratchet -run HarnessEmulationDocs -count=1`

Expected: FAIL because `docs/policy-matrix.md` does not exist and public docs do not link it.

**Step 3: Commit**

```bash
git add cmd/ratchet/harness_docs_test.go
git commit -m "test: require policy matrix docs coverage"
```

Rollback: revert this commit; docs guard returns to prior parity-only assertions.

### Task 2: Add policy matrix document

**Files:**
- Create: `docs/policy-matrix.md`

**Step 1: Write matrix**

Create `docs/policy-matrix.md` with:

- scope and non-goals;
- policy precedence section;
- layer table with owner/current status/rule/validation;
- sensitive metadata section;
- deferred automation section for background drain, extension hooks, sandbox/path/network expansion, per-agent policy scopes, credentialed third-party CI, ACPX raw event logs, ACPX TypeScript runtime, and local-first gateway/channel work;
- verification section naming the docs test.

**Step 2: Run focused test**

Run: `go test ./cmd/ratchet -run HarnessEmulationDocs -count=1`

Expected: still FAIL until public docs link the matrix.

**Step 3: Commit**

```bash
git add docs/policy-matrix.md
git commit -m "docs: add ratchet policy matrix"
```

Rollback: revert this commit plus Task 1 if the matrix is rejected.

### Task 3: Link matrix from public docs and refresh parity status

**Files:**
- Modify: `README.md`
- Modify: `docs/harness-emulation.md`
- Modify: `docs/competitor-parity.md`

**Step 1: Update README**

Add a concise note near the trust section linking `docs/policy-matrix.md`, stating runtime trust rules, persistent grants, permission prompts, and explicit ACP client drain are supported while background drain and extension hooks are deferred.

**Step 2: Update harness docs**

Link `docs/policy-matrix.md` from the parity/remaining-gaps area and trust command section. Preserve the existing wording that grant listings are sensitive local policy metadata.

**Step 3: Update competitor parity**

Refresh local ratchet-cli status from v0.20.0-era wording:

- Persistent trust policy editing is supported as of v0.22.0.
- Policy matrix is in progress or supported once this PR lands.
- Background drain, broad extension hooks, per-agent policy scopes, raw ACPX event-log archives, ACPX TypeScript runtime, and local-first gateways remain deferred.

Do not claim new sandbox or extension behavior.

**Step 4: Run docs proof**

Run: `go test ./cmd/ratchet -run HarnessEmulationDocs -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add README.md docs/harness-emulation.md docs/competitor-parity.md
git commit -m "docs: link policy matrix from harness docs"
```

Rollback: revert this commit; policy matrix remains internal until public docs are corrected.

### Task 4: Full verification and PR prep

**Files:**
- Modify: `docs/plans/2026-07-02-ratchet-cli-policy-matrix.md`

**Step 1: Run focused docs checks**

Run:

```bash
rg -n "Policy Matrix|persistent trust grants|background drain|extension hooks|sensitive local policy metadata" README.md docs/harness-emulation.md docs/competitor-parity.md docs/policy-matrix.md
go test ./cmd/ratchet -run HarnessEmulationDocs -count=1
```

Expected: `rg` finds all required phrases and the focused Go test exits 0.

**Step 2: Run full local verification**

Run:

```bash
go test ./... -count=1 -p=1
git diff --check
```

Expected: both commands exit 0.

**Step 3: Update plan notes if needed**

If implementation discovers no scope drift, leave the Scope Manifest unchanged. If a design assumption changes without manifest changes, append a compact Backport note to the design.

**Step 4: Commit**

```bash
git add docs/plans/2026-07-02-ratchet-cli-policy-matrix.md docs/plans/2026-07-02-ratchet-cli-policy-matrix-design.md
git commit -m "docs: record policy matrix verification"
```

Skip the commit if no plan/design files changed.

Rollback: revert the PR. No runtime state or release tag is affected.

