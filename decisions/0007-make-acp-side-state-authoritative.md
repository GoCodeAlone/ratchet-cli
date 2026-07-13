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

Primary state is authoritative and projections follow it. One guarded session
transition function owns enqueue, lifecycle start/writeback, stale recovery,
queue claim/completion/failure, and cancellation. Cancellation is sticky:
post-latch enqueue/start/claim reject, recovery cancels work, writeback preserves
the latch, and the terminal transition is `cancel_requested -> canceled`.

The error-bearing cancellation watcher captures one first cause. Authority or
ACP-cancel errors immediately cancel prompt/process contexts. A cancel request
is sent once, gets a bounded grace period, then force-kills and reaps an ignoring
child. The watcher joins before return; any captured cause wins over a racing
prompt result. Session status commits before the compatibility sidecar.
Projection failure is degraded, never rolled back.

Projection reconciliation holds the sessions lock, reloads current state, then
creates missing sidecars and removes orphans. Sidecars notify legacy readers on
a best-effort basis only. Backward binary downgrade after release is unsupported;
operators stop/disable policies with the current binary and recover by publishing
an upgrade-forward patch. Pre-release branch reversion remains supported.

Transition listings provide IDs only. Reconciliation acquires the session lease,
reloads the current entry, treats missing as a no-op, and never writes after
release. Audit newline is the commit marker. Audit `Read` and `Append` both repair
only a non-newline suffix under lock, reject malformed committed records, require
known actions and required fields, and ignore unknown JSON fields.

Audit state and its lock live in one dedicated owner-only namespace. One pinned
parent handle provides lock, read-repair, and append; final links, non-regular,
multiply-linked, or unowned files are rejected before each mutation. Audit repair
is not shared with raw event logs. Append is idempotent by complete-record
equality and classifies errors after newline write as commit-unconfirmed; retry
reads/repairs first. Required records have nonzero time and nonempty
session/profile/hash/outcome. Allowed outcomes are action-specific.

Profile operations use a process lock. Stored-profile resolution retains that
trust lease through durable policy/audit commit. Every actual child start then
reacquires the profile lock, verifies the pinned hash/trust, calls `exec.Cmd.Start`
synchronously, and releases only after start success/failure acknowledgement.
Launch trust
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
- Released backward downgrade is unsupported; rollback is upgrade-forward.
- AIX remains compile-supported but process-locked mutations fail before writing.
- No data migration is introduced.
