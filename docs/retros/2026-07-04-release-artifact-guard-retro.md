# Retro: Release Artifact Guard

**PR:** #76 - test: guard release artifacts
**Merged:** 2026-07-04
**Branch:** feat/release-artifact-guard
**Design:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Plan:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Related ADRs:** decisions/0001-smoke-package-list-boundary.md

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D15-D24 release boundary, artifact guard, Windows-boundary precision, and Homebrew/tap proof requirements | Important / Minor | Prescient; PR #76 implemented the artifact/cask guard slice and kept Windows runner changes deferred |
| plan | P1-P8 stale artifact, Windows proof, tap prerequisite accounting, and releaseguard taxonomy gaps | Critical / Important / Minor | Resolved upfront for Tasks 7-8; tap cleanup remains locked for Task 9 |
| plan | P9-P17 tagged helper tests, releaseguard interface consistency, manifest drift, tap env, and draft preflight gaps | Important / Minor | Prescient; Copilot found draft/tap mode env values were required but not consumed |
| plan | P18-P24 source manifest, Homebrew fallback, redaction scope, and Windows/archive proof boundaries | Important / Minor | Prescient; local review added missing packaged-binary enforcement before PR, Copilot tightened generated cask validation |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| Generated cask validation accepted `ratchet-cli` as evidence of `ratchet` binary because it used substring matching | requesting-code-review | Local review checked cask presence but not exact Ruby directive semantics | Prefer literal directive assertions for generated package-manager files |
| Duplicate archive binaries produced an error that still said the binary was missing | requesting-code-review | The missing-binary test was added locally, but the duplicate count branch was not separately asserted until Copilot reviewed it | Add separate zero-count and duplicate-count test rows for count invariants |
| `draft-assets` required `RATCHET_RELEASE_GUARD_VERSION` but did not validate metadata against it | requesting-code-review | Mode env-contract tests covered missing env, not env consumption | For each required env var, add one test proving changing the value changes behavior |
| `tap-postcheck` required names, commits, and version env vars but initially reused tap-preflight behavior | requesting-code-review | The task kept postcheck scaffolding in PR3 while full release workflow usage is in later tasks, so semantics were under-tested | Stubbed future modes should still consume every required input or not require it yet |

## Missed skill activations

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | unverified | Design artifacts exist, but activation log evidence is unavailable in this checkout |
| adversarial-design-review (design) | unverified | Multiple cycles recorded in design review report; activation log unavailable |
| writing-plans | unverified | Locked implementation plan exists; activation log unavailable |
| adversarial-design-review (plan) | unverified | Multiple cycles recorded in plan review report; activation log unavailable |
| alignment-check | unverified | Alignment artifact exists; activation log unavailable |
| scope-lock | yes | Lock checker passed before branch work and before PR creation |
| subagent-driven-development | unverified | Work was executed inline because subagent spawning lacked explicit authorization |
| finishing-a-development-branch | unverified | PR #76 was created from locked PR3 row; activation log unavailable |
| pr-monitoring | yes | CI and five Copilot review threads were monitored and resolved before admin merge |
| post-merge-retrospective | yes | This file |
| finishing Step 1e (doc-reconciliation) | yes | PR body recorded no README/RATCHET drift; workflow docs remain deferred to later locked tasks |

## What worked

- The releaseguard wrapper proved a fresh GoReleaser snapshot plus manifest-only validation, including packaged host `ratchet version` and `ratchet help`.
- The smoke-source allowlist kept releaseguard token constants out of the smoke runtime manifest without weakening the runtime smoke-file boundary.
- Copilot review found real contract gaps in generated cask validation and mode env consumption before merge.
- CI, local full tests, lint, race-focused releaseguard tests, and Windows compile all stayed green after the review fixes.

## What didn't

- The first implementation treated future `draft-assets` and `tap-postcheck` modes as env-gated aliases instead of making their required env values behaviorally meaningful.
- Archive binary count testing initially covered the missing case only; duplicate count behavior and error wording were not separately proven.
- The generated cask check used a broad substring assertion where package-manager output needed exact directive matching.

## Plugin-level follow-ups

- Add a review checklist item: if a mode requires an env var, at least one test must prove that env var is consumed, not just present.
- Add a release-artifact review item: generated package-manager files should be asserted with exact directives/fields, not substring presence.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| docs/design-guidance.md | not created | No repo-local guidance file exists; lessons are releaseguard review checklist refinements, not durable project architecture guidance |
