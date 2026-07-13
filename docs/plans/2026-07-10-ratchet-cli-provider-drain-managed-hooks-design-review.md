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

## Cycle 6: Durable Provider Saves

**Status:** FAIL

**Findings (Critical):** none.

**Findings (Important):**

- `D34` [Concurrency] Per-alias execution lacks admission semantics during a
  hung non-cancellable `Set`; different operation IDs must be rejected without
  retaining credentials, while same-ID calls attach to the active result.

**Findings (Minor):**

- `D35` Cleanup workers need deduplication, a fixed concurrency cap, bounded
  backoff, and retirement.
- `D36` Startup gates cleanup discovery/journaling; physical deletion is async,
  so “serially sweeps before acceptance” is stale wording.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Guidance/precedent | Clean | Existing Go/provider/registry/Windows shapes preserved. |
| Assumptions/failures | Finding | Alias serialization alone does not prevent queued abandoned saves. |
| YAGNI/security | Clean | Durable primitives and E2E secret proof are justified/minimal. |
| Infrastructure | Finding | Cleanup fan-out is uncapped. |
| Integration/UI/rollback/runtime | Clean | Full TUI save, lock quiescence, and provider context semantics are covered. |
| Simpler alternative | Finding | One admitted alias operation plus busy rejection avoids a queue. |

**Alternatives:** single admitted operation per alias; bounded deduplicated
cleanup pool; synchronized ownership map without idle alias goroutines.

**Verdict reasoning:** FAIL; same-alias admission during a hung operation remains
Important, while cleanup bounds and wording are Minor.

## Cycle 7: Durable Provider Saves

**Status:** PASS

**Findings (Critical/Important):** none.

**Findings (Minor):**

- `D37` Worker boundaries should recover panics, classify failure, preserve
  durable rows, and retire ownership.
- `D38` Default/model provider mutations need explicit ordering with applied
  saves; use the same short row-mutation critical section.
- `D39` Cleanup retries should persist `next_attempt_at`; a due-row dispatcher
  feeds at most two short workers so poison entries cannot monopolize slots.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Guidance/security/intent/rollback | Clean | Existing Go, secret, Windows, integration, and downgrade boundaries hold. |
| Assumptions/precedent/failure | Finding (Minor) | Panic, sibling row mutation, and cleanup fairness need conservative implementation details. |
| Artifact/YAGNI/UI | Clean | Files and primitives remain established and justified. |
| Infra/runtime | Finding (Minor) | Due-row retry metadata prevents worker starvation. |
| Multi-component/integration | Clean | Full persistent-root TUI durable-save proof is specified. |

**Alternatives:** unified row mutation executor; due-row cleanup dispatcher;
shared panic-safe worker guard.

**Verdict reasoning:** PASS; all Critical/Important findings D1-D36 are resolved.
D37-D39 are straightforward conservative implementation refinements.

## Cycle 8: Task 6 Authority-First Rewrite

**Status:** FAIL

**Findings (Critical):** none.

**Findings (Important):**

- `D40` Cancellation authority read errors were undefined and boolean callers
  could continue unattended work. Require an error-bearing, fail-closed check.
- `D41` A compatibility sidecar cannot atomically notify an older worker after
  primary commit. Define degradation, reconciliation, and quiesced downgrade.
- `D42` Append-only tail repair cannot unblock startup `Read`. Use one lock-held
  audit repair primitive from both read and append.
- `D43` Newline commit, required fields/actions, malformed committed records,
  and unknown-field compatibility were unspecified.
- `D44` Path-only canonicalization leaves parent retarget and hard-link races.
  Pin lock/data operations to one parent identity and validate the opened file.
- `D46` Shared append repair could alter raw ACP event-log semantics. Share only
  secure open; keep framing/repair audit-specific.
- `D47` The rewrite lacked explicit crash, process-race, restart, downgrade, and
  native Windows proofs.

**Findings (Minor):**

- `D45` Specify `list IDs -> lease -> reload`, missing behavior, and no writes
  after release.
- `D48` Specify hash field order, nil/empty, env ordering, and legacy retrust.
- `D49` State that AIX process-locked mutation is unsupported/fail-closed even
  though cross-compilation remains required.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Guidance/intent | Finding | Cancellation and owner-only claims were incomplete. |
| Assumptions/failure/rollback | Finding | Authority errors, old-worker notification, and startup repair were undefined. |
| Security/concurrency | Finding | Parent identity, hard links, and transition lock order needed contracts. |
| Compatibility/portability | Finding | JSON/hash evolution, downgrade, Windows, and AIX needed explicit behavior. |
| Repo precedent/scope | Finding | Audit repair must not change raw event logs. |
| Validation | Finding | Real restart/process/native-platform proofs were missing. |
| YAGNI/infrastructure | Clean | No SQLite, migration, or external infrastructure is justified. |

**Alternatives:** audit-specific recovery over shared secure open; honest
best-effort legacy projection with reconciliation instead of atomicity claims.

**Verdict reasoning:** FAIL; D40-D44, D46, and D47 remain Important.

## Cycle 9: Task 6 Authority-First Rewrite

**Status:** FAIL

**Cycle 8 mapping:** D42, D45, D46, D48 resolved; D40, D41, D43, D44, D47
remained open through D51-D56; D49 behavior resolved but lacked proof.

**Findings (Critical):**

- `D50` Cancellation was not monotonic: queue/lifecycle writeback could replace
  `cancel_requested` after a successful authority check.

**Findings (Important):**

- `D51` Boolean cancellation callbacks could not propagate authority/ACP-cancel
  errors into prompt and child-process termination.
- `D52` Mutating audit `Read` lacked the same pinned-handle protections as
  append; audit needed a dedicated owner-only namespace.
- `D53` Projection reconciliation lock order and an executable quiesced
  downgrade-readiness operation were undefined.
- `D54` Fault injection did not prove real crash, cross-process race,
  mixed-reader executable, native Windows, and unsupported-target boundaries.
- `D55` Profile mutation could race trusted resolution and durable launch.

**Findings (Minor):**

- `D56` Enumerate required audit fields and action/outcome consistency.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Guidance/authority/failure | Finding | D50-D51 violated fail-closed authority. |
| Crash/restart/validation | Finding | D54 required process-shaped proof. |
| Security/concurrency | Finding | D52/D55 required pinned audit/profile ownership. |
| Compatibility/rollback | Finding | D53 required transactional reconciliation and readiness. |
| Framing/schema | Finding | D56 required explicit semantic validation. |
| Intent/scope/YAGNI/infra | Clean | Locked scope and no-migration approach remain intact. |

**Alternatives:** sticky conditional queue claims; one owner-only,
handle-relative audit transaction layer.

**Verdict reasoning:** FAIL; D50 is Critical and D51-D55 remain Important.

## Cycle 10: Monotonic Cancellation and Recovery Rewrite

**Status:** FAIL

**Findings (Critical):**

- `D57` Enqueue, stale recovery, and lifecycle replacement remained independent
  status writers that could clear the cancellation latch.

**Findings (Important):**

- `D58` Error/cancel watcher precedence, grace, forced kill, join, and reaping
  were not causal or deterministic.
- `D59` Downgrade readiness lacked durable admission/lifetime semantics; explicit
  unsupported downgrade is safer.
- `D60` Audit errors after newline write lacked commit classification and
  idempotent retry reconciliation.
- `D61` A worker goroutine was not child-start acknowledgement for releasing a
  profile trust lease.
- `D62` Windows pinned-parent/file-ID and opened-object validation mechanics were
  unnamed.
- `D63` Cancellation and profile races lacked separate-process real-fixture
  proofs.
- `D64` The proposed downgrade command was absent from the locked Task 8 command
  contract and rollback.

**Findings (Minor):**

- `D65` Enumerate the exact audit action/outcome matrix and evolution rule.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Guidance/state/failure | Finding | D57-D61 left state and commit ambiguity. |
| Security/platform | Finding | D62 required named Windows mechanics. |
| Validation/integration | Finding | D63 required real process interleavings. |
| Rollback/scope | Finding | D59/D64 made downgrade non-executable. |
| YAGNI/intent/infra | Clean | No migration or remote control plane was added. |

**Alternatives:** one guarded session transition function; upgrade-forward-only
released recovery.

**Verdict reasoning:** FAIL; D57 is Critical and D58-D64 remain Important.

## Cycle 11: Authoritative Transition Closure

**Status:** FAIL

**Findings (Critical):** none.

**Findings (Important):**

- `D66` Guarded transitions omitted whole-record/import writers; require every
  writer to use the guard or an explicit create-only collision check.
- `D67` Cancellation observation and a blocked ACP cancel send lacked one
  bounded first-cause contract; latch `ErrCancelRequested`, bound send, then
  kill/reap and join every watcher/send goroutine.
- `D68` Last-record audit deduplication fails after an interleaved append; use a
  stable record ID or scan all committed records for complete equality.
- `D69` Windows parent revalidation retained a replacement window; exclude
  `FILE_SHARE_DELETE` and pin `FileIdInfo` before child open/validation.
- `D70` Profile mutation proof could remain in-process; require a second
  mutator process racing a fixture child's real start acknowledgement.
- `D71` Task 8 rollback conflicted with upgrade-forward-only released state;
  permit source reversion only before release and retain authority-aware state
  handling in post-release patches.

**Prior mapping:** D57-D65 resolved or narrowed to D66-D71. Sticky cancellation,
session-primary projection, ID/lease/reload transitions, audit schema, Unix
`openat`, and AIX fail-before-write remain clean.

**Verdict reasoning:** FAIL; D66-D71 are concrete Important contract gaps.

## Cycle 12: Executable Authority Contract

**Status:** FAIL

**Findings (Critical):** none.

**Findings (Important):**

- `D72` Task 6 omitted store/archive/client/drain/platform files and the writer
  inventory/process proofs required by its rewrite. Recommendation: add an
  authority-first execution backport inside Task 6; manifest stays unchanged.
- `D73` Execution-context cancellation could cancel the ACP cancel send, and
  authority failures were conflated with user cancellation. Recommendation:
  define callback outcomes and use an independent bounded send context.
- `D74` Visible-metadata-derived audit IDs can collide for distinct same-time
  events. Recommendation: persist a random event ID before first append.
- `D75` Native Windows attack tests were not named in the plan despite the
  existing `^TestBackgroundWindows` CI selector. Recommendation: name the tests
  and require that CI job, not only a cross-build.
- `D76` A copied-profile resolver cannot retain a process lease through real
  `exec.Cmd.Start`. Recommendation: define a lease-owning callback boundary and
  a second-process fixture race.
- `D77` Released downgrade was advisory rather than mechanically blocked.
  Recommendation: isolate old readers or explicitly accept unsupported manual
  downgrade as operator risk.

**D66-D71 mapping:** D66 partial via D72; D67 partial via D73; D68 open via D74;
D69 partial via D75; D70 open via D76; D71 partial via D77.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | D75 conflicts with the named Windows runtime-proof rule. |
| Assumptions under attack | Finding | D73/D77 relied on cancellation/downgrade behavior without enforcement. |
| Repo-precedent conflicts | Finding | D76 lacked process-lock ownership for file-backed mutation. |
| Artifact-class precedent | Finding | Existing Windows selection needs explicitly named test artifacts. |
| YAGNI violations | Clean | Rewrite remains demonstrated state/security hardening. |
| Missing failure modes | Finding | Send-context loss, ID collision, and old-binary access were open. |
| Security/privacy architecture | Finding | Native Windows privileged-path attacks lacked proof. |
| Infrastructure impact | Clean | No cloud resource changes. |
| Multi-component validation | Finding | Process/profile/store/Windows boundaries were incomplete. |
| Declared integration proof | Finding | ACP recovery lacked required process/native cases. |
| Contributed UI rendering proof | Clean | Task 6 contributes no UI. |
| Rollback story | Finding | Released rollback was advisory. |
| Simpler alternative not considered | Finding | Persisted event IDs simplify audit identity. |
| User-intent drift | Clean | Locked daemon-drain scope is preserved. |
| Existence/runtime-validity | Finding | Runtime surfaces required by the contract were omitted. |

**Alternative:** persist one transition event ID and cancellation/audit payload;
reuse the ID on retry and let the profile-store callback own launch trust.

**Verdict reasoning:** FAIL; D72-D77 require contract backports or an explicit
accepted risk before implementation.
