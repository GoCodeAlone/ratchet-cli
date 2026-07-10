# 0006. Make Provider Saves Durable

**Status:** Accepted
**Date:** 2026-07-10
**Decision-makers:** project owner, implementation agent
**Related:** `decisions/0003-centralize-provider-setup.md`, `docs/plans/2026-07-10-ratchet-cli-provider-drain-managed-hooks-design.md`

## Context

`AddProvider` upserts by alias, while secret stores have no transaction API.
Cancel compensation can delete prior configuration; alias-stable secret writes
can mutate active credentials before database commit; list-after-timeout cannot
prove which request committed.

## Decision

All current CLI/TUI callers use a new `CommitProviderSave` RPC and send a
canonical UUID operation ID; old daemons return `Unimplemented` before mutation.
Legacy `AddProvider` remains and delegates to the same durable implementation
with a server-generated ID. A metadata-only `provider_operations`
journal uses pending/applied/committed/failed states and unconditional
first-write-wins replay. Same-alias reuse returns the first result regardless of
later payload; another alias conflicts. UUID randomness, not persisted request
fingerprints, prevents accidental reuse.

The daemon independently generates a secret version as
`provider-v2-<unix-seconds>-<uuid>` using server time and UUID only. It journals
pending with that secret reservation, writes through the existing
`secrets.Provider`, then atomically commits the provider pointer, cleanup entry,
non-secret result, and `applied` state. Rollback deletes only the inactive new
version. Post-commit code registers redaction, invalidates the provider cache,
marks `committed`, then retires queued old secrets. Operation queries expose
`applied` as pending. No client input forms a secret key.

Post-commit finalization uses a daemon-owned bounded context. Operation queries
also finalize `applied` rows before returning, so RPC cancellation does not
strand them until restart.

A daemon-wide provider-mutation mutex serializes save, remove, finalization, and
cleanup; configuration writes are rare. Before a later alias mutation, any
applied row for that alias must finalize or the mutation fails. Cleanup excludes
every provider pointer and pending/applied secret reservation.

`provider_secret_cleanup` stores only server secret name, attempt count,
classified outcome, and timestamps. Startup runs before RPCs: secret `List`
failure is fatal; applied rows are finalized; inherited pending rows become
failed; unreferenced reserved-prefix keys are queued. Not-found deletion succeeds;
other failures remain queued. Runtime cleanup rechecks references before delete.
Provider update/removal queues the prior key in its SQL transaction.
Legacy-prefix keys are touched only when a row explicitly retires them.

The operation RPC exposes only ID, alias, state, classified failure, timestamps,
expiry, type, model, and default flag; it omits base URL, settings, credentials,
requests, and raw errors. Clients poll pending/not-found with bounded backoff;
unresolved state pauses exit. Rejected: alias restore cannot recover secrets;
list reconciliation races; transactional-store requirements would exclude
existing Workflow providers.

## Consequences

- Active credentials remain unchanged when SQL commit fails.
- Ambiguous responses reconcile across restart and later alias writes; terminal
  rows live 24 hours, while client reconciliation is bounded to 10 seconds.
- First-write-wins idempotency lasts for that 24-hour retention window. Expired
  or unresolved work must use a new UUID.
- CLI saves use signal-aware 30-second calls and a separate 10-second
  reconciliation context. First interrupt prints reconciliation status; a
  second exits nonzero with the operation ID, queryable through
  `ratchet provider operation <id>`.
- The daemon acquires and retains an OS-level exclusive data-directory lock
  before PID/socket cleanup, migration, or reconciliation. Unix `flock` and
  Windows `LockFileEx` prevent concurrent owners; crash closes release the lock.
- Startup, before RPC acceptance, finalizes `applied` rows whose provider points
  at their secret version, fails inherited `pending`, then serially sweeps
  unreferenced reserved-prefix secrets and cleanup entries.
- Guarantees are logical/process-crash atomicity within the existing provider
  contract; storage power-loss durability is not newly claimed.
- Old redaction values may remain over-redacted until restart; secret values are
  never removed from the redactor in-process.
- Reverting code is schema-compatible: old binaries ignore operation rows and
  resolve exact versioned names. Legacy empty-ID writes remain accepted; an old
  writer may leave a v2 orphan that a later upgraded startup safely sweeps.
