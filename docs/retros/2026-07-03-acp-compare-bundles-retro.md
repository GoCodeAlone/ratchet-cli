# Retro: ACP Compare Bundles

**PR:** #68 - feat: persist acp compare bundles
**Merged:** 2026-07-03
**Branch:** feat/acp-compare-bundles
**Design:** docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow-design.md
**Plan:** docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow.md
**Related ADRs:** none

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1 required typed event propagation into compare/flow bundles. | Important | Prescient: PR2 code review found a compare bundle event-file semantics gap. |
| design | D2 required raw export to fail closed when history is unavailable. | Important | Resolved upfront in PR1; no PR2 fallout. |
| design | D3 required ACPX-shaped raw archive fixture proof. | Important | Resolved upfront in PR1; no PR2 fallout. |
| design | D5 warned replay bundles were sizable but justified. | Minor | Inconclusive until PR3. |
| plan | P1 required live sidecar persistence from command paths. | Important | Resolved upfront in PR1; no PR2 fallout. |
| plan | P2 required optional flow events via type assertion, not interface mutation. | Important | Resolved upfront; PR3 owns implementation. |
| plan | P3 moved docs guard changes to Task 9. | Important | Resolved upfront; PR2 did not touch docs guard. |
| plan | P4 fixed scope-lock helper working directory. | Important | Resolved upfront; closeout not reached yet. |
| plan | P5 standardized compare JSON wrapper shape. | Minor | Resolved upfront; wrapper shape shipped. |
| plan | P6 noted broad closeout task. | Minor | Inconclusive until PR4. |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| Initial CI lint failed on `bundle.CompareRun.Rows`; local lint was not run before the first push. | verification-before-completion | Focused tests and Windows builds ran, but linter proof was delayed until after CI. | For Go PRs, run `golangci-lint run` before initial PR push, not only after CI failure. |
| `CompareRunStore.Save` skipped rows with no events, so `--save` did not create one `events.ndjson` per compare row. | requesting-code-review | The first test covered one eventful row and did not assert the no-event row contract. | Include empty/no-event rows in bundle persistence tests whenever docs/PR text says "per-agent". |
| Generated compare run IDs used second precision and could collide. | adversarial-design-review (plan) | Plan required `--run-id` but did not attack default generated ID collision behavior. | Add collision checks when a plan introduces generated local artifact IDs. |
| Event validation/write errors lacked agent/path context. | requesting-code-review | Local tests covered success paths and unsafe path escaping but not diagnostic quality on invalid events. | Add failure-path assertions for multi-agent artifact writers. |

## Missed skill activations

Activation log unavailable at `.claude/autodev-state/in-progress.jsonl`; rows below are reconstructed from committed artifacts and PR evidence.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | unverified | Locked plan existed before this PR slice. |
| adversarial-design-review (design) | yes | `docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow-design-review.md`. |
| writing-plans | yes | `docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow.md`. |
| adversarial-design-review (plan) | yes | `docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow-plan-review.md`. |
| alignment-check | yes | `docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow-alignment.md`. |
| scope-lock | yes | `docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow.md.scope-lock`. |
| requesting-code-review | yes | Copilot reviewer produced three comments; all were addressed. |
| pr-monitoring | yes | CI failure and review threads were fixed, then PR was merged. |
| finishing Step 1e (doc-reconciliation) | unverified | Diff included the PR1 retro doc but no public docs/examples. |

## What worked

- The locked PR split kept compare persistence independent from flow replay and docs/release work.
- Copilot review found three concrete persistence-quality issues, all fixed before merge.
- PR monitoring caught the CI lint failure, reran local lint/tests/Windows builds, and waited for master CI after merge.

## What didn't

- The first PR push missed `golangci-lint run`, causing an avoidable CI round trip.
- The initial compare store test did not include a no-event row, so the per-agent file contract was under-tested.
- Generated artifact ID collision was not challenged until code review.

## Plugin-level follow-ups

No plugin-level change yet. The lint miss is already covered by `verification-before-completion`; watch future Go PR retros for a repeated "lint only after CI" pattern.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `docs/design-guidance.md` | no change | No durable cross-design lesson beyond this plan's local artifact testing discipline. |
