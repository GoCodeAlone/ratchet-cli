### Adversarial Review Report

**Phase:** design
**Artifact:** `docs/plans/2026-07-16-ratchet-cli-lifecycle-reliability-design.md`
**Status:** PASS

**Findings (Critical):**

None.

**Findings (Important):**

- `D1` [Missing failure modes] [`Provider Cleanup`]: logging every failed dispatch on the 250 ms ticker can flood logs during a database outage. Recommendation: suppress equivalent repeats and emit at most once per minute until a successful dispatch. _Resolution: design revised in `## Provider Cleanup`._
- `D2` [User-intent drift] [`ACP Cancellation And Process Smoke`]: moving the real-start test out of race coverage did not explicitly replace the named fixed five-second acknowledgment window. Recommendation: specify a larger isolated transition bound plus an outer package timeout. _Resolution: design now requires a 30-second transition bound and five-minute job timeout._
- `D3` [Project-guidance conflict] [`ACP Cancellation And Process Smoke`; `docs/design-guidance.md` Quality/Security/Operations]: Linux-only process smoke conflicts with the native-operating-system proof rule for process behavior. Recommendation: run the same selector on the existing Windows runner without changing runner ownership. _Resolution: design now requires Linux and native Windows process smoke._

**Findings (Minor):**

- `D4` [Declared integration proof] [`Multi-Component Validation`]: the original wording implied an OS-pipe write proved peer handling of an ACP notification. Recommendation: separate bounded OS-process cancellation/reap proof from exact in-process peer handling. _Resolution: validation wording revised._

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | D3 corrected native-platform process proof; no runner change is introduced. |
| Assumptions under attack | Clean | ACP notification, `exec.Cmd.Wait`, durable cleanup row, and smoke budget assumptions include challenges/fallbacks. |
| Repo-precedent conflicts | Clean | Dedicated smoke plus race skip follows `.github/workflows/ci.yml`'s existing ACP binary-smoke split. |
| Artifact-class precedent | Clean | CI assertions remain in `internal/releaseguard/workflow_test.go`; daemon/ACP regressions stay beside owning packages. |
| YAGNI violations | Clean | No observer API, schema, metrics backend, retry-policy, or new runner is added. |
| Missing failure modes | Finding | D1 corrected persistent query-error log flooding; joined close errors and restart-idempotent cleanup are covered. |
| Security / privacy at architecture level | Clean | Diagnostics exclude secret/provider/prompt/command payloads; cancellation reduces child lifetime. |
| Infrastructure impact | Clean | Existing jobs only; no cloud, IAM, migration, network, or production deployment impact. |
| Multi-component validation | Clean | SQLite/provider, service/manager, SDK/process, profile/process-lock, and CI guard boundaries have real proofs. |
| Declared integration proof | Finding | D4 corrected proof strength; runtime and config-only integrations are classified. |
| Contributed UI rendering proof | Clean | No UI contribution exists in scope. |
| Rollback story | Clean | Revert path has no data migration; ACP compatibility fallback retains reap guarantees. |
| Simpler alternative not considered | Clean | Tests-only and broad observer alternatives are explicitly evaluated and rejected. |
| User-intent drift | Finding | D2 corrected the fixed-start acknowledgment follow-up rather than only relocating it. |
| Existence / runtime-validity | Clean | Cited tests, CI jobs, structured workflow parser, service method, and cleanup paths exist on the base tree. |

**Options the author may not have considered:**

1. Keep all process tests in race coverage and only increase timeouts: smaller CI diff, but conflicts with durable guidance and continues whole-suite contention.
2. Add a public lifecycle-event API: richer diagnostics, but no current consumer justifies its compatibility and synchronization cost.

**Verdict reasoning:** All Important findings were incorporated into the design. Remaining risk is bounded to internal lifecycle implementation and existing CI jobs; the design now matches native-platform guidance, directly addresses every named follow-up, and avoids new public state or infrastructure.

### Process Amendment

The release section now distinguishes the implementation PR from the required post-merge retro/closeout PR. This does not change feature scope; it prevents pre-merge CI/release evidence from being fabricated in a retro and makes both merges follow the operator's release rule.
