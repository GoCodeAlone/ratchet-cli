# ADR 0003: Centralize Provider Setup Metadata

## Status

Accepted

## Context

The provider CLI and TUI wizard define different provider lists and setup
behavior. The CLI currently exposes settings and providers that the TUI cannot
configure. Adding tests around two definitions would detect some drift but
would preserve duplicated product policy.

## Decision

Create one UI-agnostic provider setup catalog under `internal/provider`.
CLI and TUI adapters render catalog entries and delegate model/auth behavior to
existing provider packages. Test-only providers are excluded, accepted runtime
aliases map to canonical visible entries, and contract tests enforce coverage.

The catalog contains setup metadata, not provider SDK implementations or
credentials.

## Consequences

- CLI and TUI expose the same provider capability set.
- Adding a provider requires one catalog entry plus strategy implementation
  only when existing strategies are insufficient.
- Catalog validation becomes a CI boundary against plugin registry drift.
- The TUI needs a broader catalog-driven state machine rather than hardcoded
  provider branches.

## Alternatives

- A daemon-rendered wizard RPC couples presentation policy to protobuf.
- Separate definitions plus conformance tests preserve the duplication that
  caused the defect.

## References

- `docs/plans/2026-07-10-ratchet-cli-provider-drain-managed-hooks-design.md`
