# Retro: Ratchet Workflow Export And Profile CI

**PRs:** #94 `feat: add Workflow messaging blackboard export`, #96 `feat: add ACP profile verification`
**Merged:** 2026-07-05
**Branches:** `feat/ratchet-blackboard-workflow-export`, `feat/ratchet-profile-verify-ci-master`
**Design:** `docs/plans/2026-07-05-ratchet-workflow-profile-ci-design.md`
**Plan:** `docs/plans/2026-07-05-ratchet-workflow-profile-ci.md`
**Related ADRs:** none

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1: full Workflow pipeline emission would overbuild the blackboard handoff. | Minor | Resolved upfront: export emits a small local Workflow messaging projection only. |
| design | D2: profile verification could leak prompt or response data into CI logs. | Major | Resolved upfront: outputs include status, session id, stop reason, fingerprint, and byte count only. |
| design | D3: messaging-core integration proof could be overclaimed. | Minor | Resolved upfront: docs and integration matrix label it config-only. |
| plan | P1: docs guard naming could drift from the existing test. | Minor | Resolved upfront: execution used the current docs guard test name. |
| plan | P2: declared integration matrix was missing from the plan. | Minor | Resolved upfront: plan records config-only Workflow messaging and runtime ACP fixture proof. |
| plan | P3: fake-runner tests alone could miss process/CLI behavior. | Major | Resolved upfront: PR #96 added built CLI plus fixture ACP agent smoke coverage. |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| `omitempty` on a value struct did not omit `workflow` from default blackboard exports. | test-driven-development | Initial tests focused on the workflow-enabled JSON shape, not the default export shape. | When adding optional JSON object fields in Go, add a default-shape assertion and prefer pointer fields or custom marshaling. |
| Human `profiles verify` output omitted the command fingerprint even though JSON and docs needed it. | doc-reconciliation | The contract was checked in JSON first; human output was treated as secondary. | For CLI metadata contracts, test both JSON and human output when the field is security/audit relevant. |
| README and policy matrix did not initially mention the verify fingerprint. | requesting-code-review | Implementation and docs landed together, but the docs checklist did not enumerate every emitted field. | Keep a small emitted-field checklist in docs tasks for command contracts. |
| Binary smoke failure diagnostics captured stdout but not stderr. | test-driven-development | The assertion path optimized for redaction checks, not failure triage. | Built-binary smoke helpers should capture both stdout and stderr, while still asserting neither leaks sensitive content. |

## Missed skill activations

Activation log evidence was not usable from the repo-local autodev state, so this table is reconstructed from committed artifacts and the execution transcript.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | yes | `docs/plans/2026-07-05-ratchet-workflow-profile-ci-design.md`. |
| adversarial-design-review (design) | yes | `docs/plans/2026-07-05-ratchet-workflow-profile-ci-design-review.md`. |
| writing-plans | yes | `docs/plans/2026-07-05-ratchet-workflow-profile-ci.md`. |
| adversarial-design-review (plan) | yes | `docs/plans/2026-07-05-ratchet-workflow-profile-ci-plan-review.md`. |
| alignment-check | yes | `docs/plans/2026-07-05-ratchet-workflow-profile-ci-alignment.md`. |
| scope-lock | yes | Manifest locked before implementation and completed after PR #96 merged. |
| subagent-driven-development | no | The two small PRs were implemented inline. |
| pr-monitoring | yes | PRs #94 and #96 were monitored through review fixes and admin merge. |
| post-merge-retrospective | yes | This retro. |

## What worked

- The two-PR split kept blackboard export and ACP profile verification independently reviewable.
- Local verification covered focused command tests, ACP fixture tests, docs guards, and Windows amd64 cross-builds before admin merge.
- Copilot review found concrete contract issues in both PRs before the closeout step.
- Creating a clean replacement PR #96 avoided force-pushing over a stacked branch after PR #94 squash-merged.

## What didn't

- PR #95 was opened as a stacked PR and then superseded, adding process noise outside the locked PR grouping.
- Broad `./cmd/ratchet` baseline tests were slow enough that focused test slices were more practical during review turnaround.
- Admin merge happened before all GitHub checks settled; this matched the user's instruction for delayed checks, but the release should still monitor the eventual master and tag workflows.

## Plugin-level follow-ups

No plugin-level change is warranted. The Workflow messaging export intentionally stays a config envelope for `workflow-plugin-messaging-core`; provider delivery and secrets remain outside ratchet-cli.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `docs/design-guidance.md` | no change | The lessons are Go JSON/CLI contract testing practices, not durable project architecture constraints. |
