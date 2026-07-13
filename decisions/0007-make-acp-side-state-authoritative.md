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

Primary state is authoritative and projections follow it. An error-bearing
cancellation check reads session status; `cancel_requested` or `canceled` stops
work, and read errors stop unattended execution. Session status commits before
the compatibility sidecar. Projection failure is degraded, never rolled back;
reconciliation creates missing projections and removes orphans before downgrade.

Transition listings provide IDs only. Reconciliation acquires the session lease,
reloads the current entry, treats missing as a no-op, and never writes after
release. Audit newline is the commit marker. Audit `Read` and `Append` both repair
only a non-newline suffix under lock, reject malformed committed records, require
known actions and required fields, and ignore unknown JSON fields.

Secure append pins one canonical parent handle for lock and data opens, rejects
final links/non-regular/multiply-linked/unowned files, and applies owner-only
permissions. Audit repair is not shared with raw event logs. Launch trust hashes
the existing deterministic payload with exact command/cwd, ordered args, sorted
env keys, and existing nil/empty encoding; legacy mismatches require retrust.

Rejected: best-effort rollback cannot prove paired state; moving this local state
to SQLite would expand migration and operational scope.

## Consequences

- Crash recovery has one authority per decision.
- Older workers receive a best-effort sidecar after primary commit; projection
  failure is explicit and cannot guarantee cancellation to an old process.
- A final unsynced JSONL fragment may be discarded; complete records remain strict.
- Downgrade requires quiescence plus cancel-projection reconciliation.
- AIX remains compile-supported but process-locked mutations fail before writing.
- No data migration is introduced.
