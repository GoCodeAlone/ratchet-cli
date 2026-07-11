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
descriptor hash. Start requires acknowledgement of unattended execution.
Restart resumes only a profile whose trust hash still equals its current
descriptor hash and the pinned policy hash; drift blocks launch. The general
profile registry enforces the same descriptor-bound trust rule. Terminal errors
stop automatic retries.

Persisted policy never contains arbitrary argv, prompt content, responses,
environment values, or credentials.

## Consequences

- Background drains share daemon lifecycle, queue ownership, cancellation, and
  cross-platform context handling.
- Users gain start/status/stop semantics and deterministic restart behavior.
- Daemon/protobuf/client surfaces grow to support the policy lifecycle.
- Operators must explicitly restart an errored or blocked policy.
- Test and smoke service constructors must inject a disabled manager so local
  persisted state cannot launch work during verification.

## Alternatives

- Detached foreground commands and cron wrappers have weak supervision and
  portability.
- Persisting arbitrary launch commands creates an unattended command-execution
  store and is rejected.

## References

- `docs/plans/2026-07-10-ratchet-cli-provider-drain-managed-hooks-design.md`
