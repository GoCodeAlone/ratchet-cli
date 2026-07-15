# Retro: Provider, Drain, and Managed Hooks

**PRs:** workflow-plugin-agent #40; ratchet-cli #133, #135, #136
**Related fix:** ratchet-cli #134
**Merged:** 2026-07-10 through 2026-07-15
**Releases:** workflow-plugin-agent `v0.12.7`; ratchet-cli `v0.30.28`,
`v0.30.29` (fix), `v0.30.30`, `v0.30.31`
**Branches:** `feat/provider-types-contract`, `feat/provider-setup-unification`,
`feat/daemon-acp-background-drain`, `feat/managed-hook-policy`
**Design:** `docs/plans/2026-07-10-ratchet-cli-provider-drain-managed-hooks-design.md`
**Plan:** `docs/plans/2026-07-10-ratchet-cli-provider-drain-managed-hooks.md`
**Related ADRs:** `decisions/0003-centralize-provider-setup.md` through
`decisions/0010-pin-hook-audit-anchor.md`

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1-D6: runtime provider discovery, ACP trust/ownership, final hook ordering, secure policy input, and pre-launch audit | Important | Resolved upfront: they produced the upstream registry contract and the core provider, drain, and hook boundaries before implementation. |
| design | D7-D39: durable provider-save identity, secret cleanup, alias admission, restart/downgrade, native lock, and real TUI proof | Important / Minor | Resolved upfront overall; D27 and D30 were prescient because native Windows and full-process proof remained decisive downstream. |
| design | D40-D77: authoritative ACP transitions, monotonic cancellation, audit framing/identity, pinned handles, process leases, and upgrade-forward recovery | Important / Minor | Prescient: these classes drove multiple implementation corrections and native process/security proofs before PR #135 became green. |
| design | D78-D83: production launch lease, cross-process audit retry, exact native Windows selectors, fresh-process recovery, schema completeness, and releaseguard formatting | Important / Minor | Prescient: exact selectors and process-shaped fixtures were necessary to prevent vacuous CI success. |
| plan | P1-P6: real daemon/TUI integration, native Windows execution, opened-object DACL checks, pre-launch audit, and executable commands | Important | Resolved upfront; all were represented in the locked task verification. |
| plan | P7-P34: provider-save decomposition, pinned downgrade ancestry, physical cleanup convergence, exact CI selectors, transcript safety, and secret-safe diagnostics | Important / Minor | Resolved upfront overall; P9/P26 were prescient because native-selector and fixture gaps still surfaced in CI. |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| PR #133 was admin-merged while its required `Test` check was still running and later failed after a 10-minute timeout in `TestProviderOperationBlockingSecretAdmissionAndRestart`. | `pr-monitoring` / `verification-before-completion` | Local evidence was treated as permission to merge before CI settled. PR #134 was needed to repair main. | Hard-block merge until all required checks reach success; an admin override must not reinterpret pending as green. |
| Three actionable PR #133 review threads were still open at merge: duplicate provider-type diagnostics and the public `APPLIED` state being rewritten to `PENDING`. | `requesting-code-review` / `pr-monitoring` | Review arrived before merge but was not drained. The state rewrite also reflected an ADR/protobuf inconsistency. | Resolve the diagnostic and public-state contract in the immediate follow-up PR; require zero unresolved actionable threads. |
| PR #136 review found that unsupported-platform policy behavior and its trusted fallback were broader than intended. | `adversarial-design-review` (plan) | The plan named cross-builds but did not enumerate every unsupported-GOOS caller and trust decision. | Inventory callers for platform-stub errors and test each fail-open/fail-closed decision. |
| Native Windows CI rejected the original ancestry access mask and later rejected POSIX-shaped policy/audit fixtures. | `verification-before-completion` / `runtime-launch-validation` | Cross-compilation proved buildability, not Windows DACL semantics, access-denied replacement behavior, or fixture ACL inheritance. | Classify native security tests separately and require platform-native fixtures plus exact hosted selectors. |
| Linux race/coverage exposed leaked restrictive umask, finite-watch terminal observation, and an oversized nested binary smoke. | `verification-before-completion` | Focused tests did not run the exact merge-gating process shape and timeout budget. | Run the exact race/coverage selector; isolate heavyweight binary smoke and restore process-global test state. |

## Missed skill activations

The canonical `.claude/autodev-state/in-progress.jsonl` activation log is
unavailable. Committed artifacts prove outputs existed, but not hook-recorded
activations.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | unavailable | Design artifact exists; activation log is absent. |
| adversarial-design-review (design) | unavailable | Committed report contains D1-D83. |
| writing-plans | unavailable | Locked 11-task, four-PR plan exists. |
| adversarial-design-review (plan) | unavailable | Committed report contains P1-P34. |
| alignment-check / scope-lock | unavailable | Alignment artifact and lock existed; activation log is absent. |
| subagent-driven-development | unavailable | Activation log is absent. |
| finishing Step 1e (doc-reconciliation) | yes | PR #136 records `Doc-reconciliation: 1 item fixed`; PR #133 records `clean`. |
| pr-monitoring | incomplete | PR #133 merged before required CI and review threads settled; later PRs were monitored to green. |
| post-merge-retrospective | yes | This retro. |

## What worked

- The registry query and shared catalog eliminated the CLI/TUI provider-list
  split and made future upstream drift testable.
- Repeated adversarial cycles converted ambiguous provider saves and ACP drains
  into durable, authority-first state machines with explicit secret and process
  boundaries.
- Native Windows CI, exact runtime fixtures, merge-commit CI, and releaseguard
  ultimately produced a public `v0.30.31` with six platform archives,
  checksums, matching Homebrew formula/cask data, and a time-bounded installed
  binary proof.

## What didn't

- PR #133 violated the settled-green and zero-review-thread merge gates; PR
  #134 repaired the resulting main-branch race-test failure.
- Cross-compiles and POSIX-local fixtures were repeatedly overcredited as proof
  of Windows security behavior.
- Heavy real-binary smoke initially shared race/coverage timeout budgets and
  process-global test state was not consistently restored.

## Plugin-level follow-ups

- `pr-monitoring` should refuse its merge path while any required check is
  pending/failing or any actionable thread remains unresolved. This recurs
  beyond the nested-help miss recorded in
  `docs/retros/2026-07-06-ratchet-cli-windows-policy-surface-retro.md`.
- `runtime-launch-validation` should explicitly label cross-compilation as
  build-only evidence and require native execution for filesystem security,
  terminal, service, and process-control claims.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `docs/design-guidance.md` | created | Settled merge gates, native-platform proof, exact merge selectors, shared contracts, and release-runtime checks are durable constraints for future ratchet-cli designs. |
