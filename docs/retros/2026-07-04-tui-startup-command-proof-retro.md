# Retro: TUI Startup Command Proof

**PR:** #74 - test: prove startup and command surfaces
**Merged:** 2026-07-04
**Branch:** feat/tui-startup-command-proof
**Design:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Plan:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Related ADRs:** decisions/0001-smoke-package-list-boundary.md

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1-D5 hidden shipped smoke surface, overclaimed launch path, unsafe harness extraction, real-state leakage, and integration-taxonomy gaps | Important / Minor | Resolved upfront |
| design | D6-D14 docs overclaim, temp workdir/socket containment, client seam, daemon cleanup, release artifact guard, helper placement, and redaction gaps | Important / Minor | Prescient for Task 4 daemon cleanup and redaction; remaining rows resolved upfront or remain locked for later PRs |
| design | D15-D24 release boundary, Unix-only smoke tags, Windows-boundary precision, docs-negative claims, and tap/release guard requirements | Important / Minor | Resolved upfront for PR2 scope; release/tap rows remain planned in later PRs |
| plan | P1-P8 PTY tag enforcement, tap prerequisite, stale artifact checks, Windows proof, workflow syntax, packaged Windows proof, and releaseguard taxonomy | Critical / Important / Minor | Resolved upfront; release/tap rows remain planned in later PRs |
| plan | P9-P17 tagged helper tests, smoke-client socket security, draft-asset interface, manifest drift, tap env, draft preflight, and negative-check robustness | Important / Minor | Resolved upfront for PR2 scope; release/tap rows remain planned in later PRs |
| plan | P18-P24 command matrix, Windows private-module setup, source manifest staging, scoped trust assertions, docs overclaim, Homebrew fallback, and redaction scope | Important / Minor | Prescient for Task 5 command-surface depth and Task 6 docs boundary wording; remaining rows resolved upfront or remain locked for later PRs |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| CI caught `TestAutocompleteSetFilter` still expecting `/mo` to match one command after `/mode` was added | verification-before-completion | Local verification covered the new autocomplete coverage test but not the whole component package under the same race/full-suite shape as CI | When adding command names, run `go test -race ./internal/tui/components` before PR creation |
| Copilot found `harnessredact.New` arguments out of order in startup smoke failure output | requesting-code-review | Review checked redaction coverage but did not compare each argument position against the marker mapping | Prefer a small named helper or table assertion for redactor marker order in smoke tests |
| Copilot found `capturePrintUsage` could leak pipe descriptors and fail to restore `os.Stdout` on mid-function failure | requesting-code-review | The helper was small enough to look harmless, so local review missed cleanup-on-failure behavior | Add fd/stdout restoration to the review checklist for output-capture test helpers |
| Copilot found `helpCommandRows` only captured top-level command tokens, missing subcommand coverage such as `/provider list` and `/trust grants` | requesting-code-review | The command-surface contract existed, but row extraction was too shallow for literal subcommands | Parse and assert lowercase literal subcommands whenever help rows include command families |

## Missed skill activations

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | unverified | Design artifacts exist, but activation log evidence is unavailable in this checkout |
| adversarial-design-review (design) | unverified | Multiple cycles recorded in design review report; activation log unavailable |
| writing-plans | unverified | Locked implementation plan exists; activation log unavailable |
| adversarial-design-review (plan) | unverified | Multiple cycles recorded in plan review report; activation log unavailable |
| alignment-check | unverified | Alignment artifact exists; activation log unavailable |
| scope-lock | unverified | Scope lock artifact exists and was re-verified; activation log unavailable |
| subagent-driven-development | unverified | Task work and local review were recorded in session/PR evidence; activation log unavailable |
| finishing-a-development-branch | unverified | PR #74 was created from locked PR2 row; activation log unavailable |
| pr-monitoring | unverified | CI failure and Copilot threads were monitored and addressed; activation log unavailable |
| post-merge-retrospective | yes | This file |
| finishing Step 1e (doc-reconciliation) | yes | PR body recorded `Doc-reconciliation: 1 item fixed` |

## What worked

- The release-shaped startup smoke found and fixed a real daemon lifecycle bug: `Shutdown` RPC needed to cancel the serving context so pid/socket cleanup could complete.
- Command-surface fixture coverage made CLI help, TUI help, autocomplete, focused shortcut tests, and PTY-proven command rows converge on one declared matrix.
- The docs guard forced README, RATCHET, and harness docs to distinguish release-shaped startup proof from build-tagged PTY proof, Windows package boundaries, and tap-safety limits.
- PR monitoring caught the stale autocomplete assumption quickly, and Copilot review strengthened redaction, output-capture cleanup, and subcommand coverage before merge.

## What didn't

- Local pre-PR verification did not run the full component package with the same race shape as CI, so a stale existing autocomplete fixture escaped.
- Local review missed two helper-level test hygiene bugs: pipe cleanup and positional redactor mapping.
- The first command-surface extractor was still too top-level for command families, despite the fixture existing specifically to prevent surface drift.

## Plugin-level follow-ups

- Add a recurring review prompt item for output-capture helpers: restore global streams with `defer` and close both pipe ends even on assertion failure.
- Add a command-surface review prompt item: when a help row includes literal lowercase subcommands, contract tests should assert those subcommands, not only the family command.
- No plugin-level implementation change is warranted from this single retro, but these two checklist items should be compared against the previous TUI smoke retro if they recur.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| docs/design-guidance.md | not created | No repo-local guidance file exists; this retro did not introduce a durable project architecture lesson that warrants creating one |
