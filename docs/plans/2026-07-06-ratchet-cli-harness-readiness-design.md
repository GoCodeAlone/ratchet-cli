# Ratchet CLI Harness Readiness Design

## Goal

Ship the next bounded ratchet-cli slice by making existing harness primitives easier to verify, triage, and hand off without adding hidden automation or new credential paths.

## Global Design Guidance

Source: workspace `docs/design-guidance.md`; ratchet-cli `docs/policy-matrix.md`.

| guidance | design response |
|---|---|
| Primary language is Go and stdlib-first. | Implement command parsing/output in Go without new dependencies. |
| Reuse existing primitives. | Extend `profiles verify`, `policy matrix`, and `retro instructions` instead of adding a new harness service. |
| Secrets never logged. | Profile verification output remains redacted; retro bundles contain analysis and instructions, not raw evidence. |
| Keep deferred automation explicit. | No daemon background drain, managed hooks, SDK execution, credentialed third-party CI, or automatic PR creation. |
| Build for Windows. | Use portable filesystem/path handling and keep tests runnable on Windows. |

## Scope

1. Add `ratchet acp client profiles verify --all [--json]` to verify all trusted local ACP launch profiles and summarize pass/fail results without printing prompts, responses, or env values.
2. Add `ratchet policy matrix --status <status> [--json]` so operators can filter supported, partial, explicit-operator, or deferred layers from the existing static matrix.
3. Add `ratchet retro bundle --evidence <evidence.jsonl> [--session ID] --output <dir>` to write a portable local handoff directory containing `analysis.json`, `instructions.md`, and `manifest.json`.
4. Reconcile README, harness emulation, policy, and retro-loop docs.

## Architecture

`profiles verify --all` stays in `cmd/ratchet` and reuses the existing profile store, default registry, trusted-profile resolution, verification prompt runner, timeout behavior, fingerprinting, and redacted output shape. It skips untrusted profiles by reporting them as `skipped_untrusted`; it only launches trusted profiles. JSON output is an array of result records suitable for credential-free CI.

`policy matrix --status` filters the existing in-memory matrix rows. The Markdown matrix remains the source of truth and the command remains read-only static metadata, not a policy evaluator.

`retro bundle` reuses `buildRetroAnalyzeOutput` and `renderRetroInstructionsMarkdown`. The bundle manifest records file names, finding/action counts, session id, and creation time. It intentionally does not copy raw evidence JSONL because evidence can contain sensitive local context even after redaction.

## Security Review

No new credentials, providers, network calls, or background workers are introduced. Profile verification still requires local trust before launch and emits command fingerprints plus counts instead of prompts, responses, env values, or raw command args. Policy filtering emits static public metadata only. Retro bundles omit raw evidence and must write files with user-only permissions.

## Infrastructure Impact

No infrastructure changes. CI uses existing Go tests. Credentialed third-party provider matrices remain deferred until secret handling, provider isolation, and artifact redaction are designed.

## Multi-Component Validation

Local validation covers command parser tests, command execution tests with fixture ACP agents, retro bundle file content tests, docs guard tests, focused package tests, a full command-package test, local binary launch of the new commands, and Windows cross-build.

## Assumptions

- Operators need a fast way to prove all reviewed profiles remain usable before adding real provider matrices.
- Deferred policy layers should be discoverable by command line filters, not buried in Markdown.
- Self-improvement handoffs need a portable artifact, but automatic mutation and PR creation remain policy decisions for a future design.

## Alternatives Considered

1. Credentialed third-party agent CI: rejected for this slice because it needs secrets, failure isolation, and artifact redaction.
2. Background profile watch/drain: rejected because hidden background execution remains deferred.
3. Copy raw retro evidence into bundles: rejected because it increases accidental disclosure risk.
4. Add a new harness readiness command group: rejected because existing commands already own the relevant primitives.

## Self-Challenge

- `profiles verify --all` can be slow if many trusted profiles launch real agents. The command should honor the existing per-profile timeout and report partial failures instead of hiding the first failure.
- Filtering the static policy table can drift from docs. Docs guard tests must keep required statuses and command mentions visible.
- Retro bundles could be mistaken for safe-to-publish artifacts. Docs must state they are local handoffs and may still include sensitive summarized context.

## Rollback

Revert the PR. The commands are additive and write no migrated state except user-requested retro bundle output directories.
