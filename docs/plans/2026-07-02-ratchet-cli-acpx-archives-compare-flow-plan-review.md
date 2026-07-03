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

## Cycle 2

### Adversarial Review Report

**Phase:** plan
**Artifact:** `docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow.md`
**Status:** PASS

**Findings (Critical):**
- None.

**Findings (Important):**
- None.

**Findings (Minor):**
- `P6` [Over-decomposition] Task 10 mixes release, retro, and workspace state. Acceptable because this repo's recent release plans use a closeout task after feature PRs, and the verification steps are explicit.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Windows, SDK reuse, local-sensitive metadata, and deferred automation are wired into tasks. |
| Assumptions under attack | Clean | Live event sidecar persistence and raw export fail-closed behavior are now task requirements. |
| Repo-precedent conflicts | Clean | Follows prior ratchet-cli plan/PR/release closeout pattern. |
| Artifact-class precedent | Clean | Test, CLI, docs, and retro artifacts follow sibling layout. |
| YAGNI violations | Clean | Scope maps to archive/compare/flow/release ask. |
| Missing failure modes | Clean | Raw-history-unavailable path, path containment, invalid JSON-RPC, and no-payload human summaries are covered. |
| Security/privacy architecture | Clean | Sensitive artifact warnings and local-only proof included. |
| Infrastructure impact | Clean | Local state plus release assets; release verification is explicit. |
| Multi-component validation | Clean | Fixture ACP agent, CLI, sidecars, compare bundles, flow replay, docs, release are covered. |
| Declared integration proof | Clean | Matrix classifies runtime/config/deferred surfaces. |
| Contributed UI rendering proof | Clean | No UI. |
| Rollback story | Clean | Runtime tasks have rollback notes; release rollback avoids rewriting public history. |
| Simpler alternative not considered | Clean | Covered in design review. |
| User-intent drift | Clean | Archive, compare, flow orchestration, Windows, release, workspace state all included. |
| Existence/runtime-validity | Clean | Scope-lock helper invocation is now from ratchet worktree with explicit helper path placeholder. |
| Over/under-decomposition | Minor | Task 10 is broad but matches closeout precedent. |
| Verification-class mismatch | Clean | CLI, runtime-integrated, docs, release, and Windows checks match change classes. |
| Auth/authz chain composition | Clean | No auth chain added. |
| Hidden serial dependencies | Clean | Docs guard moved to docs task; PR3 no longer depends on PR4 docs updates. |
| Missing rollback wiring | Clean | Present in runtime and release tasks. |
| Missing integration proof | Clean | Live event sidecar persistence now required in Task 3. |
| Missing declared integration matrix | Clean | Present. |
| Missing contributed UI route proof | Clean | No UI. |
| Infrastructure verification mismatch | Clean | GoReleaser check, release workflow, assets, and Homebrew cask check are required. |
| Plugin-loader runtime layout | Clean | No plugin loading change. |
| Config-validation schema rules | Clean | No generated config schema. |
| Identifier/naming convention match | Clean | CLI names match existing `acp client` hierarchy. |
| Planned-code compile-validity | Clean | Optional flow event access now uses Go type assertion instead of interface mutation. |

**Options the author may not have considered:**
1. Split release/retro/workspace into separate plans: rejected because recent ratchet release closeouts already use a release-closeout PR plus workspace PR.
2. Store compare bundles only when `--json`: rejected because `--save` is the explicit artifact control independent of output format.

**Verdict reasoning:** PASS. Prior Important findings P1-P4 and minor P5 were resolved in plan commit `381363b`; no remaining blocker.
