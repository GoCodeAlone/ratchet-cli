# Retro: ratchet-cli Auto-Drain

**PR:** #59 - feat: add explicit ACP client watch drain
**Merged:** 2026-07-02
**Branch:** `feat/ratchet-cli-auto-drain-policy`
**Design:** `docs/plans/2026-07-02-ratchet-cli-auto-drain-design.md`
**Plan:** `docs/plans/2026-07-02-ratchet-cli-auto-drain.md`
**Related ADRs:** none

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1: foreground watch could drift from the user's broader background-drain intent. | Minor | Resolved upfront: the design and docs keep daemon scheduling deferred and make watch explicit foreground policy. |
| design | D2: watch output could leak prompt text. | Minor | Resolved upfront: CLI/unit tests assert aggregate-only output, and docs keep queue contents sensitive local policy metadata. |
| design | D3: SIGINT/SIGTERM behavior differs on Windows. | Minor | Prescient: Copilot caught missing SIGTERM handling in the top-level watch command before merge. |
| plan | P1: a single PR could invite daemon scheduler or extension-hook scope creep. | Minor | Resolved upfront: implementation stayed inside the locked watch/drain scope. |
| plan | P2: binary smoke is slower but needed for CLI/process proof. | Minor | Resolved upfront: focused tests and binary smoke both ran locally and in CI. |
| plan | P3: Windows proof is compile-only locally. | Minor | Resolved upfront: local Windows amd64/arm64 builds and CI Windows Build passed. |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| `watch --stop-when-empty` initially treated a queue with only stale `running` items as empty, so `DrainQueue` recovery would not run. | adversarial-design-review (plan) | The design named stale running-item recovery as an existing `DrainQueue` behavior, but the plan's watch-loop tests only required pending-item polling. | Future queue-worker plans should test every state that delegates to recovery logic, not just the nominal ready state. |
| `ratchet acp client watch` initially listened for `os.Interrupt` but not SIGTERM on Unix. | test-driven-development | The plan called out platform signal differences, but command tests drove cancellation through context and did not assert the top-level signal set. | For long-running CLI commands, add a small platform-specific signal-list test when the implementation owns signal registration. |

## Missed skill activations

Activation log unavailable at the canonical repo root, so skill firing cannot be reconstructed from `.claude/autodev-state/in-progress.jsonl`.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | unavailable | Design was already present when this closeout evidence was reconstructed. |
| adversarial-design-review (design) | yes | `docs/plans/2026-07-02-ratchet-cli-auto-drain-design-review.md`. |
| writing-plans | yes | `docs/plans/2026-07-02-ratchet-cli-auto-drain.md`. |
| adversarial-design-review (plan) | yes | `docs/plans/2026-07-02-ratchet-cli-auto-drain-plan-review.md`. |
| alignment-check | yes | `docs/plans/2026-07-02-ratchet-cli-auto-drain-alignment.md`. |
| scope-lock | yes | Manifest locked before implementation and completed after merge. |
| subagent-driven-development | no | Implementation ran inline against the locked manifest. |
| finishing Step 1e (doc-reconciliation) | yes | PR body included `Doc-reconciliation: clean`; docs and docs-guard tests were updated. |
| pr-monitoring | yes | PR #59 was monitored through CI, Copilot review, fixes, admin merge, and green master CI. |
| post-merge-retrospective | yes | This retro. |

## What worked

- The scope lock kept daemon scheduling, launch profiles, ACPX compatibility, and extension hooks out of this PR.
- Binary smoke covered the real CLI-to-fixture-agent boundary instead of only parser/unit paths.
- Copilot review found two real long-running-worker edge cases before merge; both were fixed with regression tests.
- Master CI and CodeQL were verified green for the squash merge commit before closeout.

## What didn't

- The original watch-loop tests did not cover stale `running` records, even though the delegated drain path supports recovery.
- The command-level tests did not assert Unix SIGTERM registration for a long-running foreground worker.
- Skill activation evidence was unavailable from repo-local autodev logs, so the retro relies on committed artifacts and PR evidence.

## Plugin-level follow-ups

No plugin-level change is warranted from one feature, but future autonomous plans for long-running CLI workers should explicitly include stale-state recovery tests and platform signal registration tests.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `docs/design-guidance.md` | no change | The lessons are feature-local queue-worker checks, not durable product or architecture constraints for all ratchet-cli designs. |
