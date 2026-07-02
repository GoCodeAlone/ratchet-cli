# Retro: ACPX Flow Hardening

**PR:** #61 - feat: harden acp client flows
**Merged:** 2026-07-02
**Branch:** `feat/acpx-flow-hardening`
**Design:** `docs/plans/2026-07-02-acpx-flow-hardening-design.md`
**Plan:** `docs/plans/2026-07-02-acpx-flow-hardening.md`
**Related ADRs:** none

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1: shell permission must be implicit for action nodes. | Minor | Resolved upfront: implementation preflights implicit `shell` before any node runs. |
| design | D2: environment inheritance can expose local secrets to action commands. | Minor | Resolved upfront: design/docs mark action output as sensitive local metadata; no secret expansion was added. |
| design | D3: dynamic `node.input` expansion would broaden scope. | Minor | Resolved upfront: action input remains static JSON. |
| plan | P1: shell-specific smoke commands would weaken Windows evidence. | Minor | Resolved upfront: binary smoke uses built binaries/direct command+args. |
| plan | P2: fake-runner tests alone would miss real CLI/process behavior. | Minor | Resolved upfront: binary smoke proves built CLI + local process action + fixture ACP agent. |
| plan | P3: docs must not imply cwd containment is a sandbox. | Minor | Resolved upfront: docs label sandbox/path/network expansion deferred. |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| Cwd containment initially used cleaned absolute paths only, so a symlink under the base dir could point outside without `outside-cwd`. | adversarial-design-review (plan) | The plan required cwd escape coverage but did not name symlink escape, even though `callbacks.go` already uses real-path handling. | Future path-containment plans should include symlink escape tests when the repo has real-path precedent. |
| Non-`exec.ExitError` action start failures persisted `exit_code:0` alongside an error. | test-driven-development | Tests covered non-zero exits from a runner, but not command start failures from the default runner. | For process runners, test both process exit and process start failure. |
| Design text said platform shell selection while implementation used direct command+args. | doc-reconciliation | The implementation intentionally avoided shell syntax, but the design guidance row kept stale wording. | During doc reconciliation, grep design docs for rejected implementation terminology such as `sh -c`, `cmd /C`, and `shell mode`. |

## Missed skill activations

Activation log unavailable at the canonical repo root, so skill firing cannot be reconstructed from `.claude/autodev-state/in-progress.jsonl`.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | yes | `docs/plans/2026-07-02-acpx-flow-hardening-design.md`. |
| adversarial-design-review (design) | yes | `docs/plans/2026-07-02-acpx-flow-hardening-design-review.md`. |
| writing-plans | yes | `docs/plans/2026-07-02-acpx-flow-hardening.md`. |
| adversarial-design-review (plan) | yes | `docs/plans/2026-07-02-acpx-flow-hardening-plan-review.md`. |
| alignment-check | yes | `docs/plans/2026-07-02-acpx-flow-hardening-alignment.md`. |
| scope-lock | yes | Manifest locked before implementation and completed after merge. |
| subagent-driven-development | no | Tool policy disallowed subagents without explicit user delegation; implementation ran inline against the locked manifest. |
| finishing Step 1e (doc-reconciliation) | yes | PR body included `Doc-reconciliation: clean`; Copilot still found one stale design wording issue. |
| pr-monitoring | yes | PR #61 was monitored through CI, Copilot review, fix push, admin merge, and green master CI. |
| post-merge-retrospective | yes | This retro. |

## What worked

- The locked manifest kept TypeScript ACPX runtime, replay UI, extension hooks, and sandbox expansion out of the PR.
- Binary smoke exercised the real built CLI, a local action process, persisted flow bundle files, and the fixture ACP agent.
- Copilot review caught three concrete issues before merge; each got a regression test or design correction.
- Windows cross-builds and CI Windows Build stayed green after the review fixes.

## What didn't

- The original path-containment tests missed symlink escape despite existing real-path precedent in `internal/acpclient/callbacks.go`.
- The default process runner did not have a start-failure test until review.
- Doc reconciliation did not catch stale shell-mode wording in the design before PR review.

## Plugin-level follow-ups

No plugin-level change is warranted from this single retro, but if another path-containment miss appears, add a standard adversarial-review bug class for symlink/realpath escape.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `docs/design-guidance.md` | no change | The lessons are feature-local process/path test cases, not durable project-wide architecture constraints. |
