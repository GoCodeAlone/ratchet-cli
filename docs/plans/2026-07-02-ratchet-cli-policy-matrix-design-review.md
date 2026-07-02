### Adversarial Review Report

**Phase:** design
**Artifact:** `docs/plans/2026-07-02-ratchet-cli-policy-matrix-design.md`
**Status:** PASS

**Findings (Critical):**
- None.

**Findings (Important):**
- None.

**Findings (Minor):**
- `D1` [User-intent drift] The design intentionally ships a docs/test slice instead of new auto-drain or extension behavior. Recommendation: keep the out-of-scope list explicit and ensure the plan labels this as the prerequisite slice, not the whole remaining roadmap. _Resolution: accepted; design out-of-scope and user-intent sections state this._
- `D2` [Security / overclaiming] A policy precedence table can be misread as fully enforced runtime behavior even where sandbox/path/per-agent scopes are partial. Recommendation: the implementation doc must label each layer as supported, partial, or deferred and avoid claiming new enforcement. _Resolution: accepted; design requires status labels and partial/deferred wording._
- `D3` [Existence/runtime-validity] `docs/competitor-parity.md` currently says the snapshot was refreshed for v0.20.0, while v0.22.0 shipped after it. Recommendation: the implementation must refresh the relevant status rows so the matrix does not cite stale ratchet-cli status. _Resolution: accepted; design includes competitor-parity updates._

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Design follows workspace guidance by avoiding duplicate policy engines and keeping automation deferred until boundaries are explicit. |
| Assumptions under attack | Clean | Assumptions are stated; the most fragile one is that docs/tests are enough for this prerequisite slice, and the design mitigates with explicit regression tests. |
| Repo-precedent conflicts | Clean | Uses existing docs regression pattern in `cmd/ratchet/harness_docs_test.go`. |
| Artifact-class precedent | Clean | New design/review docs match existing `docs/plans/*-design.md` and `*-design-review.md` shape. |
| YAGNI violations | Clean | No new scheduler, hook runtime, sandbox engine, or policy evaluator is added. |
| Missing failure modes | Clean | The main failure mode is docs overclaiming behavior; design requires supported/partial/deferred labels. |
| Security/privacy | Minor | Policy metadata sensitivity is called out, but implementation must preserve that wording. |
| Infrastructure impact | Clean | Docs/tests only; no cloud, DB, network, release, or deploy impact. |
| Multi-component validation | Clean | Uses docs grep plus Go test coverage; no runtime boundary is introduced in this slice. |
| Declared integration proof | Clean | Existing integrations are documented only; no new integration is declared as installed/enabled. |
| Contributed UI rendering proof | Clean | No UI contribution. |
| Rollback story | Clean | Revert docs/test PR; no runtime state. |
| Simpler alternative | Clean | README-only update considered and rejected because a guarded matrix is needed before behavioral automation. |
| User-intent drift | Minor | This is a prerequisite, not all remaining harness work. |
| Existence/runtime-validity | Minor | Existing parity doc status must be refreshed from v0.20.0/v0.22.0 wording. |

**Options the author may not have considered:**
1. Add `ratchet policy matrix` as a CLI command now. This would make the matrix scriptable, but it would add a new command surface before the policy model is proven useful. Docs plus tests are lower-risk for this prerequisite slice.
2. Move the matrix into `docs/competitor-parity.md` only. This keeps parity context in one file, but it buries the local policy contract in a broad comparison document and makes later implementation plans harder to cite.

**Verdict reasoning:** PASS. The design is deliberately narrow, avoids new behavior, identifies sensitive metadata, and provides a testable source-of-truth document. The plan must keep the status labels precise and update stale parity wording.
