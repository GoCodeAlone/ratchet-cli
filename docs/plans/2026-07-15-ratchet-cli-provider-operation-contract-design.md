# Ratchet CLI Provider Operation Contract Design

**Status:** Approved
**Date:** 2026-07-15
**Decision:** `decisions/0011-expose-provider-applied-state.md`

## Goal

Close three review gaps from the durable provider-save work:

1. make the existing public `APPLIED` state reachable;
2. preserve that state when on-read finalization fails and retry on later reads;
3. make duplicate canonical provider-type diagnostics accurate and stable.

No new operation state, storage format, or provider integration is introduced.
The README documents the reachable lifecycle so human and automation users can
interpret status output without reading protobuf source.

## Global Design Guidance

Source: `docs/design-guidance.md`

| Guidance | Design response |
|---|---|
| Shared CLI/TUI/daemon contract | Daemon emits the state already defined and handled by both clients. |
| One durable-state authority | SQLite journal remains authoritative; protobuf is a faithful projection. |
| Existing secrets/redaction | Existing `secrets.Provider` and redactor paths remain unchanged. |
| Real consumer proof | Exercise `GetProviderOperation` through a real gRPC client and file secret provider. |
| Native/release proof | Run full CI, Windows cross-build/native checks, release archives, Homebrew, and installed startup probes. |

## Approaches

| Option | Trade-off | Decision |
|---|---|---|
| Expose existing `APPLIED` | Compatible, truthful, no schema churn; clients still poll | Selected |
| Remove `APPLIED` from protobuf | Makes current projection internally consistent but breaks the public contract | Rejected |
| Add `FINALIZING` | More vocabulary without a distinct durable phase or user action | Rejected (YAGNI) |

## Contract And Flow

`providerOperationStatePB` maps each durable state one-to-one. `get(..., true)`
still attempts finalization for `applied`; success re-reads the row as
`COMMITTED`, while failure returns the originally queried `APPLIED` response.
The response retains its non-secret result. A later query retries finalization.
An `APPLIED` response has `failure=UNSPECIFIED`; finalization details remain
internal. The durable row must still be `applied` after a failed attempt.

The catalog validator reports a duplicate canonical type as `duplicate provider
type <type>`. Ownership remains in alias/name collision errors, where it carries
distinct information; it is removed from the same-type diagnostic.

## Failure Handling

| Failure | Public behavior | Durable behavior |
|---|---|---|
| Secret unavailable during finalization | `APPLIED` + result, unspecified failure, no raw error | row remains `applied`; next query retries |
| Later finalization succeeds | `COMMITTED` + result | row becomes `committed` |
| Unknown persisted state | `UNSPECIFIED` | unchanged |
| Duplicate canonical type | deterministic validation error | no mutation |

## Security Review

- No credential, secret name, settings, base URL, or raw provider error is added
  to operation responses, logs, or diagnostics.
- The real-boundary regression uses a sentinel credential and asserts the wire
  payload excludes it.
- Authorization is unchanged: this is an existing local daemon RPC and no new
  network surface or trust decision is added.
- Dependency and secret-custody boundaries do not change.

## Infrastructure Impact

None. No cloud resource, network listener, queue, database column, migration,
secret store, IAM rule, or deployment order changes. The local daemon reads and
updates the same journal row and existing secret provider. Production approval
is not required.

## Multi-Component Validation

| Integration | Class | Proof |
|---|---|---|
| Catalog validator | runtime-integrated | focused exact-error unit regression |
| Journal to protobuf projection | runtime-integrated | focused daemon state mapping test |
| Built CLI to daemon to file secret provider | runtime-integrated | restart with an `applied` row and unavailable secret; daemon remains available; query returns `APPLIED`; add secret, rerun, observe `COMMITTED` |
| CLI and TUI reconciliation | runtime-integrated, unchanged | existing tests prove both poll `APPLIED`; rerun focused packages and full suite |
| README lifecycle | config-only | docs guard plus exact state/recovery wording review |
| Release archives/Homebrew | runtime-integrated | post-merge tag, checksums, six archives, installed time-bounded commands |

## Assumptions

| ID | Assumption | Challenge | Fallback |
|---|---|---|---|
| A1 | Existing clients treat `APPLIED` as unresolved | A client could fail on the newly reachable enum | Repo CLI/TUI tests and protobuf contract prove support; no external compatibility claim beyond protobuf semantics |
| A2 | Repeated `Get` may safely retry finalization | Side effects could be non-idempotent | Existing finalizer checks durable state and performs cache/redactor updates before one state update; tests cover retry to commit |
| A3 | Result is valid once durable state is `applied` | Partial SQL apply could omit result | Apply writes provider pointer, result, and state in one SQL transaction; regression asserts result fields |

## Self-Challenge

1. Simplest fix is only changing the mapping; insufficient because the explicit
   fallback rewrite would still hide `APPLIED` after the most important failure.
2. A2 is most fragile; the design therefore proves failure then successful
   retry through the real service boundary.
3. No new status, retry setting, migration, UI, or provider behavior is added.

## Out Of Scope

- Changing the advisory cancellation semantics of existing secret providers.
- Returning raw or newly classified finalization errors to clients.
- Adding database columns, RPCs, operation states, or retry settings.
- Claiming a Windows daemon runtime while production daemon IPC remains
  Unix-only; portable mapping tests and native Windows build/check gates remain.

## Rollback

Revert the daemon projection and diagnostic commits. Persisted `applied` rows,
protobuf clients, and database schema remain compatible; subsequent startup or
queries still finalize them under ADR 0006 behavior. If a release regresses,
install the prior Homebrew version or prior platform archive, then re-upgrade
after correction.

## Verification

- Focused catalog, daemon, CLI, and TUI tests.
- Real gRPC/file-secret failure-to-retry boundary test with payload redaction.
- `go test ./...`, `go test -race` using the merge-gating selector, `go vet
  ./...`, lint, generated-code check, Windows cross-build, and native CI.
- Exact merge commit release with archive/checksum/Homebrew/installed-binary
  verification.

### Backport 2026-07-15: Startup Must Preserve The Recovery Boundary

Cause: the initial smoke seeded `applied` after startup, bypassing
`reconcileStartup`; an unavailable finalization secret made restart fail before
the recovery RPC could serve.

Change: startup attempts finalization but retains an unsuccessful row as
`applied` and continues serving; the built-binary smoke now restarts before
querying.

Scope: no manifest change; this is required by Tasks 2-3 retry/restart behavior.

Evidence: `TestProviderOperationStartupKeepsUnfinalizedAppliedRetryable` failed
with `finalize provider operation: startup finalization unavailable` before the
fix and passes after it.
