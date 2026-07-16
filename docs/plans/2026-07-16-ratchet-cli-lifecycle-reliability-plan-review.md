### Adversarial Review Report

**Phase:** plan
**Artifact:** `docs/plans/2026-07-16-ratchet-cli-lifecycle-reliability.md`
**Status:** PASS

**Findings (Critical):**

None.

**Findings (Important):**

- `P1` [Verification-class mismatch] [Task 1 Step 1]: stopping the manager before closing the DB lets canceled context mask the intended candidate-query failure. Recommendation: use an unstarted manager with live test context and closed DB. _Resolution: plan revised._
- `P2` [Verification-class mismatch] [Task 1 Step 4]: restoring the old void signature would prove only a compile failure, not that tests catch dropped query/close/reporting behavior. Recommendation: retain signatures and behaviorally disable each fix during revert proof. _Resolution: plan revised._
- `P3` [Missing failure mode] [Task 3 Step 2]: increasing child wait bounds without joining after timeout can leave a child/zombie on the failure path. Recommendation: kill and drain `Wait` before failing. _Resolution: plan revised._

**Findings (Minor):**

- `P4` [Over-decomposition] [Task 6]: a docs-only closeout PR causes a second patch release. Recommendation: omit only if operator release policy changes. _Resolution: accepted because post-merge evidence cannot be written truthfully before PR #1 and the operator explicitly requires releases for every merge._

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Tasks map bounded failure, shared contracts, smoke isolation, native Windows, and release proof. |
| Assumptions under attack | Clean | Plan implements the reviewed ACP notification, process reap, durable row, and smoke-budget assumptions. |
| Repo-precedent conflicts | Clean | Workflow tests use existing structured `loadWorkflow`/`requireRun` helpers and current jobs. |
| Artifact-class precedent | Clean | Daemon/ACP tests remain in owner packages; workflow contract remains in releaseguard. |
| YAGNI violations | Clean | No public API, schema, framework, runner, or dependency change. |
| Missing failure modes | Finding | P3 corrected process cleanup on timeout; cleanup restart and log-flood cases are planned. |
| Security / privacy at architecture level | Clean | Tests require sanitized startup errors and cleanup logs omit secret/prompt payloads. |
| Infrastructure impact | Clean | Existing Linux/Windows jobs only; no deploy, resource, IAM, network, or migration. |
| Multi-component validation | Clean | Real SQLite/provider, service/manager, SDK/process, profile/lock, CI, archive, and package boundaries are exercised. |
| Declared integration proof | Clean | ACP and secrets are runtime-proved; Actions is config-guarded and executed on PR. |
| Contributed UI rendering proof | Clean | No UI contribution. |
| Rollback story | Clean | Runtime/CI task has rollback; no data migration exists. |
| Simpler alternative not considered | Clean | Design rejects tests-only timeout changes with rationale. |
| User-intent drift | Clean | Every queued lifecycle follow-up, Windows proof, merge, release, and continuation is represented. |
| Existence / runtime-validity | Clean | Named files/tests/jobs/helpers and commands exist; new selectors are created before workflow use. |
| Over/under-decomposition | Finding | P4 is accepted process overhead; code tasks remain independently revertible. |
| Verification-class mismatch | Finding | P1/P2 corrected behavioral RED/revert evidence; runtime/CI/release classes have matching proof. |
| Auth/authz chain composition | Clean | No auth/authz change. |
| Hidden serial dependencies | Clean | Tasks 1-4 are explicitly serial on one branch; Task 6 depends on PR #1 evidence. |
| Missing rollback wiring | Clean | CI/runtime rollback is in Task 4/5; internal logic reverts cleanly. |
| Missing integration proof | Clean | Task 5 requires real host runtime and native Windows PR execution. |
| Missing declared integration matrix | Clean | Design matrix is implemented by Tasks 1, 3, 4, and 5. |
| Missing contributed UI route proof | Clean | No UI route. |
| Infrastructure verification mismatch | Clean | Structured YAML guard and hosted execution match the CI-only impact. |
| Plugin-loader runtime layout | Clean | No external Workflow plugin process is built or loaded. |
| Config-validation schema rules | Clean | No Workflow config is emitted. |
| Identifier/naming-convention match | Clean | Test names, job IDs, step names, branches, and Go identifiers match repository style. |
| Planned-code compile-validity | Clean | Rows interface matches `sql.Rows`; commands/selectors and Go snippets are compile-valid. |

**Options the author may not have considered:**

1. Keep only package-timeout enforcement and no per-transition process bound: fewer helpers, but slower deadlock diagnosis and less actionable failures.
2. Put all reliability work in separate daemon/ACP PRs: smaller diffs, but CI skip/selector changes would temporarily mismatch one side and require an extra release.

**Verdict reasoning:** The three actionable Important findings are resolved. The accepted second-PR overhead is required for truthful post-merge evidence and explicit release policy. The plan is TDD-oriented, structurally bounded, native-Windows aware, and provides matching unit, race, process, workflow, package, and release verification.
