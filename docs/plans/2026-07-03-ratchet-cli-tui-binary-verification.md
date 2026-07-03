# TUI Binary Verification Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Add credential-free automated proof for the Ratchet TUI binary, release-shaped startup behavior, Windows cross-build/package archive safety, and release/tap artifact guards that prevent `ratchet-tui-smoke` from leaking into public artifacts.

**Architecture:** Keep the real release binary and the test-only TUI driver separate: untagged `ratchet` gets startup/daemon proof, while build-tagged Unix-only `ratchet-tui-smoke` drives the Bubble Tea event loop through PTY with a smoke daemon service. Add mode-gated `internal/releaseguard` Go tests plus thin scripts/workflows for GoReleaser, draft release assets, Windows archive checks, and Homebrew Cask tap checks.

**Tech Stack:** Go 1.26, Bubble Tea v2, Unix PTY tests, gRPC daemon/client, GoReleaser v2.16+ config, GitHub Actions, Homebrew tap Ruby files, `gopkg.in/yaml.v3`.

**Base branch:** master

---

## Scope Manifest

**PR Count:** 6
**Tasks:** 13
**Estimated Lines of Change:** ~2600
**External prerequisites:** 1 Homebrew tap state proof recorded by Task 9 before Tasks 10-11 start: either a merged cleanup PR SHA or existing tap HEAD SHA plus `TestTapPreflight` PASS proving stale unmanaged root/Formula surfaces are already absent. Direct tap commits are out of scope unless a fresh explicit plan amendment records that override.

**Out of scope:**
- Windows interactive ConPTY proof.
- New CI runner classes, including `windows-latest`, until a separate runner-change plan is approved.
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
| 4 | `chore: gate tap and windows archive proof` | Task 9, Task 10, Task 11 | `feat/release-tap-windows-smoke` |
| 5 | `docs: publish harness evidence` | Task 12 | `docs/tui-binary-verification-release` |
| 6 | `docs: close tui verification release` | Task 13 | `docs/tui-verification-closeout` |

**External prerequisite:**

| Repo | Work | Evidence | Gate |
|---|---|---|---|
| `GoCodeAlone/homebrew-tap` | Remove stale unmanaged root `ratchet-cli.rb` and legacy `Formula/ratchet-cli.rb` if present; preserve active `Casks/ratchet-cli.rb` as the supported GoReleaser v2.16+ install surface. | Merged cleanup PR SHA, or existing tap HEAD SHA plus `TestTapPreflight` PASS proving stale root/Formula surfaces already absent, recorded in Task 9 backport note. Direct tap commit requires a fresh explicit plan amendment. | Tasks 10-11 must not enable fail-closed tap/release enforcement until evidence is recorded. |

**Status:** Locked 2026-07-03T21:38:07Z

## Global Design Guidance

Source: no repo-local `docs/design-guidance.md`, `AGENTS.md`, or `CLAUDE.md` found. Plan follows workspace guidance plus repo `README.md`/`RATCHET.md`.

| guidance | plan response |
|---|---|
| Build for Windows honestly. | Tasks 10-11 add Windows cross-build and packaged archive inspection without changing CI runner classes; ConPTY and Windows executable runtime remain out of scope. |
| Avoid duplicated plumbing. | Use existing daemon, client, TUI, GoReleaser, and Homebrew tap mechanisms; releaseguard is internal test/helper logic only. |
| Runtime claims need real boundaries. | Tasks 2-6 launch binaries, daemon/client RPCs, mock provider, PTY, and docs/command contracts. |
| Sensitive local data must not leak. | Tasks 1-12 add temp home/workdir plus one shared redaction helper for TUI, daemon, command, docs-guard, GoReleaser, releaseguard, tap, draft-asset, workflow, and artifact-manifest failure payloads. |
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
| GoReleaser snapshot/draft assets | runtime-integrated | Tasks 7,10 inspect generated/uploaded archives, checksums, packaged binaries, and generated cask material. |
| Homebrew tap | config-only + cleanup + preflight + postcheck | Tasks 8-11 keep supported `homebrew_casks`, reject deprecated `brews`, remove stale root/Formula files, preflight active surfaces, postcheck exact path-changing commits. |
| Windows packaged archives | artifact-integrated | Task 11 requires Windows amd64/arm64 snapshot zips, byte-scans archive members and packaged executables, and cross-builds Windows binaries on existing runners; Windows executable runtime is deferred because no runner change is in scope. |
| Windows interactive PTY | deferred | Explicit out of scope. |

### Task 1: Smoke Binary Boundary And Source Manifest

**Files:**
- Create: `cmd/ratchet-tui-smoke/main.go`
- Create: `cmd/ratchet-tui-smoke/main_test.go`
- Create: `internal/daemon/service_tui_smoke.go`
- Create: `internal/tui/smoke_source_manifest_test.go`
- Create: `internal/tui/race_enabled_test.go`
- Create: `internal/tui/race_disabled_test.go`
- Modify: `internal/client/client.go`
- Create: `internal/client/client_tui_smoke.go`
- Create: `internal/client/client_tui_smoke_unix_test.go`
- Test: `cmd/ratchet-tui-smoke/*`, `internal/tui/*_test.go`, `internal/client/*_test.go`

**Step 1: Write failing boundary tests**

Add tests that assert:
- every non-test smoke file is listed in a test-owned manifest with exact `//go:build tui_smoke && !windows`;
- no unmanifested non-test Go file contains `ratchet-tui-smoke`, `tui_smoke`, or the smoke-only exported client constructor;
- `go list -f '{{len .GoFiles}}' ./cmd/ratchet-tui-smoke` reports zero non-test buildable files and `go build ./cmd/ratchet-tui-smoke` fails without tags on Unix;
- `go build -tags tui_smoke -o <tmp>/ratchet-tui-smoke ./cmd/ratchet-tui-smoke` succeeds on Unix;
- `GOOS=windows GOARCH=amd64/arm64 go list -f '{{len .GoFiles}}' -tags tui_smoke ./cmd/ratchet-tui-smoke` reports zero non-test buildable files and `GOOS=windows GOARCH=amd64/arm64 go build -tags tui_smoke ./cmd/ratchet-tui-smoke` fails with the expected Unix-only no-buildable-files class.
- tagged `internal/client` tests prove `ConnectSmokeUnix(ctx, tempRoot, socketPath)` accepts only a valid Unix socket inside `tempRoot` and rejects outside-temp paths, symlink final components, wrong file mode, non-`0600` permissions, unresolved parent paths, and TCP/non-`unix://` addresses.

**Step 2: Run red checks**

```bash
go test ./internal/tui ./cmd/ratchet-tui-smoke -run 'SmokeSource|SmokeBinaryBuildTags' -count=1
go test -tags tui_smoke ./internal/client -run 'ConnectSmokeUnix' -count=1
```

Expected: FAIL with missing package/manifest/client constructor.

**Step 3: Implement minimal boundary**

Add Unix-only smoke main that launches the TUI using an injected smoke client/service. Add the minimal `internal/daemon/service_tui_smoke.go` constructor/stub needed for `go build -tags tui_smoke ./cmd/ratchet-tui-smoke` to succeed; Task 2 expands its daemon behavior. Add `internal/client/client_tui_smoke.go` with explicit socket constructor behind `tui_smoke && !windows`; leave untagged `client.Connect()` unchanged.

**Step 4: Verify**

```bash
gofmt -w cmd/ratchet-tui-smoke internal/client internal/daemon internal/tui
go test ./internal/tui ./cmd/ratchet-tui-smoke -run 'SmokeSource|SmokeBinaryBuildTags' -count=1
go test -tags tui_smoke ./internal/client -run 'ConnectSmokeUnix' -count=1
```

Expected: PASS; `ConnectSmokeUnix` rejects the negative socket/path/permission cases; no repo-root binary output.

**Step 5: Commit**

```bash
git add cmd/ratchet-tui-smoke internal/client internal/daemon internal/tui
git commit -m "test: add tui smoke binary boundary"
```

Rollback: revert commit; no release binary behavior changes because smoke code is build-tagged.

### Task 2: Smoke Daemon Service And Job Panel RPC Proof

**Files:**
- Modify: `internal/daemon/engine.go`
- Modify: `internal/daemon/service.go`
- Modify: `internal/daemon/service_tui_smoke.go`
- Create: `internal/daemon/service_tui_smoke_test.go`
- Modify: `internal/tui/smoke_source_manifest_test.go`
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
go test -tags tui_smoke ./internal/daemon -run 'SmokeService|ListJobs' -count=1
go test ./internal/tui/components -run 'JobPanel.*Error' -count=1
```

Expected: FAIL with incomplete smoke constructor behavior and hidden job-panel error.

**Step 3: Implement smoke service and observable errors**

Expand the private smoke option/constructor under `tui_smoke && !windows`. Update the Task 1 smoke-source manifest to keep `internal/daemon/service_tui_smoke.go` as an allowed smoke runtime file with exact build-tag/exported-token constraints. Initialize safe daemon service pieces needed by provider/session/trust/chat/jobs. Add job panel error field/render anchor and success marker/empty-state assertion.

**Step 4: Verify**

```bash
gofmt -w internal/daemon internal/tui/components internal/tui/pages
go test -tags tui_smoke ./internal/daemon -run 'SmokeService|ListJobs' -count=1
go test ./internal/daemon -run 'SmokeService|ListJobs' -count=1
go test ./internal/tui/components -run 'JobPanel.*Error' -count=1
go test ./internal/tui -run SmokeSource -count=1
```

Expected: PASS; tagged daemon test proves smoke helper behavior, and untagged daemon test proves the helper is not exposed in release builds.

**Step 5: Commit**

```bash
git add internal/daemon internal/tui/smoke_source_manifest_test.go internal/tui/components internal/tui/pages
git commit -m "test: wire tui smoke daemon service"
```

Rollback: revert commit; production daemon path remains through existing constructors.

### Task 3: Unix PTY TUI Binary Smoke

**Files:**
- Create: `internal/tui/commands/testdata/command_surface_spec.json`
- Create: `internal/harnessredact/redact.go`
- Create: `internal/harnessredact/redact_test.go`
- Create: `internal/tui/tui_binary_smoke_unix_test.go`
- Create: `internal/tui/pty_capture_test.go`
- Modify: `internal/tui/pty_test.go`
- Modify: `internal/tui/pages/session_tree_test.go`
- Modify: `internal/tui/components/statusbar.go`
- Test: `internal/tui/*_test.go`

**Step 1: Write failing PTY tests**

Add `TestTUIBinarySmoke` in a Unix-only test file with `//go:build !windows`; do not require `tui_smoke` on the test file itself. Skip under `-race` via package-local helper. The test builds `cmd/ratchet-tui-smoke` to a temp path with `go build -tags tui_smoke -o <tmp>/ratchet-tui-smoke ./cmd/ratchet-tui-smoke`, launches that binary in PTY with fixed size, temp home/state/workdir, harmless prompts, and synchronized bounded read snapshots.

Create the initial `internal/tui/commands/testdata/command_surface_spec.json` fixture before the PTY test consumes it. Include the `pty-proven` rows required by the PTY run; Task 5 expands the same fixture for focused/help/autocomplete classifications.

PTY run must assert:
- splash/onboarding boundary, chat prompt, input visible;
- mock provider response stream completion;
- every `pty-proven` slash row from `internal/tui/commands/testdata/command_surface_spec.json`, including `/help`, `/provider list`, `/tree`, `/exit`, every documented mode value (`/mode conservative`, `/mode permissive`, `/mode locked`, `/mode sandbox`, `/mode custom`), and the scoped trust matrix (`/trust list`, `/trust allow "smoke:allow" --scope smoke`, `/trust deny "smoke:deny" --scope smoke`, `/trust persist allow "smoke:persist-allow" --scope smoke`, `/trust persist deny "smoke:persist-deny" --scope smoke`, `/trust grants`, `/trust revoke "smoke:persist-allow" --scope smoke`, `/trust reset`);
- follow-up state assertions after each mutating trust command: `/trust list` or `/trust grants` must show the expected pattern, action, and scope `smoke`; post-`/trust reset` assertions from the design must prove mode returns to smoke config default `conservative`, runtime allow/deny rules reset to config defaults, and unreverted persisted grants remain listed or are explicitly revoked by the tested command sequence;
- `ctrl+b`, `ctrl+s`, `ctrl+t`, `ctrl+j`, and advertised branch-tree navigation where classified `pty-proven`;
- job panel path has no RPC error and shows marker/empty state;
- `/exit`, `ctrl+c`, and `ctrl+d` each terminate through bounded subprocess/subtests.
- shared redaction helper removes real home, workspace, temp, Unix socket, executable, generated artifact paths, prompt bodies, and trust grant bodies from synthetic runtime/build/command-error payloads before any failure logging.

**Step 2: Run red check**

```bash
go test ./internal/harnessredact -run Redact -count=1
go test ./internal/tui -run TestTUIBinarySmoke -count=1 -timeout=8m
```

Expected: FAIL with missing shared redactor and smoke binary/service assertions.

**Step 3: Implement PTY capture helpers**

Use display-cell width checks (`lipgloss.Width` or `runewidth`) after ANSI stripping. Implement `internal/harnessredact` as the single failure-payload redaction path for home/workspace/temp/socket/executable/generated-artifact paths plus prompt/trust bodies. Route PTY capture, subprocess stdout/stderr, build errors, and command errors through it before `t.Fatal`/`t.Fatalf`. Reject runtime output containing instruction surfaces from `internal/agent/instructions.go` or hook config surfaces from `internal/hooks/hooks.go`.

**Step 4: Verify**

```bash
gofmt -w internal/harnessredact internal/tui
go test ./internal/harnessredact -run Redact -count=1
go test ./internal/tui -run TestTUIBinarySmoke -count=1 -timeout=8m
go test -race ./internal/tui -run TestTUIBinarySmoke -count=1
```

Expected: redactor command PASS and raw sensitive fixture strings absent from failure-rendered output; TUI command PASS; race command SKIP with race-disabled message.

**Step 5: Commit**

```bash
git add internal/harnessredact internal/tui internal/tui/commands/testdata/command_surface_spec.json
git commit -m "test: drive tui smoke binary through pty"
```

Rollback: revert commit; only test/smoke-tag code is removed.

### Task 4: Release-Shaped Startup Smoke And Daemon Shutdown

**Files:**
- Modify: `internal/daemon/daemon.go`
- Modify: `internal/daemon/service.go`
- Modify: `internal/daemon/shutdown_test.go`
- Modify: `internal/client/client.go`
- Modify: `cmd/ratchet/harness_smoke_test.go`
- Modify: `cmd/ratchet/race_disabled_test.go`
- Modify: `cmd/ratchet/race_enabled_test.go`
- Test: `cmd/ratchet/*_test.go`, `internal/client/*_test.go`, `internal/daemon/*_test.go`

**Step 1: Write failing tests**

Add tests that assert:
- production `daemon.Start` installs a real `Shutdown` callback that cancels server context, gracefully stops gRPC, and removes pid/socket files;
- `internal/client.Client` exposes `Shutdown(ctx context.Context) error` as a thin public wrapper over the generated daemon RPC;
- public `Shutdown` over normal background daemon path removes pid/socket files;
- release-shaped built `ratchet` launches without `tui_smoke`, temp home/state/workdir, and reaches help/onboarding/provider setup boundary;
- cleanup sets parent test `HOME`/`USERPROFILE`/`XDG_STATE_HOME` to temp before normal untagged `client.Connect()`, verifies socket containment/`ModeSocket`/`0600`, then uses RPC/process handle only;
- startup smoke never calls `ratchet daemon stop` and never signals pidfile PID.

**Step 2: Run red checks**

```bash
go test ./internal/client ./internal/daemon ./cmd/ratchet -run 'Shutdown|StartupSmoke|ClientShutdown' -count=1 -timeout=8m
```

Expected: FAIL because production shutdown callback/startup cleanup is incomplete.

**Step 3: Implement shutdown and startup smoke**

Add `Client.Shutdown(ctx)` to call `pb.RatchetDaemonClient.Shutdown`. Wire `daemon.Start` callback and bounded cleanup. Add redacted diagnostics for leftovers without terminating unrelated PIDs. Keep startup smoke skipped under race if needed; add focused non-race CI in Task 10.

**Step 4: Verify**

```bash
gofmt -w internal/client internal/daemon cmd/ratchet
go test ./internal/client ./internal/daemon ./cmd/ratchet -run 'Shutdown|StartupSmoke|ClientShutdown' -count=1 -timeout=8m
```

Expected: PASS; pid/socket temp paths removed.

**Step 5: Commit**

```bash
git add internal/client internal/daemon cmd/ratchet
git commit -m "test: prove release startup cleanup"
```

Rollback: revert commit; existing daemon start behavior returns. Check no temp daemon remains with `ratchet daemon status` under the test temp home if rerunning manually.

### Task 5: Command Surface, Help, And Shortcut Contracts

**Files:**
- Modify: `internal/tui/commands/testdata/command_surface_spec.json`
- Modify: `internal/tui/commands/commands_test.go`
- Modify: `internal/tui/components/autocomplete_test.go`
- Modify: `cmd/ratchet/main.go`
- Create: `cmd/ratchet/cli_help_surface_test.go`
- Modify: `cmd/ratchet/harness_docs_test.go`
- Modify: `internal/harnessredact/redact_test.go`
- Modify: `internal/tui/pages/chat.go`
- Modify: `internal/tui/pages/session_tree_test.go`
- Modify: `internal/tui/app_session_tree_test.go`
- Test: `internal/tui/commands`, `internal/tui/components`, `cmd/ratchet`, `internal/tui`

**Step 1: Write failing contract tests**

Expand the Task 3 fixture rows to classify every slash command as `pty-proven`, `focused-proven`, or `deferred-runtime`. Tests assert:
- the fixture contains exact `pty-proven` rows for all five `/mode` values and each scoped `/trust` matrix command used by `TestTUIBinarySmoke`, including `--scope smoke` where applicable and required follow-up assertions for pattern/action/scope state;
- parser switch cases, `/help`, autocomplete literals, `modeCmd`, `trustCmd`, and `providerCmd` surfaces are classified;
- nonliteral/generated command cases fail unless fixture marks them runtime-tested;
- `cmd/ratchet` public help slash section and extracted `printUsage` rows match fixture;
- docs cannot claim PTY proof for focused/deferred rows;
- shortcut matrix distinguishes `pty-proven` and `focused-proven`;
- focused tests cover conditional `ctrl+h`, advertised branch-tree navigation, App-level branch switch into child chat history, and job-panel `Esc` close.
- docs-guard and CLI help failure paths call `internal/harnessredact` before reporting command output, extracted docs snippets, or generated command-error payloads.

**Step 2: Run red checks**

```bash
go test ./internal/tui/commands ./internal/tui/components ./internal/tui ./cmd/ratchet -run 'CommandSurface|CLIHelp|Shortcut|Docs' -count=1
go test ./internal/harnessredact -run 'Redact.*Docs|Redact.*Command' -count=1
```

Expected: FAIL with missing fixture/extractor classifications.

**Step 3: Implement contracts**

Add JSON fixture loader in tests only. Export or test-wrap `printUsage` without changing public runtime behavior. Add docs overclaim scanner using sentence/table-row claim units and accepted evidence terms.

**Step 4: Verify**

```bash
gofmt -w internal/harnessredact internal/tui cmd/ratchet
go test ./internal/tui/commands ./internal/tui/components ./internal/tui ./cmd/ratchet -run 'CommandSurface|CLIHelp|Shortcut|Docs' -count=1
go test ./internal/harnessredact -run 'Redact.*Docs|Redact.*Command' -count=1
go test ./internal/tui -run TestTUIBinarySmoke -count=1 -timeout=8m
```

Expected: PASS; any fixture row newly marked `pty-proven` is exercised again by `TestTUIBinarySmoke`.

**Step 5: Commit**

```bash
git add internal/harnessredact internal/tui cmd/ratchet
git commit -m "test: lock tui command surface"
```

Rollback: revert commit; no production CLI command behavior intentionally changes.

### Task 6: Documentation Evidence Guards

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
- Unix PTY rows and Windows cross-build/package archive rows;
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

### Task 7: Releaseguard Package And Wrapper

**Files:**
- Create: `internal/releaseguard/guard.go`
- Create: `internal/releaseguard/guard_test.go`
- Create: `internal/releaseguard/goreleaser.go`
- Create: `internal/releaseguard/tap.go`
- Create: `internal/releaseguard/testdata/*`
- Modify: `internal/harnessredact/redact.go`
- Modify: `internal/harnessredact/redact_test.go`
- Modify: `internal/tui/smoke_source_manifest_test.go`
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
- `TestGoReleaserReleaseDraftConfig` fails unless `.goreleaser.yaml` contains `release.draft: true`;
- smoke-source guard tooling allowlist includes `internal/releaseguard` files that hold forbidden artifact tokens, while keeping them out of the smoke runtime manifest;
- strict top-level taxonomy: current publish keys `builds`, `archives`, `checksum`, `homebrew_casks`, `release`; deprecated `brews` and unknown publishable keys fail;
- strict top-level taxonomy: current nonpublishable metadata keys `version` and `changelog` are allowed but not scanned as publishable artifact surfaces;
- fallback scalar scan under artifact/publish sections rejects smoke tokens;
- archive matrix derives linux/darwin/windows amd64/arm64 and checks all archives/checksums/members/packaged binaries;
- wrapper extracts the host-compatible GoReleaser archive from snapshot/draft assets, runs the packaged `ratchet version` and `ratchet help`, and fails if output contains `ratchet-tui-smoke`, `tui_smoke`, smoke command/flag/help markers, or missing expected release identity text;
- generated/fallback cask material only references release `ratchet` binary and cask file name `ratchet-cli`.
- releaseguard, wrapper, and manifest failure paths all use `internal/harnessredact` and tests cover representative GoReleaser snapshot output, artifact-manifest output, draft-assets output, tap-preflight/tap-postcheck output, workflow-command errors, docs-guard output, generic command errors, and generated artifact paths;
- releaseguard redaction tests inject raw home/workspace/temp/socket/executable/artifact paths plus prompt and trust bodies into those representative failures and assert raw strings are absent while stable placeholders such as `<home>`, `<workspace>`, `<temp>`, `<socket>`, `<executable>`, `<artifact>`, `<prompt>`, and `<trust>` are present.

**Step 2: Run red checks**

```bash
go test ./internal/releaseguard -count=1
go test ./internal/harnessredact ./internal/releaseguard -run 'Redact|ReleaseGuardRedaction' -count=1
```

Expected: FAIL with missing package.

**Step 3: Implement releaseguard**

Implement Go helpers and shell wrapper. Route every releaseguard error, wrapper-captured GoReleaser stdout/stderr, manifest diff, packaged `ratchet version/help` output, draft-asset/tap diagnostic, workflow-command fixture, docs-guard fixture, and command-error fixture through `internal/harnessredact` before logging or failing. Update the smoke-source tooling allowlist for `internal/releaseguard` exact forbidden-token constants without adding releaseguard files to the smoke runtime manifest. Wrapper defaults to `goreleaser check`, `goreleaser release --snapshot --clean --skip=publish`, then `--manifest-only dist`; `--manifest-only <dir>` skips generation and runs explicit manifest mode. In both generated and manifest-only modes, the wrapper extracts the host-compatible packaged archive and executes the packaged `ratchet version` and `ratchet help` as part of the artifact proof.

**Step 4: Verify unit/fallback behavior**

```bash
gofmt -w internal/harnessredact internal/releaseguard
go test ./internal/releaseguard -count=1
go test ./internal/harnessredact ./internal/releaseguard -run 'Redact|ReleaseGuardRedaction' -count=1
logfile="$(mktemp)"
if RATCHET_RELEASE_GUARD_MODE=manifest go test ./internal/releaseguard -run TestManifestGuard -count=1 >"$logfile" 2>&1; then
  echo "expected missing RATCHET_RELEASE_GUARD_DIST failure"
  exit 1
fi
rg 'RATCHET_RELEASE_GUARD_DIST' "$logfile"
go test ./internal/tui -run SmokeSource -count=1
```

Expected: first command PASS with artifact-mode skip message; redaction command PASS and no raw home/workspace/temp/socket/executable/artifact/prompt/trust fixtures survive; negative manifest command fails before scan and log contains `RATCHET_RELEASE_GUARD_DIST`.

**Step 5: Verify local snapshot if GoReleaser available**

```bash
scripts/check-release-artifacts.sh
scripts/check-release-artifacts.sh --manifest-only dist
```

Expected: PASS; no manifest/checksum/archive member or packaged executable contains `ratchet-tui-smoke`; host-compatible packaged `ratchet version` and `ratchet help` execute from the extracted archive and their output contains release identity text with no smoke markers.

**Step 6: Commit**

```bash
git add internal/harnessredact internal/releaseguard internal/tui/smoke_source_manifest_test.go scripts/check-release-artifacts.sh go.mod go.sum
git commit -m "test: guard release artifacts"
```

Rollback: revert commit; release workflow remains pre-existing tag-only publish until Task 11 lands.

### Task 8: GoReleaser Homebrew Cask Guard

**Files:**
- Modify: `internal/releaseguard/goreleaser.go`
- Modify: `internal/releaseguard/tap.go`

**Step 1: Write failing config tests**

Add releaseguard tests asserting:
- `.goreleaser.yaml` has `homebrew_casks` configured for `ratchet-cli`;
- `.goreleaser.yaml` has no `brews` section because GoReleaser v2.16 fully deprecates Formula generation and local/CI `goreleaser check` must remain green on the pinned v2 action;
- `homebrew_casks[0].ids == ["ratchet"]`;
- `homebrew_casks[0].binaries == ["ratchet"]`;
- `homebrew_casks[0].repository` targets `GoCodeAlone/homebrew-tap` `main`;
- fixture tap preflight fails if root `ratchet-cli.rb` exists, if legacy `Formula/ratchet-cli.rb` exists, or if active `Casks/ratchet-cli.rb` lacks matching GoReleaser cask automation.

**Step 2: Run red checks**

```bash
go test ./internal/releaseguard -run 'GoReleaserHomebrew|TapPreflight' -count=1
```

Expected: FAIL because the tap fixture has stale unmanaged root/Formula files or because cask automation assertions are not implemented yet.

**Step 3: Implement cask-only guard**

Keep `.goreleaser.yaml` on the current GoReleaser-supported `homebrew_casks` surface. Implement releaseguard parsing that rejects any `brews` section, validates the cask id/binary/repository/branch/token/author fields, and treats legacy root/Formula tap files as unmanaged install surfaces that must be cleaned up in Task 9 before fail-closed release checks merge.

**Step 4: Verify repo config with fresh snapshot**

```bash
goreleaser check
scripts/check-release-artifacts.sh
scripts/check-release-artifacts.sh --manifest-only dist
go test ./internal/releaseguard -run 'GoReleaserHomebrew|TapPreflight' -count=1
```

Expected: PASS; wrapper regenerates fresh `dist` before manifest-only check; no smoke token in cask/tap material.
Expected also: `goreleaser check` passes with GoReleaser v2.16+ and no deprecated `brews` warning/error path is introduced.

**Step 5: Commit**

```bash
git add internal/releaseguard
git commit -m "test: guard homebrew cask release"
```

Rollback: revert ratchet-cli commit if fail-closed checks have not merged.

### Task 9: Homebrew Tap Cleanup Prerequisite

**Files:**
- External modify: `GoCodeAlone/homebrew-tap:ratchet-cli.rb` removal only if stale root file exists
- External modify: `GoCodeAlone/homebrew-tap:Formula/ratchet-cli.rb` removal only if legacy Formula file exists
- External inspect: `GoCodeAlone/homebrew-tap:Casks/ratchet-cli.rb`
- Modify: `docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md`

**Step 1: Clone and inspect tap**

```bash
gh repo clone GoCodeAlone/homebrew-tap <tap-checkout>
test -f <tap-checkout>/Casks/ratchet-cli.rb
if test -f <tap-checkout>/ratchet-cli.rb; then
  echo "stale root present"
else
  echo "stale root already absent"
fi
if test -f <tap-checkout>/Formula/ratchet-cli.rb; then
  echo "legacy formula present"
else
  echo "legacy formula already absent"
fi
```

Expected before cleanup: active Cask exists; output records whether stale root and legacy Formula surfaces are present or already absent.

**Step 2: Run preflight before cleanup**

```bash
RATCHET_RELEASE_GUARD_MODE=tap-preflight RATCHET_RELEASE_GUARD_TAP=<tap-checkout> go test -count=1 ./internal/releaseguard -run TestTapPreflight
```

Expected before cleanup: FAIL naming stale unmanaged root/Formula files, unless both were already removed. If stale root/Formula surfaces are already absent and preflight PASSes after Task 8 cask guard, record the current tap HEAD SHA as state proof and skip PR creation.

**Step 3: Remove only unsupported legacy files**

If stale root or legacy Formula exists, remove only root `ratchet-cli.rb` and `Formula/ratchet-cli.rb`. Preserve `Casks/ratchet-cli.rb`. Commit, push, open PR, wait for tap checks, and admin merge only when required checks are green and review requirements are satisfied. Direct commits to the tap are out of scope unless a fresh explicit plan amendment records that override. Record the merged tap commit SHA. If stale root and legacy Formula are already absent, make no tap commit; record the current tap HEAD SHA instead after Step 4 passes.

**Step 4: Verify cleanup with cask guard**

```bash
git -C <tap-checkout> fetch origin main
git -C <tap-checkout> checkout origin/main
RATCHET_RELEASE_GUARD_MODE=tap-preflight RATCHET_RELEASE_GUARD_TAP=<tap-checkout> go test -count=1 ./internal/releaseguard -run TestTapPreflight
```

Expected after cleanup and Task 8 cask guard: PASS.

**Step 5: Record prerequisite evidence**

Append a compact backport note to this plan:

```markdown
### Backport YYYY-MM-DD: Homebrew tap cleanup prerequisite

Evidence: GoCodeAlone/homebrew-tap@<sha> either removed stale root `ratchet-cli.rb` and legacy `Formula/ratchet-cli.rb` via merged PR or already had both surfaces absent at recorded HEAD; `Casks/ratchet-cli.rb` remains; `TestTapPreflight` PASS.
Ratchet cask guard: GoCodeAlone/ratchet-cli@<sha> validates GoReleaser `homebrew_casks` and rejects deprecated `brews`; `scripts/check-release-artifacts.sh` PASS.
Scope: no manifest change.
```

Tasks 10 and 11 must not enable fail-closed `tap-preflight` or release postcheck until this tap state SHA and Task 8's ratchet-cli cask-guard commit SHA are recorded.

**Step 6: Commit plan evidence**

```bash
git add docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md
git commit -m "docs: record homebrew tap cleanup"
```

Rollback: if fail-closed checks have not merged, revert the ratchet-cli cask-guard commit and leave stale root/legacy Formula removed because they are unmanaged under GoReleaser v2.16+. If tap cleanup itself must be reverted, restore only the removed root/Formula files from the recorded tap SHA and rerun tap preflight to confirm the expected failure before disabling fail-closed enforcement.

### Task 10: CI Release-Check And Non-Race Smoke Jobs

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `scripts/check-release-artifacts.sh`
- Modify: `cmd/ratchet/harness_smoke_test.go`
- Test: `.github/workflows/ci.yml`

**Precondition:** Task 9 backport note records tap state proof SHA (merged cleanup PR SHA or existing tap HEAD SHA with `TestTapPreflight` PASS proving stale unmanaged root/Formula surfaces absent) and Task 8 cask-guard commit SHA.

**Step 1: Add workflow checks**

Modify CI:
- `release-check`: checkout `fetch-depth: 0`, setup Go `1.26`, private-module Git rewrite, GoReleaser action `goreleaser/goreleaser-action@v7` with `version: "~> v2"` and `args: check`, GoReleaser action `goreleaser/goreleaser-action@v7` with `version: "~> v2"` and `args: release --snapshot --clean --skip=publish`, `scripts/check-release-artifacts.sh --manifest-only dist`, upload `ratchet-snapshot-dist`.
- existing `windows-build`: replace fixed `/tmp/ratchet-windows-*.exe` outputs with `$RUNNER_TEMP/ratchet-windows-*.exe` or an equivalent unique temp directory, and add workflow assertions that no CI step writes `/tmp/ratchet-windows-*.exe`.
- `tui-smoke`: setup equivalent to existing CI and run untagged smoke plus tagged helper contracts without `-race`:
  - `go test ./cmd/ratchet ./internal/tui -run 'HarnessSmoke|TUIBinarySmoke|StartupSmoke' -count=1 -timeout=10m`;
  - `go test -tags tui_smoke ./internal/client -run 'ConnectSmokeUnix' -count=1`;
  - `go test -tags tui_smoke ./internal/daemon -run 'SmokeService|ListJobs' -count=1`.
- `tap-preflight`: read-only clone `GoCodeAlone/homebrew-tap`, then run `RATCHET_RELEASE_GUARD_MODE=tap-preflight RATCHET_RELEASE_GUARD_TAP=<tap-checkout> go test -count=1 ./internal/releaseguard -run TestTapPreflight`.

**Step 2: Verify workflow syntax and fresh release snapshot**

```bash
go test ./cmd/ratchet ./internal/tui -run 'HarnessSmoke|TUIBinarySmoke|StartupSmoke' -count=1 -timeout=10m
go test -tags tui_smoke ./internal/client -run 'ConnectSmokeUnix' -count=1
go test -tags tui_smoke ./internal/daemon -run 'SmokeService|ListJobs' -count=1
scripts/check-release-artifacts.sh
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/ci.yml .github/workflows/release.yml
```

Expected: PASS locally where GoReleaser is installed; pinned actionlint command succeeds; CI workflow uses `goreleaser/goreleaser-action@v7` with `version: "~> v2"` for every GoReleaser action step; `scripts/check-release-artifacts.sh` regenerates fresh `dist` and executes packaged host-compatible `ratchet version/help`.
Expected also: CI workflow contains no fixed `/tmp/ratchet-windows-*.exe` output paths; existing Windows cross-build writes to `$RUNNER_TEMP` or a unique temp directory.

**Step 3: Commit**

```bash
git add .github/workflows/ci.yml scripts/check-release-artifacts.sh cmd/ratchet internal/tui
git commit -m "ci: add release and tui smoke checks"
```

Rollback: revert CI commit; runtime code remains independently tested.

### Task 11: Release Workflow, Draft Assets, Tap Postcheck, Windows Archive Proof

**Files:**
- Modify: `.github/workflows/release.yml`
- Modify: `.github/workflows/ci.yml`
- Modify: `internal/releaseguard/guard.go`
- Modify: `internal/releaseguard/tap.go`
- Test: `.github/workflows/*`, `internal/releaseguard/*_test.go`

**Precondition:** Task 9 backport note records tap state proof SHA (merged cleanup PR SHA or existing tap HEAD SHA with `TestTapPreflight` PASS proving stale unmanaged root/Formula surfaces absent) and Task 8 cask-guard commit SHA.

**Step 1: Write failing releaseguard tests**

Add tests for:
- `TestGoReleaserReleaseDraftConfig` is run by release preflight and fails unless `.goreleaser.yaml` contains `release.draft: true`;
- draft release assets mode reads an already-downloaded asset directory from `RATCHET_RELEASE_GUARD_ASSETS` plus `RATCHET_RELEASE_GUARD_VERSION`; it fails if expected archives/checksums are missing, if forbidden smoke tokens appear in artifact names, archive member names, packaged executable bytes, Homebrew generated material, or tap material, or if fixture metadata says the release is not draft;
- tap postcheck resolves exact path-changing commit per tap file, scans content/metadata, groups rollback targets by SHA, and warns on mixed commits;
- Windows archive fixture requires both amd64 and arm64 zips and proves no workflow step executes `ratchet.exe` or adds a Windows runner class in this slice.
- mode-selected fixture tests provide testdata env for `draft-assets`, `tap-postcheck`, and Windows archive checks instead of relying on ordinary skip behavior;
- draft-assets, tap-postcheck, Windows archive, workflow-command, GoReleaser release, and tap clone/auth failure fixtures prove they use `internal/harnessredact`; raw release asset directory, generated archive path, Windows executable path, workspace path, token-like prompt/trust body, and temp tap checkout path must be absent from failing output.

**Step 2: Update release workflow**

Before publish:
- private-module env + Git rewrite;
- GoReleaser action `goreleaser/goreleaser-action@v7` with `version: "~> v2"` and `args: check`;
- GoReleaser action `goreleaser/goreleaser-action@v7` with `version: "~> v2"` and `args: release --snapshot --clean --skip=publish`;
- manifest guard;
- pre-publish draft config guard with `go test -count=1 ./internal/releaseguard -run TestGoReleaserReleaseDraftConfig` before `goreleaser release --clean`;
- tap preflight with recorded cleanup/cask-guard SHA evidence.

After publish and before undraft:
- resolve draft release id by listing releases with retries;
- use GitHub Script/API to verify the resolved release is still draft, download all assets for that release id into `$RUNNER_TEMP/release-assets`, write a small metadata file in that directory containing the release id/tag/draft state, then run draft asset postcheck with `RATCHET_RELEASE_GUARD_MODE=draft-assets`, `RATCHET_RELEASE_GUARD_ASSETS=$RUNNER_TEMP/release-assets`, and `RATCHET_RELEASE_GUARD_VERSION=<tag-or-version>` using `go test -count=1 ./internal/releaseguard -run TestDraftAssets`;
- clone the configured Homebrew tap repo/branch using `HOMEBREW_TAP_TOKEN` with the same owner/name/branch from `.goreleaser.yaml` (`GoCodeAlone/homebrew-tap`, `main`) and derive exact path-changing commits from that authenticated checkout;
- run tap postcheck with the full required env:
  `RATCHET_RELEASE_GUARD_MODE=tap-postcheck`,
  `RATCHET_RELEASE_GUARD_TAP=<tap-checkout>`,
  `RATCHET_RELEASE_GUARD_TAP_NAMES=<tap-names>`,
  `RATCHET_RELEASE_GUARD_TAP_COMMITS=<path=sha,...>`, and
  `RATCHET_RELEASE_GUARD_VERSION=<tag-or-version>` using
  `go test -count=1 ./internal/releaseguard -run TestTapPostcheck`;
- only then undraft.

**Step 3: Add Windows package archive proof without runner change**

In the existing `release-check` job, keep the current runner class and extend artifact inspection after GoReleaser snapshot generation: require both `ratchet_windows_amd64.zip` and `ratchet_windows_arm64.zip`, list archive members, assert each zip contains exactly one `ratchet.exe` payload in the expected package layout, reject smoke tokens in archive file names/member names/checksum entries, byte-scan contained executable payloads and generated Homebrew/tap material for `ratchet-tui-smoke`, `tui_smoke`, smoke-only flags, and smoke help text, and compare generated checksums against the Windows archives. Packaged Markdown docs such as `RATCHET.md` are not binary-leak inputs; they are validated by the docs guard/approved wording templates. Do not add `windows-latest` or any new runner class; Windows executable runtime smoke is deferred to a future runner-change plan.

Expected: `release-check` remains on its existing runner; Windows archive inspection fails if either zip is missing, if `ratchet.exe` is missing or duplicated, if checksums omit either zip, if any artifact name/member name/executable/Homebrew/tap material contains forbidden smoke tokens, or if workflow YAML contains `windows-latest` or a `ratchet.exe` execution step. Packaged docs may mention approved `ratchet-tui-smoke` evidence boundaries only through docs-guarded templates.

**Step 4: Verify local Windows cross-build and releaseguard**

```bash
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
GOOS=windows GOARCH=amd64 go build -o "$tmpdir/ratchet-windows-amd64.exe" ./cmd/ratchet
GOOS=windows GOARCH=arm64 go build -o "$tmpdir/ratchet-windows-arm64.exe" ./cmd/ratchet
go test ./internal/releaseguard -run 'DraftAssets|TapPostcheck|WindowsArchive' -count=1
RATCHET_RELEASE_GUARD_MODE=draft-assets RATCHET_RELEASE_GUARD_ASSETS=internal/releaseguard/testdata/draft-assets RATCHET_RELEASE_GUARD_VERSION=v0.0.0-test go test ./internal/releaseguard -run TestDraftAssets -count=1
RATCHET_RELEASE_GUARD_MODE=tap-postcheck RATCHET_RELEASE_GUARD_TAP=internal/releaseguard/testdata/tap RATCHET_RELEASE_GUARD_TAP_NAMES=ratchet-cli RATCHET_RELEASE_GUARD_TAP_COMMITS=Casks/ratchet-cli.rb=fixture-cask-sha RATCHET_RELEASE_GUARD_VERSION=v0.0.0-test go test ./internal/releaseguard -run TestTapPostcheck -count=1
RATCHET_RELEASE_GUARD_MODE=manifest RATCHET_RELEASE_GUARD_DIST=internal/releaseguard/testdata/windows-dist go test ./internal/releaseguard -run TestWindowsArchive -count=1
go test ./internal/releaseguard -run TestGoReleaserReleaseDraftConfig -count=1
go test ./internal/harnessredact ./internal/releaseguard -run 'Redact|ReleaseGuardRedaction|WorkflowCommandRedaction' -count=1
scripts/check-release-artifacts.sh
scripts/check-release-artifacts.sh --manifest-only dist
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/ci.yml .github/workflows/release.yml
```

Expected: PASS; draft config test proves `release.draft: true` before publish; explicit fixture-mode tests prove draft-assets/tap-postcheck/Windows archive behavior without skip-only false positives; redaction tests prove draft/tap/workflow/Windows failure payloads use the shared helper; Windows binaries are written only under the unique `mktemp -d` directory; wrapper regenerates fresh `dist` and executes packaged host-compatible `ratchet version/help`; pinned workflow lint is clean; every GoReleaser action step uses `goreleaser/goreleaser-action@v7` with `version: "~> v2"`; `tap-preflight` uses `RATCHET_RELEASE_GUARD_MODE=tap-preflight`, `RATCHET_RELEASE_GUARD_TAP`, and `go test -count=1`; workflow diff contains no `windows-latest`, no new runner class, and no `ratchet.exe` execution step; `release-check` proves Windows amd64/arm64 archives/checksums/member-name/executable-byte scans while packaged docs are handled by docs guard; release workflow clones the tap with `HOMEBREW_TAP_TOKEN`; release workflow sets `RATCHET_RELEASE_GUARD_TAP`, `RATCHET_RELEASE_GUARD_TAP_NAMES`, `RATCHET_RELEASE_GUARD_TAP_COMMITS`, and `RATCHET_RELEASE_GUARD_VERSION` for tap-postcheck.

**Step 5: Commit**

```bash
git add .github/workflows internal/harnessredact internal/releaseguard
git commit -m "ci: verify release assets and tap"
```

Rollback: revert workflow/releaseguard commit; if a tag release failed after GoReleaser publish, leave GitHub release draft, delete/supersede contaminated assets, and revert/supersede tap path-changing commits reported by postcheck.

### Task 12: Final Docs, Harness Table, And Overclaim Proof

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
- Windows cross-build and packaged archive safety proof; Windows executable runtime remains deferred pending approved runner changes;
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

### Task 13: Release, Retro, And Closeout State

**Files:**
- Modify: `docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md`
- Create: `docs/retros/2026-07-03-ratchet-cli-tui-binary-verification-retro.md`
- Optional modify: `internal/version/version.go` only if repo uses source version bumps for releases

**Precondition:** PRs 1-5 have merged and `master` is green. This task runs on the PR6 closeout branch `docs/tui-verification-closeout`.

**Step 1: Run local verification**

```bash
go test ./internal/releaseguard -count=1
go test ./internal/harnessredact ./internal/releaseguard ./cmd/ratchet ./internal/tui -run 'Redact|ReleaseGuardRedaction|WorkflowCommandRedaction|HarnessDocs|TUIBinarySmoke' -count=1 -timeout=12m
go test ./internal/client ./internal/daemon ./cmd/ratchet ./internal/tui/commands ./internal/tui/components ./internal/tui -run 'Shutdown|StartupSmoke|ClientShutdown|CommandSurface|CLIHelp|Shortcut|Docs|SmokeService|ListJobs|JobPanel|TUIBinarySmoke' -count=1 -timeout=12m
go test -tags tui_smoke ./internal/client -run 'ConnectSmokeUnix' -count=1
go test -tags tui_smoke ./internal/daemon -run 'SmokeService|ListJobs' -count=1
go test ./internal/tui -run TestTUIBinarySmoke -count=1 -timeout=8m
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
GOOS=windows GOARCH=amd64 go build -o "$tmpdir/ratchet-windows-amd64.exe" ./cmd/ratchet
GOOS=windows GOARCH=arm64 go build -o "$tmpdir/ratchet-windows-arm64.exe" ./cmd/ratchet
go test -race ./... 
go vet ./...
goreleaser check
scripts/check-release-artifacts.sh
scripts/check-release-artifacts.sh --manifest-only dist
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/ci.yml .github/workflows/release.yml
```

Expected: PASS for all commands from current `master`; `-race` may skip smoke-specific PTY test with explicit race-disabled message; Windows binaries are written only under a unique temp directory; release wrapper regenerates fresh `dist` before manifest-only inspection.

**Step 2: PR and monitor**

Confirm PRs 1-5 were opened, monitored, and merged in manifest order. For each merged PR:
- ensure local focused tests pass before push;
- monitor CI until green;
- address code review with `autodev:receiving-code-review`;
- admin merge only after required GitHub checks are green and review requirements are satisfied.

**Step 3: Release**

After PR5 merges and `master` is green, tag the next semver patch/minor according to existing release history:

```bash
git fetch origin --tags
git checkout master
git pull --ff-only origin master
intended_sha="$(git rev-parse origin/master)"
test "$(git rev-parse HEAD)" = "$intended_sha"
test -z "$(git status --porcelain)"
git describe --tags --abbrev=0
git tag v<next>
git push origin v<next>
```

Expected: local `master` is fast-forwarded to the intended merged `origin/master` SHA, worktree is clean before the immutable tag is created, release workflow stays draft until postchecks pass, then publishes release; Homebrew tap Cask updates contain current version/checksum and no smoke tokens.

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

**Step 6: PR6 and merge closeout**

Open the PR6 closeout branch `docs/tui-verification-closeout` after the state commit, monitor required GitHub checks and review requirements, and admin merge PR6 only after required checks are green. After merge, verify `master` contains the retro and closeout plan update.

Expected: PR6 is merged green; no closeout state remains only on an unmerged branch.

Rollback: if release fails before undraft, leave draft private and fix assets/tap before publishing; if release publishes but tap postcheck fails, cut corrective patch release and path-specific tap corrective commit using reported SHA/path list.
