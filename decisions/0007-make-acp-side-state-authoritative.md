# 0007. Make ACP Side State Authoritative

**Status:** Accepted
**Date:** 2026-07-13
**Decision-makers:** project maintainers
**Related:** `decisions/0004-daemon-own-background-drain.md`, `docs/plans/2026-07-10-ratchet-cli-provider-drain-managed-hooks-design.md`

## Context

Task 6 review found that paired-file rollback, pre-lease transition snapshots,
and append-only JSONL without tail repair leave crash and concurrency windows.
Cancellation also has two apparent authorities: session state and a sidecar.

## Decision

Primary state is authoritative and projections follow it. Cancellation is a
sticky session-state transition: queue claim atomically rejects the latch, and
worker completion/failure cannot overwrite it. An error-bearing cancellation
callback propagates authority or ACP-cancel errors by canceling the prompt and
agent process contexts. Session status commits before the compatibility sidecar.
Projection failure is degraded, never rolled back.

Projection reconciliation holds the sessions lock, reloads current state, then
creates missing sidecars and removes orphans. A downgrade-preparation operation
requires zero owner/background leases, excludes new current-version owners,
reconciles projections, and records readiness; older processes must be quiesced.

Transition listings provide IDs only. Reconciliation acquires the session lease,
reloads the current entry, treats missing as a no-op, and never writes after
release. Audit newline is the commit marker. Audit `Read` and `Append` both repair
only a non-newline suffix under lock, reject malformed committed records, require
known actions and required fields, and ignore unknown JSON fields.

Audit state and its lock live in one dedicated owner-only namespace. One pinned
parent handle provides lock, read-repair, and append; final links, non-regular,
multiply-linked, or unowned files are rejected before each mutation. Audit repair
is not shared with raw event logs. Required records have nonzero time and
nonempty session/profile/hash/outcome; actions and their outcome sets are known.

Profile operations use a process lock. Stored-profile resolution retains that
trust lease through durable policy/audit commit and worker launch. Launch trust
hashes the existing deterministic payload with exact command/cwd, ordered args,
sorted env keys, and existing nil/empty encoding; legacy mismatches require
retrust.

Rejected: best-effort rollback cannot prove paired state; moving this local state
to SQLite would expand migration and operational scope.

## Consequences

- Crash recovery has one authority per decision.
- Older workers receive a best-effort sidecar after primary commit; projection
  failure is explicit and cannot guarantee cancellation to an old process.
- A final unsynced JSONL fragment may be discarded; complete records remain strict.
- Downgrade requires quiescence plus cancel-projection reconciliation.
- The downgrade readiness operation is part of the existing Task 8 ACP client
  command surface and does not add another daemon capability.
- AIX remains compile-supported but process-locked mutations fail before writing.
- No data migration is introduced.
