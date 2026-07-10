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
