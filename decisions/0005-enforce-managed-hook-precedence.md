# ADR 0005: Enforce Managed Hook Policy After All Sources Merge

## Status

Accepted

## Context

Ratchet loads user and project hooks before plugin hooks are merged. Existing
local trust and disable state cannot express administrator-owned hooks or a
managed-only rule, and filtering before plugin load would be bypassable.

## Decision

Load optional administrator-owned policy from a fixed platform path, preserve
managed provenance, and enforce additive or managed-only precedence after
plugin hooks merge. A missing file means no policy; a malformed present file
fails closed. Managed hooks cannot be changed by local trust/disable commands.
Present policy must be a non-symlink regular file with root-only mutation on
Unix or Administrators/SYSTEM-only mutation on Windows.

Durably append a metadata-only `started` record before launching a managed
hook, then append a terminal record. A failed start append blocks execution;
terminal append failure is surfaced as degraded audit state. Do not record
command text, environment, payloads, output, or error text, and do not create
another redaction implementation.

## Consequences

- Plugin reload cannot bypass managed-only policy.
- Administrators can require a controlled hook set without a remote service.
- Hook loading and execution must surface policy/audit failures explicitly.
- Diagnostics expose source and suppression status while preserving secrets.

## Alternatives

- Seeding the local trust store does not enforce exclusivity or immutability.
- A remote management service adds infrastructure and is unnecessary for this
  local enforcement boundary.

## References

- `docs/plans/2026-07-10-ratchet-cli-provider-drain-managed-hooks-design.md`
