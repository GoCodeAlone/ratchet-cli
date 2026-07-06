### Adversarial Review Report

**Phase:** design
**Artifact:** `docs/plans/2026-07-06-ratchet-cli-windows-policy-surface-design.md`
**Status:** PASS

**Findings (Minor):**
- `D1` [assumptions under attack]: The CLI policy table can drift from `docs/policy-matrix.md`. Recommendation: keep the Markdown matrix as source of truth, document the command as a convenience view, and test required layer/status terms. _Resolution: accepted in design scope and Task 3/4._
- `D2` [user-intent drift]: Windows startup proof can sound like full Windows runtime parity. Recommendation: docs must say non-interactive command startup is proven while full packaged TUI/installer parity remains deferred. _Resolution: accepted in Task 4._
- `D3` [simpler alternative]: A docs-only update would be cheaper. Recommendation: justify CLI value as local operator discoverability and avoid any enforcement logic. _Resolution: accepted in design architecture._

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Uses Go, no new dependencies, no secrets, and no new standalone repo. |
| Assumptions under attack | Minor | Drift risk recorded as D1. |
| Repo-precedent conflicts | Clean | Mirrors existing `cmd/ratchet` flat command handlers and `internal/releaseguard` workflow guards. |
| Artifact-class precedent | Clean | CI workflow proof is guarded in the existing releaseguard package. |
| YAGNI violations | Clean | Does not add policy enforcement, providers, installers, or SDK hooks. |
| Missing failure modes | Clean | Windows smoke failure rolls back by reverting one CI job; policy command uses static data. |
| Security/privacy | Clean | No local sensitive policy metadata is read or emitted. |
| Infrastructure impact | Clean | One non-cloud GitHub-hosted Windows job; no resources or secrets. |
| Multi-component validation | Clean | CI job provides real Windows runner proof; local command launch verifies CLI surface. |
| Declared integration proof | Clean | GitHub Actions is config+runtime proof; no external provider integration declared. |
| Rollback story | Clean | Revert PR; no migration or state cleanup. |
| Simpler alternative | Minor | Docs-only alternative recorded as D3. |
| User-intent drift | Minor | Windows overclaim risk recorded as D2. |
| Existence/runtime-validity | Clean | Existing workflow and command surfaces are verified before mutation. |

**Options the author may not have considered:**
1. Put policy output in `doctor`: lower command count, but weaker discoverability and it mixes static policy docs with local diagnostics.
2. Generate policy rows from Markdown: avoids drift but adds brittle Markdown parsing to a user-facing command.

**Verdict reasoning:** PASS. The design is bounded, testable, and aligned with the existing deferred-boundary policy.
