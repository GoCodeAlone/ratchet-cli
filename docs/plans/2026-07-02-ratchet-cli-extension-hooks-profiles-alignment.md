### Alignment Report

**Status:** PASS

**Design:** `docs/plans/2026-07-02-ratchet-cli-extension-hooks-profiles-design.md`
**Plan:** `docs/plans/2026-07-02-ratchet-cli-extension-hooks-profiles.md`

**Coverage:**

| Design Requirement | Plan Task(s) | Status |
|---|---|---|
| Extend existing hooks/plugins/ACP client surfaces; no second runtime. | Task 1, Task 4, Task 5 | Covered |
| Hash-review project/plugin hooks, preserve user-hook compatibility exception. | Task 1, Task 2, Task 7 | Covered |
| Workdir-scoped project hook loading for daemon events. | Task 2 | Covered |
| Hook review CLI: list/trust/untrust/disable. | Task 3 | Covered |
| Plugin hook path containment. | Task 1 | Covered |
| Windows hook command selection/skipping. | Task 1, Task 8 | Covered |
| ACP launch profile store and trusted profile resolution. | Task 4, Task 6 | Covered |
| Plugin-distributed `acpProfiles` templates with containment. | Task 5 | Covered |
| Profile execution proof through ACP fixture, watch/drain, compare, flow. | Task 6 | Covered |
| Docs/policy/parity updates and no broad SDK/background drain claims. | Task 7 | Covered |
| Release, retro, scope closeout, Windows assets. | Task 8 | Covered |

**Scope Check:**

| Plan Task | Design Requirement | Status |
|---|---|---|
| Task 1 | Hook review/trust, plugin path containment, Windows hook command handling. | Justified |
| Task 2 | Workdir-scoped project hook loading and runtime skip semantics. | Justified |
| Task 3 | Hook review CLI. | Justified |
| Task 4 | ACP launch profile local store and profile-aware registry. | Justified |
| Task 5 | Plugin-distributed ACP profile templates. | Justified |
| Task 6 | ACP profile runtime use and fixture proof. | Justified |
| Task 7 | Public docs/policy/parity refresh. | Justified |
| Task 8 | Release, scope closeout, retro. | Justified |

**Manifest Trace:**

| Check | Result |
|---|---|
| PR count matches PR Grouping rows | PASS: 4 rows / `PR Count: 4` |
| Task count matches body headings | PASS: 8 tasks |
| Every task appears in one PR row | PASS |
| `plan-scope-check.sh --plan` | PASS |

**Drift Items:** none.
