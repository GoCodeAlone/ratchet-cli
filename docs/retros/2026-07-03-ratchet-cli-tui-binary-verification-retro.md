# Retro: TUI Binary Verification

**PR:** #81 - docs: close tui binary verification
**Merged:** pending
**Branch:** docs/tui-verification-closeout
**Design:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Plan:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Related ADRs:** decisions/0001-smoke-package-list-boundary.md

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1 rejected a hidden shipped smoke path. | Important | Resolved upfront: the smoke entrypoint is build-tagged and release artifacts are scanned for smoke surfaces. |
| design | D6 required public docs to split release-shaped startup proof from non-release PTY proof. | Important | Prescient: PR #80 needed per-document release-evidence guards after review found aggregate-only assertions. |
| design | D15 required symlink-aware socket containment and mode checks. | Important | Resolved upfront: smoke client tests assert temp-root containment before dialing. |
| design | D17/D21 required deterministic release artifact and GoReleaser guards. | Important | Prescient: PRs #76 and #78 added releaseguard manifest, draft asset, and publish-surface checks before `v0.26.0`. |
| design | D125 required tap cleanup evidence before fail-closed tap enforcement. | Important | Prescient: GoCodeAlone/homebrew-tap#63 landed before CI and release tap guards merged. |
| plan | P3 required fresh GoReleaser snapshot generation before manifest inspection. | Important | Resolved upfront: CI and release workflows regenerate artifacts before manifest-only releaseguard checks. |
| plan | P4 required Windows proof without changing runners. | Important | Resolved upfront: local and CI proof covers Windows amd64/arm64 cross-builds plus packaged archive inspection. |
| plan | P16 required draft release configuration and postcheck proof. | Important | Prescient: release workflow kept `v0.26.0` draft until asset and tap postchecks passed. |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| `GuardDraftAssets` accepted metadata with no `draft` field. | test-driven-development / implementation review | The first regression tested `draft:false`, not a missing required field. | For release metadata invariants, test explicit bad values and omitted fields. |
| Release evidence docs guard was aggregate-only. | test-driven-development / implementation review | Required phrases could appear somewhere in the public docs set without every public doc carrying the release boundary evidence. | For public evidence docs, test each document independently, with whitespace-normalized phrase matching. |

No CI failures slipped past local verification after review fixes. PR checks and master checks were green for each merged PR, and release run `28694540082` completed successfully for `v0.26.0`.

## Missed skill activations

Activation log unavailable at the canonical repo root `.claude/autodev-state/in-progress.jsonl`; rows below are reconstructed from committed artifacts and PR evidence.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | unknown | Activation log unavailable. |
| adversarial-design-review (design) | yes | `docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design-review.md` exists. |
| writing-plans | yes | Locked implementation plan exists with 13 tasks and six PR closeout groups. |
| adversarial-design-review (plan) | yes | `docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-plan-review.md` exists. |
| alignment-check | yes | `docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-alignment.md` exists. |
| scope-lock | yes | `docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md.scope-lock` exists. |
| subagent-driven-development | unknown | Activation log unavailable; implementation evidence is in the PR sequence. |
| finishing Step 1e (doc-reconciliation) | yes | User-facing docs changed in PR #80 and were guarded by `TestHarnessDocs`. |
| pr-monitoring | yes | PRs #72, #74, #76, #78, and #80, plus retros #73, #75, #77, and #79, were monitored through checks and review resolution. |
| post-merge-retrospective | yes | Phase retros exist, and this final closeout retro records the overall design outcome. |

## What worked

- The locked PR sequence kept PTY smoke, startup/command proof, releaseguard, tap and Windows gates, public docs, and closeout state independently reviewable.
- The releaseguard modes let CI, release preflight, draft asset postcheck, and tap postcheck share one fail-closed invariant set.
- `v0.26.0` proved the end-to-end release path: GitHub release remained draft until postchecks passed, then published Linux, macOS, and Windows amd64/arm64 archives plus checksums.
- Admin merge plus PR monitoring caught Copilot review issues without leaving unresolved threads or unmerged state.

## What didn't

- `Release Check` and release snapshot work are slow enough to dominate PR and tag monitoring.
- Local `gh pr merge --admin` from a worktree can try local branch operations; remote `--repo` admin merge is the reliable path.
- The activation log was unavailable, so skill firing had to be reconstructed from committed artifacts.
- Windows interactive ConPTY proof remains deferred because this plan intentionally avoided runner changes.

## Plugin-level follow-ups

No plugin-level change is warranted from this closeout alone. The useful local patterns are already visible in phase retros: test missing required metadata fields, avoid aggregate public-doc evidence assertions, and keep release artifact checks tied to exact generated fields.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `docs/design-guidance.md` | not created | No repo-local durable guidance file exists, and the closeout lessons are release/test review patterns rather than new ratchet-cli product or architecture constraints. |

## Release evidence

- PRs #72, #74, #76, #78, and #80 merged green in locked order; retro PRs #73, #75, #77, and #79 also merged green.
- External prerequisite GoCodeAlone/homebrew-tap#63 removed stale tap install surfaces before fail-closed tap enforcement.
- Local closeout verification covered focused releaseguard, harness redaction, startup/TUI/daemon/client/docs tests, tagged `tui_smoke` tests, Windows amd64/arm64 cross-builds, `go test -race ./...`, `go vet ./...`, `goreleaser check`, release artifact guards, and pinned actionlint.
- Release `v0.26.0` published from `b348d8543675fab109bf3f4c9e20bbd537225f71`; Homebrew tap `Casks/ratchet-cli.rb` was updated to version `0.26.0` at GoCodeAlone/homebrew-tap@7f31504f38accaa7763a8826fc381de4bac9c348.
