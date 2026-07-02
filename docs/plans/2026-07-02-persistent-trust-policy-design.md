# Persistent Trust Policy Design

**Status:** Draft
**Date:** 2026-07-02
**Project:** `GoCodeAlone/ratchet-cli`

## Goal

Expose durable trust grants in ratchet-cli so operators can list, add, and revoke persistent allow/deny policy through daemon-backed CLI and TUI commands. Runtime slash-command rules remain ephemeral; durable grants are stored by `workflow-plugin-agent/policy.PermissionStore`.

## Global Design Guidance

Source: workspace guidance, `README.md`, `docs/harness-emulation.md`, and prior trust design docs.

| guidance | design response |
|---|---|
| Avoid duplicated application plumbing and prefer reusable Workflow/plugin APIs. | Reuse `workflow-plugin-agent/policy.PermissionStore`; ratchet-cli only exposes daemon and UI control surfaces. |
| Treat secrets and policy as security-sensitive. | No new secret storage. Grant data is pattern/action/scope metadata only and remains in the daemon SQLite state DB. |
| Keep ratchet-cli a thin consumer of agent-plugin trust logic. | No new matcher or persistence implementation; daemon calls `PermissionStore.Grant`, `Revoke`, and `List`. |
| Build for Windows. | CLI and proto changes remain platform-neutral; verification includes Windows cross-builds before release. |

## Current State

- v0.21.0 added `GetTrustState`, `SetTrustMode`, `AddTrustRule`, and `ResetTrust`.
- `Service.NewService` already initializes `policy.PermissionStore` from the daemon DB and attaches it to `TrustEngine`.
- `/trust allow` and `/trust deny` add runtime-only rules in memory; `/trust reset` clears only runtime rules.
- README and `docs/harness-emulation.md` explicitly say current slash commands do not edit config or delete persisted permission grants.
- The agent plugin already owns persistent grant schema and matching behavior.

## Recommended Approach

Add a first-class persistent-grant surface to the existing trust RPC group:

- Extend proto with `TrustGrant`, `AddTrustGrant`, `RevokeTrustGrant`, and include grants in `TrustState`.
- Add client wrappers and daemon handlers that validate action, pattern, and scope, then call `PermissionStore`.
- Add `ratchet trust grants|persist|revoke` CLI commands for scriptable policy editing.
- Extend TUI slash commands with `/trust grants`, `/trust persist allow|deny "pattern" [--scope scope]`, and `/trust revoke "pattern" [--scope scope]`.
- Update docs to distinguish runtime rules from persistent grants.

## Alternatives Considered

1. Store persistent rules in `~/.ratchet/config.yaml`.
   - Rejected because ratchet-cli would need to edit user-authored YAML and duplicate policy persistence semantics already provided by the agent plugin.
2. Make `/trust allow` persistent by default.
   - Rejected because v0.21.0 documented it as runtime-only. Changing default persistence would surprise operators and make short-lived experiments sticky.
3. Add a separate ratchet-owned SQLite table.
   - Rejected because it duplicates `workflow-plugin-agent/policy.PermissionStore` and risks two policy sources disagreeing.

## Data Model

`TrustGrant` mirrors `policy.PermissionGrant`:

- `id`
- `pattern`
- `action`
- `scope`
- `granted_by`
- `created_at`

`TrustState` continues to expose effective in-memory rules and adds persisted grants. Grants are listed separately because they are evaluated by `PermissionStore`, not appended to `TrustEngine.Rules()`.

## RPC Behavior

- `GetTrustState` returns mode, effective runtime/config rules, and persisted grants.
- `AddTrustGrant(pattern, action, scope)` accepts only `allow` or `deny`; blank scope defaults to `global`; `granted_by` is daemon-set to `operator`.
- `RevokeTrustGrant(pattern, scope)` removes the matching persisted grant; blank scope defaults to `global`; revoking a missing grant is idempotent.
- If the daemon has no permission store, persistent grant RPCs return `FailedPrecondition`.

## CLI and TUI Behavior

CLI:

```sh
ratchet trust list
ratchet trust grants
ratchet trust allow "bash:go test *" --scope repo
ratchet trust deny "bash:rm *" --scope global
ratchet trust persist allow "bash:go test *" --scope repo
ratchet trust revoke "bash:go test *" --scope repo
ratchet trust reset
```

TUI:

```text
/trust list
/trust grants
/trust allow "pattern" [--scope scope]
/trust deny "pattern" [--scope scope]
/trust persist allow|deny "pattern" [--scope scope]
/trust revoke "pattern" [--scope scope]
/trust reset
```

`allow` and `deny` remain runtime-only. `persist` is the explicit durable path.

## Security Review

- No live secrets are stored or printed.
- Pattern strings can contain sensitive paths or commands. They are already user-authored policy metadata, so docs mark grant listings and exports as local sensitive state.
- Persisted grants can widen tool access. Commands require daemon access, matching existing local trust controls.
- Deny grants must remain supported because deny-wins semantics are the safety fallback.
- No AWS, cloud, registry, or external SDK functionality is introduced.

## Infrastructure Impact

No cloud resources, network exposure, secrets, IAM, queues, or production deploys. The daemon DB already creates `permission_grants` through the agent plugin. This design adds no ratchet-owned migration.

## Multi-Component Validation

Minimum proof:

- Daemon unit/integration test adds a persistent grant through RPC, observes it in `GetTrustState`, constructs a fresh service over the same DB, and observes the grant after reload.
- CLI test exercises argument parsing and client method calls for grant add/list/revoke.
- TUI command tests exercise slash-command parsing through the daemon-backed client interface.
- Full verification includes `go test ./...`, `go build ./cmd/ratchet`, and Windows amd64/arm64 cross-builds.

## Rollback

Revert the ratchet-cli PRs and release a patch version. Existing rows in `permission_grants` are harmless when not exposed by ratchet-cli control surfaces because the agent plugin owns their evaluation. Operators can revoke stale grants before or after rollback with a version that supports `revoke`, or by direct DB maintenance if needed.

## Assumptions

- `policy.PermissionStore` remains the canonical durable grant store.
- Daemon SQLite state is the right persistence boundary for ratchet-cli runtime policy.
- Existing local daemon access is sufficient authorization for trust policy edits.
- Proto additions are backward compatible because they add messages, fields, and RPCs without removing existing fields.

## Self-Challenge

1. Simpler solution: edit config YAML. Rejected because it duplicates persistence and risks breaking user formatting.
2. Fragile assumption: daemon-local access is enough authorization. This matches current trust RPCs, but future multi-user daemon modes would need authenticated policy admins.
3. Failure mode: store unavailable. RPCs return `FailedPrecondition`; runtime-only trust commands still work.

## Out of Scope

- Interactive permission prompt UI overhaul.
- New matching semantics or policy precedence in the agent plugin.
- Config-file editing.
- ACPX or flow policy transport.
- Cloud or registry publication changes.
