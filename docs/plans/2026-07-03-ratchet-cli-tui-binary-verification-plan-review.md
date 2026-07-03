# TUI Binary Verification Plan Review

## Cycle 1

### Adversarial Review Report

**Phase:** plan
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Status:** FAIL

**Findings (Critical):**
- `P1` [Verification-class mismatch / Missing integration proof] [plan:181,512; design:177-183,434-436]: `TestTUIBinarySmoke` was planned with build constraint `tui_smoke && !windows`, but the design requires the test itself to run in normal non-race Unix CI while building the smoke binary with `-tags tui_smoke`. Fix: make the test file `//go:build !windows`, keep smoke runtime files tagged `tui_smoke && !windows`, and have the test build `cmd/ratchet-tui-smoke` with `-tags tui_smoke`.

**Findings (Important):**
- `P2` [Hidden serial dependency / Scope manifest mismatch / Homebrew tap prerequisite] [plan:17-37,444,469-471,550-557; design:574-577,588-602,908]: External `GoCodeAlone/homebrew-tap` cleanup/merge prerequisite was embedded inside Task 8 but not represented in the locked manifest. Fix: add explicit prerequisite task with clone/branch/PR/commit verification and block fail-closed release/tap enforcement until tap cleanup SHA and ratchet-cli formula automation SHA are recorded.
- `P3` [Release proof / stale artifact verification] [plan:481-489,578-585; design:551-563,665-747]: Plan used `scripts/check-release-artifacts.sh --manifest-only dist` after GoReleaser config changes without regenerating `dist`. Fix: run default wrapper or explicit GoReleaser snapshot before manifest-only checks.
- `P4` [Missing local Windows proof] [plan:639-649; design:489-514,890-910]: Final local verification omitted Windows amd64/arm64 cross-build commands. Fix: add temp-output Windows cross-builds to Task 11/13 verification.

**Findings (Minor):**
- `P5` [Workflow verification mismatch] [plan:515-522,535-541]: Workflow edits made `actionlint` optional. Fix: require `actionlint .github/workflows/ci.yml .github/workflows/release.yml` or equivalent required workflow syntax check.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | P1/P4 weakened executable TUI and Windows proof. |
| Assumptions under attack | Finding | Tagged test file and external tap cleanup manifest ownership were load-bearing. |
| Repo-precedent conflicts | Clean | Plan mostly follows existing Go test, CI private-module setup, and GoReleaser workflow shapes. |
| Artifact-class precedent | Finding | External tap artifact has separate repo/PR ownership but was not scoped as such. |
| Missing failure modes | Finding | Stale release artifact proof after config changes. |
| Infrastructure impact | Finding | Tap/release gates and artifact validation affected. |
| Multi-component validation | Finding | PTY proof could be skipped in CI. |
| Declared integration proof | Finding | Homebrew tap prerequisite cleanup was not locked. |
| Rollback story | Finding | External tap rollback was not tied to manifest ownership. |
| Verification-class mismatch | Finding | P1/P3/P4/P5. |
| Hidden serial dependencies | Finding | Tap cleanup must land before fail-closed release enforcement. |

**Options the author may not have considered:**
1. Separate locked prerequisite phase for tap cleanup.
2. Untagged Unix-only `TestTUIBinarySmoke` building tagged smoke binary internally.
3. Replace post-config-change manifest-only checks with the default fresh snapshot wrapper.

**Verdict reasoning:** FAIL. The plan could complete while proving less than the design requires for PTY, tap, release artifacts, and Windows.
