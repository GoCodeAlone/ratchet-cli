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

Provider-save callers may send a UUID operation ID. The daemon records the
committed result in `provider_operations` in the same SQL transaction as the
provider row and exposes a read-only operation query. The TUI polls that query
after ambiguous transport outcomes. New credentials use operation-versioned
secret names: create before commit, delete on rollback, switch the database
pointer atomically, then retire the previous secret after commit.

Rejected: alias deletion/restore cannot recover prior secrets; provider-list
reconciliation races and loses history; requiring transactional secret stores
would duplicate or exclude existing Workflow secret providers.

## Consequences

- Active credentials remain unchanged when SQL commit fails.
- Ambiguous responses reconcile across daemon restarts and later alias writes.
- Provider saves add one table, one RPC, and bounded operation retention.
- Crash-created unreferenced versioned secrets are inactive; cleanup can use the
  existing secret-provider `List`/`Delete` APIs without changing their contract.
- Reverting code is schema-compatible: old binaries ignore operation rows and
  continue resolving the exact secret name stored on each provider row.
