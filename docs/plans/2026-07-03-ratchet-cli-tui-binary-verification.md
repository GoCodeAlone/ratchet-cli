# TUI Binary Verification Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Add credential-free automated proof for the Ratchet TUI binary, release-shaped startup behavior, Windows packaged CLI smoke, and release/tap artifact guards that prevent `ratchet-tui-smoke` from leaking into public artifacts.

**Architecture:** Keep the real release binary and the test-only TUI driver separate: untagged `ratchet` gets startup/daemon proof, while build-tagged Unix-only `ratchet-tui-smoke` drives the Bubble Tea event loop through PTY with a smoke daemon service. Add mode-gated `internal/releaseguard` Go tests plus thin scripts/workflows for GoReleaser, draft release assets, Windows zip smoke, and Homebrew Cask/Formula tap checks.

**Tech Stack:** Go 1.26, Bubble Tea v2, Unix PTY tests, gRPC daemon/client, GoReleaser v2.16+ config, GitHub Actions, Homebrew tap Ruby files, `gopkg.in/yaml.v3`.

**Base branch:** master

---

## Scope Manifest

**PR Count:** 5
**Tasks:** 12
**Estimated Lines of Change:** ~2600

**Out of scope:**
- Windows interactive ConPTY proof.
- Credentialed external provider CI.
- Broad slash-command registry refactor.
- Split-publish pre-public Homebrew/tap gating.
- New user-facing releaseguard CLI command.
- Replacing GoReleaser or Homebrew tap publishing.

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|------|-------|-------|--------|
| 1 | `test: add tui smoke binary harness` | Task 1, Task 2, Task 3 | `feat/tui-smoke-binary-harness` |
| 2 | `test: prove startup and command surfaces` | Task 4, Task 5, Task 6 | `feat/tui-startup-command-proof` |
| 3 | `chore: guard release artifacts` | Task 7, Task 8 | `feat/release-artifact-guard` |
| 4 | `chore: gate tap and windows release smoke` | Task 9, Task 10 | `feat/release-tap-windows-smoke` |
| 5 | `docs: publish harness evidence` | Task 11, Task 12 | `docs/tui-binary-verification-release` |

**Status:** Draft

## Global Design Guidance

Source: no repo-local `docs/design-guidance.md`, `AGENTS.md`, or `CLAUDE.md` found. Plan follows workspace guidance plus repo `README.md`/`RATCHET.md`.

| guidance | plan response |
|---|---|
| Build for Windows honestly. | Tasks 8-10 add Windows cross-build and packaged non-PTY command smoke; ConPTY remains out of scope. |
| Avoid duplicated plumbing. | Use existing daemon, client, TUI, GoReleaser, and Homebrew tap mechanisms; releaseguard is internal test/helper logic only. |
| Runtime claims need real boundaries. | Tasks 2-6 launch binaries, daemon/client RPCs, mock provider, PTY, and docs/command contracts. |
| Sensitive local data must not leak. | Tasks 1-6 add temp home/workdir, hook/instruction leak checks, and shared redaction for failure payloads. |
| CI/CD should stay portable through `wfctl`-style simple commands where possible. | Workflows run standard Go, GoReleaser, and shell wrapper commands; no platform-specific release logic beyond GitHub Actions release/tap mechanics already present. |

## Integration Matrix

| integration | classification | proof |
|---|---|---|
| Untagged `ratchet` binary | runtime-integrated | Task 4 builds/runs release-shaped `ratchet` for `version`, `help`, `daemon status`, and startup/onboarding boundary. |
| Build-tagged `ratchet-tui-smoke` binary | runtime-integrated | Tasks 1-3 build with `-tags tui_smoke` on Unix and drive PTY frames. |
| TUI event loop | runtime-integrated | Task 3 PTY asserts splash/chat/sidebar/tree/team/job frames and exit paths. |
| Daemon gRPC service/client | runtime-integrated | Task 2 smoke service/client RPCs cover providers, sessions, trust, chat, jobs. |
| Built-in mock provider | runtime-integrated | Task 2/3 chat prompt streams deterministic mock response. |
| Slash commands and shortcuts | runtime-integrated + focused proof | Tasks 3,5 split `pty-proven` and `focused-proven` rows with guards against overclaiming. |
| GoReleaser snapshot/draft assets | runtime-integrated | Tasks 7,10 inspect generated/uploaded archives, checksums, packaged binaries, cask/formula material. |
| Homebrew tap | config-only + cleanup + preflight + postcheck | Tasks 8-10 remove stale root file, add `brews`, preflight active surfaces, postcheck exact path-changing commits. |
| Windows packaged commands | runtime-integrated | Task 10 downloads snapshot zips and runs `ratchet.exe version/help/daemon status` on `windows-latest`. |
| Windows interactive PTY | deferred | Explicit out of scope. |

## Task 1: Smoke Binary Boundary And Source Manifest

**Files:**
- Create: `cmd/ratchet-tui-smoke/main.go`
- Create: `cmd/ratchet-tui-smoke/main_unix_test.go`
- Create: `internal/tui/smoke_source_manifest_test.go`
- Create: `internal/tui/race_enabled_test.go`
- Create: `internal/tui/race_disabled_test.go`
- Modify: `internal/client/client.go`
- Create: `internal/client/client_tui_smoke.go`
- Test: `cmd/ratchet-tui-smoke/*`, `internal/tui/*_test.go`, `internal/client/*_test.go`

**Step 1: Write failing boundary tests**

Add tests that assert:
- every non-test smoke file is listed in a test-owned manifest with exact `//go:build tui_smoke && !windows`;
- no unmanifested non-test Go file contains `ratchet-tui-smoke`, `tui_smoke`, or the smoke-only exported client constructor;
- `go list ./cmd/ratchet-tui-smoke` and `go build ./cmd/ratchet-tui-smoke` fail without tags on Unix;
- `go build -tags tui_smoke -o <tmp>/ratchet-tui-smoke ./cmd/ratchet-tui-smoke` succeeds on Unix;
- `GOOS=windows GOARCH=amd64/arm64 go list/build -tags tui_smoke ./cmd/ratchet-tui-smoke` fails with the expected Unix-only no-buildable-files class.

**Step 2: Run red checks**

```bash
go test ./internal/tui ./cmd/ratchet-tui-smoke -run 'SmokeSource|SmokeBinaryBuildTags' -count=1
```

Expected: FAIL with missing package/manifest/client constructor.

**Step 3: Implement minimal boundary**

Add Unix-only smoke main that launches the TUI using an injected smoke client/service. Add `internal/client/client_tui_smoke.go` with explicit socket constructor behind `tui_smoke && !windows`; leave untagged `client.Connect()` unchanged.

**Step 4: Verify**

```bash
gofmt -w cmd/ratchet-tui-smoke internal/client internal/tui
go test ./internal/tui ./cmd/ratchet-tui-smoke -run 'SmokeSource|SmokeBinaryBuildTags' -count=1
```

Expected: PASS; no repo-root binary output.

**Step 5: Commit**

```bash
git add cmd/ratchet-tui-smoke internal/client internal/tui
git commit -m "test: add tui smoke binary boundary"
```

Rollback: revert commit; no release binary behavior changes because smoke code is build-tagged.

## Task 2: Smoke Daemon Service And Job Panel RPC Proof

**Files:**
- Modify: `internal/daemon/engine.go`
- Modify: `internal/daemon/service.go`
- Create: `internal/daemon/service_tui_smoke.go`
- Create: `internal/daemon/service_tui_smoke_test.go`
- Modify: `internal/tui/components/jobpanel.go`
- Modify: `internal/tui/components/jobpanel_test.go`
- Modify: `internal/tui/pages/chat.go`
- Test: `internal/daemon/*_test.go`, `internal/tui/components/*_test.go`

**Step 1: Write failing tests**

Add tests that assert:
- smoke service constructor disables MCP discovery, plugin loading/daemon tools, autoresponder loading, cron/background work, and host `PATH` plugin scans;
- smoke service still initializes `JobRegistry` and safe `ListJobs` providers;
- `ListJobs` RPC succeeds and returns either a marker job or explicit empty-state metadata;
- `JobPanel.fetchJobs` surfaces refresh errors in test-observable state/UI instead of silently converting errors to empty list.

**Step 2: Run red checks**

```bash
go test ./internal/daemon ./internal/tui/components -run 'SmokeService|ListJobs|JobPanel.*Error' -count=1
```

Expected: FAIL with missing smoke constructor and hidden job-panel error.

**Step 3: Implement smoke service and observable errors**

Add private smoke option/constructor under `tui_smoke && !windows`. Initialize safe daemon service pieces needed by provider/session/trust/chat/jobs. Add job panel error field/render anchor and success marker/empty-state assertion.

**Step 4: Verify**

```bash
gofmt -w internal/daemon internal/tui/components internal/tui/pages
go test ./internal/daemon ./internal/tui/components -run 'SmokeService|ListJobs|JobPanel.*Error' -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/daemon internal/tui/components internal/tui/pages
git commit -m "test: wire tui smoke daemon service"
```

Rollback: revert commit; production daemon path remains through existing constructors.

## Task 3: Unix PTY TUI Binary Smoke

**Files:**
- Create: `internal/tui/tui_binary_smoke_unix_test.go`
- Create: `internal/tui/pty_capture_test.go`
- Modify: `internal/tui/pty_test.go`
- Modify: `internal/tui/pages/session_tree_test.go`
- Modify: `internal/tui/components/statusbar.go`
- Test: `internal/tui/*_test.go`

**Step 1: Write failing PTY tests**

Add `TestTUIBinarySmoke` with `tui_smoke && !windows`; skip under `-race` via package-local helper. Test builds `ratchet-tui-smoke` to temp path, launches in PTY with fixed size, temp home/state/workdir, harmless prompts, and synchronized bounded read snapshots.

PTY run must assert:
- splash/onboarding boundary, chat prompt, input visible;
- mock provider response stream completion;
- `/help`, `/provider list`, `/tree`, `/mode`, `/trust`, `/exit`;
- `ctrl+b`, `ctrl+s`, `ctrl+t`, `ctrl+j`, and advertised branch-tree navigation where classified `pty-proven`;
- job panel path has no RPC error and shows marker/empty state;
- `/exit`, `ctrl+c`, and `ctrl+d` each terminate through bounded subprocess/subtests.

**Step 2: Run red check**

```bash
go test -tags tui_smoke ./internal/tui -run TestTUIBinarySmoke -count=1 -timeout=8m
```

Expected: FAIL with missing smoke binary/service assertions.

**Step 3: Implement PTY capture helpers**

Use display-cell width checks (`lipgloss.Width` or `runewidth`) after ANSI stripping. Route every failure payload through shared redaction for home/workspace/temp/socket/executable paths and prompt/trust bodies. Reject runtime output containing instruction surfaces from `internal/agent/instructions.go` or hook config surfaces from `internal/hooks/hooks.go`.

**Step 4: Verify**

```bash
gofmt -w internal/tui
go test -tags tui_smoke ./internal/tui -run TestTUIBinarySmoke -count=1 -timeout=8m
go test -race ./internal/tui -run TestTUIBinarySmoke -count=1
```

Expected: first command PASS; second command SKIP with race-disabled message.

**Step 5: Commit**

```bash
git add internal/tui
git commit -m "test: drive tui smoke binary through pty"
```

Rollback: revert commit; only test/smoke-tag code is removed.

## Task 4: Release-Shaped Startup Smoke And Daemon Shutdown

**Files:**
- Modify: `internal/daemon/daemon.go`
- Modify: `internal/daemon/service.go`
- Modify: `internal/daemon/shutdown_test.go`
- Modify: `cmd/ratchet/harness_smoke_test.go`
- Modify: `cmd/ratchet/race_disabled_test.go`
- Modify: `cmd/ratchet/race_enabled_test.go`
- Test: `cmd/ratchet/*_test.go`, `internal/daemon/*_test.go`

**Step 1: Write failing tests**

Add tests that assert:
- production `daemon.Start` installs a real `Shutdown` callback that cancels server context, gracefully stops gRPC, and removes pid/socket files;
- public `Shutdown` over normal background daemon path removes pid/socket files;
- release-shaped built `ratchet` launches without `tui_smoke`, temp home/state/workdir, and reaches help/onboarding/provider setup boundary;
- cleanup sets parent test `HOME`/`USERPROFILE`/`XDG_STATE_HOME` to temp before normal untagged `client.Connect()`, verifies socket containment/`ModeSocket`/`0600`, then uses RPC/process handle only;
- startup smoke never calls `ratchet daemon stop` and never signals pidfile PID.

**Step 2: Run red checks**

```bash
go test ./internal/daemon ./cmd/ratchet -run 'Shutdown|StartupSmoke' -count=1 -timeout=8m
```

Expected: FAIL because production shutdown callback/startup cleanup is incomplete.

**Step 3: Implement shutdown and startup smoke**

Wire `daemon.Start` callback and bounded cleanup. Add redacted diagnostics for leftovers without terminating unrelated PIDs. Keep startup smoke skipped under race if needed; add focused non-race CI in Task 10.

**Step 4: Verify**

```bash
gofmt -w internal/daemon cmd/ratchet
go test ./internal/daemon ./cmd/ratchet -run 'Shutdown|StartupSmoke' -count=1 -timeout=8m
```

Expected: PASS; pid/socket temp paths removed.

**Step 5: Commit**

```bash
git add internal/daemon cmd/ratchet
git commit -m "test: prove release startup cleanup"
```

Rollback: revert commit; existing daemon start behavior returns. Check no temp daemon remains with `ratchet daemon status` under the test temp home if rerunning manually.

## Task 5: Command Surface, Help, And Shortcut Contracts

**Files:**
- Create: `internal/tui/commands/testdata/command_surface_spec.json`
- Modify: `internal/tui/commands/commands_test.go`
- Modify: `internal/tui/components/autocomplete_test.go`
- Modify: `cmd/ratchet/main.go`
- Create: `cmd/ratchet/cli_help_surface_test.go`
- Modify: `cmd/ratchet/harness_docs_test.go`
- Modify: `internal/tui/pages/chat.go`
- Modify: `internal/tui/pages/session_tree_test.go`
- Modify: `internal/tui/app_session_tree_test.go`
- Test: `internal/tui/commands`, `internal/tui/components`, `cmd/ratchet`, `internal/tui`

**Step 1: Write failing contract tests**

Shared fixture rows classify slash commands as `pty-proven`, `focused-proven`, or `deferred-runtime`. Tests assert:
- parser switch cases, `/help`, autocomplete literals, `modeCmd`, `trustCmd`, and `providerCmd` surfaces are classified;
- nonliteral/generated command cases fail unless fixture marks them runtime-tested;
- `cmd/ratchet` public help slash section and extracted `printUsage` rows match fixture;
- docs cannot claim PTY proof for focused/deferred rows;
- shortcut matrix distinguishes `pty-proven` and `focused-proven`;
- focused tests cover conditional `ctrl+h`, advertised branch-tree navigation, App-level branch switch into child chat history, and job-panel `Esc` close.

**Step 2: Run red checks**

```bash
go test ./internal/tui/commands ./internal/tui/components ./internal/tui ./cmd/ratchet -run 'CommandSurface|CLIHelp|Shortcut|Docs' -count=1
```

Expected: FAIL with missing fixture/extractor classifications.

**Step 3: Implement contracts**

Add JSON fixture loader in tests only. Export or test-wrap `printUsage` without changing public runtime behavior. Add docs overclaim scanner using sentence/table-row claim units and accepted evidence terms.

**Step 4: Verify**

```bash
gofmt -w internal/tui cmd/ratchet
go test ./internal/tui/commands ./internal/tui/components ./internal/tui ./cmd/ratchet -run 'CommandSurface|CLIHelp|Shortcut|Docs' -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tui cmd/ratchet
git commit -m "test: lock tui command surface"
```

Rollback: revert commit; no production CLI command behavior intentionally changes.

## Task 6: Documentation Evidence Guards

**Files:**
- Modify: `docs/harness-emulation.md`
- Modify: `README.md`
- Modify: `RATCHET.md`
- Modify: `docs/competitor-parity.md`
- Modify: `docs/policy-matrix.md`
- Modify: `cmd/ratchet/harness_docs_test.go`

**Step 1: Write failing docs assertions**

Docs guard must require exact positive automated evidence wording in README harness table and `docs/harness-emulation.md`, while RATCHET/parity/policy receive links plus negative overclaim scans unless they claim TUI binary evidence.

**Step 2: Run red check**

```bash
go test ./cmd/ratchet -run TestHarnessDocs -count=1
```

Expected: FAIL until docs name the new evidence boundaries.

**Step 3: Update docs**

Document:
- release-shaped startup smoke ≠ full TUI PTY proof;
- `ratchet-tui-smoke` is build-tagged test-only;
- Unix PTY rows and Windows packaged safe-command rows;
- Homebrew/tap safety is prechecked + postchecked/rollback, not fully pre-public gated.

**Step 4: Verify**

```bash
go test ./cmd/ratchet -run TestHarnessDocs -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add docs README.md RATCHET.md cmd/ratchet/harness_docs_test.go
git commit -m "docs: describe tui binary evidence"
```

Rollback: revert docs/test commit.

## Task 7: Releaseguard Package And Wrapper

**Files:**
- Create: `internal/releaseguard/guard.go`
- Create: `internal/releaseguard/guard_test.go`
- Create: `internal/releaseguard/goreleaser.go`
- Create: `internal/releaseguard/tap.go`
- Create: `internal/releaseguard/testdata/*`
- Create: `scripts/check-release-artifacts.sh`
- Modify: `go.mod`
- Modify: `go.sum`
- Test: `internal/releaseguard/*_test.go`, `scripts/check-release-artifacts.sh`

**Step 1: Write failing tests**

Add tests for:
- typed modes `manifest`, `draft-assets`, `tap-preflight`, `tap-postcheck`;
- ordinary `go test ./internal/releaseguard` runs unit fixtures and artifact tests skip with `releaseguard artifact mode not requested`;
- explicit mode with missing env fails before scanning;
- GoReleaser YAML parsing via `gopkg.in/yaml.v3`, no shell YAML parsing;
- strict top-level taxonomy: current publish keys `builds`, `archives`, `checksum`, `homebrew_casks`, `brews`, `release`; unknown publishable key fails;
- fallback scalar scan under artifact/publish sections rejects smoke tokens;
- archive matrix derives linux/darwin/windows amd64/arm64 and checks all archives/checksums/members/packaged binaries;
- generated/fallback cask and formula material only references release `ratchet` binary and formula/cask file name `ratchet-cli`.

**Step 2: Run red checks**

```bash
go test ./internal/releaseguard -count=1
```

Expected: FAIL with missing package.

**Step 3: Implement releaseguard**

Implement Go helpers and shell wrapper. Wrapper defaults to `goreleaser check`, `goreleaser release --snapshot --clean --skip=publish`, then `--manifest-only dist`; `--manifest-only <dir>` skips generation and runs explicit manifest mode.

**Step 4: Verify unit/fallback behavior**

```bash
gofmt -w internal/releaseguard
go test ./internal/releaseguard -count=1
RATCHET_RELEASE_GUARD_MODE=manifest go test ./internal/releaseguard -run TestManifestGuard -count=1
```

Expected: first command PASS with artifact-mode skip message; second command FAIL before scan with missing `RATCHET_RELEASE_GUARD_DIST`.

**Step 5: Verify local snapshot if GoReleaser available**

```bash
scripts/check-release-artifacts.sh
scripts/check-release-artifacts.sh --manifest-only dist
```

Expected: PASS; no manifest/checksum/archive member contains `ratchet-tui-smoke`.

**Step 6: Commit**

```bash
git add internal/releaseguard scripts/check-release-artifacts.sh go.mod go.sum
git commit -m "test: guard release artifacts"
```

Rollback: revert commit; release workflow remains pre-existing tag-only publish until Task 10 lands.

## Task 8: GoReleaser Formula Automation And Tap Cleanup Prereq

**Files:**
- Modify: `.goreleaser.yaml`
- Modify: `internal/releaseguard/goreleaser.go`
- Modify: `internal/releaseguard/tap.go`
- External modify: `GoCodeAlone/homebrew-tap:ratchet-cli.rb` removal only if stale root file exists
- External inspect: `GoCodeAlone/homebrew-tap:Formula/ratchet-cli.rb`, `GoCodeAlone/homebrew-tap:Casks/ratchet-cli.rb`

**Step 1: Write failing config tests**

Add releaseguard tests asserting:
- `.goreleaser.yaml` has `homebrew_casks` and `brews`;
- `brews[0].name == "ratchet-cli"`;
- `brews[0].ids == ["ratchet"]`;
- `brews[0].repository` matches `homebrew_casks[0].repository`;
- `brews[0].install` installs only `bin.install "ratchet"`;
- tap preflight fails if root `ratchet-cli.rb` exists or if active Formula/Cask lacks matching GoReleaser automation.

**Step 2: Run red checks**

```bash
go test ./internal/releaseguard -run 'GoReleaserHomebrew|TapPreflight' -count=1
```

Expected: FAIL because `brews` is absent and tap fixture has stale root file.

**Step 3: Add `brews` config**

Add GoReleaser v2 `brews` section targeting `GoCodeAlone/homebrew-tap` `main` with same token/author as cask and install block `bin.install "ratchet"`.

**Step 4: Prepare external tap cleanup**

In a separate checkout of `GoCodeAlone/homebrew-tap`, remove only stale root `ratchet-cli.rb`; preserve active `Formula/ratchet-cli.rb` and `Casks/ratchet-cli.rb`. Open and merge a tap PR or record the direct commit SHA if admin-merging. Record the SHA in this plan as a backport note before enabling fail-closed release workflow checks.

Verification:

```bash
RATCHET_RELEASE_GUARD_MODE=tap-preflight RATCHET_RELEASE_GUARD_TAP=<tap-checkout> go test ./internal/releaseguard -run TestTapPreflight -count=1
```

Expected before tap cleanup: FAIL naming stale root `ratchet-cli.rb`. Expected after cleanup plus `brews`: PASS.

**Step 5: Verify repo config**

```bash
goreleaser check
scripts/check-release-artifacts.sh --manifest-only dist
go test ./internal/releaseguard -run 'GoReleaserHomebrew|TapPreflight' -count=1
```

Expected: PASS after snapshot `dist` exists; no smoke token in cask/formula material.

**Step 6: Commit**

```bash
git add .goreleaser.yaml internal/releaseguard
git commit -m "chore: automate homebrew formula release"
```

Rollback: revert ratchet-cli commit and tap cleanup commit if fail-closed checks have not merged; if tap cleanup already merged, leave stale root removed because it is unmanaged.

## Task 9: CI Release-Check And Non-Race Smoke Jobs

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `scripts/check-release-artifacts.sh`
- Modify: `cmd/ratchet/harness_smoke_test.go`
- Test: `.github/workflows/ci.yml`

**Step 1: Add workflow checks**

Modify CI:
- `release-check`: checkout `fetch-depth: 0`, setup Go `1.26`, private-module Git rewrite, GoReleaser action `check`, GoReleaser action `release --snapshot --clean --skip=publish`, `scripts/check-release-artifacts.sh --manifest-only dist`, upload `ratchet-snapshot-dist`.
- `tui-smoke`: setup equivalent to existing CI and run `go test ./cmd/ratchet ./internal/tui -run 'HarnessSmoke|TUIBinarySmoke|StartupSmoke' -count=1 -timeout=10m` without `-race`.
- `tap-preflight`: read-only clone `GoCodeAlone/homebrew-tap`, run explicit tap preflight.

**Step 2: Verify workflow syntax**

```bash
go test ./cmd/ratchet ./internal/tui -run 'HarnessSmoke|TUIBinarySmoke|StartupSmoke' -count=1 -timeout=10m
scripts/check-release-artifacts.sh
```

Expected: PASS locally where GoReleaser is installed. If GitHub action syntax checker is available, run `actionlint`; expected PASS.

**Step 3: Commit**

```bash
git add .github/workflows/ci.yml scripts/check-release-artifacts.sh cmd/ratchet internal/tui
git commit -m "ci: add release and tui smoke checks"
```

Rollback: revert CI commit; runtime code remains independently tested.

## Task 10: Release Workflow, Draft Assets, Tap Postcheck, Windows Smoke

**Files:**
- Modify: `.github/workflows/release.yml`
- Modify: `.github/workflows/ci.yml`
- Modify: `internal/releaseguard/guard.go`
- Modify: `internal/releaseguard/tap.go`
- Create: `internal/releaseguard/github_assets.go`
- Test: `.github/workflows/*`, `internal/releaseguard/*_test.go`

**Step 1: Write failing releaseguard tests**

Add tests for:
- draft release assets mode downloads/reads archive/checksum fixtures by release id and requires draft state before undraft;
- tap postcheck resolves exact path-changing commit per tap file, scans content/metadata, groups rollback targets by SHA, and warns on mixed commits;
- Windows archive fixture requires both amd64 and arm64 zips; executes only amd64 in Windows job contract.

**Step 2: Update release workflow**

Before publish:
- private-module env + Git rewrite;
- GoReleaser `check`;
- GoReleaser snapshot `release --snapshot --clean --skip=publish`;
- manifest guard;
- tap preflight with recorded cleanup/formula automation SHA evidence.

After publish and before undraft:
- resolve draft release id by listing releases with retries;
- run draft asset postcheck against release id;
- clone tap and derive exact path-changing commits;
- run tap postcheck with `RATCHET_RELEASE_GUARD_TAP_NAMES`, `RATCHET_RELEASE_GUARD_TAP_COMMITS`, and current tag/version;
- only then undraft.

**Step 3: Add Windows packaged safe-command smoke**

In CI, add `windows-safe-command-smoke` on `windows-latest` with `needs: release-check`; build source `ratchet.exe`, download `ratchet-snapshot-dist`, require amd64/arm64 Windows zips, byte-scan both, extract amd64, run:

```powershell
.\ratchet.exe version
.\ratchet.exe help
.\ratchet.exe daemon status
```

Expected daemon status output contains `daemon is not running` under temp Windows home/state env.

**Step 4: Verify local releaseguard**

```bash
go test ./internal/releaseguard -run 'DraftAssets|TapPostcheck|WindowsArchive' -count=1
scripts/check-release-artifacts.sh --manifest-only dist
```

Expected: PASS.

**Step 5: Commit**

```bash
git add .github/workflows internal/releaseguard
git commit -m "ci: verify release assets and tap"
```

Rollback: revert workflow/releaseguard commit; if a tag release failed after GoReleaser publish, leave GitHub release draft, delete/supersede contaminated assets, and revert/supersede tap path-changing commits reported by postcheck.

## Task 11: Final Docs, Harness Table, And Overclaim Proof

**Files:**
- Modify: `docs/harness-emulation.md`
- Modify: `README.md`
- Modify: `RATCHET.md`
- Modify: `docs/competitor-parity.md`
- Modify: `docs/policy-matrix.md`
- Modify: `cmd/ratchet/harness_docs_test.go`

**Step 1: Update public docs**

Document final evidence:
- automated Unix PTY TUI smoke through `ratchet-tui-smoke`;
- release-shaped startup smoke for untagged `ratchet`;
- Windows packaged safe-command smoke;
- GoReleaser/draft asset/Homebrew tap checks;
- exact deferred Windows interactive PTY boundary.

**Step 2: Run docs guard**

```bash
go test ./cmd/ratchet -run TestHarnessDocs -count=1
```

Expected: PASS and no broad shortcut/slash/release overclaims.

**Step 3: Commit**

```bash
git add docs README.md RATCHET.md cmd/ratchet/harness_docs_test.go
git commit -m "docs: publish tui verification evidence"
```

Rollback: revert docs commit.

## Task 12: Full Verification, PRs, Release, And Retro

**Files:**
- Modify: `docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md`
- Create: `docs/retros/2026-07-03-ratchet-cli-tui-binary-verification-retro.md`
- Optional modify: `internal/version/version.go` only if repo uses source version bumps for releases

**Step 1: Run local verification**

```bash
go test ./internal/releaseguard -count=1
go test ./internal/daemon ./cmd/ratchet ./internal/tui/commands ./internal/tui/components ./internal/tui -run 'Shutdown|StartupSmoke|CommandSurface|CLIHelp|Shortcut|Docs|SmokeService|ListJobs|JobPanel|TUIBinarySmoke' -count=1 -timeout=12m
go test -tags tui_smoke ./internal/tui -run TestTUIBinarySmoke -count=1 -timeout=8m
go test -race ./... 
go vet ./...
goreleaser check
scripts/check-release-artifacts.sh
```

Expected: PASS for all commands; `-race` may skip smoke-specific PTY test with explicit race-disabled message.

**Step 2: PR and monitor**

Open PRs in manifest order. For each PR:
- ensure local focused tests pass before push;
- monitor CI until green;
- address code review with `autodev:receiving-code-review`;
- admin merge once green/approved or once local tests pass and checks are delayed per user approval.

**Step 3: Release**

After PR5 merges and `master` is green, tag the next semver patch/minor according to existing release history:

```bash
git fetch origin --tags
git describe --tags --abbrev=0
git tag v<next>
git push origin v<next>
```

Expected: release workflow stays draft until postchecks pass, then publishes release; Homebrew tap Formula/Cask updates contain current version/checksum and no smoke tokens.

**Step 4: Retro**

Use `autodev:post-merge-retrospective` and add retro with:
- design gates that caught real issues;
- CI/release/tap evidence;
- any follow-up for split-publish tap gating or Windows ConPTY.

**Step 5: Commit state**

```bash
git add docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md docs/retros/2026-07-03-ratchet-cli-tui-binary-verification-retro.md
git commit -m "docs: close tui binary verification"
```

Rollback: if release fails before undraft, leave draft private and fix assets/tap before publishing; if release publishes but tap postcheck fails, cut corrective patch release and path-specific tap corrective commit using reported SHA/path list.
