# Retro: ACP Flow Replay Bundles

**PR:** #69 - feat: add acp flow replay bundles
**Merged:** 2026-07-03
**Branch:** feat/acp-flow-replay-bundles
**Design:** docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow-design.md
**Plan:** docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow.md
**Related ADRs:** none

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1 required typed event propagation into compare/flow bundles. | Important | Prescient: PR3 review found replay bundle self-consistency and event/path hardening gaps. |
| design | D5 warned replay bundles were sizable but justified. | Minor | Prescient: size increased review surface, but scope stayed within PR3. |
| plan | P2 required optional flow events via type assertion, not interface mutation. | Important | Resolved upfront; implementation used `interface{ LastEvents() []EventLogLine }`. |
| plan | P3 moved docs guard updates to Task 9. | Important | Resolved upfront; PR3 left docs guard unchanged. |
| plan | P6 noted closeout breadth. | Minor | Inconclusive until PR4. |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| Writable `trace.ndjson` close error was ignored. | requesting-code-review | Local tests covered success but not write-close failure propagation. | Keep writable-close handling on the review checklist for new file writers. |
| Failed ACP steps with nil output could reference a missing `steps/<node>.json`. | requesting-code-review | Initial failure-path fix covered action output but not nil ACP output. | For bundle manifests, test nil-output failure paths as well as nonzero action output. |
| Replay loader used lexical containment only and would follow symlinks outside the run dir. | adversarial-design-review (plan) | Plan required path containment but did not explicitly include symlink escape as an untrusted-bundle case. | Add symlink escape to future archive/replay loader test matrices. |
| Replay summaries stored absolute `manifest_path`, making bundles non-relocatable. | requesting-code-review | The docs/design wanted local artifacts but did not assert relocatability in tests. | For persisted bundles, assert metadata paths are relative unless explicitly external. |

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
| requesting-code-review | yes | Inline review found failed-step file gap; GitHub reviewers found five more issues. |
| pr-monitoring | yes | Review threads were fixed/resolved and CI was monitored through master. |
| finishing Step 1e (doc-reconciliation) | yes | PR body recorded doc reconciliation; public docs were deferred to PR4. |

## What worked

- The flow event capture used `LastEvents()` type assertion, preserving the existing runner interface.
- Binary smoke proved archive, compare, and flow replay through one built CLI path.
- Review monitoring caught security, portability, and diagnostic gaps before merge.

## What didn't

- Path containment tests initially missed symlink escape, which matters for untrusted replay bundles.
- The first failed-step hardening covered nonzero action output but not nil ACP failure output.
- Absolute paths leaked into replay projections until review pushed relocatable metadata.

## Plugin-level follow-ups

No plugin-level change yet. If symlink-escape misses recur, add "existing path symlink containment" to the adversarial review path-safety checklist.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `docs/design-guidance.md` | no change | Lessons are local to replay/archive bundle validation and are captured in this plan. |
