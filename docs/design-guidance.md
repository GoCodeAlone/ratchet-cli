# Design Guidance

**Status:** Active
**Last updated:** 2026-07-16
**Source:** workspace guidance;
`docs/retros/2026-07-15-provider-drain-managed-hooks-retro.md`;
`docs/retros/2026-07-16-ratchet-cli-lifecycle-reliability-retro.md`

## Product Direction

- Build a reliable, local-first agent harness whose CLI, TUI, daemon, and
  automation surfaces share authoritative domain contracts.
- Prefer explicit operator control, inspectable state, and bounded failure over
  hidden background behavior.

## Architecture Constraints

- Use Go and existing Workflow plugins, provider SDKs, secret providers, and
  `secrets.Redactor`; do not duplicate those integrations or secret custody.
- Keep one authority for durable state and derive compatibility projections
  from it. UI-specific code may present a contract but must not redefine it.
- Preserve supported Windows, macOS, and Linux release paths. Cross-compilation
  is build evidence only, not native runtime evidence.

## UX / Domain Principles

- Interactive and non-interactive paths for the same operation must expose the
  same capabilities, validation, lifecycle states, and recovery instructions.
- Commands that may wait on daemon, provider, hook, or agent work need visible
  progress, bounded cancellation, and a queryable reconciliation identifier.

## Quality / Security / Operations

- Do not merge while required checks or actionable review threads are pending
  or failing. Administrative merge permission does not waive this gate.
- Security, filesystem, terminal, and process behavior must run on the native
  operating system in CI. Cross-builds supplement but never replace that proof.
- Run the exact merge-gating race/coverage selector locally when practical.
  Isolate heavyweight real-binary process smoke from race/coverage jobs so one
  test cannot consume the suite timeout.
- Bound cleanup and join paths independently. A timeout-triggered cancel or
  process kill must not be followed by an unbounded `Wait` or channel receive.
- Keep credentials, request payloads, command environments, and raw provider
  errors out of durable metadata, logs, snapshots, audits, and PR evidence.
- Release only merge commits. Verify checksums, every declared platform
  archive, Homebrew state, and a time-bounded invocation of the installed
  binary before closing a feature.

## Infrastructure / Integration Impact

- Designs must name files, sockets, processes, network paths, plugins, secrets,
  migrations, and external services they create or mutate, including rollback
  and upgrade-forward constraints.
- Production deployment or destructive production changes still require the
  recorded approval applicable to that environment.

## Multi-Component Validation

- Exercise each runtime-integrated boundary through its real consumer: CLI or
  TUI to daemon, daemon to plugin/provider, and released archive or package to
  operating system.
- For asynchronous notifications, prove send completion and receiver handling
  separately; use an explicit receiver barrier when claiming handler behavior.
- Stateful flows prove results after restart or reload where feasible. Security
  boundaries include a negative authorization, trust, or mutation path.

## Non-Goals

- Reimplementing provider SDKs, cloud storage clients, secret stores, ACP/MCP
  transports, or Workflow plugin plumbing inside ratchet-cli.
- Claiming full platform support from compilation or test doubles alone.

## Evolution Triggers

- Revisit this guidance when supported operating systems, daemon ownership,
  provider/plugin boundaries, deployment model, or compliance requirements
  change, or when another retro identifies a repeated gate miss.

## Change Log

| Date | Source | Change |
|---|---|---|
| 2026-07-15 | `docs/retros/2026-07-15-provider-drain-managed-hooks-retro.md` | Established shared-contract, native-platform, settled-merge, exact-test, and release-runtime gates after repeated Windows and PR-monitoring misses. |
| 2026-07-16 | `docs/retros/2026-07-16-ratchet-cli-lifecycle-reliability-retro.md` | Added independent cleanup-join bounds and separate notification send/handler proof after ACP stress and PR review findings. |
