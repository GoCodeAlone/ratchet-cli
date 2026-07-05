# Retro: Runtime Extension Lifecycle

**PR:** #97 — feat: add runtime plugin reload
**Merged:** 2026-07-05
**Branch:** feat/runtime-extension-lifecycle
**Design:** docs/plans/2026-07-05-ratchet-runtime-extension-lifecycle-design.md
**Plan:** docs/plans/2026-07-05-ratchet-runtime-extension-lifecycle.md
**Related ADRs:** decisions/0002-runtime-extension-lifecycle.md

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | Avoid a full workflow runtime in the first slice. | Important | Resolved upfront |
| design | Do not inject every skill body by default. | Critical | Prescient |
| design | Marketplace trust must not imply hook trust. | Critical | Resolved upfront |
| design | Raw prompts in hook payloads are a leak. | Critical | Resolved upfront |
| design | Dynamic reload must be daemon-owned and stop old plugin daemons. | Important | Prescient |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| `SelectForPrompt` tokenized plain words, so common skill names could inject without `$` or `/`. | adversarial-design-review (plan) | The design warned against broad skill injection, but the implementation review did not require a parser-level negative test for common plain words. | Add a prompt-injection checklist item requiring explicit-prefix negative tests for skill activation. |
| Plugin skill capability paths skipped the containment guard already used by hooks/profiles. | adversarial-design-review (plan) | The design covered marketplace and hook trust but did not generalize containment to all file-backed plugin capabilities. | Treat every manifest path as untrusted input in plugin lifecycle reviews. |
| `RunHooks` read `ec.Hooks` without the extension reload lock. | adversarial-design-review (plan) | The plan named daemon-owned reload but did not include a concurrency audit for each runtime field swapped by reload. | Add a reload-swap checklist: every replaced field needs lock-protected readers or atomic publication. |
| Initial MCP discovery raced with plugin reload publication under `go test -race`. | verification-before-completion | Local pre-PR verification ran non-race full tests; CI's race/coverage job exposed the registry publication race. | Run the repository CI-equivalent race command before PR creation for extension/runtime changes. |

## Missed skill activations

Activation log unavailable at `.claude/autodev-state/in-progress.jsonl`; this workspace did not record the expected autodev skill events for this run.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | unverified | Skill was read and used, but no activation log was present. |
| adversarial-design-review (design) | unverified | Committed design-review artifact exists. |
| writing-plans | unverified | Committed implementation plan exists. |
| adversarial-design-review (plan) | no artifact | No deterministic `*-plan-review.md` artifact was committed for this PR. |
| alignment-check | unverified | Committed alignment artifact exists. |
| scope-lock | unverified | Committed scope-lock artifact exists. |
| subagent-driven-development | unverified | Skill was read for planning, but implementation ran inline. |
| finishing Step 1e (doc-reconciliation) | unverified | PR touched docs; PR body did not include a `Doc-reconciliation:` line. |
| pr-monitoring | yes | CI failure was monitored, fixed, and rerun to green before merge. |
| post-merge-retrospective | yes | This retro closes the loop after merge. |

## What worked

- The design review correctly constrained the first PR to daemon reload, skills, and hooks instead of workflows/routines.
- Copilot review found real security and concurrency issues before merge.
- CI race coverage caught the MCP registry publication bug that the local non-race full suite missed.
- Review-thread replies and GraphQL resolution kept the merge gate explicit.

## What didn't

- The first PR missed negative tests for common-word skill names.
- Manifest path containment was applied inconsistently across plugin capabilities until review.
- Runtime reload needed a stronger reader/writer audit; hooks and MCP discovery both exposed gaps.
- The activation log was absent, so skill firing had to be reconstructed from committed artifacts.

## Plugin-level follow-ups

- Add adversarial-review prompts for file-backed plugin capability containment and reload publication/read locking.
- Add verification-before-completion guidance to prefer `go test -race -coverprofile=coverage.out -covermode=atomic ./...` for daemon/runtime extension work.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `decisions/0002-runtime-extension-lifecycle.md` | no change | Durable lifecycle boundary already captured in the existing ADR; no separate project guidance file exists. |
