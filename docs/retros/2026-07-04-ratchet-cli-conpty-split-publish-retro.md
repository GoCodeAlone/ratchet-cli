# Retro: Ratchet CLI ConPTY and Split Publish

**PR:** #84 - feat: prove Windows ConPTY and split publish
**Merged:** 2026-07-04
**Branch:** `feat/conpty-split-publish`
**Design:** `docs/plans/2026-07-04-ratchet-cli-conpty-split-publish-design.md`
**Plan:** `docs/plans/2026-07-04-ratchet-cli-conpty-split-publish.md`
**Related ADRs:** none

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | Require proof that `skip_upload: true` still writes the generated cask path. | Important | Resolved upfront |
| design | Tap push failure after draft asset validation must leave rollback evidence and keep the release draft. | Important | Resolved upfront |
| design | Windows proof must drive a ConPTY-backed terminal, not only cross-compile. | Important | Prescient |
| plan | Repo-local scope helper was absent; use the autodev helper instead. | Important | Resolved upfront |
| plan | Release workflow and releaseguard edits are coupled and should be verified serially. | Minor | Resolved upfront |
| plan | Windows ConPTY runtime proof requires hosted Windows CI. | Important | Prescient |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| Initial Windows ConPTY smoke timed out in CI because `termtest.Expect` could block past the intended timeout and Windows line input needed explicit carriage returns. | local verification boundary | The non-Windows local host could only compile the Windows smoke path; runtime behavior existed only on hosted Windows. | Keep the Windows CI job required, and keep bounded ConPTY expect/send wrappers in the test. |

## Missed skill activations

Activation log unavailable in the canonical ratchet-cli checkout. Gates are scored from committed artifacts and PR evidence.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | yes | Design captures alternatives and out-of-scope runner changes. |
| adversarial-design-review (design) | yes | `docs/plans/2026-07-04-ratchet-cli-conpty-split-publish-design-review.md` |
| writing-plans | yes | `docs/plans/2026-07-04-ratchet-cli-conpty-split-publish.md` |
| adversarial-design-review (plan) | yes | `docs/plans/2026-07-04-ratchet-cli-conpty-split-publish-plan-review.md` |
| alignment-check | yes | `docs/plans/2026-07-04-ratchet-cli-conpty-split-publish-alignment.md` |
| finishing Step 1e (doc-reconciliation) | yes | PR body and diff included README, RATCHET, harness, policy, and competitor docs. |
| pr-monitoring | yes | PR #84 was monitored through one failed CI run, a fix commit, green PR checks, and admin merge. |

## What worked

- The adversarial review correctly forced Windows runtime proof instead of accepting cross-compilation as equivalent.
- Releaseguard tests gave a tight place to enforce split-publish ordering and token boundaries.
- Copilot review produced no change requests; the important feedback came from CI, not code review.
- Keeping the Windows smoke path behind `tui_smoke` avoided broadening release binaries while still proving the terminal boundary.

## What didn't

- Local verification could not reproduce the hosted Windows ConPTY behavior, so the first CI run found a runtime hang.
- The macOS local race retry after merge was interrupted by host disk pressure (`errno=28`), so post-merge closeout relied on green PR CI plus delayed master CI observation.
- Master CI for the merge commit was still delayed in `Release Check` and race `Test` when the closeout branch was prepared; the PR run for the same change had already passed both.

## Plugin-level follow-ups

No plugin-level change yet. This is a single expected gap for OS-specific runtime behavior, and the required hosted Windows job caught it before merge.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `docs/design-guidance.md` | no change | The repo does not carry a project guidance file, and this retro produced no durable cross-design rule beyond the already-committed Windows CI requirement. |
