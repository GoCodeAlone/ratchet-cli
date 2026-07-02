# Retro: Persistent Trust Policy

**PR:** #55 - feat: add persistent trust policy controls
**Merged:** 2026-07-02
**Branch:** feat/ratchet-cli-persistent-trust-policy
**Design:** docs/plans/2026-07-02-persistent-trust-policy-design.md
**Plan:** docs/plans/2026-07-02-persistent-trust-policy.md
**Related ADRs:** None

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1: persistent grants do not solve the interactive prompt's "Always allow" gap | Minor | Resolved upfront |
| design | D2: missing-grant revocation should be idempotent | Minor | Resolved upfront |
| design | D3: grant patterns are sensitive local policy metadata | Minor | Resolved upfront |
| plan | P1: CLI tests should use a fake client, not a live daemon | Minor | Resolved upfront |
| plan | P2: TUI grant commands depend on Task 1 client wrappers | Minor | Resolved upfront |
| plan | P3: proto rollback must include generated files | Minor | Resolved upfront |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| `PermissionStore.List` errors were initially hidden, making `GetTrustState` report empty grants on store failure | adversarial-design-review (plan) | The plan required store-unavailable coverage for grant mutations, but did not explicitly require list-failure propagation from trust-state construction | Add "read/list failure must propagate, not degrade to empty state" to persistent-store review checks |

Documentation review comments on README inline spans, harness-emulation inline spans, and the obsolete `TrustGrantList` design reference were caught by PR review and fixed before merge. They were not process-level gate misses beyond ordinary doc reconciliation.

## Missed skill activations

Activation log unavailable in `.claude/autodev-state/in-progress.jsonl`; rows below are reconstructed from committed artifacts and PR evidence.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | yes | Design artifacts were produced before implementation. |
| adversarial-design-review (design) | yes | `docs/plans/2026-07-02-persistent-trust-policy-design-review.md`. |
| writing-plans | yes | `docs/plans/2026-07-02-persistent-trust-policy.md`. |
| adversarial-design-review (plan) | yes | `docs/plans/2026-07-02-persistent-trust-policy-plan-review.md`. |
| alignment-check | yes | `docs/plans/2026-07-02-persistent-trust-policy-alignment.md`. |
| scope-lock | yes | Manifest locked before implementation; completed after release. |
| subagent-driven-development | no | Codex tool instructions prevented subagent spawning without explicit user request; implementation ran inline. |
| finishing Step 1e (doc-reconciliation) | unverified | PR touched docs, but the PR body did not include a Doc-reconciliation line. |
| pr-monitoring | yes | CI, Copilot review threads, master checks, release, and tap publish were monitored. |
| post-merge-retrospective | yes | This retro. |

## What worked

- The design kept persistence in `workflow-plugin-agent/policy.PermissionStore`, avoiding a second ratchet-owned policy store.
- TDD across client, daemon, CLI, and TUI caught missing RPC surfaces before implementation stabilized.
- Release verification covered GitHub release assets, Windows archives, and the Homebrew cask update.

## What didn't

- The plan did not name list/read-store failure as a distinct failure mode, so the first implementation silently converted a failing grant store into an empty grant list.
- Markdown command-span issues and a stale proto type name reached PR review instead of being caught during doc reconciliation.
- Skill activation evidence was not available from the repo-local autodev jsonl, which makes retro attribution less precise.

## Plugin-level follow-ups

No plugin-level change is warranted from a single retro, but future persistent-store plans should explicitly test read/list failure propagation and doc/API name consistency.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `docs/design-guidance.md` | no change | The repo does not currently have a project guidance file, and the findings are feature-local rather than durable project policy. |
