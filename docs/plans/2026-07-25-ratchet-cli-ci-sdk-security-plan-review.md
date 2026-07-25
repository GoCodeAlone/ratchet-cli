### Adversarial Review Report

**Phase:** plan
**Artifact:** `docs/plans/2026-07-25-ratchet-cli-ci-sdk-security.md`
**Status:** PASS

**Findings (Critical):**

- None.

**Findings (Important):**

- `P1` [scope-lock / assumptions under attack] [`Scope Manifest`; Tasks 3-7]: the plan named version-derived registry branches but also allowed patch numbers to advance without a manifest change. That would violate the locked Branch rows. Recommendation: require amendment, alignment, and re-lock for post-lock tag/branch changes. _Resolution: design and plan updated._
- `P2` [verification-class mismatch] [`Task 1 Step 6`]: the live smoke originally dispatched canonical `workflow-plugin-agent`, which could pass without exercising the repaired `agent` alias. Recommendation: dispatch `agent` and assert canonical filter output. _Resolution: plan updated._
- `P3` [test hermeticity / scope creep risk] [`Task 3 Step 5`; `Task 5 Step 5`]: workflow-plugin-agent's GoReleaser before hook rewrites `plugin.json` and runs `go mod tidy`; running it in the feature worktree can create unplanned staged changes. Recommendation: run snapshot validation from a disposable source copy and assert the feature tree stays unchanged. _Resolution: plan updated._

**Findings (Minor):**

- `P4` [cross-platform proof] [`Task 5`]: the owner plugin has no native Windows job; its SDK release gets cross-build proof before ratchet supplies native downstream proof in Task 7. Recommendation: add a plugin Windows job. _Resolution: accepted because the operator excluded runner work from this cluster; Task 7's existing native Windows jobs are the real consumer gate and Task 5 still cross-builds both Windows architectures._
- `P5` [over-decomposition] [`Scope Manifest`]: eight PRs and generated registry rows are operationally expensive. Recommendation: combine plugin Actions and SDK remediation. _Resolution: accepted because release-every-merge and independently diagnosable action/SDK changes are explicit requirements._
- `P6` [version-pin trust] [`Task 2`; `Task 3`]: publisher major tags remain mutable. Recommendation: SHA-pin. _Resolution: accepted repository precedent; this scope migrates existing major-tag contracts only._

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Owner-first custody, bounded gates, native ratchet Windows proof, and release-only merge commits are wired. |
| Assumptions under attack | Finding | P1 fixed version-derived branch drift; remaining SDK assumptions have test/fallback paths. |
| Repo-precedent conflicts | Clean | Cross-repo plugin-first release and generated registry PRs match existing plans/workflows. |
| Artifact-class precedent | Clean | Registry shell fixture, plugin root workflow test, and ratchet releaseguard match sibling artifact locations. |
| YAGNI violations | Clean | No org sweep, runner replacement, SDK fork, UX feature, or unrelated dependency bump. |
| Missing failure modes | Clean | False-green sync, tag collision, API drift, draft release, delayed alerts, and rollback are explicit. |
| Security / privacy | Clean | Exact alias identity, private bulk-index exclusion, no credential output, and advisory floors are tested. |
| Infrastructure impact | Clean | Existing Pages deployment/release paths are verified; production publication was explicitly approved. |
| Multi-component validation | Clean | Release dispatch, registry PR/Pages, plugin wrappers, ratchet consumer, native Windows, and Homebrew cross boundaries. |
| Declared integration proof | Clean | Integration Matrix classifies every integration and assigns real proof tasks. |
| Contributed UI rendering proof | Clean | No UI contribution. |
| Rollback story | Clean | Every runtime/version/deployment task carries a rollback note. |
| Simpler alternative | Clean | Combined PRs and downstream overrides are considered and rejected. |
| User-intent drift | Clean | Actions, registry, owner SDKs, Windows, merges, releases, and continuation are covered. |
| Existence / runtime-validity | Finding | P2 and P3 corrected live-consumer and GoReleaser behavior; target files/actions/releases exist. |
| Over/under decomposition | Finding | P5 accepted; each PR remains independently revertible. |
| Verification-class mismatch | Finding | P2 fixed; version pins include skew audit plus artifact launch. |
| Auth/authz chain composition | Clean | No auth chain change; private registry visibility follows existing static contract. |
| Hidden serial dependencies | Clean | Tasks are explicitly ordered by registry deploy, plugin tags, sync PRs, and downstream release. |
| Missing rollback wiring | Clean | Rollback is repeated in every runtime-affecting task. |
| Missing integration proof/matrix | Clean | Task 4/6 Pages proof and Task 7 released-consumer proof are mandatory. |
| Infrastructure verification mismatch | Clean | Registry tests, main deployment, and Pages polling match the existing infrastructure class. |
| Plugin-loader runtime layout | Clean | Task 3/5 requires wfctl-compatible binary plus sibling manifest. |
| Config-validation schema rules | Clean | Registry manifests run repository validators; no new Workflow config. |
| Identifier / naming match | Clean | Branches, payload `agent`, canonical directory, tags, and action identifiers match repository code. |
| Planned-code compile validity | Clean | Go snippets use valid slices/maps and no non-comparable struct operations. |

**Options the author may not have considered:**

1. Add native Windows CI directly to workflow-plugin-agent. This gives earlier
   owner proof but changes runner scope the operator asked this cluster to avoid.
2. Replace release-generated registry PRs with direct signed manifest commits.
   This reduces PR count but bypasses the registry's reviewed-sync architecture.

**Verdict reasoning:** PASS after P1-P3 resolution. P4-P6 are explicit,
precedent-aligned trade-offs and do not leave a Critical/Important gap.
