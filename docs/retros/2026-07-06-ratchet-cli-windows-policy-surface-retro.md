# Retro: Ratchet CLI Windows Policy Surface

**PR:** #115 — feat: expose policy matrix and smoke Windows command startup
**Merged:** 2026-07-06
**Branch:** feat/windows-policy-surface
**Design:** docs/plans/2026-07-06-ratchet-cli-windows-policy-surface-design.md
**Plan:** docs/plans/2026-07-06-ratchet-cli-windows-policy-surface.md
**Related ADRs:** none

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1: CLI policy table can drift from the Markdown source of truth. | Minor | Resolved upfront: docs and command text call the CLI a read-only view, and tests assert required source/status terms. |
| design | D2: Windows startup proof can overclaim full Windows runtime parity. | Minor | Resolved upfront: docs guard now forbids the old broad runtime claim and requires the narrower command-startup boundary. |
| design | D3: docs-only alternative might be enough. | Minor | False positive: local operator discoverability justified the CLI surface and no enforcement logic was added. |
| plan | P1: local validation cannot execute the Windows binary. | Minor | Prescient: PR CI supplied the hosted Windows proof; `Windows Release Smoke` passed before merge. |
| plan | P2: docs guard updates depend on command wording. | Minor | Prescient: Copilot found a help-wording mismatch; the single-PR grouping kept the fix cheap. |
| plan | P3: CI rollback must not remove existing checks. | Minor | Resolved upfront: rollback notes and workflow tests touched only the new Windows command-startup job. |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| `ratchet policy matrix -h/--help` returned `flag.ErrHelp` with discarded output. | test-driven-development | Initial tests covered text, JSON, and unknown args but not nested help. | For new CLI subcommands, include `-h`/`--help` in the first test batch. |
| Top-level `policy` help under-described partial and explicit-operator statuses. | adversarial-design-review (plan) | The plan required help wiring but did not require status vocabulary parity in top-level help. | Keep command help expected strings aligned with the status vocabulary any new command exposes. |

## Missed skill activations

Activation log unavailable at the canonical repo root, so this is scored from committed artifacts and transcript evidence.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | yes | Design alternatives and assumptions were recorded in the design doc. |
| adversarial-design-review (design) | yes | Committed design-review report exists. |
| writing-plans | yes | Committed implementation plan with Scope Manifest exists. |
| adversarial-design-review (plan) | yes | Committed plan-review report exists. |
| alignment-check | yes | Plan review includes the alignment report; manifest was scope-locked. |
| subagent-driven-development | no | Work was executed inline because this was a single-repo, single-PR slice. |
| finishing Step 1e (doc-reconciliation) | yes | PR body and docs guard changes covered README/RATCHET/harness/parity/policy docs. |
| pr-monitoring | yes | Copilot comments were addressed, threads resolved, and all checks were green before merge. |
| post-merge-retrospective | yes | This retro. |

## What worked

- Releaseguard tests caught the exact workflow shape for the new Windows startup job before CI ran it.
- Hosted `Windows Release Smoke` passed and proved native `ratchet.exe --version` plus `help` on `windows-2025`.
- Docs guards prevented the old overbroad Windows runtime wording from surviving.

## What didn't

- The initial CLI tests missed the standard nested help path.
- The first top-level help wording was technically true but incomplete relative to the command's status vocabulary.

## Plugin-level follow-ups

No plugin-level change yet. If another ratchet-cli PR misses nested command help behavior, add a reusable CLI-help checklist item to the planning or code-review gate.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| workspace `docs/design-guidance.md` | no change | This was a one-off CLI test coverage miss, not a durable cross-project constraint. |
