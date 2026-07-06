# Retro: Ratchet CLI TUI Session Usability

**PRs:** #125 - test: isolate TUI sidebar smoke shortcut; #126 - fix: improve TUI session usability
**Merged:** 2026-07-06
**Branches:** fix/tui-smoke-sidebar-isolation; feat/tui-usability-flow
**Design:** none; follow-up fixes from released TUI verification and usability feedback
**Plan:** none; operator-approved follow-up cluster
**Related ADRs:** none

## Adversarial-review findings, scored

These PRs were not created from a locked design/plan pair, so there are no committed adversarial-review findings to score.

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| n/a | No design-review or plan-review artifact existed for this follow-up cluster. | n/a | n/a |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| Master CI TUI Smoke still failed after the first flow-control stabilization patch at `f060831`. | runtime-launch-validation | The original local proof did not isolate the `Ctrl+S` sidebar shortcut from the broader PTY shortcut loop, so terminal state contamination remained possible. | Keep stateful PTY shortcuts in fresh-PTY tests when the shortcut can alter terminal/session state. |
| Copilot found the status-bar candidate text could render as `Ctrl+S Ctrl+C quit`, making the recovery hint ambiguous. | requesting-code-review | Local tests asserted key visibility but not the exact human-readable phrase around adjacent shortcut labels. | For compact TUI hints, assert representative rendered phrases, not only token presence. |
| Copilot found `/model setup` reported `/model add` in output. | requesting-code-review | The command alias shared behavior but the test only covered the new alias path. | Alias tests should verify user-facing wording for each accepted synonym. |
| Copilot found `/sessions` guidance used `<message_id>` while the CLI equivalent uses `<message-id>`. | doc-reconciliation | The help text was expanded manually and drifted from the CLI placeholder spelling. | Prefer copying command usage strings or testing help snippets against known CLI syntax. |
| Copilot found `View()` mutating a copied chat model with `SetSize`, which could make sidebar relayout unreliable. | requesting-code-review | Local tests covered visible output, not Bubble Tea model update semantics. | When changing TUI layout, test the update-time state transition and avoid mutation inside render-only paths. |

## Missed skill activations

Pipeline gates expected to fire per `using-autodev`; activation log was unavailable because `.claude/autodev-state/in-progress.jsonl` was absent in the canonical repo root.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | unverified | Follow-up cluster was directed by operator feedback, but no activation log was present. |
| adversarial-design-review (design) | n/a | No design artifact was created for these small follow-up PRs. |
| writing-plans | n/a | No locked implementation plan was created for these small follow-up PRs. |
| adversarial-design-review (plan) | n/a | No plan-review artifact existed. |
| alignment-check | n/a | No design/plan pair existed to align. |
| test-driven-development | yes | Failing focused tests were added for sidebar shortcut isolation, shortcut visibility, sidebar affordances, model setup guidance, sessions guidance, and sidebar relayout. |
| finishing Step 1e (doc-reconciliation) | n/a | Diffs changed tests and TUI help text, not public docs/examples. |
| pr-monitoring | yes | PRs #125 and #126 were monitored through green checks; #126 Copilot threads were addressed and resolved before merge. |
| post-merge-retrospective | yes | This retro. |

## What worked

- Isolating `Ctrl+S` in its own fresh PTY test fixed the main-branch TUI Smoke failure without weakening release-shaped TUI launch coverage.
- User-reported TUI pain points translated cleanly into focused tests for visible recovery shortcuts, sidebar contrast markers, session action guidance, and provider setup entry points.
- Copilot review caught four small but real UX/architecture issues in #126 before merge.
- Releases v0.30.20 and v0.30.21 both published the expected Linux, macOS, Windows, and checksum assets after green master CI.

## What didn't

- The first stabilization attempt treated PTY flow control as the whole failure class, but the smoke loop also needed state isolation for the sidebar shortcut.
- Some TUI assertions were token-level rather than phrase-level, so ambiguous compact help wording escaped local tests.
- The sidebar relayout change initially lived in `View()`, which is the wrong place for persistent Bubble Tea model state updates.

## Plugin-level follow-ups

No plugin-level change yet. The recurring local lesson is narrower: TUI smoke plans should classify shortcuts by whether they mutate terminal/session state and give those shortcuts isolated PTY coverage.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| docs/design-guidance.md | not created | This produced useful TUI test guidance but no durable cross-design product or architecture constraint. |

## Verification evidence

- PR #125 merge commit `b0ca84c9147ee9289bd40fd3b176b93eda51b72f`; master CI, Code Quality, and CodeQL push checks succeeded.
- Release `v0.30.20` succeeded with checksum plus Linux, macOS, and Windows assets.
- PR #126 merge commit `be02649cd9718b4a91f063530a5db89fc52f15d9`; master CI, Code Quality, and CodeQL push checks succeeded.
- Release `v0.30.21` succeeded with checksum plus Linux, macOS, and Windows assets.
- Closeout baseline: `go test ./cmd/ratchet ./internal/tui/components ./internal/tui/commands ./internal/tui -run 'StatusBarHints|Sidebar|Model|Sessions|TUIBinarySmoke|StartupSmoke|HarnessSmoke' -count=1 -timeout=10m`.
