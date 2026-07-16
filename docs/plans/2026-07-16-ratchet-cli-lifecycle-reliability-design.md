# Ratchet CLI Lifecycle Reliability Design

**Status:** Approved
**Date:** 2026-07-16
**Scope:** Provider cleanup completion/diagnostics, provider-operation shutdown coverage, ACP cancellation teardown, and real-process smoke isolation.

## Context

The released `v0.30.35` tree passes the full serial suite, 30 repeated provider-cleanup runs, 50 repeated ACP cancellation/reap runs, and 20 repeated trusted-profile real-start runs. The remaining follow-ups are load-sensitive lifecycle gaps:

- `TestProviderCleanupDispatcherFairness` observes the secret-provider delete hook before the worker removes its durable cleanup row.
- cleanup candidate query failures are silent; scan/iteration failures discard a secondary `rows.Close` error.
- ACP process cancellation sleeps for a fixed 100 ms after writing the cancellation notification even though ACP defines no peer acknowledgment.
- real-process cancellation and trusted-profile launch tests run inside the race/coverage job, where process startup and fixed five-second acknowledgments compete with the whole suite.
- post-stop `GetProviderOperation` is covered through the manager rather than the service boundary; concurrent stop during startup reconciliation is not covered.

## Global Design Guidance

Source: `docs/design-guidance.md`

| Guidance | Design response |
|---|---|
| Prefer bounded failure and inspectable state. | Return cleanup dispatch errors, preserve close failures, and use the durable cleanup row as completion authority. |
| CLI, daemon, and provider contracts share one authority. | Add service-boundary shutdown coverage without adding a second operation state model. |
| Isolate heavyweight real-binary smoke from race/coverage. | Run ACP process lifecycle proofs in a dedicated CI step and retain in-process race coverage. |
| Preserve Windows, macOS, and Linux paths. | Keep process behavior portable; cross-build both Windows architectures and require existing native Windows jobs. |
| Do not duplicate SDK, plugin, or secret custody. | Continue using `acp-go-sdk`, Workflow `secrets.Provider`, and `secrets.Redactor`; add no integration or secret store. |

## Approaches

| Approach | Trade-off | Decision |
|---|---|---|
| Tests only | Smallest diff, but leaves silent cleanup errors and arbitrary runtime grace. | Reject. |
| Target lifecycle boundaries | Fixes observable runtime defects and makes tests causal without adding public API. | Choose. |
| General lifecycle event/observer API | Could support future diagnostics, but adds public surface and synchronization ownership without a consumer. | Reject as YAGNI. |

## Design

### Provider Cleanup

- Change cleanup dispatch to return an error. The loop logs a stable operation label plus the error; secret names and credential values remain absent.
- Rate-limit repeated equivalent dispatch failures to one log per minute and clear the suppression state after a successful dispatch. A persistent database outage must not emit on every 250 ms tick.
- Extract candidate-row collection behind the minimal `Next`/`Scan`/`Err`/`Close` shape. Always close rows and return `errors.Join(primary, closeErr)` so a secondary close failure is preserved.
- Treat the SQL row removal as cleanup completion in the fairness regression. The test waits for both provider deletion and zero durable cleanup rows before asserting attempts/concurrency.
- Keep the worker count, retry schedule, secret naming, and durable schema unchanged.

### Provider Operation Lifecycle

- Exercise post-stop `GetProviderOperation` through `Service.GetProviderOperation`; expect `codes.Unavailable` before any database access.
- Add concurrent `Stop` coverage after `Start` has entered `reconcileStartup`, using a blocking secret read on an applied operation. `Stop` must cancel startup, `Start` must return a context-classified error, and shutdown must join.
- No RPC, protobuf, state, or error-code changes.

### ACP Cancellation And Process Smoke

- ACP cancellation remains one bounded notification attempt followed by transport closure and process kill/reap. Remove the fixed 100 ms sleep after `ClientSideConnection.Cancel` returns; notification write completion is the strongest available transport signal because ACP cancellation has no response.
- Keep exact-once peer handling assertions in the in-process pipe test, where write completion is synchronized with the reader. The real OS-process test proves prompt return and process reap, not peer handling before forced termination.
- Rename/collect real-process lifecycle tests under a stable binary-smoke selector. Run them once in a dedicated, bounded Linux CI step without race instrumentation; skip only that selector in the race/coverage command.
- Replace the trusted-profile helper's fixed five-second start wait with a shared 30-second process-smoke bound. The dedicated job retains an outer five-minute package timeout, so a deadlock still fails without competing with race/coverage load.
- Run the same real-process lifecycle selector on the existing `windows-2025` job. This adds a step but does not add or change runners.
- Keep in-process cancellation, blocked-send, queue, lock, and persistence tests in the race job. Keep native Windows persistence, daemon lock, release startup, and ConPTY jobs unchanged.
- Add a releaseguard assertion that the dedicated smoke command and matching race skip remain paired.

## Error Handling

| Failure | Result |
|---|---|
| Cleanup candidate query | Dispatch returns wrapped error; loop logs classification. |
| Scan/iteration plus close failure | Joined error retains both failures. |
| Cancellation write failure | Error joins with teardown result; process still killed/reaped. |
| Agent ignores cancellation | Transport/process teardown completes without waiting for a nonexistent ACP acknowledgment. |
| Startup reconcile canceled by stop | `Start` returns a context-classified failure; `Stop` joins startup. |
| Dedicated smoke timeout | Linux and Windows CI fail independently without consuming race/coverage timeout. |

## Security Review

- No auth/authz boundary changes.
- Cleanup diagnostics contain operation labels and driver errors only; they must not include secret names, values, provider request payloads, command environments, or raw ACP prompt text.
- Cancellation continues closing every owned transport and reaping the child. Removing grace reduces the interval in which an untrusted child remains alive after cancellation.
- No new dependency or executable trust boundary.

## Infrastructure Impact

- CI only: one additional Linux Go test step, one step on the existing Windows job, and a narrower race skip for named real-process tests.
- No cloud resources, storage schema, migration, IAM, network listener, secret, runner, or production deployment change.
- Existing Windows hosted jobs remain required; this design does not change runners.

## Multi-Component Validation

| Boundary | Proof |
|---|---|
| cleanup worker + SQLite + secret provider | Fairness test observes retry, bounded concurrency, secret deletion, and durable row removal. |
| service + provider operation manager | Post-stop service call returns `Unavailable`; reconcile-time stop cancels and joins startup. |
| ACP SDK + OS process | Linux and Windows dedicated smoke launch the real test child and prove bounded cancellation attempt, prompt return, and process reap; in-process tests prove exact peer handling. |
| trusted profile + process lock + real agent | Linux and Windows dedicated smoke prove the profile process lock remains held through real `Start` success/failure acknowledgment. |
| CI contract | releaseguard rejects a missing dedicated smoke or mismatched race skip. |

Declared integrations:

| Integration | Class | Validation |
|---|---|---|
| `github.com/coder/acp-go-sdk` | runtime-integrated | real child process plus in-process exact-once cancellation tests |
| Workflow `secrets.Provider` | runtime-integrated | provider cleanup service test with durable SQLite state |
| GitHub Actions | config-only | releaseguard workflow assertions and Linux/native-Windows PR check execution |

## Assumptions

| ID | Assumption | Challenge | Fallback |
|---|---|---|---|
| A1 | ACP cancel is a notification with no response/acknowledgment. | A future protocol version may add one. | Adopt the SDK acknowledgment when available; retain bounded teardown fallback. |
| A2 | `exec.Cmd.Wait` completion is the process-reap authority on supported OSes. | Platform-specific process groups may outlive the direct child. | Add process-group/job-object ownership in a separate design if a reproducer demonstrates descendants. |
| A3 | Cleanup-row deletion is the durable completion signal. | Delete can fail after secret removal. | Existing startup retry remains authoritative; dispatch diagnostics expose the failure. |
| A4 | In-process cancellation tests provide sufficient race coverage for notification ownership. | OS pipe behavior differs from `io.Pipe`. | Dedicated real-process smoke covers OS behavior without race load. |
| A5 | Existing provider/ACP public contracts are correct. | A consumer may need explicit lifecycle events later. | Add a consumer-driven API design; do not preemptively expose internals. |
| A6 | A 30-second isolated process-start bound is sufficient on hosted Linux and Windows. | Runner degradation may exceed it. | Preserve the outer five-minute job timeout and diagnose runner health before increasing the per-transition bound. |

## Self-Challenge

1. **Simplest solution:** Polling SQL and increasing timeouts would reduce flakes, but would leave silent cleanup errors and runtime delay; the targeted design is the minimum that fixes both behavior and evidence.
2. **Fragile assumption:** A1 means Ratchet cannot prove the child handled cancellation before kill. The contract is therefore explicitly notification-attempt plus bounded termination, not peer acknowledgment.
3. **Partial failure:** Cleanup can remove a secret and fail to remove its queue row. Startup/retry is idempotent through `secrets.ErrNotFound`, and the new joined error is observable.

## Adversarial Review Resolutions

- Repeated cleanup errors are transition/rate-limited rather than logged every tick.
- The fixed five-second real-start wait is replaced explicitly, not merely moved to another job.
- Real-process lifecycle proof runs on existing Linux and Windows runners; cross-compilation is not treated as native proof.

## Backport 2026-07-16: ACP Handler Scheduling

The in-process pipe synchronizes notification bytes with the peer reader, but
`acp-go-sdk` dispatches the decoded notification to `handleInbound` in a new
goroutine. Stress evidence observed the handler count before that goroutine ran.
Runtime behavior remains immediate bounded teardown because ACP has no cancel
acknowledgment; in-process exact-once tests now wait on an explicit peer-handler
barrier. The Scope Manifest is unchanged.

## Non-Goals

- New daemon lifecycle RPCs, metrics backend, log framework, retry policy, worker count, or cleanup schema.
- ACP protocol changes, process-tree management, arbitrary scheduler work, or runner changes.
- GitHub Actions major-version maintenance; that remains the next independent follow-up.

## Rollback

- Revert the implementation commit and restore the prior CI selector. No data rollback or migration is required.
- Cleanup rows and provider operations remain forward/backward compatible.
- If immediate ACP termination exposes an agent compatibility defect, restore the bounded grace in a patch release while collecting a protocol-level reproducer; do not weaken process reap guarantees.

## Release

Two PRs: one implementation PR, then one post-merge retro/plan-closeout PR containing evidence that can only exist after the first merge and release. For each PR, wait for green required checks and resolved review threads, admin squash-merge, publish the next patch release from the merge commit, verify every archive/checksum plus Homebrew, and run time-bounded installed `ratchet --version` and provider catalog probes.
