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

All current CLI/TUI callers send a canonical UUID operation ID; the daemon
validates it and generates an ID only for legacy clients. A metadata-only
`provider_operations` journal uses pending/applied/committed/failed states and
first-write-wins replay. Alias or safe-shape reuse conflicts; the safe shape
contains no base URL, settings, credential, or credential hash. Fields excluded
from the shape are ignored on replay, never rewritten.

The operation ID is a unique key. Same-alias/same-shape `committed` returns its
stored result; `pending`/`applied` reports in-progress; `failed` stays terminal;
alias/shape mismatch returns a deterministic conflict. The safe shape is type,
model, max tokens, default flag, and credential-presence only.

The daemon independently generates a UUID secret version under reserved prefix
`provider-v2-`. It journals pending, writes through the existing
`secrets.Provider`, then atomically commits the provider pointer, cleanup entry,
non-secret result, and `applied` state. Rollback deletes only the inactive new
version. Post-commit code registers redaction, invalidates the provider cache,
marks `committed`, then retires queued old secrets. Operation queries expose
`applied` as pending. No client input forms a secret key.

`provider_secret_cleanup` stores only server secret name, attempt count,
classified outcome, and timestamps. Startup runs before RPCs: secret `List`
failure is fatal; prior pending rows become failed; applied rows are finalized;
unreferenced reserved-prefix keys are queued; not-found deletion succeeds;
other deletion failures remain queued. Runtime cleanup rechecks that a key is
unreferenced before delete. Provider update/removal queues the prior key in its
SQL transaction. Legacy-prefix keys are touched only when a row explicitly
retires them.

The operation RPC exposes only ID, alias, state, classified failure, timestamps,
expiry, and existing non-secret `Provider`. Clients poll pending/not-found with
bounded backoff; unresolved state pauses exit. Rejected: alias restore cannot
recover secrets; list reconciliation races; transactional-store requirements
would exclude existing Workflow providers.

## Consequences

- Active credentials remain unchanged when SQL commit fails.
- Ambiguous responses reconcile across restart and later alias writes; terminal
  rows live 24 hours, while client reconciliation is bounded to 10 seconds.
- Startup, before RPC acceptance, finalizes `applied` rows whose provider points
  at their secret version, marks prior `pending` rows failed, then serially
  sweeps unreferenced reserved-prefix secrets plus durable cleanup entries.
- Guarantees are logical/process-crash atomicity within the existing provider
  contract; storage power-loss durability is not newly claimed.
- Old redaction values may remain over-redacted until restart; secret values are
  never removed from the redactor in-process.
- Reverting code is schema-compatible: old binaries ignore operation rows and
  resolve exact versioned names. Legacy empty-ID writes remain accepted; an old
  writer may leave a v2 orphan that a later upgraded startup safely sweeps.
