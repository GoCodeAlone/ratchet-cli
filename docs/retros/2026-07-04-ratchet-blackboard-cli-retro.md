# Retro: Ratchet Blackboard CLI

**PR:** https://github.com/GoCodeAlone/ratchet-cli/pull/89
**Merged:** 2026-07-04 at merge commit `0dc5e64e6e255ba60595c43f3883215137263d4a`
**Branch:** feat/blackboard-cli
**Design:** docs/plans/2026-07-04-ratchet-blackboard-notify-design.md
**Plan:** docs/plans/2026-07-04-ratchet-blackboard-notify.md
**Related ADRs:** none

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1 warned daemon-memory volatility could surprise users. | Minor | Resolved upfront: README and harness docs say daemon-scoped volatile state. |
| design | D2 challenged whether MCP-only docs were enough. | Minor | Rejected: terminal/script use needs first-class CLI. |
| design | D3 flagged sensitive value echo. | Minor | Resolved upfront: docs call values sensitive local coordination data. |
| plan | P1 required an exact smoke-test file path. | Important | Resolved before implementation: `cmd/ratchet/blackboard_harness_test.go`. |
| plan | P2 required exact repo test names. | Important | Resolved before implementation with concrete docs/help test command. |
| plan | P3 constrained JSON schema breadth. | Minor | Resolved: JSON emits existing protobuf-shaped responses only. |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| Task 2 smoke did not produce a red failure because Task 1 already supplied the command behavior. | test-driven-development | Task 2 validates integration over behavior already implemented by Task 1. | For dependent smoke tasks, mark expected red as "fails before prior task; may pass after prior implementation" or write the smoke before Task 1 when practical. |
| Initial PR lint failed on capitalized CLI usage errors. | local pre-PR verification | `go vet` passed, but `golangci-lint --new-from-rev=origin/master` was not run until after CI surfaced staticcheck ST1005. | Include the PR lint command in the completion checklist for CLI text changes, not only `go vet`. |
| Retro was drafted before the PR merged, so it had pending merge metadata. | post-merge-retrospective timing | The retro file was committed in the feature PR as a closeout artifact before the post-merge gate could be known. | Keep retro drafts out of feature PRs or add an explicit finalization PR after post-merge CI is green. |

## Missed skill activations

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | yes | Design written from user-preapproved direction after portfolio/context review. |
| adversarial-design-review (design) | yes | `docs/plans/2026-07-04-ratchet-blackboard-notify-design-review.md`. |
| writing-plans | yes | `docs/plans/2026-07-04-ratchet-blackboard-notify.md`. |
| adversarial-design-review (plan) | yes | `docs/plans/2026-07-04-ratchet-blackboard-notify-plan-review.md`. |
| alignment-check | yes | `docs/plans/2026-07-04-ratchet-blackboard-notify-alignment.md`. |
| scope-lock | yes | `docs/plans/2026-07-04-ratchet-blackboard-notify.md.scope-lock`. |
| subagent-driven-development | partial | Native subagent tool exists but requires explicit delegation permission; implementation ran inline with lock checkpoints. |

## What worked

- Existing daemon blackboard and gRPC methods made the same-device coordination slice small.
- Built CLI smoke proved separate process invocations share the daemon state.
- Keeping Notify out of ratchet-cli avoided Slack/Discord dependency sprawl and matched the plugin ecosystem direction.
- Post-merge `master` CI and CodeQL both completed successfully for `0dc5e64e6e255ba60595c43f3883215137263d4a`.

## What didn't

- The existing standalone `ratchet mcp blackboard` still uses an in-process blackboard; daemon-backed sharing requires `ratchet mcp daemon` or the new CLI.
- The blackboard remains volatile and memory-backed. That is acceptable for handoffs/status, but not a durable coordination log.

## Plugin-level follow-ups

- Design `workflow-plugin-notify` around `github.com/nikoksr/notify` for outbound notification fanout. Requirements: stubbed external transports in CI, explicit non-critical-delivery docs, anti-spam/rate controls, secret-backed credentials, registry manifest, GoReleaser workflow, and a later ratchet bridge that can emit selected blackboard/team events through Workflow rather than direct service adapters.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `docs/design-guidance.md` | none | Existing reuse/plugin guidance already covered the decision to defer Notify into a Workflow plugin. |
