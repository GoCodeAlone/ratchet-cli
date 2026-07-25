### Adversarial Review Report

**Phase:** design
**Artifact:** `docs/plans/2026-07-25-ratchet-cli-ci-sdk-security-design.md`
**Status:** PASS

**Findings (Critical):**

- None.

**Findings (Important):**

- `D1` [security/privacy; declared integration proof] [`Release-Driven Registry Sync`]: “public/main manifest” conflated repository state, the public per-plugin endpoint, and bulk-index exposure for a private plugin. Recommendation: prove repository-main and Pages per-plugin metadata, retain bulk-index private exclusion, and do not invent an authenticated static endpoint. _Resolution: design updated after runtime-validity check._
- `D2` [security; assumptions under attack] [`Release-Driven Registry Sync`]: “repository identifies that plugin repository” left alias identity normalization ambiguous and could authorize an unintended owner/prefix. Recommendation: require exact normalized `GoCodeAlone/workflow-plugin-<name>` identity and negative mismatch coverage. _Resolution: design updated._
- `D3` [missing failure mode; multi-component validation] [`Release-Driven Registry Sync`]: the prior failure returned a green run after `skip=1`; requiring only workflow success/generated-PR observation could repeat the false green when `changed=0` or no PR appears. Recommendation: compare repository manifest version to the released tag after every dispatch and stop the release cascade on mismatch. _Resolution: design updated._

**Findings (Minor):**

- `D4` [dependency trust boundary] [`GitHub Action Runtimes`]: floating major tags are mutable. Recommendation: consider immutable SHAs. _Resolution: accepted because both target repos consistently use publisher major tags and this phase updates only those existing contracts; workflow-registry's separate SHA policy is unchanged._
- `D5` [overhead / simpler alternative] [`Release And Closeout`]: eight PRs and five tags impose substantial release cost. Recommendation: combine action and SDK changes if speed is the primary constraint. _Resolution: accepted because the operator requires a release for every merge and independent rollback/diagnosis is the design's central risk control._

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Owner-first SDK custody, native Windows consumer proof, release-only merge commits, and bounded gates follow `docs/design-guidance.md`. |
| Assumptions under attack | Finding | D2 tightened alias identity; SDK/API and version-number assumptions already have fallbacks. |
| Repo-precedent conflicts | Clean | Plugin-first release then ratchet consumption matches prior provider plans. |
| Artifact-class precedent | Clean | Registry fix extends `tests/test-resolve-sync-plugin-filter.sh`; ratchet extends releaseguard; plugin adds a repository workflow contract test. |
| YAGNI violations | Clean | Scope excludes org-wide Actions, unrelated dependencies, runner replacement, and queued lifecycle fixes. |
| Missing failure modes | Finding | D3 adds a version-state gate independent of workflow conclusion. |
| Security / privacy at architecture level | Finding | D1 and D2 tighten private visibility and alias authorization. |
| Infrastructure impact | Clean | Existing Actions, registry deployment, and release paths are named; no new resources or IAM. |
| Multi-component validation | Finding | D3 strengthens release-to-registry state proof. |
| Declared integration proof | Finding | D1 aligns proof with the real Pages per-plugin endpoint and bulk-index exclusion; integration matrix covers all dependencies. |
| Contributed UI rendering proof | Clean | No UI contribution. |
| Rollback story | Clean | Action, SDK, registry, tag, and module rollback constraints are explicit. |
| Simpler alternative not considered | Clean | Combined repo PRs and downstream overrides are evaluated and rejected. |
| User-intent drift | Clean | Design covers Actions, owner-first SDKs, Windows, registry publication, releases, and autonomous merges. |
| Existence / runtime-validity | Clean | Target workflows, release tags/assets, registry manifest/resolver/tests, and releaseguard exist; v0.12.8 run logs reproduce the boundary failure. |

**Options the author may not have considered:**

1. Pin every Action to an immutable commit SHA. This reduces tag-mutation risk but diverges from both target repositories and expands maintenance beyond the requested Node runtime migration.
2. Publish one combined plugin Actions+SDK release. This removes one plugin and registry release cycle but loses the ability to distinguish release-runtime migration failures from Docker/Ollama API churn.

**Verdict reasoning:** PASS after D1-D3 resolution. Remaining D4-D5 are explicit, precedent-aligned trade-offs rather than correctness or security blockers.
