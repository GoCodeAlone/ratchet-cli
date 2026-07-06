# Ratchet CLI Windows and Policy Surface Design

## Goal

Ship the next bounded ratchet-cli slice by proving the real Windows command binary can start in hosted Windows CI and by exposing the existing policy matrix through a read-only CLI command.

## Global Design Guidance

Source: workspace `docs/design-guidance.md`; ratchet-cli `docs/policy-matrix.md`.

| guidance | design response |
|---|---|
| Primary language is Go and stdlib-first. | Implement command parsing/output in Go without new dependencies. |
| Reuse over rebuild. | Reuse `docs/policy-matrix.md` as the policy source of truth; the CLI reports a concise mirror rather than creating a new policy evaluator. |
| CI on public runners is acceptable for non-cloud builds. | Add only a hosted Windows compile/startup smoke; no cloud credentials or provider access. |
| Secrets never logged. | Policy output is static metadata; Windows smoke runs `--version` and `help` only. |
| Deferred automation must stay explicit. | Keep managed hooks, SDK execution, background drain, credentialed agent CI, and full Windows installer/TUI runtime out of scope. |

## Scope

1. Add a Windows-hosted CI smoke that builds `./cmd/ratchet` as `ratchet.exe` and runs `--version` plus `help`.
2. Update release/docs guards so public docs can say the non-interactive Windows binary startup path is proven without claiming full packaged TUI or installer parity.
3. Add `ratchet policy matrix [--json]` as a read-only command summarizing supported, partial, explicit-operator, and deferred policy layers.
4. Document the new command and its non-goals.

## Architecture

The Windows proof belongs in GitHub Actions CI and is guarded by `internal/releaseguard` workflow tests. It uses a native `windows-2025` runner and writes output under `$RUNNER_TEMP` so it matches existing temporary-artifact hygiene.

The policy command belongs in `cmd/ratchet` as a local command surface. It owns a small, curated row table that mirrors the layer names and statuses in `docs/policy-matrix.md`; docs guard tests keep the command and docs from drifting on the required terms. JSON output is for scripts and diagnostics only.

## Security Review

No credentials are introduced. The Windows job uses GitHub-hosted CI and the repo's existing private-module Git rewrite. The policy command emits static layer/status/rule text and must not read local trust grants, hook descriptors, launch profiles, queues, archives, retro evidence, or other sensitive local policy metadata.

## Infrastructure Impact

The only infrastructure change is one additional non-cloud GitHub Actions job on `windows-2025`. It may add CI minutes but does not create resources, secrets, services, queues, storage, or production deploy steps.

Rollback: revert the workflow/docs/command PR. Existing cross-builds, releaseguard checks, and release workflows remain unchanged.

## Multi-Component Validation

The CI workflow is validated structurally by releaseguard tests and behaviorally by the hosted Windows job after the PR opens. Local verification covers command behavior with Go tests and launches a built `ratchet` binary for `--version`, `help`, and `policy matrix --json`.

## Assumptions

- The next useful ratchet-cli work should improve user/operator confidence without entering deferred managed-hook, SDK, or background autonomy work.
- Native Windows startup proof for non-interactive commands is a meaningful improvement even though full packaged TUI runtime remains deferred.
- The policy matrix document remains the source of truth; the CLI surface is a convenience mirror, not a second authority.

## Alternatives Considered

1. Credentialed third-party agent CI: rejected for this slice because it needs secret handling and provider isolation.
2. Managed hooks or broader SDK execution: rejected because policy docs require a new lifecycle design first.
3. Background daemon drain: rejected because hidden background autonomy remains explicitly deferred.

## Self-Challenge

- The simplest solution for policy visibility is only documentation. The CLI is still justified because users diagnosing local behavior should not have to leave the binary for a supported/deferred summary.
- The policy row table can drift from the Markdown matrix. Tests must assert required layer/status terms and docs must point to the source of truth.
- The Windows smoke might overclaim packaging. Wording must stay precise: native command startup is proven; full packaged TUI/installer/runtime parity remains deferred.

## Rollback

Revert the PR. If the Windows job proves flaky, remove only that job and keep the command/docs if their tests remain green; no data migration or state cleanup is required.
