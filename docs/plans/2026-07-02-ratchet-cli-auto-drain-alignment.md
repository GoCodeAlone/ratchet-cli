### Alignment Report

**Status:** PASS

**Design:** `docs/plans/2026-07-02-ratchet-cli-auto-drain-design.md`
**Plan:** `docs/plans/2026-07-02-ratchet-cli-auto-drain.md`
**Manifest check:** `plan-scope-check.sh --plan` passed on 2026-07-02.

**Coverage:**

| Design Requirement | Plan Task(s) | Status |
|---|---|---|
| Add explicit opt-in ACP client auto-drain worker. | Task 1, Task 2, Task 3, Task 4 | Covered |
| Use foreground `watch` command, not hidden daemon scheduler. | Scope Manifest, Task 3, Task 4, Task 5 | Covered |
| Reuse existing `DrainQueue`, owner locks, cancel requests, and command resolution. | Task 1, Task 2, Task 4 | Covered |
| Require explicit `--agent`/`--command`; do not reconstruct argv from fingerprint. | Scope Manifest, Task 2, Task 4 | Covered |
| Support scheduling flags: interval, max-per-cycle, max-cycles, stop-when-empty. | Task 1, Task 2, Task 3, Task 4 | Covered |
| Support human and JSON aggregate cycle output. | Task 3, Task 4 | Covered |
| Preserve prompt privacy in watch output. | Task 1, Task 3, Task 4, Task 5 | Covered |
| Handle empty queue, max cycles, drain errors, cancellation, and busy owner. | Task 1, Task 2, Task 3, Task 4 | Covered |
| Prove CLI-to-agent boundary through binary smoke. | Task 3, Task 4, Task 6 | Covered |
| Update public docs and policy matrix while keeping daemon background drain deferred. | Task 5 | Covered |
| Verify full Go test, vet, lint, Windows builds, docs grep, and diff check. | Task 6 | Covered |
| Avoid schema, release, Homebrew, registry, hooks, sandbox, and ACPX expansion. | Scope Manifest | Covered |

**Scope Check:**

| Plan Task | Design Requirement | Status |
|---|---|---|
| Task 1: Add internal watch loop tests. | Unit-test watch loop behavior from the design. | Justified |
| Task 2: Implement internal watch loop. | Implement design runtime behavior and policy boundary in `internal/acpclient`. | Justified |
| Task 3: Add CLI and binary watch tests. | Prove CLI shape and real ACP child-process boundary before implementation. | Justified |
| Task 4: Implement CLI watch command. | Ship `ratchet acp client watch` with explicit command resolution and aggregate output. | Justified |
| Task 5: Update public docs and policy matrix. | Document foreground watch and keep hidden background drain deferred. | Justified |
| Task 6: Full verification, PR, and closeout. | Run design-required local/CI gates and close the locked plan. | Justified |

**Drift Items:** None.
