# ADR 0004: Let the Daemon Own Background ACP Drains

## Status

Accepted

## Context

ACP queue drain behavior already supports claims, cancellation, and stale
recovery, but a foreground command must remain attached. Detached shell
processes and platform schedulers would duplicate supervision and provide
inconsistent Windows behavior.

## Decision

The ratchet daemon supervises explicitly enabled background queue drains.
Policies pin a trusted built-in or stored ACP launch profile by name and
fingerprint. Start requires acknowledgement of unattended execution. Restart
resumes only an unchanged trusted profile; drift blocks launch. Terminal errors
stop automatic retries.

Persisted policy never contains arbitrary argv, prompt content, responses,
environment values, or credentials.

## Consequences

- Background drains share daemon lifecycle, queue ownership, cancellation, and
  cross-platform context handling.
- Users gain start/status/stop semantics and deterministic restart behavior.
- Daemon/protobuf/client surfaces grow to support the policy lifecycle.
- Operators must explicitly restart an errored or blocked policy.

## Alternatives

- Detached foreground commands and cron wrappers have weak supervision and
  portability.
- Persisting arbitrary launch commands creates an unattended command-execution
  store and is rejected.

## References

- `docs/plans/2026-07-10-ratchet-cli-provider-drain-managed-hooks-design.md`
