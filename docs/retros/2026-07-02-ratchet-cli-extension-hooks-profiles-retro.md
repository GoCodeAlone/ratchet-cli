# Retro: Extension Hooks And ACP Launch Profiles

**PRs:** #63 `feat: add hook trust controls`, #64 `feat: add ACP launch profiles`, #65 `docs: document hooks profiles release state`
**Merged:** 2026-07-02
**Branches:** `feat/ratchet-cli-hook-trust-controls`, `feat/ratchet-cli-acp-launch-profiles`, `docs/ratchet-cli-hooks-profiles-release`
**Design:** `docs/plans/2026-07-02-ratchet-cli-extension-hooks-profiles-design.md`
**Plan:** `docs/plans/2026-07-02-ratchet-cli-extension-hooks-profiles.md`
**Release:** `v0.24.0` via Release run `28622077111`
**Related ADRs:** none

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1: user hooks remain active by compatibility exception. | Minor | Resolved upfront: README, harness docs, and policy matrix state the exception. |
| design | D2: `--agent` profile resolution could hide built-in precedence. | Minor | Resolved upfront: profile store rejects built-in shadowing and docs state built-ins win. |
| design | D3: reserved `managed` hook source could imply enterprise policy. | Minor | Resolved upfront: docs say managed hooks remain deferred and no managed config was added. |
| plan | P1: trusted hook execution errors needed coverage distinct from untrusted skip behavior. | Minor | Prescient: Copilot caught discarded hook errors in cron/token-limit paths; fixed before merge. |
| plan | P2: ACP profile template and execution tasks shared CLI files and needed strict order. | Minor | Resolved upfront: PR2 kept Task 5 before Task 6 and merged cleanly. |
| plan | P3: `v0.24.0` tag conflict needed stop-and-reconcile behavior. | Minor | Resolved upfront: remote tag absence was verified before tagging. |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| Hook descriptor hash initially omitted event, allowing one trusted event descriptor to imply trust for another. | test-driven-development / requesting-code-review | The design named event as part of identity, but the first regression set did not assert event-specific hash separation. | Hash/trust work should include one test per identity component named in the design. |
| Windows drive-relative plugin capability paths were not rejected. | adversarial-design-review (plan) / test-driven-development | Path escape coverage included absolute and parent traversal, but not Windows volume-relative syntax. | Path-containment plans should include OS-specific forms: absolute, parent, drive-relative, UNC where applicable. |
| Hook execution errors were discarded in cron and token-limit paths. | executing-plans | Plan review P1 predicted the runtime error-policy class, but implementation did not audit every hook call site before PR review. | When a review finding is accepted, add a call-site search item to the task checklist. |
| ACP profile template loading ignored `os.UserHomeDir` errors. | requesting-code-review | Normal path tests used temp homes but did not force home-resolution failure. | Tests for default user-state discovery should force provider errors, not only happy-path temp homes. |
| ACP profile `Trust` and `Remove` did not trim names. | test-driven-development | Add/list validation trimmed names, but mutation operations did not share a name-normalization invariant. | Store APIs that take the same identifier should share or test the same normalization behavior. |

## Missed skill activations

Activation log was unavailable in the canonical repo root. Committed artifacts show design review, plan review, alignment, scope lock, PR monitoring, and this retrospective were produced.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | unavailable | No activation log present. |
| adversarial-design-review (design) | yes | `docs/plans/2026-07-02-ratchet-cli-extension-hooks-profiles-design-review.md`. |
| writing-plans | yes | `docs/plans/2026-07-02-ratchet-cli-extension-hooks-profiles.md`. |
| adversarial-design-review (plan) | yes | `docs/plans/2026-07-02-ratchet-cli-extension-hooks-profiles-plan-review.md`. |
| alignment-check | yes | `docs/plans/2026-07-02-ratchet-cli-extension-hooks-profiles-alignment.md`. |
| scope-lock | yes | Scope lock was present during execution and completed after `v0.24.0`. |
| subagent-driven-development | no | Implementation ran inline against the locked manifest. |
| pr-monitoring | yes | PRs #63, #64, and #65 were monitored to green and merged. |
| post-merge-retrospective | yes | This retro. |
| finishing Step 1e | unverified | Docs changed in PR1 and PR3; PR bodies had verification evidence but no dedicated `Doc-reconciliation:` marker. |

## What worked

- The locked 4-PR split kept hook trust, ACP profiles, docs, and release closeout independently reviewable.
- Copilot review found concrete security and reliability misses before merge; every thread was resolved with regression coverage or targeted fixes.
- The release gate caught the full cross-platform path before tagging: full tests, vet, lint, Windows amd64/arm64 builds, GoReleaser check, release assets, and Homebrew cask.

## What didn't

- Accepted plan-review finding P1 did not fully translate into a call-site audit; code review had to catch two discarded hook errors.
- Path-containment testing initially missed Windows drive-relative syntax, a known cross-platform edge case for Go path handling.
- Profile store identifier normalization was not treated as a store-wide invariant until review.

## Plugin-level follow-ups

No plugin-level changes are warranted from this single retro. Future adversarial-review prompts should watch for repeated misses around stable identity hashes, OS-specific path containment, and shared identifier normalization.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `docs/design-guidance.md` | no change | No repo-local durable guidance file exists, and the lessons are implementation-review patterns rather than new ratchet-cli product or architecture constraints. |
