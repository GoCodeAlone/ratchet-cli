### Adversarial Review Report

**Phase:** plan
**Artifact:** `docs/plans/2026-07-05-ratchet-runtime-extension-lifecycle.md`
**Status:** PASS

**Findings (Critical):**
- None.

**Findings (Important):**
- None.

**Findings (Minor):**
- `P1` [Scope-lock metadata] [prior plan shape]: The previously merged plan had a non-standard `Scope locked` status and a hand-written sidecar that did not hash a `## Scope Manifest`. Recommendation: rewrite the plan into the current manifest format and regenerate the sidecar with `scope-lock-apply`. _Resolution: repaired in this plan revision._
- `P2` [Hidden serial dependencies] [PR Grouping]: Task 9 spans ratchet docs, workspace state, and release closeout, so it must remain after feature PRs and messaging-core bridge work. Recommendation: keep Task 9 in PR #4 only. _Resolution: manifest groups Task 9 in PR #4._
- `P3` [YAGNI] [Task 7]: A workflow primitive can sprawl into a full runtime. Recommendation: limit Task 7 to persisted declarative definitions and run records; defer JavaScript/TypeScript execution. _Resolution: Task 7 and out-of-scope list state this boundary._

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Plan cites workspace inventory/follow-ups and keeps messaging delivery in Workflow plugins. |
| Assumptions under attack | Clean | The main assumption is that PR #97 completed Tasks 3-5; the plan records commit evidence and does not reimplement it. |
| Repo-precedent conflicts | Clean | Commands, stores, and docs tests follow existing `cmd/ratchet`, `internal/plugins`, and policy-matrix patterns. |
| Artifact-class precedent | Clean | CLI command tests and package store tests match sibling command/store packages. |
| YAGNI violations | Minor | Workflow runtime breadth is constrained to definitions/run records in Task 7. |
| Missing failure modes | Clean | Malformed catalogs, disabled plugins, temp state, hidden background work, and direct messaging credentials are covered. |
| Security / privacy | Clean | Sensitive local metadata and no raw prompt/default credential exposure are explicit constraints. |
| Infrastructure impact | Clean | No cloud resources, migrations, or production deploys are introduced. |
| Multi-component validation | Clean | Runtime-integrated rows require package/CLI/daemon or messaging-core proof. |
| Declared integration proof | Clean | Integration matrix classifies ratchet stores, daemon reload, routines/workflows, messaging-core, and channel plugins. |
| Contributed UI rendering proof | Clean | No UI contribution is introduced. |
| Rollback story | Clean | Each task has a rollback note. |
| Simpler alternative not considered | Clean | Flat JSON state is selected for local registry/routine/workflow definitions. |
| User-intent drift | Clean | The next two ratchet features are the next approved runtime-extension slices: marketplace lifecycle and routines/workflows. |
| Existence / runtime-validity | Clean | PR #97 evidence exists in current tree; messaging-core work is explicitly deferred to PR #4. |
| Over/under-decomposition | Clean | Nine tasks match the original four PR slices; the next two features are two PRs. |
| Verification-class mismatch | Clean | CLI/store tasks use package and command tests; cross-repo bridge uses messaging-core package tests. |
| Auth/authz chain composition | Clean | No server auth chain is introduced. |
| Hidden serial dependencies | Minor | Task 9 must remain after feature and bridge tasks. |
| Missing rollback wiring | Clean | Rollback notes are embedded in each task. |
| Missing integration proof | Clean | The matrix includes runtime-integrated proof requirements. |
| Missing declared integration matrix | Clean | Present. |
| Missing contributed UI route proof | Clean | Not applicable. |
| Infrastructure verification mismatch | Clean | Not applicable. |
| Plugin-loader runtime layout | Clean | Plugin lifecycle tasks use existing installer/loader layout rather than new process shape. |
| Config-validation schema rules | Clean | No Workflow config schema is emitted by ratchet-cli in PR #2/#3. |
| Identifier / naming-convention match | Clean | CLI flags and nouns follow existing `plugin`, `hooks`, `profiles`, and `sessions` command style. |
| Planned-code compile-validity | Clean | The plan contains no production Go code snippets. |

**Options the author may not have considered:**
1. Finish only marketplace lifecycle and leave routines/workflows for a new plan: smaller, but it would ignore the already approved runtime-extension sequence.
2. Move routines/workflows into Workflow itself: stronger platform reuse long term, but ratchet still needs local visible harness state for agent scheduling/composition.

**Verdict reasoning:** PASS. The repaired plan preserves the approved design and PR sequence while making scope-lock enforcement real. Remaining risks are controlled by explicit non-goals and per-task verification.
