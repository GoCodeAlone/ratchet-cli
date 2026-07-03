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

## Cycle 2

### Adversarial Review Report

**Phase:** plan
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `P6` [Windows packaged proof / Verification-class mismatch] [plan:620-630; design:494-506,682-688]: Windows job builds a source `ratchet.exe`, downloads/extracts the snapshot zip, then runs `.\\ratchet.exe`. The plan does not force the executed binary to be the extracted packaged executable, so the job could prove the source-built binary while only byte-scanning the package. Recommendation: build source output and extracted package into separate directories, run the extracted path explicitly, and assert it came from `ratchet_windows_amd64.zip`.
- `P7` [Scope manifest / Hidden external dependency] [plan:17-37,513-515,717-721]: Manifest still says `PR Count: 5`, but Task 9 may require external `GoCodeAlone/homebrew-tap` PR/direct commit before Tasks 10-11. That external merge is represented as a task but not in locked PR/prerequisite accounting. Recommendation: add explicit external prerequisite accounting with required SHA fields before Task 10 can start.

**Findings (Minor):**
- `P8` [GoReleaser taxonomy drift] [plan:388-396; `.goreleaser.yaml`:1,36-42; design:733-738]: Task 7's strict taxonomy tests name publish keys but omit explicit nonpublishable metadata keys `version` and `changelog`, both present today. Recommendation: add test bullets that classify `version` and `changelog` as nonpublishable metadata.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | P6 weakens honest Windows proof. |
| Assumptions under attack | Finding | Windows job assumed `.\\ratchet.exe` resolves to packaged binary. |
| Repo-precedent conflicts | Clean | Existing Go/GitHub Actions/GoReleaser shapes are followed. |
| Artifact-class precedent | Finding | External tap PR outside manifest PR accounting. |
| Infrastructure impact | Finding | Tap workflow external merge undercounted by manifest scope. |
| Multi-component validation | Finding | P6 can miss package-to-Windows execution boundary. |
| Declared integration proof | Finding | Windows package proof not guaranteed by command path. |
| Verification-class mismatch | Finding | P6 runs an ambiguous Windows executable path. |
| Hidden serial dependencies | Finding | P7 external tap merge remains serial prerequisite. |
| Identifier / naming-convention match | Finding | P8 omits current GoReleaser metadata keys from task taxonomy. |

**Options the author may not have considered:**
1. Skip extra source build in Windows packaged job and execute only extracted snapshot binary.
2. Split Task 9 into named external prerequisite accounting outside ratchet-cli PR grouping.

**Verdict reasoning:** FAIL. P1-P5 were addressed, but the plan still had an ambiguous Windows packaged executable path and undercounted the external tap prerequisite.

## Cycle 3

### Adversarial Review Report

**Phase:** plan
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `P9` [Verification-class mismatch / executable gap] [plan:131-162,709; design:85-90]: Smoke daemon tests target `service_tui_smoke.go`, planned behind `tui_smoke && !windows`, but Task 2 and final verification run untagged `go test`. This can fail to compile if tests reference tagged symbols, or pass while skipping smoke-mode proof. Recommendation: add explicit `go test -tags tui_smoke ./internal/daemon ...` commands and keep separate untagged checks proving release builds do not expose the helper.
- `P10` [Missing task / security proof gap] [design:155-173; plan:87-113]: Design requires concrete `ConnectSmokeUnix(ctx, tempRoot, socketPath)` contract with symlink-aware containment, final `Lstat`, `ModeSocket`, `0600`, and Unix-only dialing. Task 1 implements constructor but lacks focused socket security tests. Recommendation: add tagged `internal/client` tests for valid socket, outside-temp path, symlink final component, wrong mode, wrong permissions, unresolved parent, and TCP/non-`unix://` rejection.
- `P11` [Manifest/interface drift / release proof] [plan:528-533,600,607-625; design:689-702]: Releaseguard mode table says `draft-assets` consumes `RATCHET_RELEASE_GUARD_ASSETS` and `RATCHET_RELEASE_GUARD_VERSION`, but Task 11 adds `github_assets.go` and says postcheck runs against release id. Recommendation: choose one interface and update mode table/env/workflow/tests consistently.

**Findings (Minor):**
- `P12` [External prerequisite bookkeeping] [plan:535-546,565,603]: Task 9's backport-note template records only tap cleanup SHA, while Tasks 10 and 11 require both tap cleanup SHA and Task 8 ratchet-cli formula automation commit SHA. Recommendation: include both required SHA fields in template.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | P9/P10 weaken real-boundary proof. |
| Assumptions under attack | Finding | Plan assumed untagged tests can prove tagged smoke helpers. |
| Missing failure modes | Finding | P10 omits negative socket/path/permission cases. |
| Security/privacy at architecture level | Finding | P10 leaves arbitrary Unix socket constructor contract under-tested. |
| Multi-component validation | Finding | P9/P11 can leave smoke-service and uploaded-asset boundaries unproven. |
| Declared integration proof | Finding | P9/P11 affect daemon and draft-asset proof. |
| Existence/runtime-validity | Finding | P9 has untagged commands for tagged surfaces; P11 has releaseguard interface drift. |
| Verification-class mismatch | Finding | P9/P10 use insufficient verification for tagged/security-sensitive code. |
| Hidden serial dependencies | Finding | P12 leaves prerequisite evidence slightly under-specified. |
| Missing integration proof | Finding | P9/P11 can skip required smoke-service and draft-asset proof. |
| Identifier/naming convention | Finding | P11 inconsistent draft-asset env/interface naming. |

**Options the author may not have considered:**
1. Keep `draft-assets` file-system based: workflow downloads draft assets by release id into `$RUNNER_TEMP/release-assets`, then releaseguard reads `RATCHET_RELEASE_GUARD_ASSETS`.
2. Split tagged helper verification explicitly: `go test -tags tui_smoke` for smoke-only constructors/wiring plus untagged guards for release-surface absence.

**Verdict reasoning:** FAIL. P1-P8 were addressed, but tagged smoke helper tests, smoke-client socket security, and draft asset interface consistency needed concrete plan steps.
