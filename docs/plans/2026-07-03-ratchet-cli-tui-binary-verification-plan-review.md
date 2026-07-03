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

## Cycle 11

### Adversarial Review Report

**Phase:** plan
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `P30` [Security/privacy / Missing task / Verification-class mismatch] [design:219-224,747,834,840; plan:58,222,273,395-467,621-700]: The design requires one redaction path for every runtime/test failure payload, including build output, GoReleaser snapshot output, daemon cleanup output, docs-guard output, artifact-manifest output, command errors, generated artifact paths, and trust/prompt bodies. The plan only makes redaction executable in the TUI/runtime smoke area and daemon leftover diagnostics; Tasks 7-11 add releaseguard, GoReleaser, draft-asset, tap, workflow, and artifact-manifest failure paths without requiring tests or implementation steps proving those outputs use the same redactor. Recommendation: add an explicit releaseguard/shared-redaction task or Task 7/11 test bullets that exercise representative GoReleaser, manifest, draft-assets, tap-postcheck, workflow-command, docs-guard, and command-error failures and assert real home/workspace/temp/socket/executable/artifact paths plus trust/prompt bodies are redacted before logging/failing.

**Findings (Minor):**
- None.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Security/privacy | Finding | Release/tap/workflow/docs failure payloads lacked executable shared-redaction proof. |
| Verification-class mismatch | Finding | Design-level redaction promise was not mapped to releaseguard/task tests. |
| Missing task | Finding | No shared helper task/file owned the cross-path redaction contract. |

**Options the author may not have considered:**
1. Create `internal/harnessredact` before PTY tests and reuse it from releaseguard/docs tests.
2. Add Task 7/11 representative failure fixtures for GoReleaser, manifest, draft-assets, tap, workflow, docs guard, and command errors.

**Verdict reasoning:** FAIL. P1-P29 were materially addressed; P30 remained as the executable security gap for full redaction coverage outside runtime smoke paths.

## Cycle 12

### Adversarial Review Report

**Phase:** plan
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `P31` [User-intent drift / Infrastructure impact] [plan:73,678-692; design:494-508]: The plan adds a new `windows-latest` job for packaged Windows smoke, but the user ask explicitly says no runner changes. This is a workflow runner-class change, not just a Windows build proof. Recommendation: either get an explicit user-approved exception recorded in the plan, or replace the Windows runtime job with existing-runner cross-build/archive/byte-scan proof and defer Windows executable runtime to a separate approved runner-change plan.
- `P32` [Project-guidance conflict / Verification-class mismatch] [plan:786-790]: The closeout task allows admin merge once local tests pass and checks are delayed, but the user ask says admin merge once green and asks for autodev pipeline discipline. This plan can execute by merging without green CI. Recommendation: remove the delayed-check bypass or require a fresh explicit user override recorded as a plan amendment before any non-green merge.
- `P33` [Planned-code compile-validity / Missing failure mode] [plan:250-266; `internal/client/client.go`:20-38; `internal/daemon/service.go`:240-254]: Task 4 says cleanup uses normal untagged `client.Connect()` plus the public Shutdown RPC, but `client.Client` exposes no `Shutdown` method and its generated daemon client field is unexported. The task files also omit `internal/client/client.go`, so the planned cmd-level cleanup path is not executable as written. Recommendation: add an explicit `Client.Shutdown(ctx)` wrapper and include `internal/client/client.go` in Task 4, or specify a direct generated-proto gRPC dial path in the test plan.

**Findings (Minor):**
- None.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | P32 conflicts with green-CI/admin-merge discipline. |
| Assumptions under attack | Finding | P31 assumes adding `windows-latest` is not a runner change; P33 assumes `client.Connect()` exposes Shutdown. |
| Repo-precedent conflicts | Clean | Existing Go test, GoReleaser, harness smoke, and docs-guard shapes are mostly followed. |
| Artifact-class precedent | Clean | The plan uses existing docs/plans, smoke test, workflow, GoReleaser, and tap artifact classes. |
| YAGNI violations | Finding | P31 may add a new runner class where the user constrained runner changes. |
| Missing failure modes | Finding | P33 misses failure where startup cleanup cannot call Shutdown through public client wrapper. |
| Security/privacy at architecture level | Clean | P30 appears fixed across runtime, releaseguard, workflow, docs, artifact, prompt, trust, and path payloads. |
| Infrastructure impact | Finding | P31 changes CI runner topology; P32 weakens merge gate discipline. |
| Multi-component validation | Finding | P33 leaves release-shaped binary startup cleanup under-specified across cmd/client/daemon. |
| Declared integration proof | Finding | P31 declares Windows runtime proof through a runner change not authorized by the user ask. |
| Contributed UI rendering proof | Clean | No plugin-contributed UI route is in scope. |
| Rollback story | Clean | Rollback covers smoke code, workflows, release assets, and tap contamination. |
| Simpler alternative not considered | Finding | Existing-runner Windows proof was not considered as an alternative to `windows-latest`. |
| User-intent drift | Finding | P31/P32 drift from no runner changes and admin merge once green. |
| Existence/runtime-validity | Finding | P33 references a client cleanup capability absent from current public client API. |
| Over-decomposition/under-decomposition | Clean | Thirteen tasks across six PRs is heavy but proportional. |
| Verification-class mismatch | Finding | P32 allows local-only verification to substitute for green CI; P33 lacks a runnable cleanup path. |
| Auth/authz chain composition | Clean | Trust command proof goes through daemon state and scoped follow-up assertions. |
| Hidden serial dependencies | Clean | Fixture/tap/order dependencies are represented as Task 3 fixture creation and Task 9 preconditions. |
| Missing rollback wiring | Clean | Rollback is wired per task and release/tap path. |
| Missing integration proof | Finding | P33 leaves cmd/client/daemon Shutdown proof incomplete. |
| Missing declared integration matrix | Clean | Plan includes matrix classifications. |
| Missing contributed UI route proof | Clean | Not applicable. |
| Infrastructure verification mismatch | Finding | P31 verifies Windows by adding an unauthorized runner class. |
| Plugin-loader runtime layout | Clean | No external Workflow plugin loader layout is introduced. |
| Config-validation schema rules | Clean | No new schema-validated Workflow config is introduced. |
| Identifier/naming-convention match | Clean | Env vars, task names, and command names match conventions. |
| Planned-code compile-validity | Finding | P33 depends on an unexposed Shutdown RPC wrapper. |

**Options the author may not have considered:**
1. Existing-runner Windows proof: keep `GOOS=windows` cross-builds, archive matrix checks, Windows zip byte scans, and releaseguard package-boundary checks on the current runner; defer actual `ratchet.exe` execution to a future approved runner-change plan.
2. Explicit shutdown wrapper: add `func (c *Client) Shutdown(ctx context.Context) error` in `internal/client`.
3. Strict merge gate: require green GitHub checks before admin merge.

**Verdict reasoning:** FAIL. P30 is fixed, but the plan still had an unauthorized Windows runner change, a non-green merge bypass, and a concrete cmd/client/daemon cleanup path that was not executable with the current client API.

## Cycle 13

### Adversarial Review Report

**Phase:** plan
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `P34` [User-intent drift / Verification-class mismatch] [plan:655; design:501-503,929,1095]: Task 11 still says the Windows archive fixture executes only amd64 in Windows job contract. The main P31 runner/job shape is fixed, but this remaining executable-runtime wording can steer implementation back toward a Windows command-smoke contract despite the no-runner-change scope. Recommendation: delete the execution clause and require only Windows cross-build plus amd64/arm64 archive/member/checksum/executable byte inspection, with workflow guards proving no `windows-latest` and no `ratchet.exe` execution step.
- `P35` [Artifact-class precedent / Verification-class mismatch] [`.goreleaser.yaml`:30-31; plan:364-386,683-685,716-727; design:229-231,735-738]: The plan updates packaged `RATCHET.md` docs to mention `ratchet-tui-smoke`, while Task 11 still says to byte-scan each Windows archive/member/executable for smoke tokens. Because `.goreleaser.yaml` packages `RATCHET.md`, a content-level archive/member token scan can either fail on intended evidence docs or pressure the author to hide the documented proof boundary. Recommendation: split checks explicitly: member names and executable bytes forbid smoke tokens; packaged Markdown docs are checked by docs guard/allowed templates, not by the release-binary leak scan.
- `P36` [Hidden serial dependency / Infrastructure impact] [plan:747-810]: Task 13 runs on PR6 and commits closeout state, but the PR/monitor/merge step only covers PRs 1-5. That leaves the closeout plan/retro commit without an explicit green-check/review/admin-merge path, drifting from the user’s admin merge once green and workspace per-PR green-CI discipline. Recommendation: add a final PR6 step after the closeout commit: open PR, monitor required GitHub checks, satisfy review requirements, then admin merge only when green.

**Findings (Minor):**
- `P37` [Missing failure mode / Existence-runtime validity] [plan:689-700; design:1030,1066]: Local Windows cross-build verification writes fixed `/tmp/ratchet-windows-*.exe` paths. That is weaker than the design’s temp-output-path invariant and can collide with stale files or local symlinks. Recommendation: use a unique temp dir from `mktemp -d` or `t.TempDir`-equivalent wrapper and clean it up.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | P36 conflicts with per-PR CI green before merge; P34 conflicts with no-runner-change correction. |
| Assumptions under attack | Finding | Stale Windows execution wording and PR6 omission can change execution behavior. |
| Repo-precedent conflicts | Finding | P36 skips the same PR green-check discipline applied to other PRs. |
| Artifact-class precedent | Finding | P35 mixes packaged docs content with release binary/member leak scanning. |
| YAGNI violations | Clean | No new user-facing CLI or broad command registry is added. |
| Missing failure modes | Finding | P37 leaves fixed `/tmp` cross-build outputs open to stale-file/collision cases. |
| Security/privacy at architecture level | Clean | P30 remains fixed via shared redaction coverage. |
| Infrastructure impact | Finding | P34 affects CI/release workflow scope; P36 affects PR6 merge discipline. |
| Multi-component validation | Finding | P35 can make releaseguard fail against the real GoReleaser archive/docs combination. |
| Declared integration proof | Finding | P34 leaves Windows proof phrased as runtime execution in one task bullet. |
| Contributed UI rendering proof | Clean | No contributed UI route is in scope. |
| Rollback story | Clean | Rollback covers smoke code, workflows, release assets, and tap contamination. |
| Simpler alternative not considered | Finding | P35 can split executable/member scans from docs guards. |
| User-intent drift | Finding | P34/P36 drift from no Windows runtime proof and admin merge only once green. |
| Existence/runtime-validity | Finding | P34 stale Windows execution contract and P37 fixed `/tmp` paths are executable gaps. |
| Over-decomposition/under-decomposition | Clean | Thirteen tasks across six PRs is proportional. |
| Verification-class mismatch | Finding | P34/P35 mismatch archive inspection vs runtime execution or broad docs-content token scans. |
| Auth/authz chain composition | Clean | Trust proof goes through daemon state and scoped follow-up assertions. |
| Hidden serial dependencies | Finding | P36 omits final PR6 merge dependency after closeout commit. |
| Missing rollback wiring | Clean | Rollback is wired. |
| Missing integration proof | Finding | P35 lacks coherent proof split for archives that intentionally contain docs. |
| Missing declared integration matrix | Clean | Matrix includes Windows runtime as deferred. |
| Missing contributed UI route proof | Clean | Not applicable. |
| Infrastructure verification mismatch | Finding | P36 does not wire final closeout PR into required-check path. |
| Plugin-loader runtime layout | Clean | No external plugin layout introduced. |
| Config-validation schema rules | Clean | No new schema config introduced. |
| Identifier/naming-convention match | Clean | Identifiers otherwise match conventions. |
| Planned-code compile-validity | Clean | P33 remains fixed by `Client.Shutdown(ctx)` plan. |

**Options the author may not have considered:**
1. Split releaseguard checks by artifact layer: archive filenames/member names and executable bytes forbid smoke tokens; packaged Markdown docs are validated by docs guards with approved evidence templates.
2. Add a PR6 closeout template: after release/retro commit, open PR6, monitor checks/reviews, then admin merge when green.
3. Add a negative workflow assertion that searches release/CI YAML for Windows executable invocation patterns, not just `windows-latest`.

**Verdict reasoning:** FAIL. P30 is fixed, and P31-P33 corrections are present, but stale Windows runtime wording, releaseguard/docs packaging contradiction, and missing green-check merge path for PR6 remained.

## Cycle 14

### Adversarial Review Report

**Phase:** plan
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `P38` [Existence/runtime-validity / Verification-class mismatch] [plan:458-462]: The negative manifest-mode verification pipes `go test` through `tee` without `pipefail`, so the pipeline exits with `tee` status. The `if` branch will run even when the intended failing `go test` fails, making the verification block fail unconditionally. Recommendation: use `set -o pipefail`, capture output to a temp file without a pipeline, or invert the command with explicit status capture.
- `P39` [User-intent drift / Infrastructure impact] [plan:567]: Task 9 allows a direct admin commit to `GoCodeAlone/homebrew-tap` if repository policy permits it, bypassing the plan’s own PR/checks/green-merge discipline and the user’s admin merge once green constraint. Recommendation: require a PR with green checks for the tap cleanup, unless a fresh explicit override is recorded as a plan amendment.
- `P40` [Security/privacy / Missing task wiring / Verification-class mismatch] [plan:310,326,331-333,343-354]: Task 5 lists `internal/harnessredact/redact_test.go` and requires docs/CLI help redaction paths, but Step 4 does not rerun the new harnessredact tests and Step 5 does not stage `internal/harnessredact`. That can leave the docs/command redaction proof uncommitted. Recommendation: include `internal/harnessredact` in gofmt/verification/staging for Task 5.

**Findings (Minor):**
- None.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | P39 conflicts with green-check merge discipline for external tap prerequisite. |
| Assumptions under attack | Finding | P38 assumes piped negative checks preserve status; P39 assumes direct commit is compatible; P40 assumes listed redaction tests are verified and committed. |
| Repo-precedent conflicts | Finding | P39 diverges from the PR/monitor/admin-merge pattern. |
| Artifact-class precedent | Finding | P40 lists a test artifact but omits it from verify/commit flow. |
| YAGNI violations | Clean | No new broad command registry, runner migration, or user-facing releaseguard CLI is introduced. |
| Missing failure modes | Finding | P38 misses shell pipeline status behavior; P40 misses uncommitted redaction test changes. |
| Security/privacy at architecture level | Finding | P40 weakens docs/CLI failure-payload redaction proof. |
| Infrastructure impact | Finding | P39 affects external Homebrew tap merge path and release prerequisite evidence. |
| Multi-component validation | Finding | P40 leaves cmd/ratchet docs surface to harnessredact proof partially wired. |
| Declared integration proof | Finding | P40 declares docs/CLI redaction proof but does not commit it. |
| Contributed UI rendering proof | Clean | No contributed UI route is in scope. |
| Rollback story | Clean | Rollback paragraphs cover smoke code, workflows, release assets, and tap corrections. |
| Simpler alternative not considered | Finding | P38 can use explicit status capture; P39 can require PR-only tap cleanup. |
| User-intent drift | Finding | P39 remains drift from admin merge once green. |
| Existence/runtime-validity | Finding | P38 is an executable shell validity bug. |
| Over-decomposition/under-decomposition | Clean | Thirteen tasks across six PRs remains coherent. |
| Verification-class mismatch | Finding | P38/P40 break declared verification commands or omit changed proof surface. |
| Auth/authz chain composition | Clean | Trust proof is daemon-backed. |
| Hidden serial dependencies | Clean | Tap cleanup prerequisite is represented before Tasks 10-11. |
| Missing rollback wiring | Clean | Rollback wiring is present. |
| Missing integration proof | Finding | P40 incompletely wires docs/CLI redaction proof. |
| Missing declared integration matrix | Clean | Matrix classifies runtime, artifact, config-only, and deferred items. |
| Missing contributed UI route proof | Clean | Not applicable. |
| Infrastructure verification mismatch | Finding | P38 breaks releaseguard verification; P39 permits non-green external prerequisite path. |
| Plugin-loader runtime layout | Clean | No external plugin layout added. |
| Config-validation schema rules | Clean | No new schema config. |
| Identifier/naming-convention match | Clean | Env vars and command identifiers are consistent. |
| Planned-code compile-validity | Clean | No embedded Go compile issue found. |

**Options the author may not have considered:**
1. Replace the `tee` negative check with explicit status capture into a temp log file.
2. Make tap cleanup PR-only for this plan.
3. Treat `internal/harnessredact` as a required touched package in every task that adds redaction obligations.

**Verdict reasoning:** FAIL. P30-P37 are materially fixed, but P38-P40 remained as executable verification and merge-discipline gaps.

## Cycle 15

### Adversarial Review Report

**Phase:** plan
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `P41` [User-intent drift / Infrastructure impact] [plan:20,46,569]: P39 is fixed in Task 9’s execution step, but the Scope Manifest and external prerequisite table still say the Homebrew tap prerequisite may be a PR/direct commit without requiring an explicit amendment override. That leaves a top-level executable path that conflicts with the user’s green-merge discipline. Recommendation: change manifest/prerequisite wording to PR-only; direct commit only with fresh explicit plan amendment override.
- `P42` [Project-guidance conflict / Infrastructure verification mismatch] [workspace `docs/design-guidance.md`:39; plan:793-800]: The release tagging step fetches tags and pushes `v<next>`, but never asserts `git rev-parse HEAD` equals the intended merged commit before pushing an immutable release tag. Recommendation: add a pre-tag check that records the intended `master` SHA, verifies local HEAD equals it after fetch/fast-forward, verifies the worktree is clean, then tags.
- `P43` [Security/privacy / Missing integration proof / Verification-class mismatch] [plan:98,156-158,170,766-767 vs 617-619,783-789]: Required tagged smoke-client and smoke-daemon tests prove security-sensitive socket containment and smoke-service inertness, but Task 10’s CI checks only add untagged `tui-smoke`/release/tap jobs. Admin merge once green can therefore mean GitHub checks are green while `go test -tags tui_smoke ./internal/client` and `./internal/daemon` were never run in CI. Recommendation: add a CI job or extend `tui-smoke` to run the tagged client/daemon commands with `-count=1`.

**Findings (Minor):**
- `P44` [Verification-class mismatch / formatting] [plan:415-416,451,456-458,482]: Task 7 modifies `internal/harnessredact`, but its gofmt command formats only `internal/releaseguard`. Recommendation: use `gofmt -w internal/harnessredact internal/releaseguard`.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | P42 conflicts with release-tag HEAD verification guidance. |
| Assumptions under attack | Finding | P41 assumes lower task text overrides manifest; P43 assumes local tagged tests are enough for green-merge discipline. |
| Repo-precedent conflicts | Finding | P42 misses workspace release-tag safety convention. |
| Artifact-class precedent | Finding | P41 leaves external tap prerequisite classification inconsistent. |
| YAGNI violations | Clean | No new user-facing releaseguard CLI, runner migration, or broad command registry is introduced. |
| Missing failure modes | Finding | P42 misses stale-HEAD release tagging; P43 misses CI skipping tagged security contracts. |
| Security/privacy at architecture level | Finding | P43 leaves socket containment and smoke-service inertness tests outside required CI proof. |
| Infrastructure impact | Finding | P41/P42 affect tap and release operations. |
| Multi-component validation | Finding | P43 omits CI proof for tagged client/daemon helper boundaries. |
| Declared integration proof | Finding | P43’s declared smoke helper proof is not in merge-gating CI matrix. |
| Contributed UI rendering proof | Clean | No contributed UI route is in scope. |
| Rollback story | Clean | Rollback covers smoke code, workflows, release assets, and tap contamination. |
| Simpler alternative not considered | Finding | P43 can add two tagged `go test` commands to CI. |
| User-intent drift | Finding | P41 still permits direct tap path at manifest level. |
| Existence/runtime-validity | Finding | P42 is an executable release command gap. |
| Over-decomposition/under-decomposition | Clean | Thirteen tasks across six PRs remains coherent. |
| Verification-class mismatch | Finding | P42/P43/P44 do not fully match release, tagged-security, or modified-file classes. |
| Auth/authz chain composition | Clean | Trust slash proof remains daemon-backed. |
| Hidden serial dependencies | Clean | Tap cleanup blocks Tasks 10-11; PR6 closeout is wired. |
| Missing rollback wiring | Clean | Rollback wiring is present. |
| Missing integration proof | Finding | P43 leaves tagged helper integration proof local/final rather than CI-gated. |
| Missing declared integration matrix | Clean | Matrix classifies runtime, artifact, config-only, and deferred items. |
| Missing contributed UI route proof | Clean | Not applicable. |
| Infrastructure verification mismatch | Finding | P42 release tag safety and P43 CI-gated proof are incomplete. |
| Plugin-loader runtime layout | Clean | No external plugin loader layout introduced. |
| Config-validation schema rules | Clean | No new schema config. |
| Identifier/naming-convention match | Clean | Env vars, modes, and filenames are consistent. |
| Planned-code compile-validity | Clean | No embedded Go compile issue found. |

**Options the author may not have considered:**
1. Add a release tag preflight block to Task 13 that verifies intended merged SHA, clean tree, and latest tag before `git tag`.
2. Make CI `tui-smoke` run both untagged PTY smoke and tagged helper contracts.
3. Remove all top-level direct commit wording for tap cleanup.

**Verdict reasoning:** FAIL. P30-P40 are mostly fixed, but top-level tap prerequisite drift, missing release-tag HEAD safety, and tagged smoke helper tests not being merge-gated by CI remained.
