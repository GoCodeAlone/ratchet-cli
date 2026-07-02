# Retro: ratchet-cli Policy Matrix

**PR:** #57 - docs: add ratchet policy matrix
**Merged:** 2026-07-02
**Branch:** `feat/ratchet-cli-policy-matrix`
**Design:** `docs/plans/2026-07-02-ratchet-cli-policy-matrix-design.md`
**Plan:** `docs/plans/2026-07-02-ratchet-cli-policy-matrix.md`
**Related ADRs:** none

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1: docs/test slice could drift from the broader harness roadmap. | Minor | Resolved upfront: the plan and matrix kept auto-drain/hooks out of scope. |
| design | D2: policy precedence could overclaim unenforced behavior. | Minor | Resolved upfront: the matrix labels supported, partial, explicit-drain, and deferred layers. |
| design | D3: competitor parity status was stale after v0.22.0. | Minor | Resolved upfront: parity docs now mark persistent trust grants as shipped and link the matrix. |
| plan | P1: docs-only verification should not imply Windows runtime behavior changed. | Minor | False positive: no runtime or release claim was made; CI still included Windows Build. |
| plan | P2: Task 2 intentionally stayed red until Task 3. | Minor | Prescient: execution recorded the expected red state after the matrix was added but before public docs linked it. |
| plan | P3: competitor parity update could broaden into a source refresh. | Minor | Resolved upfront: parity edits were limited to local ratchet-cli status. |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| Copilot found a workstation-specific home-directory path in the alignment report. | verification-before-completion | The pre-PR checks scanned behavior/docs terms but did not scan committed plan artifacts for machine-local paths. | Add a changed-file scan for absolute home-directory paths or use the repo's no-machine-path helper when plan/retro artifacts are touched. |

## Missed skill activations

Activation log unavailable at the canonical repo root, so skill firing cannot be reconstructed from `.claude/autodev-state/in-progress.jsonl`.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | unavailable | Design was present before implementation resumed. |
| adversarial-design-review (design) | yes | `docs/plans/2026-07-02-ratchet-cli-policy-matrix-design-review.md`. |
| writing-plans | yes | `docs/plans/2026-07-02-ratchet-cli-policy-matrix.md`. |
| adversarial-design-review (plan) | yes | `docs/plans/2026-07-02-ratchet-cli-policy-matrix-plan-review.md`. |
| alignment-check | yes | `docs/plans/2026-07-02-ratchet-cli-policy-matrix-alignment.md`. |
| scope-lock | yes | Plan locked before execution and completed after merge. |
| subagent-driven-development | no | Subagent tools were unavailable; implementation ran inline against the locked manifest. |
| finishing Step 1e (doc-reconciliation) | unverified | PR touched docs; no dedicated Doc-reconciliation line was present in the PR body. |
| pr-monitoring | yes | PR #57 was monitored through CI, Copilot review, fix push, and admin merge. |
| post-merge-retrospective | yes | This retro. |

## What worked

- The docs regression caught both intended red states: missing matrix file, then missing public-doc link.
- The scope lock kept the slice constrained to docs/tests and avoided pulling auto-drain or hook runtime work into the PR.
- Copilot review caught artifact hygiene that local verification missed, and the thread was resolved before merge.

## What didn't

- Pre-PR verification did not include a machine-path scan over committed plan artifacts.
- The PR body did not include a dedicated docs reconciliation line even though the change was documentation-heavy.

## Plugin-level follow-ups

No plugin-level change is warranted from one miss, but future docs-heavy closeouts should include an explicit machine-path scan when committing plan, alignment, or retro artifacts.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `docs/design-guidance.md` | No change | The miss was artifact hygiene, not a durable product or architecture constraint. |
