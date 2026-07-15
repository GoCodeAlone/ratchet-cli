# 0011. Expose Provider Applied State

**Status:** Accepted
**Date:** 2026-07-15
**Decision-makers:** project owner, implementation agent
**Related:** `decisions/0006-make-provider-saves-durable.md`, `docs/plans/2026-07-15-ratchet-cli-provider-operation-contract-design.md`

## Context

The provider-operation protobuf and both CLI/TUI consumers define `APPLIED`,
but the daemon projects internal `applied` rows as `PENDING`. A failed
query-triggered finalization also rewrites the response to `PENDING`. This makes
the public enum unreachable and hides the useful distinction between work that
has not mutated durable provider state and work awaiting post-apply finalization.

## Decision

The daemon exposes an internal `applied` row as public `APPLIED`, including when
a query-triggered finalization attempt fails. The row remains retryable: every
later `GetProviderOperation` call attempts finalization again, then returns
`COMMITTED` after success. `APPLIED` includes the existing non-secret result and
no raw finalization error. Startup also attempts APPLIED finalization, but a
secret-read failure leaves the row retryable and does not stop the daemon.
Permanent provider errors (`ErrInvalidKey`, `ErrUnsupported`, and
`ErrProviderInit`), database errors, context errors, and journal-invariant
failures still stop startup.

This supersedes only ADR 0006's statement that operation queries expose
`applied` as pending and its fail-stop startup finalization behavior. The
durable journal, startup secret-enumeration boundary, secret custody, and
terminal-state rules remain unchanged.

## Consequences

- Existing clients continue polling because CLI and TUI already classify both
  `PENDING` and `APPLIED` as unresolved.
- Automation can distinguish pre-apply uncertainty from retryable finalization.
- A transient secret-read failure cannot make the recovery RPC unavailable;
  infrastructure and invariant failures remain fail-stop.
- No protobuf, database, migration, or retention change is required.
- Rollback may restore the old projection without changing persisted rows or
  generated clients.
