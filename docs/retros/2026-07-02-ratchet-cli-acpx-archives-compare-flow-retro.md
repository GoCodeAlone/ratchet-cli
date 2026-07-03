# Retro: ACPX Archives, Compare, And Flow Replay

**PRs:** #67, #68, #69, #70
**Merged:** 2026-07-03
**Branches:** feat/acpx-raw-archive-events, feat/acp-compare-bundles, feat/acp-flow-replay-bundles, docs/acpx-archive-flow-release
**Design:** docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow-design.md
**Plan:** docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow.md
**Related ADRs:** none

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1 required typed ACP event propagation into compare and flow bundles. | Important | Prescient: compare/flow bundle PRs needed explicit event sidecar plumbing. |
| design | D2 required raw export to fail closed when raw history is unavailable. | Important | Resolved upfront: raw export does not synthesize false wire history. |
| design | D3 required an upstream-shaped `exported_by:"acpx"` raw archive fixture round-trip. | Important | Resolved upfront: archive tests covered raw import/export preservation. |
| design | D5 warned replay bundles were sizable. | Minor | Inconclusive: review caught several replay hardening issues, but scope stayed justified. |
| plan | P1 required live `exec` sidecar persistence before raw export. | Important | Resolved upfront by command/store tests and binary smoke. |
| plan | P2 rejected optional Go interface mutation for flow events. | Important | Resolved upfront with `LastEvents()` type assertion. |
| plan | P3 moved docs guard updates to Task 9. | Important | Resolved upfront; PR3 shipped without premature docs guard drift. |
| plan | P4 required running the scope-lock helper from the ratchet worktree. | Important | Resolved upfront and used during closeout. |
| plan | P6 noted Task 10 mixed release, retro, and workspace state. | Minor | Prescient: release/tag/tap sequencing needed extra attention. |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| PR #70 docs used `raw|summary|both` while CLI help used `summary|raw|both`. | doc-reconciliation | The check verified command presence but not option-order parity with help text. | When docs show enum-like CLI values, grep the command help or source usage string. |
| `v0.25.0` was already published from PR #69 before PR #70 docs merged. | release gate | The plan said "if no tag exists"; it did not state how to handle a tag created early by another runner. | Treat existing release tags as immutable; record whether docs-only deltas landed after the tag. |
| Homebrew cask was 0.25.0, but `Formula/ratchet-cli.rb` remained 0.2.0. | release gate | The release check looked for a cask update but not the formula path. | Verify both `Casks/ratchet-cli.rb` and `Formula/ratchet-cli.rb` when the tap has both. |

## Missed skill activations

Activation log unavailable at `.claude/autodev-state/in-progress.jsonl`; rows below are reconstructed from committed artifacts, PR bodies, and local evidence.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | unverified | Design/plan already existed when this closeout resumed. |
| adversarial-design-review (design) | yes | `docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow-design-review.md`. |
| writing-plans | yes | `docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow.md`. |
| adversarial-design-review (plan) | yes | `docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow-plan-review.md`. |
| alignment-check | yes | `docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow-alignment.md`. |
| scope-lock | yes | `.scope-lock` existed until closeout and was removed by `scope-lock-complete`. |
| finishing Step 1e (doc-reconciliation) | yes | PR #70 body recorded one doc-reconciliation fix. |
| pr-monitoring | yes | PRs #67-#70 and tap PR #60 were monitored through green checks and review resolution. |
| post-merge-retrospective | yes | Per-PR retros exist for PRs #67-#69; this file closes the full plan. |

## What worked

- The 4-PR split kept raw archive, compare bundle, flow replay, and docs/release review surfaces separate.
- The design review forced raw archive compatibility to be proved through preservation, not approximate reconstruction.
- Review monitoring caught replay path hardening issues and final docs/help drift before merge.
- The release gate found the stale Homebrew formula even after the cask and release assets were already published.

## What didn't

- The public `v0.25.0` tag was cut before the final docs PR. That is acceptable for code assets, but the closeout should call out post-tag docs-only changes.
- The tap had two install surfaces, and only one was automatically updated by the release workflow.
- The docs reconciliation check was too coarse for enum ordering.

## Plugin-level follow-ups

No plugin-level change yet. If release retros repeatedly find partial tap updates, add a release-gate checklist item for every tap file that can install the artifact.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `docs/design-guidance.md` | no change | No repo-local guidance file exists, and the lesson is release-checklist-specific rather than a durable product or architecture constraint. |
