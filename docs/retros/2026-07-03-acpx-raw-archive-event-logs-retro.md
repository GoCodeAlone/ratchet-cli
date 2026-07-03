# Retro: ACPX Raw Archive Event Logs

**PR:** #67 — feat: add acpx raw archive event logs
**Merged:** 2026-07-03
**Branch:** feat/acpx-raw-archive-events
**Design:** docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow-design.md
**Plan:** docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow.md
**Related ADRs:** none

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1: compare/flow bundles needed typed event propagation from ACP results. | Important | Resolved upfront |
| design | D2: raw export must fail when raw sidecar history is unavailable. | Important | Resolved upfront |
| design | D3: upstream-shaped ACPX archive fixture was required for raw `history` proof. | Important | Resolved upfront |
| design | D4/D5: flow replay bundle breadth may be larger than PR1/PR2 need. | Minor | False positive |
| plan | P1: live exec/drain/watch paths needed to persist `Result.Events` before raw export. | Important | Prescient |
| plan | P2: Go optional interface wording would not compile. | Important | Resolved upfront |
| plan | P3: docs guard should not move ahead of docs task. | Important | Resolved upfront |
| plan | P4: closeout helper must run from the ratchet worktree. | Important | Resolved upfront |
| plan | P5: compare saved JSON shape needed standardizing. | Minor | Resolved upfront |
| plan | P6: closeout task mixes release, retro, and workspace state. | Minor | Inconclusive |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| JSON-RPC `error.data` was constrained to object-like JSON, but JSON-RPC allows any JSON value. | requesting-code-review | The custom validator was reviewed for request/response shape, not every optional JSON-RPC field cardinality. | Add optional-field permissiveness to JSON-RPC validator review checklist. |
| Writable file handles in event-log append/copy ignored close-time errors. | requesting-code-review | Local tests cover successful writes and permissions, not close-time failure handling. | Keep code-quality bot in the PR gate; add writable-close handling to file helper patterns. |
| `sessions export` usage text omitted the new `--history` flag. | requesting-code-review | Parser tests checked failure, not the exact usage message. | For new CLI flags, add one assertion against usage text when the command has custom usage strings. |

No CI failures slipped past local verification. PR checks and master checks were green after review fixes.

## Missed skill activations

Activation log unavailable: `.claude/autodev-state/in-progress.jsonl` was not present in the canonical repo root or nearby worktrees, so fired/not-fired status cannot be proven from log evidence.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | unverified | Design artifacts exist, but activation log evidence is unavailable. |
| adversarial-design-review (design) | yes | Committed design-review report exists and shaped the final design. |
| writing-plans | unverified | Plan artifact exists, but activation log evidence is unavailable. |
| adversarial-design-review (plan) | yes | Committed plan-review report exists and shaped the final plan. |
| alignment-check | yes | Committed alignment report exists. |
| scope-lock | yes | Scope-lock sidecar and `plan-scope-check --verify-lock` passed. |
| requesting-code-review | partial | Inline adversarial review caught one ACPX raw-history parser edge case before PR; bot review caught implementation details after PR. |
| pr-monitoring | yes | CI and review comments were monitored, fixed, resolved, and merged. |
| post-merge-retrospective | yes | This file. |

## What worked

- The design and plan review gates found the main architectural risks before implementation: raw export fail-closed behavior, typed event propagation, and real ACPX fixture proof.
- Binary smoke coverage caught the command-level path from fixture ACP execution to raw archive export, not just package-level APIs.
- PR monitoring caught and resolved review feedback before admin merge instead of merging over bot comments.

## What didn't

- The JSON-RPC validator was too strict for optional `error.data` despite covering core request/notification/response shapes.
- File-close error handling was left to generic code-quality review instead of being caught in local implementation review.
- The local git push guard misidentified the feature worktree as `main`, forcing a GitHub git-data API update for the review-fix commit.

## Plugin-level follow-ups

No plugin-level change yet. The JSON-RPC optional-field and writable-close misses should be watched across future retros before changing global gates.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| docs/design-guidance.md | no change | No durable cross-design lesson; misses were implementation-level and already covered by code review. |
