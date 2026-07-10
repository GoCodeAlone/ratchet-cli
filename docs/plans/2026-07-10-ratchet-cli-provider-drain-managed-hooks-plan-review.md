### Adversarial Review Report

**Phase:** plan
**Artifact:** `docs/plans/2026-07-10-ratchet-cli-provider-drain-managed-hooks.md`
**Status:** PASS

Required framing: Find at least three things wrong with this plan, even if they
seem minor. This is an attack on assumptions, verification, and executable task
shape; reflexive approval is forbidden.

**Findings (Critical):**

- None.

**Findings (Important):**

- `P1` [Design drift / missing integration proof] [Task 6]: The design requires
  redacted JSONL evidence for background policy changes, but the first plan
  draft persisted only current policy/status. Recommendation: add a dedicated
  metadata-only background audit, privacy tests, and lifecycle event proof.
  _Resolution: Task 6 now creates `background_audit.go`, tests every lifecycle
  class, and excludes content/argv/env/credentials._
- `P2` [Verification-class mismatch] [Task 9]: The draft promised native
  Windows DACL tests, but current CI's Windows jobs do not run `internal/hooks`;
  local macOS can only cross-compile them. Recommendation: extend the existing
  `windows-2025` job with the focused managed-policy tests. _Resolution: Task 9
  now modifies `.github/workflows/ci.yml` and requires a named native check._
- `P3` [Security / existence-runtime validity] [Task 9]: Describing the Windows
  path as `%ProgramData%` risks implementing it with attacker-controlled process
  environment despite the design rejecting environment overrides.
  Recommendation: obtain ProgramData through the Windows Known Folder API.
  _Resolution: Task 9 requires `windows.KnownFolderPath` with
  `FOLDERID_ProgramData`._
- `P4` [User-intent drift / UI integration] [Task 4]: Table-driving one test per
  strategy would not prove every catalog entry reaches the TUI; two entries can
  share a strategy while one is accidentally omitted. Recommendation: enumerate
  every visible catalog entry and add a source guard against another TUI-owned
  provider table. _Resolution: Task 4 corrected._
- `P5` [Security / missing failure mode] [Task 10]: The existing hook runner
  includes combined command output in returned errors, and daemon call sites log
  those errors. A metadata-only audit alone would not prevent a managed hook's
  output from leaking through normal logs. Recommendation: managed errors expose
  only event/hash/result classification; preserve unmanaged compatibility only
  behind the existing redactor. _Resolution: Task 10 corrected with sentinel
  tests._
- `P6` [Planned command validity] [Tasks 5, 8, 9, 11]: The first draft used a
  `timeout` binary absent on the development host and attempted `go test -c`
  across multiple packages. Recommendation: use the repository's
  context-bounded binary smoke and compile one cross-platform test package per
  command. _Resolution: all commands corrected before lock._

**Findings (Minor):**

- None after revision.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Tasks explicitly reuse provider SDKs, existing secrets/Redactor, Go, JSONL, Windows, PR monitoring, and release gates. |
| Assumptions under attack | Finding | Windows path/CI and catalog-entry coverage assumptions failed under P2-P4 and were corrected. |
| Repo-precedent conflicts | Clean | Plan uses `master`, Makefile proto generation, existing ACP client command hierarchy, daemon smoke injection, and current release jobs. |
| Artifact-class precedent | Clean | Catalog stays in `internal/provider`, ACP policy beside ACP stores, hook policy beside hook loader, and daemon integration in daemon tests. |
| YAGNI violations | Clean | No remote service, schedule engine, arbitrary argv persistence, new SDK, secret store, or self-mutation is planned. |
| Missing failure modes | Finding | P1/P5 added lifecycle audit and raw-output containment; restart drift, malformed policy, audit failure, and retries are otherwise explicit. |
| Security / privacy at architecture level | Finding | P3/P5 closed mutable path and log-leak paths; secret sentinels cover provider, ACP, and hooks. |
| Infrastructure impact | Clean | Only CI configuration and local files/processes change; no cloud resources, IAM, migration, or production deploy. |
| Multi-component validation | Clean | Real registry/catalog, CLI/TUI/daemon, daemon/ACP fixture, and loader/plugin/project/runner boundaries are executable tasks. |
| Declared integration proof | Clean | Plan repeats the design matrix with runtime/deferred status and task evidence for each integration. |
| Contributed UI rendering proof | Clean | No contributed plugin UI; Task 5 runs the host TUI through PTY/ConPTY and asserts contribution-specific provider content. |
| Rollback story | Clean | Every runtime/version/startup task has a one-line operational rollback, including policy-removal order. |
| Simpler alternative not considered | Clean | Design rejects duplicated tables, detached workers, trust seeding, and remote policy; tasks implement only chosen minimums. |
| User-intent drift | Finding | P4 prevented strategy-only tests from weakening the explicit no-drift request. |
| Existence / runtime-validity | Finding | P2/P3/P6 checked actual CI jobs, Known Folder API, Makefile proto target, and host commands. |
| Over/under-decomposition | Clean | Eleven tasks map to four independently releasable PRs; steps separate red/green/verification/commit actions without splitting trivial helpers. |
| Verification-class mismatch | Finding | P2/P6 corrected native Windows and command/runtime proof; docs guards, hook events, gRPC calls, PTY UI, and version launch match their classes. |
| Auth/authz chain composition | Clean | No network authz chain is added; local daemon access is unchanged, profile authorization is server-side trust/hash/acknowledgement validation. |
| Hidden serial dependencies | Clean | PRs are explicitly serial: upstream release before pin, provider release before background branch, background release before hooks branch. |
| Missing rollback wiring | Clean | Rollback appears inside Tasks 1, 2, 5, 7, 8, 9, and 11, not only in the design. |
| Missing integration proof | Finding | P1 added missing policy-audit integration; every remaining multi-component claim has a real consumer task. |
| Missing declared integration matrix | Clean | Matrix is present and every runtime row cites a task; deferred rows state why. |
| Missing contributed UI route proof | Clean | Task 5 enters `/provider add` through the real shell and asserts Bedrock/custom content and navigation. |
| Infrastructure verification mismatch | Clean | CI YAML change is exercised by native PR checks; no infrastructure apply is relevant. |
| Plugin-loader runtime layout | Clean | No external Workflow plugin binary is loaded; workflow-plugin-agent remains a Go module dependency. |
| Config-validation schema rules | Clean | Protobuf uses the existing generator; managed YAML validation has malformed/mode/event tests. |
| Identifier / naming-convention match | Clean | `acp client background`, snake-case provider settings, proto RPC names, and existing hook event/source names match repo conventions. |
| Planned-code compile-validity | Clean | Embedded Go uses element range, comparable operations only, existing `slices.Sort`, and valid struct tags/signatures. |

**Options the author may not have considered:**

1. Expose a static provider constant upstream instead of `ProviderTypes()`.
   This would be simpler to consume but could drift from the actual factory map;
   querying the constructed registry makes runtime registration authoritative.
2. Reuse daemon routines for background drains. Routines do not own ACP
   session claims, profile trust, or cancel files, so wrapping `WatchQueue` in a
   dedicated manager is the smaller behavioral change.
3. Validate only Windows file owner and skip DACL traversal. Owner alone does
   not prevent a Users/Everyone write ACE; native CI makes the stricter check
   supportable without a new dependency.

**Verdict reasoning:** Six Important issues were found and resolved before
scope lock. The revised plan implements every design requirement, uses valid
commands, proves native Windows security, prevents both audit and ordinary-log
secret leakage, enumerates every TUI provider entry, and keeps the four PRs
serial, independently releasable, and rollback-capable. No Critical or
unresolved Important finding remains.

## Cycle 2: Durable Provider Saves

**Status:** FAIL

**Findings (Critical):** none.

**Findings (Important):**

- `P7` Task 4 is under-decomposed; proto, migration/engine, lock, CLI, TUI, and
  CI require sequential checkpoints while retaining one task/PR.
- `P8` Red commands omit daemon lock/CRUD/workers/app tests; Task 5 has no red
  persistent-smoke command.
- `P9` Native Windows CI commands/test names are unspecified and current regexes
  would skip lock/provider-save tests.
- `P10` Legacy-schema upgrade, migration failure, and stop/lock-release/current-
  to-parent downgrade are unproved.
- `P11` Test-only smoke does not prove production daemon startup, durable save,
  operation lookup, restart, and shutdown wiring.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Guidance/security/YAGNI | Clean | Existing boundaries and minimum primitives remain sound. |
| Assumptions/precedent | Finding | CI regex, migration compatibility, and smoke/production equivalence failed. |
| Decomposition/TDD | Finding | Serial generated/schema/runtime slices need focused red-green checkpoints. |
| Windows/infra | Finding | Native test selection and migration failure proof missing. |
| Integration/UI | Finding | UI save is real, but production daemon lifecycle is not. |
| Rollback | Finding | Downgrade quiescence/version-pair proof absent. |
| Naming/compile/config | Clean | Planned identifiers and embedded code are valid. |

**Alternatives:** internal Task 4 checkpoints; one production lifecycle harness;
dedicated Windows native step; current/parent version-pair rollback fixture.

**Verdict reasoning:** FAIL; Tasks 4-5 are behaviorally complete but not yet an
executable TDD/runtime/rollback plan.

## Cycle 3: Durable Provider Saves

**Status:** FAIL

**Findings (Critical):** none.

**Findings (Important):**

- `P12` Adding all Task 4 tests before Checkpoint 4A breaks package compilation
  on undefined later symbols; each checkpoint must add only its own tests.
- `P13` Production tests run before `dist/ratchet` exists; harnesses must build
  current/parent binaries into test-owned temporary paths.
- `P14` `origin/master` may equal HEAD/current code; downgrade proof must pin,
  log, and validate a distinct SHA whose proto lacks `CommitProviderSave`.

**Findings (Minor):**

- `P15` Each checkpoint needs local `gofmt`, focused proof, exact staging, and
  commit order so final formatting cannot dirty a supposedly clean branch.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Guidance/security/infra/UI | Clean | Existing boundaries and named proofs remain sound. |
| Assumptions/runtime | Finding | Shared dist artifact and older-ref identity were assumed. |
| TDD/compile/dependencies | Finding | Go compiles all package tests regardless of `-run`. |
| Integration | Finding | Production harness must construct the binary it launches. |
| Rollback | Finding | Same-SHA parent makes downgrade proof vacuous. |
| Decomposition | Clean | Internal checkpoints preserve locked tasks/PRs. |

**Alternatives:** checkpoint-owned tests; temporary production binaries; pinned
and validated base SHA; checkpoint-local closeout.

**Verdict reasoning:** FAIL; checkpoint compilation, hermetic runtime build, and
non-vacuous downgrade identity require correction.

## Cycle 4: Durable Provider Saves

**Status:** FAIL

**Findings (Critical):** none.

**Findings (Important):**

- `P16` Dedicated CI never supplies `RATCHET_DOWNGRADE_BASE_SHA`; ordinary and
  post-merge suites would fail the non-vacuity check. Wire PR base/push-before
  only into the downgrade step and skip explicitly elsewhere.

**Findings (Minor):**

- `P17` Successful provider-save automation needs stable JSON with
  `operation_id`, not human-text parsing.
- `P18` Checkpoint 4E needs exact durable TUI delta test names and must not claim
  the already-removed hardcoded table as its red reason.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Guidance/security/integration | Clean | Production, UI, secret, and Windows proofs remain complete. |
| Assumptions/runtime/rollback | Finding | CI event base identity is not automatically available to tests. |
| Decomposition/compile | Clean | Checkpoint ownership and temporary builds are corrected. |
| Naming/TDD | Finding (Minor) | Stable operation JSON and exact TUI test names needed. |

**Alternatives:** event-specific base SHA; opt-in downgrade test; stable save
JSON; named TUI delta tests.

**Verdict reasoning:** FAIL; CI must supply a distinct older SHA only to the
merge-gating downgrade step.

## Cycle 5: Durable Provider Saves

**Status:** FAIL

**Findings (Critical):** none.

**Findings (Important):**

- `P19` The push downgrade proof is not evergreen: after the feature merge,
  `github.event.before` can already contain `CommitProviderSave`, blocking
  unrelated pushes. Recommendation: keep restart coverage on every event and
  run old-protocol downgrade coverage only against a pull request base known to
  predate the RPC. _Resolution: Task 5 corrected to a PR-only downgrade step;
  releaseguard rejects `github.event.before`._

**Findings (Minor):**

- `P20` Stable successful `provider add --json` output lacked a named contract
  test. _Resolution: Task 4D adds
  `TestProviderAddJSONIncludesStableOperationID`._
- `P21` The focused downgrade red command omitted the opt-in SHA and therefore
  skipped its central test. _Resolution: Task 5's red command pins the known
  pre-RPC revision `8cb5602166ffe529a0f05101dff583bad0919415`._

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Worktree isolation, focused tests, Go formatting, Windows support, and exact staging remain represented. |
| Assumptions under attack | Finding | P19 rejects the assumption that every push predecessor predates the new RPC. |
| Repo-precedent conflicts | Finding | P19 would make default-branch CI permanently brittle after merge. |
| Artifact-class precedent | Clean | Red/green checkpoints, exact files, commands, and commits remain explicit. |
| YAGNI violations | Clean | Durable saves and compatibility proof remain within the accepted design. |
| Missing failure modes | Finding | P19/P21 cover future push predecessors and skipped downgrade execution. |
| Security / privacy at architecture level | Clean | Metadata-only operations and sentinel secret proofs remain intact. |
| Infrastructure impact | Finding | P19 could block unrelated future default-branch pushes. |
| Multi-component validation | Clean | Proto, daemon, CLI, TUI, secrets, redactor, lifecycle, and CI remain joined. |
| Declared integration proof | Clean | Persistent-root PTY/ConPTY tests use the real RPC and secret provider. |
| Contributed UI rendering proof | Clean | TUI routing, bounded rendering, and durable states remain covered. |
| Rollback story | Finding | P19 made the automatic revision choice unsustainable; PR-base selection corrects it. |
| Simpler alternative not considered | Finding | A PR-only pre-RPC revision is simpler and evergreen. |
| User-intent drift | Clean | CLI/TUI unification, Windows support, and release validation remain in scope. |
| Existence / runtime-validity | Finding | P19 fails once a push predecessor contains the RPC. |
| Over/under-decomposition | Clean | Internal checkpoints preserve the locked 11-task/4-PR manifest. |
| Verification-class mismatch | Finding | P20/P21 require direct JSON and opt-in downgrade proofs. |
| Auth/authz chain composition | Clean | Empty/nil auth boundaries and specialized auth paths remain covered. |
| Hidden serial dependencies | Clean | Proto, schema, engine, locking, CLI, TUI, and runtime checkpoints stay ordered. |
| Missing rollback wiring | Finding | The wiring existed, but P19 made routine post-merge execution invalid. |
| Missing integration proof | Clean | Full production and terminal saves inspect operation, provider, secret, redactor, and output state. |
| Missing declared integration matrix | Clean | Unix, Windows, CLI, TUI, legacy/new RPC, restart, and downgrade paths remain named. |
| Missing contributed UI route proof | Clean | App routing and durable onboarding tests remain named in Task 4E. |
| Infrastructure verification mismatch | Finding | Releaseguard must reject push-predecessor downgrade wiring. |
| Plugin-loader runtime layout | Clean | No plugin-layout change is introduced. |
| Config-validation schema rules | Clean | Additive migrations and legacy-schema failures remain covered. |
| Identifier / naming-convention match | Finding | P20 adds a named test for the public `operation_id` field. |
| Planned-code compile-validity | Clean | Checkpoint ordering and generated-proto work remain compile-valid. |

**Alternatives:** PR-only base compatibility; pinned pre-RPC compatibility
revision; workflow-dispatch compatibility input; explicit red/green base SHA.

**Verdict reasoning:** FAIL; P16-P18 were resolved, but P19 exposed a new
evergreen-CI defect. Task 5 now limits downgrade proof to the PR compatibility
boundary while preserving restart coverage on every event; P20-P21 are also
resolved for the next cycle.

## Cycle 6: Durable Provider Saves

**Status:** FAIL

**Findings (Critical):** none.

**Findings (Important):**

- `P22` A PR base is not permanently pre-RPC; stacked and future PR bases make
  the old-protocol check fail vacuously. _Resolution: CI and local proof now use
  verified pre-RPC revision `8cb5602166ffe529a0f05101dff583bad0919415` and
  releaseguard rejects event-derived revisions._
- `P23` Downgrade proof covered new-to-old reads but not legacy mutation followed
  by re-upgrade, despite ADR 0006's cleanup claim. _Resolution: Task 5 now
  requires new save → old read/upsert → new restart/cleanup → new durable save,
  with pointers, credentials, journal, and cleanup assertions._
- `P24` The rollback procedure reverted the test before running it, allowing a
  no-match `go test -run` success. _Resolution: the named verbose proof must run
  and emit its PASS line before reverting the harness._

**Findings (Minor):**

- `P25` Broad local verification omitted the downgrade opt-in and skipped it.
  _Resolution: Step 4 repeats the exact pinned command with `-v`._

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Go, existing secrets/Redactor, Windows, real consumers, release checks, and portfolio closeout remain covered. |
| Assumptions under attack | Finding | P22 disproves permanent pre-RPC PR bases. |
| Repo-precedent conflicts | Finding | P22 would bind permanent CI to an introducing-PR-only condition. |
| Artifact-class precedent | Finding | P24 allowed a named rollback smoke to disappear before execution. |
| YAGNI violations | Clean | Journal, lock, cleanup, and RPC remain justified by the failure model. |
| Missing failure modes | Finding | P22/P23 cover future bases and legacy mutation followed by re-upgrade. |
| Security / privacy at architecture level | Clean | Sentinel checks still span SQL, RPC, logs, terminal output, files, and redaction. |
| Infrastructure impact | Finding | P22 could block all later PR CI. |
| Multi-component validation | Finding | P23 adds the missing old-writer/new-startup boundary. |
| Declared integration proof | Finding | P23 closes the mixed-version cleanup claim. |
| Contributed UI rendering proof | Clean | PTY/ConPTY still exercises catalog and persistent save state. |
| Rollback story | Finding | P23/P24 correct lifecycle coverage and execution order. |
| Simpler alternative not considered | Finding | The verified pinned revision is simpler than event history. |
| User-intent drift | Finding | P22/P24 undermined reliable autonomous merge and rollback. |
| Existence / runtime-validity | Finding | The pinned SHA exists, is reachable with full history, and lacks the RPC; the post-revert test did not exist. |
| Over/under-decomposition | Clean | Internal checkpoints preserve the locked manifest. |
| Verification-class mismatch | Finding | P23-P25 add bidirectional, non-vacuous, explicit local proof. |
| Auth/authz chain composition | Clean | No new network authz chain is introduced. |
| Hidden serial dependencies | Finding | P24 ordered test execution after deleting the test. |
| Missing rollback wiring | Finding | The prior rollback order was not executable. |
| Missing integration proof | Finding | P23 adds legacy mutation and upgraded cleanup/finalization. |
| Missing declared integration matrix | Finding | The mixed-version matrix claim now maps to the full lifecycle. |
| Missing contributed UI route proof | Clean | The actual shell and provider content remain exercised. |
| Infrastructure verification mismatch | Finding | Releaseguard now enforces a stable compatibility boundary. |
| Plugin-loader runtime layout | Clean | No external plugin executable is added. |
| Config-validation schema rules | Clean | Additive schema, upgrade, conflict, and repeat initialization remain named. |
| Identifier / naming-convention match | Clean | RPC, command, environment, and JSON identifiers match repository conventions. |
| Planned-code compile-validity | Clean | Generated proto and package test ordering remain valid. |

**Alternatives:** pinned compatibility boundary; conditional historical proof;
bidirectional compatibility lifecycle; verifier retained outside reverted code.

**Verdict reasoning:** FAIL; P19-P21 are resolved, but P22 transferred the
event-history defect to future PRs and P23-P25 exposed incomplete/vacuous
rollback proof. Task 5 now addresses all four findings for Cycle 7.
