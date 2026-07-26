# Retro: Ratchet CLI CI And SDK Security Maintenance

**PRs:** workflow-registry #629, #630, #631; ratchet-cli #144, #145;
workflow-plugin-agent #42, #43
**Merged:** 2026-07-25 through 2026-07-26
**Releases:** ratchet-cli `v0.30.38`, `v0.30.39`;
workflow-plugin-agent `v0.12.9`, `v0.12.10`
**Design:** `docs/plans/2026-07-25-ratchet-cli-ci-sdk-security-design.md`
**Plan:** `docs/plans/2026-07-25-ratchet-cli-ci-sdk-security.md`
**Related ADRs:** None

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1: private plugin repository, per-plugin Pages, and bulk-index state were conflated | Important | Resolved upfront: both registry publications proved the private per-plugin document while retaining bulk-index exclusion. |
| design | D2: short-name alias identity was under-constrained | Important | Resolved upfront: exact repository identity and mismatch tests shipped in #629. |
| design | D3: a green sync run could still skip publication | Important | Prescient: the prior v0.12.8 dispatch had returned `skip=1`; repaired short-name dispatches generated #630 and #631, and live version/hash checks independently proved state. |
| design | D4: mutable Action major tags | Minor | Inconclusive: repository precedent held and all workflows passed, but publisher tag mutability remains. |
| design | D5: eight PRs and five releases were operationally expensive | Minor | Inconclusive: the cost was real, while independent rollback and release diagnosis worked as designed. |
| plan | P1: post-lock version changes would invalidate version-derived branches | Important | Resolved upfront: all locked tags and branch rows remained exact. |
| plan | P2: canonical dispatch would not test the repaired `agent` alias | Important | Resolved upfront: live dispatch used `agent` and resolved `workflow-plugin-agent`. |
| plan | P3: plugin GoReleaser hooks could dirty feature worktrees | Important | Resolved upfront: disposable source copies produced six-platform snapshots while feature trees stayed clean. |
| plan | P4: owner plugin lacked native Windows CI | Minor | Resolved upfront: owner releases cross-built Windows and ratchet's native `windows-2025` consumer gates passed after both SDK updates. |
| plan | P5: eight-PR over-decomposition | Minor | Inconclusive: generated registry PRs added latency but isolated every release and deployment boundary. |
| plan | P6: Action major tags were not SHA-pinned | Minor | Inconclusive: unchanged repository policy passed every PR, merge, and tag workflow. |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| The design treated Docker Engine `docker-v29.6.2` as a valid `github.com/docker/docker@v29.6.2+incompatible` Go module version. | `adversarial-design-review` (design), then plan review | Existence checking stopped at the product/source tag instead of resolving the exact Go module coordinate. | Resolve every pinned module with `go mod download` or `go list -m` before lock. |
| A combined local artifact/runtime verification continued after releaseguard hit a corrupt Go build-cache object. | `verification-before-completion` command construction | The shell command lacked fail-fast mode, so a later successful probe masked the earlier non-zero command status. Log inspection caught it and the gate was rerun after rebuilding the disposable cache. | Use `set -euo pipefail` for multi-command verification transcripts. |

The only PR CI failure was workflow-registry run `30172819761`: moving workflow
`main` raised its Go floor after plan lock. Direct reproduction identified the
1.26.4/1.26.5 skew, the design was backported without changing scope, and
replacement run `30173904211` plus main validation/deploy passed. This was
post-lock dependency skew, not a missed pre-lock fact.

No GitHub review thread requested a change. Four local adversarial code-review
rounds on Task 7 found root-only graph checks, non-hermetic nested Go commands,
test-inclusive ownership proof, and host-only/parallel ownership gaps before
PR creation; the final verdict was `SHIP-IT`.

## Missed skill activations

The canonical `.claude/autodev-state/in-progress.jsonl` activation log is
unavailable. Committed artifacts and execution evidence prove outputs, not
hook-recorded activations.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | activation log unavailable | Design artifact exists. |
| adversarial-design-review (design) | activation log unavailable | Committed report contains D1-D5. |
| writing-plans | activation log unavailable | Locked eight-task, eight-PR plan exists. |
| adversarial-design-review (plan) | activation log unavailable | Committed report contains P1-P6. |
| alignment-check / scope-lock | activation log unavailable | Alignment PASS and verified scope lock exist. |
| subagent-driven-development | activation log unavailable | Task commits, releases, and cross-repository evidence exist. |
| finishing-a-development-branch | activation log unavailable | Every authored PR carried scope and verification evidence. |
| finishing Step 1e (doc-reconciliation) | yes | Docker split-module and registry Go-floor backports are committed. |
| pr-monitoring | activation log unavailable | Every PR, merge, release, registry deployment, and review thread was followed to settlement. |
| post-merge-retrospective | activation log unavailable | This retro supplies the required output artifact. |

## What worked

- Owner-first remediation removed the legacy Docker module without copying SDK
  code; plugin and ratchet Dependabot alert counts both reached zero.
- Release-state checks caught the original registry false green at the state
  boundary. The repaired dispatches produced reviewed manifests whose six
  hashes matched v0.12.9 and v0.12.10, while private bulk-index count stayed
  zero.
- Separating Action and SDK PRs made all four releases independently
  diagnosable. Release runs `30176230104`, `30177716512`, `30179497372`, and
  `30182453196` each published seven expected assets.
- Task 7's strengthened guard proves exact selected modules, no protected
  replacements or legacy Docker, and exclusive production ownership through
  workflow-plugin-agent across all six release targets.

## What didn't

- Product release tags and Go module versions were treated as interchangeable
  until `go mod download` disproved the assumption during Task 5.
- A moving cross-repository `main` dependency invalidated a static workflow Go
  floor after lock and required an execution backport.
- The first local closeout shell did not fail fast. The visible error was
  noticed, but command structure should have made a false green impossible.

## Plugin-level follow-ups

No plugin-level change is warranted from one initiative. If another retro finds
product-tag/module-version confusion or a non-fail-fast verification transcript,
promote those checks into the shared adversarial-review and
verification-before-completion skills.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `docs/design-guidance.md` | updated | Future designs must resolve real Go module coordinates before lock, prove selected dependency ownership across release targets, handle moving sibling toolchain floors, and use fail-fast verification transcripts. |
