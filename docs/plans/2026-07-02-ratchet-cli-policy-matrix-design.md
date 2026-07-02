# ratchet-cli Policy Matrix Design

**Status:** Draft
**Date:** 2026-07-02
**Project:** `GoCodeAlone/ratchet-cli`

## Goal

Define and test ratchet-cli's policy-layer matrix for permissions, sandboxing, trust, and per-agent scope so later automation such as background drain, extension hooks, and richer sandbox controls has a documented safety contract.

## User Intent

The operator asked to continue from the ratchet-cli harness roadmap after archive import/export, compare, flow orchestration, team messaging, and persistent trust grants. Workspace state now lists the remaining self-improving harness work as policy matrix, auto-drain after policy boundaries, optional extension hooks, credentialed third-party CI, raw ACPX event-log compatibility, ACPX TypeScript flow compatibility, and local-first gateway/channel work. This design addresses the policy prerequisite first.

## Global Design Guidance

Source: workspace `AGENTS.md`, `README.md`, `docs/harness-emulation.md`, `docs/competitor-parity.md`, `docs/plans/2026-04-07-trust-permission-sandbox-design.md`, and `docs/plans/2026-07-02-persistent-trust-policy-design.md`.

| Guidance | Design response |
|---|---|
| Prefer existing Workflow/plugin APIs and avoid duplicated plumbing. | Matrix names `workflow-plugin-agent/policy.TrustEngine` and `PermissionStore` as canonical trust/grant logic. ratchet-cli documents/guards the surfaces it owns. |
| Build for Windows. | This slice is docs and tests only; later policy automation must retain existing Windows cross-build gates. |
| Treat policy metadata as security-sensitive. | Matrix marks command/path/grant patterns and retro evidence as sensitive local metadata; no export or logging expansion is added. |
| Use current competitor evidence rather than memory. | The matrix cites the source-backed parity document and live-checked external docs for Codex config, Claude hooks/settings, Zed external-agent permissions, and Hermes harness direction. |
| Do not add automation before boundaries are explicit. | Background drain, mutation-capable hooks, and local-first gateways stay deferred until this matrix and regression tests exist. |

## Current State

- `docs/competitor-parity.md` says permissions, sandbox, and trust are partial and names policy matrix as the next prerequisite.
- v0.21.0 added daemon-backed runtime trust controls and team messaging.
- v0.22.0 added persistent grants backed by `workflow-plugin-agent/policy.PermissionStore`.
- `cmd/ratchet/harness_docs_test.go` already enforces minimum docs coverage for harness modes and parity references.
- `internal/daemon` owns runtime trust mode/rule mutation, permission prompts, cron/fleet/team lifecycle, and retro evidence.
- `internal/acpclient` owns ACP client exec, queue, drain, archive, compare, and flow state.
- `internal/hooks` owns existing hook event names and template expansion, but there is no broad runtime extension SDK.

## Current External Signals

Live source check on 2026-07-02:

- Codex documents config, sandbox, approvals, MCP, and profiles as explicit policy surfaces.
- Claude Code documents hooks/settings and supports lifecycle hook conditions using permission-rule syntax.
- Zed external agents keep ACP runtime/tool policy largely with the external agent, while open Zed discussions/issues show active pressure for clearer external-agent permission behavior.
- Hermes emphasizes durable multi-agent harness state, gateway/session continuity, and self-improvement loops.

These signals support a first-class matrix that distinguishes current, supported policy layers from future automation.

## Recommended Approach

Ship one documentation-and-test PR before adding behavior:

1. Add `docs/policy-matrix.md` as the durable source of truth for policy layers, precedence, owner, current status, sensitive data, and validation.
2. Update `README.md`, `docs/harness-emulation.md`, and `docs/competitor-parity.md` to point to the matrix and to mark persistent trust editing as shipped.
3. Extend `cmd/ratchet/harness_docs_test.go` so CI fails if policy docs stop naming the required layers, owners, deferred automation, and sensitive metadata warnings.

This is intentionally not a new policy engine, scheduler, or hook runtime. It makes the boundary machine-checkable enough for the next behavioral slice.

## Alternatives Considered

1. Implement auto-drain now.
   - Rejected. Auto-drain changes when queued prompts execute and needs policy decisions for agent/session ownership, trust grants, cancellation, and background execution.
2. Implement extension hooks now.
   - Rejected. Mutation-capable hooks need an explicit opt-in and redaction boundary first.
3. Build a new policy evaluator in ratchet-cli.
   - Rejected. Trust matching and persistent grants belong to `workflow-plugin-agent/policy`; ratchet-cli should not fork matcher semantics.

## Policy Matrix Shape

The matrix document must include these layers:

| Layer | Owner | Current status | Rule |
|---|---|---|---|
| Static config trust rules | `internal/config` + agent plugin trust engine | Supported | Baseline mode/rules. |
| Runtime trust rules | daemon RPC + TUI/CLI | Supported | Ephemeral session-level overrides. |
| Persistent trust grants | `workflow-plugin-agent/policy.PermissionStore` via daemon RPC | Supported | Durable allow/deny grants; deny wins. |
| Permission prompts | daemon/TUI permission gate | Supported | Human resolution remains explicit. |
| ACP client queue/drain | `internal/acpclient` | Explicit drain only | No background drain until policy-bound. |
| Sandbox/path/network controls | agent plugin trust, mesh path guard, future sandbox work | Partial/deferred | Document what exists and what is not claimed. |
| Hooks/extensions | `internal/hooks`, plugin manifests | Partial/deferred | Existing named hooks only; broad SDK/mutation hooks deferred. |
| Retro/self-improvement | `internal/retro` | Opt-in evidence only | No automatic local mutation unless configured. |
| Per-agent/team scopes | daemon team + mesh configs | Partial/deferred | Team messaging exists; per-agent policy scopes future. |

The document must also define precedence at a conceptual level:

1. Hard deny and missing authorization wins.
2. Persistent deny grants beat persistent allow grants.
3. Runtime deny rules beat runtime allow rules at the same scope.
4. Runtime rules can narrow current process behavior but are not durable.
5. Static config remains baseline for daemon startup.
6. Unknown/missing policy falls back to explicit prompt or deny, not silent auto-approval.

## Security Review

- No new secrets, credentials, cloud permissions, or network endpoints are added.
- Policy docs must mark grant/rule patterns as sensitive local metadata because they can reveal paths, commands, providers, or operational habits.
- The matrix must prevent overclaiming: sandbox/path/network controls are partial unless an implementation and proof exist.
- Background automation remains deferred until a future design defines authorization, cancellation, and audit behavior.
- Hook/extension work remains mutation-opt-in by default and must preserve redacted evidence handling.

## Infrastructure Impact

No cloud resources, database migrations, queues, releases, or production deploys. This phase updates docs and Go tests only.

## Multi-Component Validation

Minimum proof:

- `go test ./cmd/ratchet -run HarnessEmulationDocs -count=1` fails before docs cover required policy layers and passes after.
- `rg -n "Policy Matrix|persistent trust grants|background drain|extension hooks|sensitive local policy metadata" README.md docs/harness-emulation.md docs/competitor-parity.md docs/policy-matrix.md` confirms public docs link the matrix and state deferred automation.
- `go test ./... -count=1 -p=1` confirms repo-wide docs tests still pass.
- `git diff --check` confirms Markdown whitespace is clean.

## Rollback

Revert the docs/test PR. No runtime behavior, data files, release tags, or daemon state changes are introduced.

## Assumptions

- The current source-backed parity document is sufficient evidence for this docs-first slice; if a later behavioral design depends on a competitor implementation detail, it must re-check that source.
- Existing trust/grant behavior in v0.22.0 is correct and should be documented, not reimplemented.
- Auto-drain and extension hooks remain blocked until a policy matrix exists.
- CI has no separate markdown renderer; Go docs tests and `git diff --check` are the enforceable local gates.

## Self-Challenge

1. Laziest solution: add one paragraph to README. Rejected because the backlog needs a durable matrix that tests can guard before behavior changes.
2. Fragile assumption: docs-only is enough. Mitigation: tests assert required policy terms and deferred controls so future changes must update the matrix.
3. Possible overreach: adding policy precedence may imply runtime semantics not enforced everywhere. Mitigation: label conceptual precedence and mark partial/deferred layers explicitly.

## Out of Scope

- Background or scheduled auto-drain.
- New trust matching semantics.
- Config-file mutation.
- New sandbox/path/network enforcement.
- A broad runtime extension SDK.
- Credentialed third-party agent CI.
- Raw ACPX JSON-RPC event-log compatibility.
- ACPX TypeScript flow runtime compatibility.
- Local-first gateway or channel routing.

