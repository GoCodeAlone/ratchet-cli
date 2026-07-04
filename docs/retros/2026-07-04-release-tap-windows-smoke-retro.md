# Retro: Release Tap And Windows Smoke Gates

**PR:** #78 - chore: gate tap and windows archive proof
**Merged:** 2026-07-04
**Branch:** feat/release-tap-windows-smoke
**Design:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Plan:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Related ADRs:** none

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D125 required a PR/push tap preflight or recorded tap-cleanup SHA before fail-closed release enforcement. | Important | Prescient: Task 9 removed stale tap surfaces via GoCodeAlone/homebrew-tap#63, then PR #78 added CI `tap-preflight`. |
| design | D127 required one typed releaseguard mode contract for manifest, draft-assets, tap-preflight, and tap-postcheck. | Important | Resolved upfront: PR #76 added the mode enum; PR #78 wired the modes into CI/release workflows. |
| design | D128 warned that prepublish tap checks must not false-fail by expecting the new version before GoReleaser updates the tap. | Important | Resolved upfront: PR #78 keeps preflight shape/smoke checks separate from postpublish current-version checks. |
| plan | P2/P7 required external tap prerequisite accounting before Tasks 10-11. | Important | Prescient: the tap cleanup landed and was recorded before fail-closed checks merged. |
| plan | P3 required fresh GoReleaser snapshot generation before manifest-only artifact checks. | Important | Resolved upfront: CI `release-check` runs GoReleaser snapshot before `scripts/check-release-artifacts.sh --manifest-only dist`. |
| plan | P4 required local Windows amd64/arm64 cross-build proof. | Important | Resolved upfront: local verification and CI Windows build use temp-scoped outputs. |
| plan | P11/P14 required consistent draft-assets and tap-postcheck env interfaces in workflow tests. | Important | Resolved upfront: workflow tests assert every required env var and command. |
| plan | P16 required prepublish proof that `.goreleaser.yaml` keeps `release.draft: true`. | Important | Prescient: PR #78 added `TestGoReleaserReleaseDraftConfig` and release workflow preflight. |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| `GuardDraftAssets` initially accepted `metadata.json` with no `draft` field. | test-driven-development / implementation review | The first test covered `draft:false` but not omitted draft state; Copilot caught it before merge. | For release metadata invariants, test both explicit bad value and missing required field. |

## Missed skill activations

Activation log unavailable at the canonical repo root `.claude/autodev-state/in-progress.jsonl`; rows below are reconstructed from committed artifacts and PR evidence.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | unknown | Activation log unavailable. |
| adversarial-design-review (design) | yes | `docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design-review.md` exists. |
| writing-plans | yes | Locked plan exists with 13 tasks and PR grouping. |
| adversarial-design-review (plan) | yes | `docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-plan-review.md` exists. |
| alignment-check | yes | `docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-alignment.md` exists. |
| subagent-driven-development | unknown | Activation log unavailable; implementation was lead-driven in this continuation. |
| finishing Step 1e (doc-reconciliation) | yes | PR body recorded plan/backport evidence and no user-facing docs change in this PR. |
| pr-monitoring | yes | PR #78 checks were monitored through green; Copilot thread was fixed and resolved before admin merge. |
| post-merge-retrospective | yes | This file. |

## What worked

- CI caught the intended release shape on the real PR: `Release Check`, `TUI Smoke`, `Tap Preflight`, Windows build, lint, vet, tests, and CodeQL all went green before merge.
- The external tap cleanup prerequisite prevented PR #78 from enabling fail-closed tap checks against a tap that still had unmanaged install surfaces.
- Workflow-contract tests made release/tap env drift reviewable as normal Go tests instead of relying on visual YAML inspection.
- The release workflow now preserves draft state until downloaded release assets and exact tap path-changing commits pass postchecks.

## What didn't

- The first draft-assets implementation under-specified required metadata: `draft:false` failed, but missing `draft` passed until Copilot review.
- `gh pr merge --admin` attempted local git worktree operations when run inside a worktree whose base branch was checked out elsewhere; remote-only `--repo` invocation avoided the local checkout conflict.
- `Release Check` is the slowest PR/master job by a wide margin; the bounded polling still worked, but future release-check failures will need direct job-log inspection after completion.

## Plugin-level follow-ups

No plugin-level change yet. The missing-field issue is a useful local testing lesson but not a repeated autodev gate failure across retros.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `docs/design-guidance.md` | no change | No durable project-wide guidance file exists, and the lesson is releaseguard-test specificity rather than a new ratchet-cli product or architecture constraint. |
