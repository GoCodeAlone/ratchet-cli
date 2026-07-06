# Retro: Ratchet CLI Harness Readiness

**PR:** #118 - feat: add harness readiness utilities
**Merged:** 2026-07-06
**Branch:** feat/harness-readiness
**Design:** docs/plans/2026-07-06-ratchet-cli-harness-readiness-design.md
**Plan:** docs/plans/2026-07-06-ratchet-cli-harness-readiness.md
**Related ADRs:** none

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1: all-profile verification could be mistaken for credentialed third-party CI. | Minor | Resolved upfront; docs and command wording kept the feature credential-free and local. |
| design | D2: retro bundles can still contain summarized local context. | Minor | Resolved upfront; raw evidence is not copied and docs mark bundles as sensitive local handoffs. |
| design | D3: a new harness readiness command group would be broader. | Minor | Resolved upfront; the implementation extended existing command owners. |
| plan | P1: docs guard updates depend on final command wording. | Minor | Resolved upfront; docs landed after command implementation and docs guard passed. |
| plan | P2: Windows proof is a cross-build, not native runtime launch. | Minor | Resolved upfront; the plan kept this slice to portable parsing/file output and relied on prior hosted Windows command startup proof. |
| plan | P3: `--all` must not stop at the first failing profile if CI needs a matrix summary. | Minor | Prescient; Copilot caught the related missing non-zero exit behavior, and d04458f added summary-plus-error semantics. |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| `profiles verify --all` emitted per-profile error rows but exited zero. | adversarial-design-review (plan) | P3 required per-profile summaries but did not explicitly require non-zero exit on trusted-profile failures. | For CLI readiness/check commands, plan reviews should ask whether machine-readable failure rows also affect process exit status. |
| `len([]byte(result.Text))` allocated while counting response bytes. | requesting-code-review | Local tests covered value correctness, not allocation hygiene. | No plugin change; this is ordinary review-level Go cleanup. |
| Design doc said JSON output was an array while implementation emitted `{results:[...]}`. | doc-reconciliation | The design artifact was not re-read after implementation chose the object wrapper. | For command JSON contracts, reconcile design docs against the final test payload shape before PR creation. |

## Missed skill activations

Pipeline gates expected to fire per `using-autodev`; activation log was unavailable because `.claude/autodev-state/in-progress.jsonl` was absent in the canonical repo root.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | unverified | Skill was used in-session, but no activation log was present. |
| adversarial-design-review (design) | yes | Committed design-review artifact exists. |
| writing-plans | yes | Committed plan artifact exists. |
| adversarial-design-review (plan) | yes | Committed plan-review artifact exists. |
| alignment-check | yes | Alignment section is included in the plan-review artifact. |
| test-driven-development | yes | Failing tests were added before each command implementation. |
| finishing Step 1e (doc-reconciliation) | yes | README, harness, policy, and retro docs changed in the PR. |
| pr-monitoring | yes | CI and Copilot review were monitored; comments were addressed and threads resolved before merge. |
| post-merge-retrospective | yes | This retro. |

## What worked

- Focused package tests, full `go test ./...`, binary launch, and Windows cross-build caught no regressions before PR creation.
- PR monitoring caught and closed Copilot review feedback before merge.
- Scope lock kept the slice additive and out of deferred background scheduling, managed hooks, SDK execution, and credentialed provider CI.
- Release v0.30.14 published all expected Linux, macOS, Windows, and checksum assets.

## What didn't

- The plan review anticipated partial-failure handling but did not explicitly call out process exit behavior for CI readiness commands.
- The design doc JSON shape drifted from implementation until review.
- Activation-log evidence was unavailable, so skill activation scoring had to fall back to committed artifacts and transcript-backed evidence.

## Plugin-level follow-ups

No plugin-level change yet. The exit-status miss is worth watching in future retros, but one occurrence does not justify a new adversarial-review rule by itself.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| docs/design-guidance.md | no change | No durable cross-design lesson beyond existing local-only, credential-free, and explicit-operator guidance. |
