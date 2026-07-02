# ratchet-cli Policy Matrix Alignment

**Status:** PASS
**Date:** 2026-07-02
**Design:** `docs/plans/2026-07-02-ratchet-cli-policy-matrix-design.md`
**Plan:** `docs/plans/2026-07-02-ratchet-cli-policy-matrix.md`

## Coverage

| Design Requirement | Plan Task(s) | Status |
|---|---|---|
| Add `docs/policy-matrix.md` as the source of truth for policy layers, precedence, owner, current status, sensitive data, and validation. | Task 2 | Covered |
| Update public docs to point to the matrix and mark persistent trust editing as shipped. | Task 3 | Covered |
| Guard the policy docs with `cmd/ratchet/harness_docs_test.go`. | Task 1, Task 4 | Covered |
| Name every required layer: static config trust rules, runtime trust rules, persistent trust grants, permission prompts, ACP client queue/drain, sandbox/path/network controls, hooks/extensions, retro/self-improvement, and per-agent/team scopes. | Task 1, Task 2 | Covered |
| Name required statuses: Supported, Partial, Deferred, and Explicit drain only. | Task 1, Task 2 | Covered |
| Mark grant/rule patterns as sensitive local policy metadata. | Task 1, Task 2, Task 3 | Covered |
| Keep background drain, broad extension hooks, new sandbox enforcement, ACPX compatibility, and local-first gateway work out of scope. | Scope Manifest, Task 2, Task 3 | Covered |
| Do not add a new policy evaluator or duplicate trust matching from `workflow-plugin-agent/policy`. | Scope Manifest, Integration Matrix | Covered |
| Verify focused docs guard, public phrase search, full Go tests, and whitespace checks. | Task 4 | Covered |

## Scope Check

| Plan Task | Design Requirement | Status |
|---|---|---|
| Task 1: Add failing policy docs regression | Machine-check docs coverage before behavior changes. | Justified |
| Task 2: Add policy matrix document | Create the source-of-truth matrix with layers, precedence, sensitive metadata, and deferred automation. | Justified |
| Task 3: Link matrix from public docs and refresh parity status | Expose the matrix from README/harness/parity docs and correct v0.22.0 trust status. | Justified |
| Task 4: Full verification and PR prep | Prove the docs/test slice satisfies the design without adding runtime behavior. | Justified |

## Manifest Check

Command:

```bash
bash /Users/jon/.codex/plugins/cache/autodev-marketplace/autodev/6.5.11/tests/plan-scope-check.sh --plan docs/plans/2026-07-02-ratchet-cli-policy-matrix.md
```

Result: PASS. The repository does not vendor `tests/plan-scope-check.sh`, so the autodev plugin helper was used.

## Drift Items

None.
