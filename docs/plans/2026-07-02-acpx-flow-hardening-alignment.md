### Alignment Report

**Status:** PASS

**Coverage:**

| Design Requirement | Plan Task(s) | Status |
|---|---|---|
| Add JSON v1 `action` flow nodes. | Task 1, Task 2, Task 3, Task 4 | Covered |
| Add per-node cwd for `acp` and `action` nodes. | Task 1, Task 2, Task 4 | Covered |
| Add flow permission preflight with explicit `--allow`. | Task 1, Task 2, Task 3, Task 4 | Covered |
| Implicit `shell` for action nodes and `outside-cwd` for cwd escape. | Task 1, Task 2, Task 3, Task 4 | Covered |
| Persist action outputs in existing flow run bundles with bounded stdout/stderr. | Task 1, Task 2, Task 4 | Covered |
| Keep TypeScript ACPX runtime, replay UI, hooks, self-evolution, and sandbox expansion out of scope. | Scope Manifest, Task 5 | Covered |
| Update README, harness, competitor parity, policy matrix, and docs guard. | Task 5 | Covered |
| Verify built CLI through action + ACP fixture binary smoke, full tests, lint, vet, and Windows builds. | Task 4, Task 6 | Covered |
| No infra, release, registry, migration, or daemon protocol changes. | Scope Manifest, Task 6 | Covered |

**Scope Check:**

| Plan Task | Design Requirement | Status |
|---|---|---|
| Task 1 | Internal tests for action nodes, permission preflight, cwd containment, truncation, failed state. | Justified |
| Task 2 | Runtime implementation for action nodes, implicit permissions, cwd resolver, runner, persistence. | Justified |
| Task 3 | CLI parser/executor and binary smoke tests for `--allow` and action flows. | Justified |
| Task 4 | CLI flag plumbing and representative built-binary invocation. | Justified |
| Task 5 | Public docs and policy matrix updates required by design. | Justified |
| Task 6 | Scope, format, full Go, lint, Windows, runtime, PR, monitor, merge, closeout gates. | Justified |

**Manifest Check:**

`plan-scope-check.sh --plan docs/plans/2026-07-02-acpx-flow-hardening.md` → PASS.

**Drift Items:** none.
