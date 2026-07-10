### Adversarial Review Report

**Phase:** design
**Artifact:** `docs/plans/2026-07-10-ratchet-cli-provider-drain-managed-hooks-design.md`
**Status:** PASS

Required framing: Find at least three things wrong with this design, even if
they seem minor. Bias toward misconceptions, unstated assumptions, and missed
alternatives; reflexive approval is forbidden.

User intent checked: unify CLI/TUI provider support without future drift, then
implement the next two approved ratchet-cli improvements; preserve Go-native,
Windows, secret, plugin, release, and autonomous-delivery constraints.

**Findings (Critical):**

- None.

**Findings (Important):**

- `D1` [Existence/runtime-validity] [Shared Provider Setup Catalog]: The design
  required a catalog test against the runtime provider registry, but the
  orchestrator registry's factory map is private and exposes no type query.
  Recommendation: add a defensive, sorted upstream `ProviderTypes()` API,
  release it, and test ratchet against a real registry. _Resolution: design and
  ADR 0003 now make this an explicit upstream prerequisite and fourth scoped
  PR._
- `D2` [Repo precedent / naming] [Daemon-Supervised Background ACP Drain]: The
  proposed `ratchet acp queue background <queue>` path does not exist in the
  current command hierarchy; queues belong to ACP client session records under
  `ratchet acp client`, and the durable identifier is a session ID.
  Recommendation: use `ratchet acp client background ... <session-id>` and
  persist the session ID. _Resolution: design corrected._
- `D3` [Security / assumptions] [Daemon-Supervised Background ACP Drain]: A
  stored profile's `Trusted` boolean is currently accepted without proving its
  stored trust hash still equals `DescriptorHash()`. Background resume would
  convert that existing gap into unattended execution after command/env/cwd
  edits. Recommendation: require `Trusted && Hash == DescriptorHash()` at start
  and resume, pin that hash in policy, and harden the shared registry path.
  _Resolution: design and ADR 0004 corrected with negative integration proof._
- `D4` [Security / missing failure modes] [Managed Hook Policy and Audit]: A
  fixed path does not by itself prove administrator ownership, and an audit
  append performed only after execution cannot prevent unaudited launch.
  Recommendation: reject symlinks/non-regular/insecure ownership or DACLs, and
  durably append `started` before managed execution. _Resolution: design and ADR
  0005 corrected; terminal audit degradation is explicit._
- `D5` [Artifact-class precedent / failure mode] [Daemon manager lifecycle]:
  existing TUI smoke construction deliberately disables cron/background work.
  A manager created implicitly by `NewService` could execute persisted host
  state in tests. Recommendation: constructor-inject daemon ownership and use a
  disabled manager in test/smoke constructors. _Resolution: design and ADR 0004
  corrected._
- `D6` [Repo precedent / security] [Managed Hook Policy and Audit]: Project
  hooks are loaded lazily inside `EngineContext.RunHooks`, after the plugin
  reload path where the initial design placed final filtering. Managed-only
  enforcement at reload could therefore be bypassed by a project hook.
  Recommendation: apply effective policy after the per-event user/plugin/project
  composition. _Resolution: design and ADR 0005 corrected with a combined
  source execution test._

**Findings (Minor):**

- None after revision.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Revised design reuses plugin SDKs, existing secrets/Redactor, Go, JSONL audit, Windows builds, and per-PR release gates from workspace guidance. |
| Assumptions under attack | Finding | Profile trust validity and implicit test-manager disablement were load-bearing; D3/D5 resolved them. |
| Repo-precedent conflicts | Finding | Existing command/session hierarchy and late project-hook loading conflicted with the first shape; D2/D6 corrected both. |
| Artifact-class precedent | Finding | Daemon smoke constructors explicitly omit background schedulers; D5 requires the same injection pattern. |
| YAGNI violations | Clean | No remote policy service, TypeScript SDK, arbitrary scheduler, provider SDK, or self-mutation loop is included. |
| Missing failure modes | Finding | Insecure managed files, late project bypass, pre-launch audit failure, profile drift, test state leakage, and retry amplification are now specified. |
| Security / privacy at architecture level | Finding | D3/D4 closed unattended trust and administrator-boundary gaps; secrets/content remain excluded from policy and audit. |
| Infrastructure impact | Clean | Local files and daemon workers only; no production resources, IAM, migrations, or deployment approval required. |
| Multi-component validation | Clean | Revised matrix requires real catalog/registry, TUI/daemon, daemon/ACP process, and hook loader/plugin/runner proofs. |
| Declared integration proof | Clean | Explicit integration matrix marks every named upstream/runtime/deferred boundary and its proof. |
| Contributed UI rendering proof | Clean | No plugin-contributed UI exists; ratchet's own Bubble Tea wizard has PTY content/navigation proof. |
| Rollback story | Clean | Per-PR rollback preserves stored provider data, stops workers, and requires managed-policy coordination before enforcement removal. |
| Simpler alternative not considered | Clean | Separate tables, daemon-rendered UI, detached shell workers, trust-store seeding, and remote policy were considered and rejected. |
| User-intent drift | Clean | The three ratchet features directly match the approved request; the upstream API exists only to make no-drift enforcement real. |
| Existence / runtime-validity | Finding | D1 found the missing upstream registry query; actual proto, ACP store/profile, hook loader, plugin merge, and Windows release surfaces exist. |

**Options the author may not have considered:**

1. Generate the catalog from a plugin-owned descriptor schema. This would
   remove more metadata duplication but requires moving product-facing labels,
   auth steps, and TUI field policy into the runtime plugin. The read-only type
   query is the smaller contract and keeps UI policy in ratchet.
2. Run background drains as daemon routines. Existing routines provide visible
   lifecycle, but they do not own ACP session claim/cancel/profile semantics.
   A dedicated manager that delegates to `DrainQueue` avoids a second queue
   abstraction while preserving daemon supervision.
3. Treat managed hooks as one more trusted source. This is simpler but cannot
   enforce managed-only after plugin reload or protect policy from local
   disable state, so it does not meet administrator-policy intent.

**Verdict reasoning:** Six Important issues were found and resolved in the
design/ADRs before planning. The revised artifact now names its upstream
contract, established command/state identifiers, trust validity, secure policy
file boundary, audit ordering, daemon injection pattern, integration matrix,
and four-PR release shape. No Critical or unresolved Important finding remains.

## Cycle 2: Durable Provider Saves

**Status:** FAIL

**Findings (Critical):** none.

**Findings (Important):**

- `D7` [Security] Caller operation IDs lack canonical UUID validation and must
  never directly form file-provider paths.
- `D8` [Idempotency] Duplicate/concurrent operation IDs need first-write-wins
  replay and deterministic conflict rejection.
- `D9` [Intent] Every current CLI/TUI `AddProvider` caller needs operation IDs;
  older clients need explicit compatibility behavior.
- `D10` [Cleanup] Secret cleanup needs reserved namespace, startup ordering,
  SQL reference marking, idempotent deletion, and durable retries.
- `D11` [Durability] `secrets.Provider.Set` does not promise crash durability;
  ratchet must not claim more than the provider contract supplies.
- `D12` [State model] Operation rows need pending/committed/failed states,
  restart transitions, bounded polling, and retention greater than polling.
- `D13` [Runtime boundary] Commit ordering must include cache invalidation,
  redactor registration, old-secret retirement, and real registry resolution.
- `D14` [Plan wiring] Tasks 4-5 omit proto, daemon, schema, cleanup, restart, and
  mixed-platform proof despite the unchanged manifest.
- `D15` [Migration/rollback] Required migration, mixed-version writes, retained
  rows, orphan cleanup, and downgrade behavior are absent.
- `D16` [Privacy] Operation schema/RPC must forbid credentials, requests,
  sensitive settings, and raw errors.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project guidance | Finding | Windows, existing secret providers, and release-safe execution need explicit proof. |
| Assumptions | Finding | Secret durability, idempotency, and authoritative polling were underspecified. |
| Repo/artifact precedent | Finding | Provider cache/redactor order and RPC/migration task shape were omitted. |
| YAGNI | Clean | Historical operation identity is required; alias state alone is insufficient. |
| Failure/security/infra | Finding | Crash cleanup, identifier safety, retention, and migration need concrete contracts. |
| Multi-component/integration | Finding | SQL, secrets, registry, redactor, RPC, CLI, and TUI require one real boundary proof. |
| Contributed UI | Clean | Native wizard only; existing viewport proof applies. |
| Rollback | Finding | Mixed-version and versioned-secret downgrade behavior missing. |
| Simpler alternative | Finding | Serialized startup mark-and-sweep is simpler than heuristic runtime cleanup. |
| User intent | Finding | CLI save paths were outside the first durability correction. |
| Runtime validity | Finding | Real `FileProvider` does not guarantee durable atomic writes. |

**Alternatives:** two-phase operation journal; startup-only namespace sweep;
separate client operation and server secret-version IDs; upstream atomic file
writes.

**Verdict reasoning:** FAIL; ten Important architecture gaps require revision
before implementation.

## Cycle 3: Durable Provider Saves

**Status:** FAIL

**Findings (Critical):** none.

**Findings (Important):**

- `D17` [Runtime/rollback] New clients can silently use an old daemon that lacks
  operation RPC support; provider saves need capability gating.
- `D18` [Assumption/infra] Immediate startup sweep assumes exclusive daemon
  ownership; age/reference gates or an OS lock must protect live operations.
- `D19` [Failure mode] Applied rows can remain pending without restart; use a
  daemon-owned context and runtime/query-assisted finalization.
- `D20` [Intent] CLI save calls remain unbounded and lose reconciliation state
  on interrupt; all writers need signal-aware deadlines plus detached polling.

**Findings (Minor):**

- `D21` Partial conflict shapes omit behavior-changing fields; unconditional
  first-write replay is simpler and honest.
- `D22` Task 4 red commands and Task 5 commit file list are incomplete.
- `D23` Operation history should not retain provider base URLs.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Guidance/intent | Finding | Mixed-version and bounded CLI reliability remain open. |
| Assumptions/failures | Finding | Exclusive startup and post-commit completion were unsafe assumptions. |
| Repo/artifact precedent | Finding | Compatibility behavior, red commands, and commit files need wiring. |
| YAGNI | Clean | Journals have concrete responsibilities. |
| Security/infra | Finding | Endpoint retention and concurrent cleanup need correction. |
| Multi-component/integration | Finding | Capability and live finalizer proof missing. |
| UI | Clean | Native wizard proof remains declared. |
| Rollback/runtime | Finding | New-client/old-daemon behavior unresolved. |
| Simpler alternative | Finding | Unconditional operation replay avoids partial shape storage. |

**Alternatives:** capability-gated saves; daemon-owned applied finalizer;
unconditional first-write replay; cross-platform ownership lock.

**Verdict reasoning:** FAIL; capability negotiation, conservative cleanup,
runtime applied finalization, and bounded CLI saves require revision.

## Cycle 4: Durable Provider Saves

**Status:** FAIL

**Findings (Critical):** none.

**Findings (Important):**

- `D24` [Protocol] Capability preflight races daemon replacement; use a new RPC
  that an old daemon rejects before mutation.
- `D25` [Concurrency] Runtime cleanup can delete a Set-but-not-committed secret;
  pending rows must reserve keys or mutation/cleanup must serialize.
- `D26` [State] A later same-alias save can displace an applied operation before
  finalization; serialize through terminal state or define supersession.
- `D27` [Windows proof] New `LockFileEx` tests are absent from native Windows CI.

**Findings (Minor):**

- `D28` State that UUID idempotency lasts for the 24-hour retention window.
- `D29` Interrupted CLI reconciliation needs status text and second-signal UX.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Guidance/intent | Finding | Windows proof and atomic old-daemon refusal missing. |
| Assumptions/failures | Finding | Internal cleanup and alias overwrite races remain. |
| Repo/artifact precedent | Finding | Reconnecting client and Windows CI job require direct wiring. |
| YAGNI | Clean | Each durability primitive addresses a demonstrated failure. |
| Security | Clean | Identifier/path/privacy boundaries are resolved. |
| Infra/integration/runtime | Finding | Dedicated RPC, serialization, and native Windows proof needed. |
| UI | Clean | Native wizard proofs remain specified. |
| Rollback | Finding | Preflight cannot prevent downgraded-daemon mutation. |
| Simpler alternative | Finding | Pending secret reservation is deterministic. |

**Alternatives:** dedicated durable-save RPC; pending reservation; per-alias
finalization lock; native Windows lock gate.

**Verdict reasoning:** FAIL; four Important protocol/concurrency/platform gaps
remain.

## Cycle 5: Durable Provider Saves

**Status:** FAIL

**Findings (Critical):** none.

**Findings (Important):**

- `D30` [Integration] Separate PTY navigation and direct-client secret tests do
  not prove the real wizard uses the durable RPC; require one full PTY save and
  inspect operation/provider/secret/redactor/output state.
- `D31` [Failure mode] Existing file-secret calls ignore context; holding the
  global mutex across `Set`/`List`/`Delete` can block startup or all mutations.
  Define daemon-owned workers, pending reservations, bounded client waiting, and
  honest fail-stop behavior for non-cancellable provider calls.

**Findings (Minor):**

- `D32` Replace stale design text naming `AddProvider` as the current TUI RPC.
- `D33` Downgrade requires stopping the new daemon and observing lock release.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Guidance/intent | Clean | Go, existing secret provider/Redactor, Windows preserved. |
| Assumptions/failures | Finding | Secret provider context cancellation is not guaranteed. |
| Repo/artifact precedent | Finding | FileProvider ignores context; one stale RPC name remains. |
| YAGNI/security | Clean | Journals and metadata minimization are justified. |
| Infra/runtime | Finding | Blocking secret calls need explicit ownership/fail-stop behavior. |
| Integration | Finding | Real PTY submission boundary is unproved. |
| UI | Clean | Native UI render proofs remain. |
| Rollback | Finding | Downgrade quiescence precondition missing. |

**Alternatives:** per-alias operation workers; one executable full-save smoke;
explicit fail-stop secret-provider contract.

**Verdict reasoning:** FAIL; real TUI submission and non-cancellable secret-call
semantics remain Important.
