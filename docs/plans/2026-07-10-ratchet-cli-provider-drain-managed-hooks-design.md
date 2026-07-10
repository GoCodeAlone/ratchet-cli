# Ratchet CLI Provider, Drain, and Managed Hook Design

## Goal

Deliver the next three ratchet-cli improvements as separate, releasable changes:

1. Make provider and model setup one domain shared by the CLI and TUI so new
   providers, auth methods, settings, and model discovery cannot drift.
2. Let the daemon supervise explicitly enabled background ACP queue drains for
   trusted agent launch profiles.
3. Add OS-managed hook policy with immutable precedence, managed-only mode, and
   metadata-only execution auditing.

The design preserves ratchet's local-first architecture. It does not add a
remote control plane, another provider SDK, another secret store, or detached
shell scheduling.

## Current State

The CLI provider path supports AWS Bedrock settings, compatible/custom
endpoints, settings-aware model discovery, manual model fallback, ChatGPT
device auth, and CLI-backed providers. The TUI onboarding wizard has a separate
five-provider table, drops provider-specific settings, and uses a different
model discovery call. Users therefore see different capabilities depending on
which entry point they choose.

ACP queue draining already has ownership, cancellation, and stale-recovery
semantics, but it only runs while a foreground command is attached. The daemon
already owns durable lifecycle work and is the correct supervisor for optional
unattended drains.

Hooks can come from user, project, and plugin sources. Trust is local and
project-oriented; there is no administrator-enforced policy, managed-only
mode, or durable execution audit.

The intermittent long-sequence TUI smoke-test shutdown assertion discovered
during baseline verification is handled by a prerequisite PR. Fresh-process
exit tests own shutdown behavior; the long all-surfaces test owns navigation
and rendering coverage.

## Global Design Guidance

Sources: repository README and `docs/policy-matrix.md`, plus
`GoCodeAlone/workspace` `docs/design-guidance.md`.

| guidance | design response |
|---|---|
| Keep Go as the primary implementation language. | All catalog, daemon, policy, CLI, and TUI work remains Go-native. |
| Reuse provider/plugin SDKs and existing secret infrastructure. | Model listing continues through `workflow-plugin-agent/provider`; provider credentials remain in the daemon's existing secrets provider and existing `secrets.Redactor`. |
| Never log secrets or sensitive prompt content. | Background state stores queue/profile metadata only; hook audit stores hashes and outcomes only. |
| Audit privileged or unattended behavior. | Background policy changes and managed hook execution produce redacted, local JSONL metadata records. |
| Prefer daemon-owned lifecycle and explicit policy. | The daemon owns workers; CLI commands only mutate/query policy through RPC. |
| Support Windows as a release target. | No Unix process groups, shell detachment, cron, or POSIX-only managed paths are assumed. Windows tests and release smoke remain required. |
| Merge only after local verification and green PR checks. | Each implementation PR has focused tests, full repository tests, runtime proof, CI monitoring, merge, and a release tag. |

## Architecture

### 1. Shared Provider Setup Catalog

Add a UI-agnostic catalog under `internal/provider`. A catalog entry owns:

- canonical provider type and accepted aliases;
- display name, category, and description;
- auth strategy and setup strategy;
- API-key environment name, when applicable;
- base-URL policy and default URL;
- typed provider-setting fields, including secret presentation metadata;
- model discovery strategy and manual-model fallback policy;
- setup-guide fields used by human and JSON CLI output.

The catalog covers every user-selectable runtime provider registered by
`workflow-plugin-agent`, excluding test-only `mock`. Duplicate runtime aliases,
such as `bedrock` and `anthropic_bedrock`, resolve to one canonical visible
entry while remaining accepted on input.

The current orchestrator registry keeps its factory map private, so a small
upstream prerequisite adds a sorted, defensive-copy `ProviderTypes()` query to
`workflow-plugin-agent/orchestrator.ProviderRegistry`. That package remains the
runtime source of truth. Ratchet's catalog coverage test compares against the
exported runtime set after excluding documented test-only types; it does not
duplicate an expected provider list in a test.

Provider-specific network calls and SDK behavior remain in
`workflow-plugin-agent/provider`. Ratchet supplies settings and renders
responses; it does not implement AWS, OpenAI, Anthropic, or model APIs.

The CLI adapter and Bubble Tea wizard both consume catalog entries. Neither may
define a second provider list. Contract tests compare catalog coverage to the
CLI setup surface and TUI navigation surface.

#### TUI Flow

The TUI wizard becomes a catalog-driven state machine:

1. Choose a categorized provider from a scrollable, filterable list.
2. Complete only the auth/setup path declared by the entry.
3. Enter declared settings and optional base URL.
4. Run settings-aware model discovery asynchronously.
5. Choose a discovered model or enter one manually when allowed.
6. Review non-secret values and submit once to the daemon.

Secrets remain transient in the TUI model and are sent only to the dedicated
daemon `CommitProviderSave` RPC. Legacy `AddProvider` delegates to the same
durable server path. The daemon stores credentials through its existing secrets
provider and adds them to its existing redactor. Secret settings are not
included in review text, errors, or snapshots.

CLI-backed providers use their catalog setup strategy and health checks rather
than API model discovery. ChatGPT subscription auth reuses the current device
flow. Cloud and compatible providers pass their catalog-declared settings to
the existing settings-aware model lister. Unsupported listing falls back to a
manual model field instead of trapping the user.

### 2. Daemon-Supervised Background ACP Drain

Add a daemon-owned background drain manager and gRPC operations for start,
stop, status, and list. The CLI surface is:

```text
ratchet acp client background start <session-id> --agent <profile> --acknowledge-unattended
ratchet acp client background status [<session-id>]
ratchet acp client background stop <session-id>
```

Start is rejected unless unattended execution is explicitly acknowledged.
Only built-in agents and trusted stored ACP launch profiles are eligible.
Arbitrary persisted argv, shell commands, and untrusted custom profiles are not
accepted.

Each persisted policy contains the ACP client session ID, profile name,
descriptor hash, acknowledgement timestamp and policy version, enabled state,
timestamps, and last terminal outcome. It contains no prompts, responses,
environment values, or credentials. State is written atomically with
owner-only permissions in the existing ratchet state tree.

At start and daemon restart, a stored profile is eligible only when `Trusted`
is true, its stored trust hash equals its current `DescriptorHash()` (including
command, args, environment-key names, and working directory), and that hash
matches the policy's pinned hash. Built-ins pin the current `AgentSpec`
fingerprint. Any mismatch moves the entry to `blocked` without launching an
agent. The registry's general trusted-profile loader is hardened to apply the
same hash-validity rule so background execution cannot become the only secure
consumer. A failed agent run moves the entry to `error` and stops automatic
retries; an operator must explicitly start it again. This prevents
cost-amplifying retry loops.

Stop first disables the persisted policy, then cancels the active drain through
the existing cancellation path. Queue claim ownership, cancellation, and stale
recovery continue to use existing ACP client logic. The manager does not
duplicate queue semantics.

The manager uses Go contexts and daemon lifecycle hooks on all platforms. It
does not use `nohup`, shell detachment, cron, launchd, systemd, or Windows Task
Scheduler.

Service construction receives the manager as an owned dependency. Test and TUI
smoke constructors inject a disabled/no-op manager so persisted host state can
never start unattended work during tests.

### 3. Managed Hook Policy and Audit

Add an optional administrator-owned YAML document at the platform path:

| platform | default path |
|---|---|
| Linux | `/etc/ratchet/managed-hooks.yaml` |
| macOS | `/Library/Application Support/ratchet/managed-hooks.yaml` |
| Windows | `%ProgramData%\\ratchet\\managed-hooks.yaml` |

Tests inject a path directly. Environment variables do not override the
production policy path because an unprivileged process environment is not an
administrative trust boundary.

The loader opens the final file without following symlinks, requires a regular
file, and validates administrative ownership before parsing. Unix files must
be owned by root and not group/other writable. Windows files must have a DACL
that grants modification only to Administrators and SYSTEM. If the platform
cannot establish that boundary, the present file is rejected rather than
treated as managed policy.

The policy mode is one of:

- `additive`: managed hooks run with eligible user, project, and plugin hooks;
- `managed-only`: user, project, and plugin hooks remain discoverable for
  diagnostics but are suppressed at execution time.

Managed hooks have immutable source provenance and are trusted by the
administrator-owned file boundary. Local trust and disable commands reject
attempts to alter them. Plugin hooks are filtered only after plugin merge so
managed-only cannot be bypassed by reload order.

Project hooks are loaded lazily from a session working directory at event time.
The engine therefore applies effective managed policy at the final
`EngineContext.RunHooks` composition point, after daemon user/plugin hooks and
the event's project hooks have merged. Reload-time annotation alone is not an
enforcement boundary.

A missing managed file means no managed policy. An existing malformed file is
a fail-closed error during daemon startup and hook reload. The runtime must not
silently continue with unmanaged hooks after an administrator attempted to
install policy.

Hook execution appends owner-readable JSONL records containing timestamp,
event, hook configuration hash, source kind, result class, and duration. A
`started` record is durably appended before a managed hook launches; if that
append fails, the hook does not run. A terminal record follows execution; its
append failure is surfaced and marks hook auditing degraded until a later
successful write. The record excludes command text, environment, input
payload, stdout, stderr, and error text. If future fields contain text, they
must pass through the existing `secrets.Redactor`; no second redaction
implementation is introduced.

`ratchet hooks list` exposes source and suppression status. `ratchet hooks
policy` reports effective mode and policy source. `ratchet hooks audit` reads
the local metadata stream in human or JSON form.

## Data Flow

```mermaid
flowchart LR
    Catalog[Provider catalog] --> CLI[provider CLI]
    Catalog --> TUI[TUI wizard]
    CLI --> RPC[daemon provider RPC]
    TUI --> RPC
    RPC --> Secrets[existing secrets provider + Redactor]
    RPC --> Models[workflow-plugin-agent model listers]

    BGCLI[background drain CLI] --> BGRPC[daemon background RPC]
    BGRPC --> Manager[drain manager]
    Manager --> Profiles[trusted ACP profiles]
    Manager --> Queue[existing queue drain/claim/cancel]

    Admin[managed policy file] --> Loader[hook loader]
    UserProject[user/project hooks] --> Loader
    Plugins[plugin hooks] --> Merge[post-plugin policy merge]
    Loader --> Merge
    Merge --> Runner[hook runner]
    Runner --> Audit[metadata-only JSONL audit]
```

## Error Handling

- Catalog validation fails tests on duplicate aliases, unknown strategies,
  missing required fields, or runtime-provider coverage gaps.
- Provider setup keeps user-entered non-secret state when discovery fails and
  offers manual entry only when the catalog allows it.
- Ambiguous provider-save responses poll operation state for at most 10 seconds
  with bounded backoff. Pending, applied, not-found, and temporary RPC failure
  remain unresolved until the deadline; committed returns the stored provider;
  failed returns a classified outcome. Unresolved exit requests stay in the UI.
- Every current CLI writer uses a signal-aware 30-second `CommitProviderSave`
  call and a separate 10-second reconciliation context. Old daemons return
  `Unimplemented` before mutation. First interrupt shows reconciliation status;
  second interrupt exits with the operation ID for explicit status lookup.
- Applied-operation finalization uses daemon-owned timeouts and is retried by
  operation queries and startup, independent of the canceled caller context.
- Nil provider/list/test/auth success responses and whitespace-only required
  credentials or endpoints are explicit recoverable errors.
- Daemon background start validates acknowledgement, trust, fingerprint, and
  session/profile existence before persistence and launch.
- Background worker panics and terminal agent errors are contained, recorded as
  outcome classes, and do not trigger automatic retry.
- Managed policy parse or validation errors identify the file and field without
  including hook commands or environment values.
- A managed hook does not launch unless its `started` audit record is durable.
  Terminal append failure is surfaced as degraded audit state. Unmanaged-hook
  audit behavior remains best effort unless a managed policy requires auditing.

## Security Review

- **Credential custody:** credentials remain in the daemon's existing secret
  provider; server-generated UUID versions use reserved `provider-v2-` keys.
  Client IDs are canonical UUIDs but never form secret keys. Operation rows and
  RPCs exclude credentials, request bodies, settings, base URLs, and raw errors.
  Server keys contain only a reserved prefix, server timestamp, and UUID. Existing
  `secrets.Redactor` protects runtime errors and may safely over-redact retired
  values until restart.
- **Mutation serialization:** daemon-owned per-alias workers cover provider
  save/remove through terminal state while unrelated aliases proceed. One ID is
  admitted: same-ID calls attach; another gets `AliasBusy` without retention.
  Pending rows reserve keys before `Set`; a two-worker deduplicated cleanup pool
  excludes provider references and pending/applied reservations.
- **Worker safety/order:** worker guards recover panics into classified durable
  failure and release ownership. A short provider-row mutex spans apply through
  terminal finalization and orders default/model/remove row mutations, but never
  wraps secret-provider calls. Cleanup retry persists `next_attempt_at`; one
  due-row dispatcher feeds two short workers so poison rows cannot starve later
  entries.
- **Unattended execution:** opt-in is explicit and profile identity is pinned.
  Profile trust/fingerprint drift blocks resume. Persisted arbitrary commands
  are prohibited.
- **Administrative policy:** production paths are fixed by OS convention and
  rely on administrator-managed file permissions. Managed-only applies after
  plugin loading and cannot be weakened by user state.
- **Audit minimization:** audit records carry hashes and outcome classes, not
  executable commands or content. Files use owner-only permissions and atomic
  append/open behavior where supported.
- **Denial/cost controls:** background errors stop instead of retrying forever;
  only one worker may own a queue policy; start is idempotent for an identical
  active policy.
- **Dependencies:** no TypeScript runtime, AWS reimplementation, new provider
  SDK, remote service, or secret/redaction subsystem is added.

## Infrastructure Impact

No cloud infrastructure, production deployment, or remote service is required.
PR 2 adds required local SQLite `provider_operations` and
`provider_secret_cleanup` tables; migration failure is fatal before RPC
acceptance. Terminal operations retain 24 hours. An OS-level exclusive lock is
held for daemon lifetime and acquired before PID/socket cleanup, migration, or
reconciliation (`flock` on Unix; `LockFileEx` on Windows). Startup then finalizes
applied rows after registry/redactor refresh, fails inherited pending rows, and
queues only unreferenced `provider-v2-` secrets plus durable cleanup. Secret
`List` is fail-stop before RPC acceptance because the provider may ignore
context; deduplicated `Delete` runs asynchronously through a two-worker pool
with bounded backoff after references are journaled.
Release artifacts remain the existing GoReleaser matrix, including Windows.

## Multi-Component Validation

| boundary | proof |
|---|---|
| Upstream registry to catalog | A real `workflow-plugin-agent` registry exposes sorted runtime types; ratchet's contract test excludes only documented test types and requires every remaining type to resolve through the catalog. |
| Catalog to CLI | Contract tests enumerate every visible catalog entry through setup list/guide commands and accepted aliases. |
| Catalog to TUI | State-machine tests traverse auth, settings, discovery, manual fallback, CLI-backed setup, and secret review suppression from catalog entries. |
| Provider save transaction | Real daemon + SQLite + stateful secret provider prove pending journal, idempotent replay/conflict, commit result, rollback preserving the active credential, cache invalidation, redactor registration, restart transition, cleanup retry, and exact operation polling. |
| TUI runtime | A persistent smoke daemon plus local OpenAI-compatible HTTP fixture drives real PTY/ConPTY catalog navigation and one complete save through `CommitProviderSave`; tests inspect operation/provider/secret state, redaction, and sentinel-free output. |
| Daemon restart | Integration test persists an enabled policy, restarts the service, proves matching trusted profile resumes, then proves fingerprint drift blocks launch. |
| Queue lifecycle | Existing claim/cancel/stale paths are exercised through the daemon manager with fake agents and deterministic contexts. |
| Managed hooks | Loader/engine tests cover missing, malformed, insecure ownership/link, additive, managed-only, plugin reload, immutable trust/disable, pre-launch audit failure, and terminal audit failure behavior. |
| Windows | `GOOS=windows` build plus native `windows-2025` lock contention/release and ConPTY tests run before merge; CI Windows jobs remain required. |
| Release | Tag points to the merged commit; archives, checksums, Homebrew update path, and installed `ratchet --version` are verified. |

## Integration Matrix

| integration | status | proof or rationale |
|---|---|---|
| `workflow-plugin-agent/orchestrator.ProviderRegistry` types | runtime-integrated | Upstream API test plus ratchet catalog coverage test against a real registry instance. |
| `workflow-plugin-agent/provider` model listing | runtime-integrated | CLI/TUI tests pass real settings into its lister boundary; credentialed live provider calls remain deferred CI. |
| Existing daemon provider RPC, registry, and secrets provider/Redactor | runtime-integrated | A dedicated durable-save RPC makes old daemons fail pre-mutation; real daemon tests exercise legacy delegation, versioned secret pointer commit/rollback, operation query/replay/finalization, registry resolution, redactor ordering, serialized cleanup, and secret-free TUI/output. |
| Bubble Tea provider/model wizard | runtime-integrated | State tests plus real PTY navigation/render proof. |
| Daemon gRPC/client background API | runtime-integrated | Started daemon, real client, fake ACP process, persisted restart and stop proof. |
| ACP profile store and queue claim/cancel/recovery | runtime-integrated | Real stores and existing drain path; trust-hash drift has a negative launch proof. |
| User/project/plugin/managed hook sources | runtime-integrated | Engine reload plus event-time project load test merges all sources and executes only the final effective set. |
| OS managed-policy paths and ownership checks | runtime-integrated | Platform unit tests; Windows DACL and Unix ownership/link tests use platform-specific files. |
| External managed policy service/SDK | deferred | No remote control plane is required for local administrator policy. |
| Credentialed third-party provider discovery | deferred | Requires repository secrets and is not needed to prove catalog/wiring behavior. |
| VS Code-style harness optimization loop | deferred | Next design cluster after this scope, based on current primary sources and code. |

No plugin-contributed host UI is added. The TUI proof exercises ratchet's own
wizard rather than a contribution metadata route.

## PR and Release Boundaries

1. `workflow-plugin-agent`: exported provider-type introspection prerequisite.
2. `ratchet-cli`: provider catalog and unified CLI/TUI wizard, including the
   released plugin dependency bump.
3. `ratchet-cli`: daemon-supervised background ACP drains.
4. `ratchet-cli`: managed hook policy and audit.

Each PR is independently tested, reviewed, merged, and released. Later PRs
rebase onto the released predecessor. The prerequisite TUI smoke stabilization
PR is not part of this four-PR scope. Public policy/parity docs transition
background drain and managed hooks from deferred to supported only in the PR
that supplies their runtime proof.

## Assumptions

- `workflow-plugin-agent` remains the source of provider runtime and model
  listing behavior and can expose its registered type names without exposing
  factories or mutation.
- The daemon is the only ratchet process expected to supervise durable local
  jobs.
- A trusted ACP profile name plus descriptor fingerprint is sufficient stable
  launch identity for this local policy.
- Administrators protect managed policy files with OS permissions appropriate
  to their deployment.
- Missing administrator policy is normal; malformed present policy is an
  intentional configuration error.

## Out of Scope

- Remote fleet policy, centralized audit upload, or a management SDK.
- Arbitrary background shell commands, detached processes, or schedule syntax.
- New provider SDKs, model APIs, secret stores, or redaction types.
- Autonomous mutation of ratchet's own source or prompts.
- VS Code-style harness optimization experiments; this is the next planned
  cluster after the three PRs and will use current source/documentation rather
  than memory.

## Approaches Considered

### Provider Setup

1. **Chosen: shared UI-agnostic catalog.** One capability definition feeds both
   renderers while auth/network behavior stays in existing packages.
2. Daemon-rendered wizard RPC. Rejected because protobuf would own presentation
   policy and make local UI iteration a daemon contract change.
3. Separate CLI/TUI implementations with conformance tests. Rejected because
   tests would detect drift after duplication instead of removing its cause.

### Background Drain

1. **Chosen: daemon-owned workers and persisted policy.** Reuses lifecycle,
   claims, cancellation, and cross-platform contexts.
2. Detached foreground command or cron wrapper. Rejected because supervision,
   cancellation, restart semantics, and Windows behavior are unreliable.
3. Persist arbitrary launch argv. Rejected because it creates an unattended
   command-execution store with weak identity and audit semantics.

### Managed Hooks

1. **Chosen: fixed OS policy path, explicit precedence, post-plugin filtering.**
   This provides a real administrative boundary and observable enforcement.
2. Pre-seed the existing trust store. Rejected because users could still add or
   run hooks and plugin reload could bypass managed-only intent.
3. Remote policy service. Rejected as unnecessary infrastructure and an
   inappropriate expansion of this local-first slice.

## Self-Challenge

- The provider wizard refactor has the broadest visible UI blast radius. It
  must preserve navigation/back behavior, keep values on recoverable errors,
  and prove long lists do not hide controls or overflow terminals.
- The most dangerous background assumption is that restart equals permission
  to execute. The fingerprint/trust/acknowledgement gates are therefore
  negative integration tests, not documentation promises.
- The managed-hook failure boundary must distinguish absent policy from broken
  policy. Treating both as optional would silently defeat administration;
  treating both as fatal would make unmanaged installs unusable.
- Metadata-only audit is intentionally less convenient for debugging. Command
  output belongs in an explicitly redacted diagnostic path, not a durable
  privileged execution log.
- A provider catalog can become a new source of truth that lags the plugin.
  Registry coverage tests and alias validation make that drift fail CI.

## Rollback

Each PR can be reverted independently. PR 2's additive operation/cleanup tables
are ignored by older binaries; provider rows keep exact versioned secret names,
which existing registries resolve. The additive daemon lock file is ignored by
older binaries. Legacy clients without operation IDs remain
accepted; new writers call a dedicated RPC that old daemons reject before
mutation. Downgrade requires stopping the new daemon and observing lock release;
then process pending cleanup where possible. Old writers
may leave inactive `provider-v2-` versions, which a later upgraded startup
sweeps without touching referenced or legacy-prefix secrets. Background
rollback stops workers before removing RPC/state handling. Managed-hook rollback
must be coordinated with administrators because removing enforcement weakens
policy; preserve audit records.

### Backport 2026-07-10: CLI setup semantics and failed-test cleanup

Cause: Catalog consumers treated setup alias, provider alias, executable, and
working directory as interchangeable; failed provider cleanup was fire-and-forget.
Change: Name all four CLI values in the catalog contract, share health-check
arguments, execute TUI health checks, persist the working directory, and await
provider removal before review/re-add.
Scope: no manifest change; corrections satisfy Tasks 2-4.
Evidence: `go test ./internal/provider ./cmd/ratchet ./internal/tui/pages -count=1`
must pass before PR 2.

### Backport 2026-07-10: Provider upsert commit boundary

Cause: daemon `AddProvider` is an alias-keyed upsert, so wizard compensation by
alias could delete a pre-existing provider and could not restore its prior
secret. Change: Review confirmation is the commit boundary; navigation never
deletes a saved provider, save/test RPCs are bounded, cancellation reports any
committed provider to the app, and explicit provider removal remains a separate
user command. Scope: no manifest change; this corrects Task 4 lifecycle
semantics. Evidence: focused lifecycle tests cover save/cancel races, failed
test navigation, nil RPC results, process reaping, and app cache reconciliation.

### Backport 2026-07-10: Durable provider-save operations

Cause: round-five review proved alias-stable secret mutation and one-shot list
reconciliation cannot make an upsert atomic or authoritative. Change: use
two-phase pending/applied/terminal operations, server-derived versioned keys,
idempotent replay, durable old-secret cleanup, and a metadata-only operation RPC.
All current CLI/TUI writers use a dedicated durable RPC, canonical UUIDs,
bounded signal-aware saves, and detached reconciliation; legacy `AddProvider`
delegates durably on new daemons. Pending secret reservations exclude runtime
cleanup/save races. Daemon-owned per-alias workers
admit one ID, attach replay, reject other IDs as busy, and let non-cancellable
secret calls continue without blocking unrelated aliases. A deduplicated
two-worker cleanup pool rechecks references. The lifetime lock excludes
concurrent startup; startup resolves pending/applied rows and orphans before RPC
acceptance; migration failure is fatal. SQL records `applied`; a
daemon-context/query-assisted finalizer performs cache/redactor work before
externally visible `committed`. See
`decisions/0006-make-provider-saves-durable.md`.
Scope: no manifest change; Task 4 gains daemon/proto/client correction work and
Task 5 gains restart/cleanup/runtime proof. Evidence required: hostile IDs,
duplicate/replay conflict, secret rollback, registry resolution, restart states,
cleanup retry/sweep, delayed operation polling, nil list, Ctrl+C wait, CLI/TUI
coverage, and Windows build tests.

### Backport 2026-07-10: Contract test staging

Cause: Checkpoint 4A names `TestProviderOperationRPCContract` in the daemon
package but its staging command omitted the modified daemon test file.
Change: stage `internal/daemon/integration_test.go` with the generated contract
and client compile-contract test.
Scope: no manifest change; Task 4 and PR 2 are unchanged.
Evidence: removing the generated contract makes both named tests fail on the
missing RPC/types; restoring it makes the focused command pass.

### Backport 2026-07-10: Durable timestamp and harness invariants

Cause: binding `time.Time` directly produced SQLite text that `unixepoch()`
read as `NULL`; the existing E2E harness and redactor test also assumed legacy
alias-stable secret labels.
Change: persist operation/retry deadlines as RFC3339, compare them through
SQLite `unixepoch()` rather than mixed-format text ordering, assert non-null
operation timestamps through the RPC, initialize the real operation manager in
the shared harness, and derive redactor labels from the committed provider pointer.
Scope: no manifest change; Task 4B files/staging now name both shared tests.
Evidence: the focused 4B suite and `go test ./internal/daemon -count=1` pass;
removing the durable manager makes the named tests fail at compile time.

### Backport 2026-07-10: Native daemon lock dependency

Cause: Windows `LockFileEx` requires `golang.org/x/sys/windows`; the module was
already pinned transitively but 4C's staging list did not name `go.mod`.
Change: promote the existing `x/sys` version to a direct dependency and use its
platform-native `windows.Overlapped` type; Unix uses the sibling `unix.Flock`.
Scope: no manifest change; Task 4 and PR 2 are unchanged.
Evidence: Unix lock tests pass and the Windows daemon test binary cross-compiles.
