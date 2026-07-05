# Policy Matrix

This document is the source-of-truth matrix for ratchet-cli policy surfaces:
permissions, sandboxing, trust, queue execution, extension points, and
per-agent scope. It documents existing behavior and the boundaries required
before later automation such as daemon background drain or mutation-capable
extension hooks.

## Scope

The matrix covers ratchet-cli-owned command, daemon, TUI, ACP client, MCP, team,
and retro surfaces. It does not define a new policy evaluator. Trust matching,
runtime trust decisions, and persistent trust grants continue to use
`workflow-plugin-agent/policy.TrustEngine` and
`workflow-plugin-agent/policy.PermissionStore`.

## Non-Goals

- Daemon background or scheduled ACP client drain.
- Config-file mutation from trust commands.
- New sandbox/path/network enforcement.
- Broad runtime extension SDK.
- Managed hook distribution or managed-only hook policy.
- Credentialed third-party agent CI.
- Go-native ACPX durable bundle compatibility is supported; `.flow.ts` source
  execution remains deferred.
- Local-first gateway or channel routing.

## Policy Precedence

These rules describe the intended policy order for documented surfaces. Partial
or deferred rows below must not be treated as fully enforced runtime behavior.

1. Hard deny and missing authorization wins.
2. Persistent deny grants beat persistent allow grants.
3. Runtime deny rules beat runtime allow rules at the same scope.
4. Runtime rules can narrow current process behavior but are not durable.
5. Static config remains the daemon startup baseline.
6. Unknown or missing policy falls back to explicit prompt or deny, not silent
   auto-approval.

## Layer Matrix

| Layer | Owner | Status | Rule | Validation |
|---|---|---|---|---|
| Static config trust rules | `internal/config` plus `workflow-plugin-agent/policy.TrustEngine` | Supported | Config provides the daemon startup baseline for trust mode and rules. Runtime and persistent changes do not rewrite config. | Config and trust-engine tests; docs guard checks this layer name. |
| Runtime trust rules | Daemon RPC, CLI, and TUI trust commands | Supported | `/mode`, `/trust allow`, `/trust deny`, `/trust list`, `ratchet trust allow`, `ratchet trust deny`, and `ratchet trust list` mutate or inspect daemon-local runtime state. | Daemon trust tests and `cmd/ratchet` docs guard. |
| Persistent trust grants | `workflow-plugin-agent/policy.PermissionStore` through daemon RPC | Supported | `ratchet trust persist allow|deny`, `/trust persist allow|deny`, `ratchet trust grants`, `/trust grants`, `ratchet trust revoke`, and `/trust revoke` manage durable grants; deny grants preserve deny-wins semantics. | Daemon and command tests for grant persistence; docs guard checks this layer name. |
| Permission prompts | Daemon permission gate and TUI prompt flow | Supported | Human approval remains explicit for unresolved decisions. Missing or unknown policy must not silently auto-approve. | Permission prompt tests and daemon/TUI behavior. |
| ACP client queue/drain | `internal/acpclient` | Explicit watch/drain only | `--no-wait` writes prompt text to a local FIFO queue; only operator-started `ratchet acp client drain` and foreground `ratchet acp client watch` commands execute queued prompts. `watch` requires an explicit `--command` or `--agent` launch target for each run and stops when the foreground command exits. No daemon background drain is supported. | ACP client binary smoke covers queue inspection, explicit drain, and explicit foreground watch. |
| ACP archive/compare/replay artifacts | `internal/acpclient` | Supported local-only | `ratchet acp client sessions export --history summary\|raw\|both`, `ratchet acp client sessions events`, `ratchet acp client compare --save`, and `ratchet acp client flow replay` read or write local raw ACPX event logs, archives, compare bundles, and Go-native ACPX replay bundles. They may include prompts, responses, summaries, JSON-RPC messages, action stdout/stderr, and path metadata; shared evidence must use counts and filenames, not payloads. | ACP archive, compare bundle, flow replay, binary smoke, and docs guard tests. |
| ACP launch profiles | `internal/acpclient` profile store plus plugin templates | Supported with local trust | `ratchet acp client profiles list`, `add`, `install`, `trust`, and `remove` manage local reviewed launch specs. Profiles store commands, args, cwd, and env key names only, not secret values. Built-in ACP agents win over profile names, profile names cannot shadow built-ins, and untrusted profiles do not resolve through `--agent`. Profile execution remains limited to explicit foreground `exec`, `drain`, `watch`, `compare`, and `flow run` commands. | ACP profile store, plugin template, command, watch/drain/compare, and flow tests cover trusted and untrusted resolution. |
| Release artifact gates | GoReleaser, GitHub Actions, and `internal/releaseguard` | Supported for release artifacts | GoReleaser snapshot release-check, draft release asset postcheck, tap preflight, generated-cask publish, and tap postcheck gates inspect release archives, checksums, generated cask material, draft metadata, and Homebrew tap path-changing commits before public release undraft. Windows ConPTY smoke covers the test-only smoke binary; packaged release `ratchet.exe` runtime remains deferred. | Releaseguard tests, CI `Release Check`, Windows ConPTY smoke, release workflow postchecks, and docs guard tests. |
| Sandbox/path/network controls | Agent plugin trust logic, mesh path guard, and future sandbox work | Partial | Existing trust decisions and mesh path guard cover only their implemented surfaces. ratchet-cli does not claim Codex/Claude-style full sandbox, network, or per-tool escalation parity. | Existing tests for implemented guards; future sandbox work needs a separate design. |
| Hooks/extensions | `internal/hooks`, plugin manifests, and future extension work | Supported with review/trust / Deferred | User hooks in `~/.ratchet/hooks.yaml` remain trusted by default for compatibility. Project hooks in `.ratchet/hooks.yaml` and plugin hooks are listed but skipped until exact descriptor hook trust is recorded with `ratchet hooks trust <hash>`. `ratchet hooks disable <hash>` overrides trust, changed hook descriptors require re-review, and plugin hook/profile paths must stay inside plugin roots. Managed hooks remain deferred. TypeScript extension SDK remains deferred, including tool registration, hot reload, lifecycle interception SDKs, and unreviewed local mutation. | Hook trust store, CLI, daemon wiring, plugin containment, and Windows command-selection tests. Future managed hooks and extension SDK work need a separate locked design. |
| Flow action nodes | `internal/acpclient` | Supported with explicit grants | JSON v1 `action` nodes run runtime-owned local commands only after `ratchet acp client flow run` receives `--allow shell`. Node working directories outside the flow base require `--allow outside-cwd`. Action stdout/stderr persisted in run bundles is sensitive local command output. This is flow-local preflight, not a new trust engine or full sandbox. | ACP client flow tests and binary smoke cover action nodes, missing grants, cwd escapes, and persisted outputs. |
| Retro/self-improvement | `internal/retro` and local project evidence routing | Partial | Retro evidence is opt-in. `ratchet retro analyze --evidence <file> [--session ID] [--json]` reports findings, local actions, and upstream instructions without mutating config or opening PRs. Automatic local mutation and upstream PR creation are disabled unless a future configurable policy enables them. | Retro analyzer, command, and config-gating tests. |
| Blackboard notification-event export | Daemon blackboard CLI | Supported local-only | `ratchet blackboard export [section] [--json\|--jsonl]` reads daemon blackboard entries and projects them into local notification-event records with `messaging.text`. `--workflow-messaging` adds `workflow-plugin-messaging-core` `step.messaging_send` handoff metadata with required downstream `channel` config. It does not post to Slack, Discord, Teams, webhooks, email, or any network provider, and it has no credential/channel flags. External delivery remains a Workflow messaging plugin responsibility. | Blackboard command tests cover section export, all-section export, JSONL output, Workflow metadata output, and rejected provider credential flags. |
| Per-agent/team scopes | Daemon team manager and mesh configs | Partial / Deferred | Team orchestration and MCP team messaging exist. Per-agent permission scopes, worktree isolation policy, and channel routing are future work. | Team and MCP tests for current behavior; future per-agent scopes need a separate design. |

## Sensitive Metadata

Trust rules, grant patterns, hook descriptors, launch profile names and
commands, queue contents, archive exports, raw ACPX event logs, compare
bundles, flow replay bundles, retro evidence, blackboard export records, flow
action output, and policy decisions are sensitive local policy metadata.
They can reveal local paths, command names, provider usage, project conventions,
prompts, responses, stdout/stderr, or operational habits. Do not expand logging,
exports, or public docs with raw policy values unless a future design includes
redaction and user consent.

Grant listings and archive files should be handled like local credentials or
conversation data:

- prefer local-only storage under the user's state directory;
- redact command/path/provider values in shared evidence;
- avoid exporting queued prompts or grant lists unless explicitly requested;
- avoid sharing blackboard export records unless the local coordination payload
  was reviewed for secrets and prompt context;
- keep retro evidence opt-in and redacted.

## Deferred Automation

The following work is intentionally blocked until a future locked design defines
authorization, cancellation, audit evidence, and redaction boundaries:

| Work | Status | Required policy decision |
|---|---|---|
| Background drain | Deferred | Define owner/session scope, cancellation semantics, prompt persistence warnings, and whether queued prompts can execute without a foreground operator. |
| Managed hooks | Deferred | Define administrative distribution, managed-only mode, local override behavior, and audit evidence before claiming managed hook parity. |
| Extension SDK | Deferred | Define TypeScript extension SDK boundaries, tool registration, hot reload, lifecycle interception, mutation opt-in, environment redaction, command/path access, and reviewable local changes. |
| Sandbox/path/network expansion | Deferred | Define enforced filesystem and network boundaries before claiming parity with Codex or Claude sandbox controls. |
| Per-agent policy scopes | Deferred | Define how agent roles, teams, worktrees, channels, and scopes compose with persistent grants and runtime rules. |
| Credentialed third-party agent CI | Deferred | Define secret handling, provider credentials, failure isolation, and artifact redaction. |
| ACPX flow source execution | Deferred | Ratchet supports Go-native ACPX durable bundle generation and replay validation through the shared `workflow-plugin-acpx` runtime while continuing to write bundles with `acpx-go`; executing `.flow.ts` files remains out of scope unless a future design defines source compatibility and runtime boundaries. |
| Local-first gateway/channels | Deferred | Define account/channel routing, inbox persistence, and sandboxing for non-main sessions. |

## Verification

The docs regression in `cmd/ratchet/harness_docs_test.go` must keep this matrix
visible from the public docs and must fail if required policy layers, statuses,
or sensitive local policy metadata warnings disappear.

Run:

```sh
go test ./cmd/ratchet -run HarnessEmulationDocs -count=1
```
