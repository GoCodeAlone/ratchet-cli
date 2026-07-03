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

## Cycle 4

### Adversarial Review Report

**Phase:** plan
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `P13` [Manifest drift / Verification-class mismatch / Security] [plan:90-96,115,132-140,166-169,390-411,428-430; design:118-130]: Plan defines the smoke-source manifest in Task 1, but later adds `internal/daemon/service_tui_smoke.go` and `internal/releaseguard/*` without explicit manifest/tooling-allowlist updates or rerunning the `SmokeSource` guard. Recommendation: add manifest/allowlist update steps and `go test ./internal/tui -run SmokeSource -count=1` to each task that adds smoke-tagged runtime files or releaseguard forbidden-token constants.
- `P14` [Tap postcheck interface / Missing integration proof / Identifier drift] [plan:614-617,628-632,652-655; design:528-533,643-648]: Task 11 release workflow prose omits `RATCHET_RELEASE_GUARD_TAP` from tap-postcheck invocation even though design marks it required. Recommendation: spell out full workflow command/env and add workflow assertion that `release.yml` sets all required tap-postcheck env vars.

**Findings (Minor):**
- `P15` [Executable command robustness] [plan:511-518]: Task 9 uses `test -f <tap-checkout>/ratchet-cli.rb` while expected result allows that file to be absent if cleanup already landed. Recommendation: replace with conditional branch recording either stale file present or cleanup already landed.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Assumptions under attack | Finding | P13 assumes early source-manifest tests stay valid as later smoke/releaseguard files are added. |
| Artifact-class precedent | Finding | P13 misses manifest/guard artifact update when adding later source artifacts. |
| Missing failure modes | Finding | P15 leaves already-clean tap prerequisite path non-executable. |
| Security/privacy at architecture level | Finding | P13 can weaken smoke-source leak prevention. |
| Infrastructure impact | Finding | P14 affects release/tap workflow enforcement. |
| Multi-component validation | Finding | P14 does not prove real release workflow passes full tap-postcheck interface. |
| Declared integration proof | Finding | P14 leaves one declared releaseguard env unbound in workflow prose. |
| Existence/runtime-validity | Finding | P14 is executable env-interface gap. |
| Verification-class mismatch | Finding | P13/P14 focused checks can pass without proving changed surface. |
| Missing integration proof | Finding | P14 lacks workflow-level tap-postcheck proof. |
| Infrastructure verification mismatch | Finding | P14 leaves release workflow env wiring under-verified. |
| Identifier/naming convention | Finding | P14 drops required `RATCHET_RELEASE_GUARD_TAP` identifier. |

**Options the author may not have considered:**
1. Centralize releaseguard invocation in `scripts/check-release-artifacts.sh` for all modes.
2. Move smoke-source manifest ownership into one table-driven testdata file updated by every task adding smoke/runtime/tooling token surfaces.

**Verdict reasoning:** FAIL. P1-P12 were addressed, but plan could still pass focused task checks while missing source-manifest drift and tap-postcheck workflow interface proof.

## Cycle 5

### Adversarial Review Report

**Phase:** plan
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `P16` [Infrastructure verification mismatch / release proof] [design:689-690,698; plan:621-637,659-670]: Design requires pre-publish gate that fails unless `.goreleaser.yaml` has `release.draft: true`, but plan only verifies draft state after GoReleaser has published assets. Recommendation: add releaseguard config tests and release workflow preflight asserting `.goreleaser.yaml` `release.draft == true` before `goreleaser release --clean`, and include it in Task 11 verification.

**Findings (Minor):**
- `P17` [Executable command robustness] [plan:427-436]: Task 7's missing-env negative check is written as a normal command even though expected result is failure. Recommendation: wrap it as explicit negative assertion and assert missing-env diagnostic.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | P16 weakens release-safe draft gate. |
| Assumptions under attack | Finding | Plan assumed post-publish draft inspection is enough to prevent public-before-postcheck exposure. |
| Missing failure modes | Finding | P16 misses release was never draft before publish; P17 command is easy to execute incorrectly. |
| Infrastructure impact | Finding | P16 affects tag release/publication ordering. |
| Declared integration proof | Finding | Draft asset integration proof is post-publish only unless fixed. |
| Existence/runtime-validity | Finding | P16 consumer-validity gap for release workflow state; P17 command executability nit. |
| Verification-class mismatch | Finding | P16 lacks required pre-publish config verification. |
| Missing integration proof | Finding | Draft-release pre-public proof incomplete via P16. |
| Infrastructure verification mismatch | Finding | P16. |

**Options the author may not have considered:**
1. Add `TestGoReleaserReleaseDraftConfig` under `internal/releaseguard` and call it from both snapshot wrapper and release workflow preflight.
2. Workflow-only shell/YAML check before publish, but Go releaseguard keeps invariant with release safety contract.

**Verdict reasoning:** FAIL. P1-P15 were addressed; remaining blocker was proving `release.draft: true` before publish, not only after assets may be public.

## Cycle 6

### Adversarial Review Report

**Phase:** plan
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `P18` [Missing integration proof / Verification-class mismatch] [design:282-324; plan:198-204,303-309]: Design requires PTY proof for every documented `/mode` value and full trust slash-command matrix, but Task 3 only names `/mode` and `/trust` as families. Recommendation: make `TestTUIBinarySmoke` enumerate exact matrix: all five `/mode` values plus `/trust list`, allow, deny, persist allow/deny, grants, revoke, reset with follow-up state assertions.
- `P19` [Infrastructure verification mismatch / Repo-precedent conflict] [design:507-508,847; plan:654-664; `.github/workflows/ci.yml`:14-15,27-28,41-42]: Windows safe-command smoke job builds source `ratchet.exe`, but plan omits private-module Git rewrite that every existing Go-building CI job uses. Recommendation: add explicit `GOPRIVATE`/`GONOSUMCHECK` plus same Git rewrite step and workflow assertion.

**Findings (Minor):**
- None.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | P18 weakens real-boundary TUI proof; P19 weakens Windows CI reliability. |
| Assumptions under attack | Finding | Plan assumed family-level command wording implied matrix execution, and Windows source build could fetch private modules. |
| Repo-precedent conflicts | Finding | Existing CI repeats private-module Git rewrite in every Go build/test job; Windows smoke omitted it. |
| Missing failure modes | Finding | Missing PTY matrix leaves command-state regressions unproven; Windows job can fail before package proof. |
| Infrastructure impact | Finding | P19 affects added CI workflow execution. |
| Multi-component validation | Finding | P18 can leave TUI input to daemon trust/mode RPC behavior only focused-tested. |
| Declared integration proof | Finding | Integration matrix promises selected slash-command runtime proof, but task text does not force full matrix. |
| Existence/runtime-validity | Finding | P19 executable workflow setup gap against CI precedent. |
| Verification-class mismatch | Finding | P18 substitutes vague family/focused proof for required PTY matrix proof. |
| Missing integration proof | Finding | P18 misses explicit PTY execution for each required mode/trust command row. |
| Infrastructure verification mismatch | Finding | P19 needs workflow-level private-module setup proof. |

**Options the author may not have considered:**
1. Drive PTY matrix from the same command-surface JSON fixture.
2. Copy existing private-module setup into `windows-safe-command-smoke`.

**Verdict reasoning:** FAIL. P1-P17 were addressed; remaining gaps were explicit PTY matrix proof and Windows private-module setup.

## Cycle 7

### Adversarial Review Report

**Phase:** plan
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `P20` [Manifest drift / executable gap] [plan:132-140,161,170,178; plan:392-401,411,428,440,457]: Plan says Task 2 updates smoke-source manifest and Task 7 updates releaseguard tooling allowlist, but both tasks omit `internal/tui/smoke_source_manifest_test.go` from Files lists and `git add` commands. Recommendation: add manifest/allowlist file explicitly to Task 2 and Task 7 Files and commit commands.
- `P21` [Missing integration proof / PTY trust matrix] [design:295-301; plan:198-202,304-305; `internal/tui/commands/trust.go`:86-129,143-170]: Design's trust matrix proves `--scope smoke` through follow-up `/trust list` and `/trust grants` state reads, but plan examples drop `--scope smoke` flags and only partially name reset follow-up assertions. Recommendation: make Task 3 and JSON spec use exact scoped commands and require follow-up assertions for pattern/action/scope after each mutating trust command.

**Findings (Minor):**
- `P22` [Identifier / build-constraint clarity] [plan:78-80; design:177-179]: Task 1 creates `cmd/ratchet-tui-smoke/main_unix_test.go` but does not require explicit `//go:build !windows`. Go has no generic `_unix` suffix. Recommendation: rename to `main_test.go` with `//go:build !windows`, or use real GOOS-specific files.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | P20/P21 weaken real-boundary proof and guarded claims. |
| Assumptions under attack | Finding | Plan assumed prose manifest/allowlist and trust matrix updates were enough without staging mechanics/state proof. |
| Artifact-class precedent | Finding | Guard manifests are source artifacts and must be listed/staged wherever changed. |
| Missing failure modes | Finding | P20 can miss smoke-token guard drift; P21 can miss trust scope mutation regressions. |
| Security/privacy at architecture level | Finding | Trust scope proof is security-relevant policy mutation boundary. |
| Infrastructure impact | Finding | P20 affects releaseguard/smoke artifact enforcement. |
| Multi-component validation | Finding | P21 can avoid proving TUI command input through daemon trust state. |
| Declared integration proof | Finding | Releaseguard integration proof undermined if allowlist changes unstaged. |
| User-intent drift | Finding | P21 can still overstate slash/trust command verification. |
| Existence/runtime-validity | Finding | P20 commit commands incomplete; P22 relies on non-existent `_unix` build suffix. |
| Verification-class mismatch | Finding | P21 lacks state-level verification required for scoped trust mutations. |
| Missing integration proof | Finding | P21 leaves scoped trust behavior insufficiently proven. |
| Infrastructure verification mismatch | Finding | P20 can leave releaseguard/smoke guard verification out of committed artifact. |
| Identifier/naming convention | Finding | P22 uses misleading `_unix_test.go` naming without build tag. |

**Options the author may not have considered:**
1. Put smoke-source manifest/tooling allowlist in one testdata file and make every guarded-token task update it explicitly.
2. Drive PTY trust matrix from JSON command spec with exact command strings including `--scope smoke`.

**Verdict reasoning:** FAIL. P1-P19 mostly addressed; remaining blockers were incomplete guard manifest staging and insufficient scoped trust state proof.

## Cycle 8

### Adversarial Review Report

**Phase:** plan
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `P23` [Hidden serial dependency / executable gap] [plan:197-203,291-306]: Task 3 requires `TestTUIBinarySmoke` to drive every `pty-proven` row from `internal/tui/commands/testdata/command_surface_spec.json`, but that fixture is not created until Task 5. Recommendation: move command-surface fixture creation/classification into Task 3 before PTY test consumes it, or reorder Task 5 before Task 3.
- `P24` [Verification-class mismatch / release workflow cache risk] [design:523-526; plan:643,648-655]: Design requires artifact-reading releaseguard invocations through non-cacheable `go test -count=1`, but Task 11 workflow prose spells draft-config guard without `-count=1` and lists draft/tap postcheck env wiring without exact non-cacheable test commands. Recommendation: make release workflow steps explicit for draft-assets and tap-postcheck, including `go test -count=1 ./internal/releaseguard -run TestDraftAssets` / `TestTapPostcheck`, and add `-count=1` to draft config preflight.

**Findings (Minor):**
- None.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Assumptions under attack | Finding | P23 assumes command fixture exists before creating task; P24 assumes prose implies non-cacheable guard execution. |
| Existence/runtime-validity | Finding | P23 consumed-fixture existence/order gap; P24 executable workflow command specificity gap. |
| Verification-class mismatch | Finding | P24 weakens non-cacheable artifact-reading command contract. |
| Hidden serial dependencies | Finding | P23 Task 3 depends on Task 5 fixture. |
| Infrastructure verification mismatch | Finding | P24 underspecifies release workflow guard commands. |

**Options the author may not have considered:**
1. Move command-surface JSON fixture and minimal classifier into Task 3, then let Task 5 expand help/autocomplete/docs alignment.
2. Keep task order unchanged but make Task 3 use inline PTY matrix only; Task 5 later asserts JSON fixture matches that matrix.

**Verdict reasoning:** FAIL. P1-P22 fixes appear addressed; remaining issues were fixture ordering and exact non-cacheable releaseguard workflow commands.

## Cycle 9

### Adversarial Review Report

**Phase:** plan
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `P25` [Manifest drift / hidden serial dependency] [plan:17-38,733-793]: Scope Manifest assigns Task 13 to PR5, but Task 13 opens/monitors all PRs, releases only after PR5 merges, creates retro, then commits state. That post-merge/post-release commit cannot be contained in PR5. Recommendation: make Task 13 non-mutating orchestration evidence only, or add explicit PR6/direct-closeout path and update PR count/scope manifest.
- `P26` [Workflow linting / executable gap] [plan:601-609,674-686,740-756; `go.mod`:210]: Plan requires `actionlint` but only says it passes where installed. Recommendation: use pinned executable command such as `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 ...`, or add explicit install/CI step.
- `P27` [Tap postcheck auth wiring / infrastructure verification mismatch] [design:624-648; plan:649-659; `.goreleaser.yaml`:52-56]: Design requires cloning configured Homebrew tap with `HOMEBREW_TAP_TOKEN`; Task 11 only says clone tap. Recommendation: spell out clone/auth using same tap repo/branch/token and add workflow assertions for token wiring.

**Findings (Minor):**
- None.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | P25/P26 weaken executable-gate discipline. |
| Assumptions under attack | Finding | Plan assumed PR5 can contain post-PR5 release/retro commits and `actionlint` exists. |
| Repo-precedent conflicts | Finding | Closeout PR accounting drifts despite manifest passing plugin checker. |
| Artifact-class precedent | Finding | Closeout retro/release-state artifact misclassified as feature docs PR. |
| Missing failure modes | Finding | P27 misses external tap clone/auth failure before postcheck. |
| Infrastructure impact | Finding | P27 affects release/tap workflow reliability. |
| Declared integration proof | Finding | Tap postcheck integration lacks full clone auth wiring. |
| Existence/runtime-validity | Finding | P25/P26 are executable sequencing/tooling gaps. |
| Verification-class mismatch | Finding | P26 makes workflow linting optional in practice. |
| Hidden serial dependencies | Finding | P25 release/retro depends on PR5 merge but scoped inside PR5. |
| Missing integration proof | Finding | P27 leaves external tap clone/auth unproven. |
| Infrastructure verification mismatch | Finding | P27. |

**Options the author may not have considered:**
1. Split Task 13: keep PR5 docs/evidence only, add PR6 close release state.
2. Replace bare `actionlint` with pinned `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12`.
3. Centralize tap clone auth in wrapper so workflows pass same `HOMEBREW_TAP_TOKEN` path.

**Verdict reasoning:** FAIL. P23-P24 were fixed; remaining blockers were closeout PR accounting, executable workflow linting, and tap auth clone wiring.

## Cycle 10

### Adversarial Review Report

**Phase:** plan
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `P28` [Verification-class mismatch / Security privacy] [design:489-506; plan:663-673,675-687]: Windows packaged smoke must run `daemon status` with temp Windows `HOME`/`USERPROFILE`/`XDG_STATE_HOME`, but executable PowerShell snippet only runs extracted binary and leaves temp env setup implicit. Recommendation: add explicit temp env assignments under `$env:RUNNER_TEMP` before packaged command execution, plus workflow assertion that setup precedes `daemon status`.
- `P29` [Infrastructure verification mismatch / releaseguard cache risk] [design:523-533,566-572; plan:595-600]: Task 10 `tap-preflight` CI job says run explicit tap preflight but does not spell non-cacheable command/env. Recommendation: require `RATCHET_RELEASE_GUARD_MODE=tap-preflight RATCHET_RELEASE_GUARD_TAP=<tap-checkout> go test -count=1 ./internal/releaseguard -run TestTapPreflight` in CI workflow plan and workflow assertion.

**Findings (Minor):**
- None.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | P28 weakens temp-state proof; P29 weakens executable release/tap proof. |
| Assumptions under attack | Finding | Plan assumed expected prose implies env/`-count=1` wiring. |
| Repo-precedent conflicts | Finding | Existing binary smoke sets temp `HOME`/`XDG_STATE_HOME`; Windows package smoke did not specify equivalent env. |
| Missing failure modes | Finding | P28 allows real runner state; P29 allows stale cached tap-preflight evidence. |
| Security/privacy | Finding | P28 can read/write runner profile state. |
| Infrastructure impact | Finding | P29 affects CI tap gate reliability. |
| Multi-component validation | Finding | P28 under-specifies packaged Windows binary plus temp-state boundary. |
| Declared integration proof | Finding | P28/P29 leave Windows/tap integration proofs partially implicit. |
| Existence/runtime-validity | Finding | P28/P29 executable command gaps. |
| Verification-class mismatch | Finding | P28/P29 mismatch required proof class with command specificity. |
| Missing integration proof | Finding | P28/P29 leave gaps in Windows package and tap workflow proof. |
| Infrastructure verification mismatch | Finding | P29 under-specifies CI tap-preflight command. |

**Options the author may not have considered:**
1. Put Windows packaged command smoke in a small checked-in PowerShell script.
2. Route all releaseguard modes through wrapper so CI cannot forget `-count=1`.

**Verdict reasoning:** FAIL. P1-P27 were materially addressed; remaining blockers were Windows temp state and exact non-cacheable tap-preflight command.
