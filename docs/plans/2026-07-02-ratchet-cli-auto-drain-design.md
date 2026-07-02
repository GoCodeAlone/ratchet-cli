# ratchet-cli Auto-Drain Design

**Status:** Draft
**Date:** 2026-07-02
**Project:** `GoCodeAlone/ratchet-cli`

## Goal

Add an explicit opt-in ACP client auto-drain worker so queued `--no-wait` prompts can drain with low operator friction while preserving the policy matrix boundaries.

## User Intent

Continue the ratchet-cli self-improving harness backlog after the policy matrix. The next item is background/auto-drain under explicit policy boundaries; extension hooks remain next, not bundled here.

## Global Design Guidance

Source: workspace `AGENTS.md`, repo `README.md`, `docs/policy-matrix.md`, `docs/harness-emulation.md`, `docs/competitor-parity.md`. No repo-local `AGENTS.md` or `docs/design-guidance.md` exists.

| Guidance | Design response |
|---|---|
| Build for Windows. | Implement as portable Go CLI/library code; verify Windows build in CI and local cross-build gate. |
| Avoid duplicate policy engines. | Reuse `internal/acpclient.DrainQueue`, owner locks, cancel requests, and command resolution; no new trust matcher or scheduler policy store. |
| Treat queue contents as sensitive local policy metadata. | Keep output aggregate by default; do not add exports/logs containing prompt text. |
| Background drain is deferred until owner/session/cancel/audit are designed. | This design makes drain opt-in via a foreground `watch` worker and records explicit CLI output as audit evidence. |
| Keep broader extension hooks separate. | No hook SDK, lifecycle interception, or mutation-capable extension point is added. |

## Current State

- `ratchet acp client exec --no-wait --session <id>` appends prompt text to XDG-local JSON state.
- `ratchet acp client drain <id> --command <agent>` explicitly drains queued prompts through one ACP session.
- `internal/acpclient.DrainQueue` already owns FIFO order, owner-lock acquisition, stale running-item recovery, cooperative cancel, max-per-call, and owner cleanup.
- Stored records keep `Agent`, `CommandFingerprint`, and `Cwd`, but not the full command/args needed to safely reconstruct a custom agent launch.
- `docs/policy-matrix.md` marks ACP client queue/drain as `Explicit drain only` and background drain as deferred until policy is explicit.

## Approaches

| Approach | Summary | Trade-off | Decision |
|---|---|---|---|
| A. Foreground `watch` worker | `ratchet acp client watch <session-id> --command/--agent ... --interval ...` loops, calling `DrainQueue` when pending prompts exist. | Explicit, portable, testable; not a hidden daemon. | Choose for this slice. |
| B. Daemon-integrated scheduler | ratchet daemon owns background queue workers. | Better UX later, but needs daemon config, lifecycle, restart, and authz expansion. | Defer. |
| C. Shell/system scheduler recipe only | Document `while sleep; drain`. | Lowest code, but weak status/cancel/audit UX and easy to get wrong. | Reject as insufficient. |

## Design

### Command

Add:

```sh
ratchet acp client watch <session-id> [flags]
```

Flags:

- agent launch: same as `drain`: `--agent`, `--command`, repeated `--arg`, `--cwd`, `--timeout`;
- scheduling: `--interval duration` default `5s`, `--max-per-cycle int` default `1`, `--max-cycles int` default `0` for unlimited, `--stop-when-empty`;
- output: human summary per cycle; `--json` for newline-delimited JSON cycle summaries.

### Runtime Behavior

1. Resolve agent spec from supplied `--agent`/`--command`; never reconstruct from stored fingerprint alone.
2. Poll session state every interval.
3. If no pending prompts:
   - with `--stop-when-empty`, exit 0 after reporting idle;
   - otherwise sleep until next cycle or context cancel.
4. If pending prompts exist, call `DrainQueue` with `Max = --max-per-cycle`.
5. Stop on first drain error and preserve existing failed/pending queue state.
6. Respect existing `ratchet acp client cancel <session-id>` through `DrainQueue` cancel checks.
7. Exit cleanly on SIGINT/SIGTERM.

### Policy Boundary

- Authorization = an operator launched `watch` with an explicit agent command.
- Ownership = existing per-session owner lock; a second `watch` or `drain` gets `ErrDrainBusy`.
- Cancellation = existing cancel-request file and queue-cancel behavior.
- Audit = local session state plus watch output; no new central log, no prompt text expansion.
- Persistence warning = docs continue to state queued prompts are local sensitive data.

### Data Model

No persistent schema change is required. Optional in-memory `WatchOptions` and `WatchCycle` structs can live in `internal/acpclient`.

### CLI Shape

Implementation should mirror existing command parsing in `cmd/ratchet/cmd_acp_client.go` and reuse `executeACPClientDrain` internals where practical. Tests should inject a fake start runner; binary smoke should use the existing fixture ACP agent.

## Security Review

- No secrets, cloud credentials, or network endpoints added.
- No background process starts unless the operator runs the command or their shell/service manager starts it.
- Prompt text remains in existing local queue storage only; watch output should not print prompt bodies.
- Agent command remains explicit per watch invocation; stored fingerprints are not treated as launch authority.
- Owner locks prevent concurrent drain/watch workers for the same local session.

## Infrastructure Impact

No cloud resources, migrations, release tags, Homebrew changes, or production deploys. The CLI behavior is cross-platform Go code. Windows impact is limited to process signal behavior and path handling; tests/builds must cover Windows compile.

## Multi-Component Validation

- Unit tests: watch loop drains pending prompts, idles, honors `--stop-when-empty`, max cycle limits, cancel, and busy owner behavior using fake runners.
- CLI tests: parse/execute `watch` with injected runner/store.
- Binary smoke: build ratchet + fixture agent, queue prompts, run `watch --stop-when-empty --max-per-cycle 2`, then assert status shows completed queue.
- Full gate: focused tests, full `go test`, `go vet`, `golangci-lint --new-from-rev=origin/master`, Windows amd64/arm64 build, docs grep, `git diff --check`.

## Rollback

Revert the watch CLI/library/docs commits. Existing queue/drain state remains readable because no persistent schema or data migration is introduced.

## Assumptions

| ID | Assumption | Challenge | Fallback |
|---|---|---|---|
| A1 | Explicit foreground `watch` satisfies the next auto-drain prerequisite. | User may expect a hidden daemon scheduler. | Treat daemon scheduler as follow-up after foreground worker proves policy. |
| A2 | Stored command fingerprint is insufficient launch authority. | Users may want automatic reuse without flags. | Add a later signed/approved launch profile, not raw argv reconstruction in this slice. |
| A3 | Polling every few seconds is acceptable. | High-throughput queues may need event notification. | Add file watcher or daemon integration later. |
| A4 | Existing owner/cancel files are enough for local coordination. | Cross-machine shared state is not supported. | Keep scope local-only and document that boundary. |

## Self-Challenge

1. Laziest solution is a docs-only shell loop. Rejected because it would not give stable parse/status/test coverage.
2. Biggest false assumption risk is A1: foreground watch may not feel like background drain. Mitigation: name it explicit auto-drain worker and leave daemon scheduler as next phase.
3. YAGNI risk: too many scheduling knobs. Keep only interval, max-per-cycle, max-cycles, stop-when-empty, and JSON.
4. Failure-first case: agent command fails mid-cycle. Existing `DrainQueue` marks the item failed, leaves later prompts pending, clears owner, and returns error.
5. Repo pattern fit: CLI parse/execute tests and fixture binary smoke already cover ACP client features; follow that shape.

## Out of Scope

- Hidden daemon/background service ownership.
- Reconstructing or persisting agent launch argv as policy authority.
- Multi-session global scheduler.
- Extension hooks.
- New trust, sandbox, or per-agent policy engine.
- ACPX raw event-log or TypeScript runtime compatibility.
- Release/tag/Homebrew publish.
