# 0008. Close ACP State Authority

**Status:** Superseded by 0009
**Date:** 2026-07-13
**Decision-makers:** project maintainers
**Related:** `decisions/0007-make-acp-side-state-authoritative.md`, `docs/plans/2026-07-10-ratchet-cli-provider-drain-managed-hooks-design.md`

## Context

ADR 0007 selected primary-state authority but left whole-record writers,
interleaved audit retry, cancellation deadlines, Windows parent replacement,
and released rollback mechanically open.

## Decision

Every session writer uses the guarded transition; insert/import paths are
create-only and reject collisions. Cancellation observation first records
`ErrCancelRequested`; ACP cancel-send is deadline-bound and all watcher/send
goroutines join after kill/reap. Audit records carry a deterministic `recordId`
over their immutable canonical fields and append deduplicates committed IDs.
Windows opens a no-`FILE_SHARE_DELETE` parent, pins `FileIdInfo`, and validates
children opened with `FILE_FLAG_OPEN_REPARSE_POINT`.

Released state is upgrade-forward-only. Pre-release source revert is allowed;
post-release patches retain authority-aware readers/writers. Rejected:
last-record-only audit deduplication and path revalidation without replacement
exclusion, because interleaving and retarget races remain.

## Consequences

- Writer inventory and process-shaped tests become release gates.
- Retry preserves one immutable record, including `at` and `recordId`.
- Older binaries must not open released authority-aware state.
