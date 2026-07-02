### Adversarial Review Report

**Phase:** plan
**Artifact:** `docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow.md`
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `P1` [Hidden serial dependency / missing integration proof] Task 3 binary smoke exports raw history after running fixture ACP prompt, but Tasks 2-3 do not explicitly persist `Result.Events` into the store sidecar during `executeACPClientExec`, drain/watch completion, or flow/compare runs. Raw export could still fail with `ErrRawHistoryUnavailable` for new ratchet sessions. Recommendation: add a Task 3 implementation bullet and test asserting live `exec` writes sidecar events before raw export.
- `P2` [Planned-code compile-validity] Task 7 says "extend `FlowPromptRunner` with optional event access" and "existing fake runners keep compiling through a small adapter/default method path." Go interfaces cannot have optional methods. Recommendation: plan a separate `interface{ LastEvents() []EventLogLine }` type assertion helper instead of adding `LastEvents` to `FlowPromptRunner`.
- `P3` [Hidden serial dependency] Task 8 adds `HarnessEmulationDocs` expectations for new commands before Task 9 updates public docs. PR3 would fail its own docs guard until PR4. Recommendation: move docs-guard expectation changes to Task 9, or update docs in Task 8's PR. Since PR4 owns docs, keep docs guard in Task 9.
- `P4` [Runtime-validity] Task 10 says run `bash hooks/scope-lock-complete ...` from the autodev plugin checkout. The helper resolves the plan relative to `$PWD`; from the plugin checkout it will not find ratchet-cli's plan. Recommendation: run the absolute helper path from the ratchet closeout worktree.

**Findings (Minor):**
- `P5` [Verification-class mismatch] Task 4 compare bundle tests mention `run_id`/`run_dir` on rows, while Task 5 allows a wrapper result. Recommendation: standardize the expected JSON shape before implementation to avoid CLI/API drift.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Plan includes Windows builds, SDK reuse, sensitive artifact handling, and deferred policy-heavy scope. |
| Assumptions under attack | Finding | Raw export assumes sidecar persistence from live runs; P1. |
| Repo-precedent conflicts | Clean | Tasks target existing `internal/acpclient` and `cmd/ratchet` patterns. |
| Artifact-class precedent | Clean | Test/docs artifacts follow sibling file layout. |
| YAGNI violations | Clean | Multi-PR shape matches broad user ask and release requirement. |
| Missing failure modes | Finding | New live sessions with no persisted sidecar would fail raw export; P1. |
| Security/privacy architecture | Clean | Human output avoids raw payloads; artifacts local. |
| Infrastructure impact | Clean | Local state plus release assets only. |
| Multi-component validation | Finding | P1 blocks live-run raw archive proof. |
| Declared integration proof | Clean | Integration matrix enumerates runtime/deferred surfaces. |
| Contributed UI rendering proof | Clean | No UI. |
| Rollback story | Clean | Runtime-affecting tasks include rollback notes. |
| Simpler alternative not considered | Clean | Design already rejected docs-only and stdio tap. |
| User-intent drift | Clean | Archive, compare, flow, release all covered. |
| Existence/runtime-validity | Finding | P4 closeout helper invocation path invalid from stated CWD. |
| Over/under-decomposition | Clean | 10 tasks across 4 PRs is reasonable for blast radius. |
| Verification-class mismatch | Minor | Compare JSON shape ambiguity; P5. |
| Auth/authz chain composition | Clean | No auth chain added. |
| Hidden serial dependencies | Finding | P3 docs guard before docs update. |
| Missing rollback wiring | Clean | Each runtime PR has rollback notes. |
| Missing integration proof | Finding | P1 live sidecar persistence proof missing. |
| Missing declared integration matrix | Clean | Matrix present. |
| Missing contributed UI route proof | Clean | No UI. |
| Infrastructure verification mismatch | Clean | Release gate includes GoReleaser/Homebrew checks. |
| Plugin-loader runtime layout | Clean | No plugin loader changes. |
| Config-validation schema rules | Clean | No schema config emitted. |
| Identifier/naming convention match | Clean | CLI names match existing `acp client` style. |
| Planned-code compile-validity | Finding | Optional Go interface wording in P2. |

**Options the author may not have considered:**
1. Persist events in the store only from command-level execution paths instead of inside `Client`: keeps `Client` reusable and lets tests assert exact CLI side effects.
2. Use a result wrapper for compare JSON: `{run_id, run_dir, status, rows}` is cleaner than adding run fields to every row.

**Verdict reasoning:** FAIL until P1-P4 are revised; P5 can be handled as a minor clarity improvement in the same plan patch.
