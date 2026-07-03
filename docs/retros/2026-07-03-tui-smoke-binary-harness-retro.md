# Retro: TUI Smoke Binary Harness

**PR:** #72 - test: add tui smoke binary harness
**Merged:** 2026-07-03
**Branch:** feat/tui-smoke-binary-harness
**Design:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Plan:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Related ADRs:** decisions/0001-smoke-package-list-boundary.md

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1-D5 initial hidden runtime path, validation overclaim, harness API, redaction, and matrix gaps | Important / Minor | Resolved upfront |
| design | D6-D14 docs boundary, temp workdir, client seam, daemon cleanup, release guard, socket, Windows, and redaction gaps | Important / Minor | Resolved upfront |
| design | D15-D24 socket hardening, Unix-only smoke tags, release-artifact, docs-negative, and Windows-boundary precision | Important / Minor | Resolved upfront for PR1 scope; later release/tap items remain planned in PR3-PR5 |
| plan | P1-P5 PTY test tag, tap prerequisite, stale artifact, Windows proof, and workflow syntax gaps | Critical / Important / Minor | Resolved upfront |
| plan | P6-P17 Windows packaged proof, external tap accounting, tagged daemon tests, smoke-client security, source manifest drift, tap env, and draft preflight gaps | Important / Minor | Resolved upfront |
| plan | P18-P24 command matrix, CI private-module setup, smoke-source guard, docs overclaim, Homebrew fallback, redaction scope, and Windows negative build boundaries | Important / Minor | Resolved upfront for PR1 scope; release/tap rows remain locked for later PRs |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| PTY helper initially inherited ambient `RATCHET_TUI_SMOKE_EMPTY_JOBS`, making normal jobs proof environment-sensitive | requesting-code-review | First adversarial pass caught redaction/exit/job-state gaps but not env inheritance until re-review | Add ambient-env checks to PTY harness review prompts |
| Ctrl+S sidebar toggle left jobs panel latently active | requesting-code-review | Shortcut symmetry was only reviewed after the status-bar hint expansion made the shortcut set explicit | Keep shortcut fixture checks paired with UI state exclusivity assertions |
| Trust lifecycle follow-up checks were initially substring/vacuous | requesting-code-review | Command matrix existed, but row parsing was not strict enough on action/scope/grant columns | Prefer structured parsing helpers over substring checks for rendered operational tables |
| Copilot found refresh-error and empty-state contradiction after PR creation | requesting-code-review | Local adversarial review focused on PTY and trust surfaces; component error-state copy was not checked for contradictory states | Add "error state must not render stale empty/success state" to component review checklist |

## Missed skill activations

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | unverified | Design artifacts exist, but activation log evidence is unavailable in this checkout |
| adversarial-design-review (design) | unverified | Multiple cycles recorded in design review report; activation log unavailable |
| writing-plans | unverified | Locked implementation plan exists; activation log unavailable |
| adversarial-design-review (plan) | unverified | Multiple cycles recorded in plan review report; activation log unavailable |
| alignment-check | unverified | Alignment artifact exists; activation log unavailable |
| scope-lock | unverified | Scope lock artifact exists and was re-verified; activation log unavailable |
| subagent-driven-development | unverified | Sequential task reviews were recorded in session/PR evidence; activation log unavailable |
| finishing-a-development-branch | unverified | PR #72 was created from locked PR1 row; activation log unavailable |
| pr-monitoring | unverified | CI/review threads were monitored and Copilot comments addressed; activation log unavailable |
| post-merge-retrospective | yes | This file |
| finishing Step 1e (doc-reconciliation) | yes | PR body recorded `Doc-reconciliation: clean` |

## What worked

- The locked scope prevented the TUI proof from drifting into runner changes or Windows ConPTY work.
- Copilot review caught a user-visible state contradiction that local test gates had not named directly.
- The PTY proof found real app wiring issues: chat loading completion, job-panel refresh on open, persistent trust store setup, and shortcut panel exclusivity.
- The final branch proved Windows honestly with compile-only coverage for TUI tests and explicit Unix-only PTY/smoke tags.

## What didn't

- The branch carried the full design-history stack into PR1 because the design branch had not been merged separately.
- The first Task 3 implementation needed multiple review loops around redaction, env hermeticity, and trust assertions.
- `gh pr merge --delete-branch` hit a local worktree limitation; merge succeeded without local branch cleanup, then the remote branch was deleted separately.

## Plugin-level follow-ups

- Add a PTY-harness review prompt item for ambient environment variables that affect smoke modes.
- Add a component UI review item: error states must not simultaneously render empty/success states unless explicitly labeled stale.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| docs/design-guidance.md | no change | Lessons are useful review checklist refinements, not durable project architecture guidance |
