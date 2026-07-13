# 0009. Pin ACP Event Authority

**Status:** Accepted
**Date:** 2026-07-13
**Decision-makers:** project maintainers
**Related:** `decisions/0008-close-acp-state-authority.md`, `docs/plans/2026-07-10-ratchet-cli-provider-drain-managed-hooks-design.md`

## Context

ADR 0008 left audit identity derived from visible metadata, ACP cancellation on
execution context, and profile resolution detached from process start. Those
choices permit event collisions, self-cancelled cancel sends, and trust races.

## Decision

Each audit-requiring transition persists a cryptographically random `eventId`
before append; `recordId` equals that immutable ID and retries reuse it. A true
cancel request records `ErrCancelRequested`, sends ACP cancel on an independent
bounded context, and cancels execution separately. Authority-read failures fail
closed without pretending the user requested cancellation.

Profile launch uses a process-lock-owning callback that revalidates trust and
holds the lease through synchronous `exec.Cmd.Start`. Released downgrade remains
explicitly unsupported and unenforced: accidental old-binary access is accepted
operator risk. Rejected: metadata-derived audit IDs and a copied-profile
resolver, because neither carries durable ownership.

## Consequences

- Transition, audit, cancellation, and launch tests must be process-shaped.
- Native Windows tests run under the existing `TestBackgroundWindows` CI gate.
- Post-release recovery is upgrade-forward; no downgrade compatibility barrier
  or migration is added.
