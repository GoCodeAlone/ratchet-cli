# 0006. Make Provider Saves Durable

**Status:** Accepted (provider-state projection superseded by `0011`)
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

A daemon-owned executor serializes each alias from pending reservation through
terminal state while unrelated aliases proceed. Exactly one operation is
admitted per alias: same-ID calls attach to its result; another ID receives
classified `AliasBusy` immediately and its credential is not retained or
journaled. RPC handlers wait only until their context ends; a non-cancellable
secret `Set` continues with its reservation live. Replacement UUIDs are accepted
only after terminal state or startup recovery. Ownership entries then retire.

Worker boundaries recover panics, record classified failure without raw panic
text, and release ownership. A short provider-row mutex spans SQL apply through
terminal finalization and is shared by default/model/remove row mutations; it
never surrounds a secret-provider call.

`provider_secret_cleanup` stores only server secret name, attempt count,
classified outcome, and timestamps. Startup runs before RPCs: secret `List`
failure is fatal; applied rows are finalized; inherited pending rows become
failed; unreferenced reserved-prefix keys are queued. Startup discovery and
journaling complete before RPC acceptance; cleanup rows are unique by secret
name and persist `next_attempt_at` with bounded exponential retry. One due-row
dispatcher feeds at most two short workers; poison rows release slots and later
due rows still run. Not-found succeeds and other failures remain queued. Runtime
cleanup rechecks references before delete.
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
- Secret-provider contexts are advisory. Startup `List` is explicitly fail-stop:
  no RPCs are accepted until it returns, and operators may terminate a stalled
  process. Shutdown stops accepting work and leaves non-cancellable workers as
  pending for restart recovery instead of waiting without bound.
- The daemon acquires and retains an OS-level exclusive data-directory lock
  before PID/socket cleanup, migration, or reconciliation. Unix `flock` and
  Windows `LockFileEx` prevent concurrent owners; crash closes release the lock.
- Startup, before RPC acceptance, finalizes `applied` rows, fails inherited
  `pending`, and journals unreferenced reserved-prefix secrets. Physical deletion
  is asynchronous through the bounded cleanup pool.
- Guarantees are logical/process-crash atomicity within the existing provider
  contract; storage power-loss durability is not newly claimed.
- Old redaction values may remain over-redacted until restart; secret values are
  never removed from the redactor in-process.
- Reverting code is schema-compatible: old binaries ignore operation rows and
  resolve exact versioned names. Legacy empty-ID writes remain accepted; an old
  writer may leave a v2 orphan that a later upgraded startup safely sweeps.
- Downgrade requires stopping the new daemon and observing lock release before
  launching an older binary that does not participate in the OS lock.
