# TUI Binary Verification Design

**Status:** Draft
**Date:** 2026-07-03
**Project:** `GoCodeAlone/ratchet-cli`

## Goal

Close the ratchet-cli harness proof gap where `docs/harness-emulation.md` still
marks the full TUI as manual by adding credential-free binary execution proof
for interactive TUI launch, chat input, slash commands, and core shortcuts.

## User Intent

Latest continuing mandate: make ratchet-cli a functional, cross-platform agent
harness with verified TUI, shortcuts/slash commands, ACP/MCP surfaces, Windows
builds, and autonomous follow-through. Prior approval covers planning and
execution. This slice only turns an existing TUI claim into executable evidence.

## Global Design Guidance

Source: workspace `AGENTS.md`, repo `RATCHET.md`, `README.md`,
`docs/harness-emulation.md`, `docs/policy-matrix.md`,
`docs/competitor-parity.md`. No repo-local `AGENTS.md`, `CLAUDE.md`, or
`docs/design-guidance.md` exists.

| guidance | design response |
|---|---|
| Build for Windows. | Keep production code portable. PTY proof is Unix-only because it depends on pseudo-terminal behavior; add Windows compile proof and non-PTY CLI smoke so Windows remains covered honestly. |
| Prefer existing helpers and avoid duplicated plumbing. | Reuse daemon/client/TUI packages and built-in mock provider; do not add a parallel TUI runner or provider SDK. |
| Verify real boundaries, not demos. | Build the release-shaped `ratchet` binary for normal CLI/daemon/startup smoke, and build a dedicated non-release `ratchet-tui-smoke` binary for credential-free PTY proof of the same Bubble Tea TUI event loop. |
| Keep sensitive data local. | Use temp `HOME`/`XDG_STATE_HOME`; prompts are deterministic test strings; no credentials or real provider state. |
| Do not expand deferred policy work. | No managed hooks, TypeScript extension SDK, daemon background drain, local-first gateway, or self-mutation in this slice. |

## Current State

| surface | current proof | gap |
|---|---|---|
| TUI launch | `internal/tui` render/unit tests; `integration` PTY tests. | PTY tests require live Ollama/configured provider and are not CI-friendly. Docs say "full TUI remains manual." |
| Slash commands | parser/unit tests and in-process App tests. | No built-binary proof that submitted `/help`, `/provider list`, `/mode`, `/trust list`, `/tree`, and `/exit` render in the interactive TUI. |
| Shortcuts | in-process tests for `ctrl+b` session tree; render tests for input key handling. | No binary PTY proof for `ctrl+b`, `esc`, `ctrl+s`, `ctrl+j`, or `ctrl+c`/`ctrl+d`. |
| Provider path | daemon E2E harness has `e2e-mock`; binary smoke only checks version/help/daemon status. | Real binary TUI starts a background daemon from disk and may enter onboarding unless a provider exists. |
| Windows | GoReleaser emits Windows artifacts. | Interactive PTY cannot be proven with the current Unix PTY library; need compile/noninteractive proof only. |

## Approaches

| option | summary | trade-off | decision |
|---|---|---|---|
| A. Dedicated credential-free TUI smoke binary | Add `cmd/ratchet-tui-smoke` compiled only with `tui_smoke`. It starts a real in-process daemon with the mock provider, then runs the normal TUI event loop against it. The smoke binary is driven by PTY. | Strong proof without credentials and no hidden command/flag surface in `cmd/ratchet`. Docs must not overclaim release-binary chat proof. | Choose. |
| B. Seed production daemon state then launch normal `ratchet` | Use temp home and direct DB/config writes so the normal auto-daemon finds a mock provider. | Fewer hidden hooks, but brittle because tests depend on on-disk daemon internals and background process lifecycle. | Reject. |
| C. Keep in-process model tests only | Expand `internal/tui` unit tests for commands and shortcuts. | Portable and cheap, but does not satisfy binary execution proof. | Reject as insufficient. |
| D. Generated test-only smoke main | Write a tiny `main.go` under `t.TempDir()` and build it with `-tags tui_smoke`. | Avoids a persistent `cmd/ratchet-tui-smoke` package, but generated source is harder to review, cannot be discovered by normal repo tooling, and weakens docs-to-source traceability. | Reject in favor of explicit build-tagged package plus artifact guards. |

## Design

### Build-Tagged Smoke Binary

- Add `cmd/ratchet-tui-smoke/main.go` compiled only with
  `//go:build tui_smoke && !windows`; release workflows and normal
  `go build ./cmd/ratchet` do not build this package.
- PTY tests build `./cmd/ratchet-tui-smoke` with `-tags tui_smoke`; there is no
  conditional command, flag, or environment gate added to `cmd/ratchet`.
- Smoke path constructs a real daemon `Service` with temp local state and a
  default mock provider, starts gRPC on a temp Unix socket, builds a
  `client.Client` through a `tui_smoke`-only constructor against that socket,
  creates a session whose `WorkingDir` is an empty temp directory, and calls
  `tui.Run`.
- Helper contract:
  - no `testing.T` in non-test code;
  - explicit `context.Context` and `cleanup func()`;
  - listener bound to a temp Unix socket with `0600` permissions;
  - direct mock provider seeding is named smoke-only and never called from
    normal daemon startup;
  - `cmd.Dir`, session `WorkingDir`, `HOME`, and `XDG_STATE_HOME` are all
    temp directories with no instruction files or directories from
    `internal/agent/instructions.go`;
  - normal release binary has no path into this helper.
- Add negative assertions:
  - `go build ./cmd/ratchet` succeeds without `tui_smoke` and exposes no
    smoke command/flag/help text;
  - both `GOOS=windows GOARCH=amd64 go list -tags tui_smoke ./cmd/ratchet-tui-smoke`
    and `GOOS=windows GOARCH=arm64 go list -tags tui_smoke ./cmd/ratchet-tui-smoke`
    fail with no buildable Go files or an equivalent expected Unix-only
    package error;
  - both `GOOS=windows GOARCH=amd64 go build -tags tui_smoke ./cmd/ratchet-tui-smoke`
    and `GOOS=windows GOARCH=arm64 go build -tags tui_smoke ./cmd/ratchet-tui-smoke`
    fail with the same expected Unix-only package class;
  - `goreleaser check` passes and a snapshot release/archive inspection finds
    no archive member, checksum entry, Homebrew artifact, or release asset named
    `ratchet-tui-smoke`;
  - the GoReleaser fallback parser rejects unrecognized publishable top-level
    sections and fails unless all recognized publishable ids/binaries resolve to
    `ratchet`;
  - captured PTY frames/logs do not contain the developer workspace path or
    real home path.

### Smoke Client Contract

- Add `internal/client/client_tui_smoke.go` with
  `//go:build tui_smoke && !windows`.
- API:
  `func ConnectSmokeUnix(ctx context.Context, tempRoot, socketPath string) (*Client, error)`.
- Contract:
  - `tempRoot` and `socketPath` are converted to absolute clean paths;
  - symlink-aware containment is checked by resolving the existing temp root and
    socket parent with `filepath.EvalSymlinks`;
  - `socketPath` must be under `tempRoot`;
  - immediately before dialing, `Lstat(socketPath)` must reject symlink final
    components, require `ModeSocket`, and require permission bits `0600`;
  - the full resolved final socket path must remain under resolved `tempRoot`;
  - dial target is `unix://` only; no TCP target is accepted;
  - uses `grpc.WithTransportCredentials(insecure.NewCredentials())` because the
    socket is process-local, temp-owned, and permissioned `0600`;
  - returned `*Client` owns and closes the `grpc.ClientConn` via existing
    `Client.Close`;
  - file is absent from release builds, so no general arbitrary-target
    constructor is added to production client API.

### PTY Binary Smoke

- Add `internal/tui/tui_binary_smoke_unix_test.go` without the `integration`
  tag.
- Test builds `./cmd/ratchet-tui-smoke` with `-tags tui_smoke`, sets temp
  `HOME`, `XDG_STATE_HOME`, `TERM=xterm-256color`, and `cmd.Dir` to an empty
  temp workdir, then starts the smoke binary in a fixed-size PTY
  (`40x120`). This proves the TUI event loop, daemon RPCs, mock provider, slash
  commands, shortcuts, and representative rendered frames; it does not claim the
  untagged release binary can run credential-free chat without configured
  providers.
- Drive:
  - splash dismissal;
  - normal chat prompt with deterministic mock response;
  - `/help` renders "Available commands:";
  - `/provider list` renders `e2e-mock`;
  - documented mode/trust slash-command matrix listed below;
  - shortcut matrix listed below;
  - `/exit`, `ctrl+c`, and `ctrl+d` exit cleanly.
- Assertions strip ANSI and bound line width for representative frames.
- Frame assertions require header/status/input anchors to be simultaneously
  visible in normal chat and sidebar states; branch tree, team panel, and job
  panel states assert their panel-specific anchors plus status framing, then
  toggle/escape back to chat and assert the message input is visible and usable.
  Each representative frame must keep lines within the PTY width.
- Test has timeouts and cleanup that kills the child process if it hangs.
- Test includes an untagged repo-root discovery/build helper rather than
  reusing `internal/tui/pty_test.go`, which is behind the `integration` tag.
- All runtime/test failure payloads pass through one redaction helper before
  `t.Log`/`t.Fatalf`: PTY frames, stdout/stderr, build output, GoReleaser
  snapshot output, daemon cleanup output, docs-guard output, artifact-manifest
  output, and command errors. It redacts real home path, repo workspace path,
  temp roots, socket path, executable path, generated artifact paths, trust
  output body, and deterministic prompt frames down to bounded excerpts.
- Runtime PTY/TUI/daemon failure assertions reject output containing the repo
  workspace path, real home path, instruction surfaces derived from
  `internal/agent/instructions.go`, or hook config surfaces derived from
  `internal/hooks/hooks.go`.
- Release artifact manifests use an allowlist policy instead: `RATCHET.md` is
  expected because `.goreleaser.yaml` archives it, while `ratchet-tui-smoke`
  remains forbidden.

### Shortcut Matrix

The PTY smoke and focused view tests cover every implemented or advertised core
TUI shortcut. The source of truth is `internal/tui/app.go` key handling plus
`internal/tui/components/statusbar.go` hints.

| shortcut | expected evidence |
|---|---|
| `/tree` | Opens branch tree; `esc` returns to chat; frame keeps status/input anchors bounded. |
| `ctrl+b` | Opens branch tree; `esc` returns to chat; same frame checks as `/tree`. |
| `ctrl+s` | Toggles sidebar; chat input remains visible and usable. |
| `ctrl+j` | Opens job panel view; pressing `ctrl+j` again returns to chat with input visible and usable. |
| `ctrl+t` | Opens team panel view; pressing `ctrl+t` again returns to chat with input visible and usable. |
| `ctrl+c` | Exits cleanly. |
| `ctrl+d` | Exits cleanly. |

`ctrl+h` is currently advertised in the status bar but has no handler. The
implementation plan must either wire a real thinking-panel toggle and prove it,
or remove the stale status-bar hint and add a focused test that advertised
shortcuts match implemented shortcuts. No docs may claim `ctrl+h` proof unless
the handler exists.

### Mode And Trust Slash Command Matrix

The PTY smoke drives every publicly documented TUI mode/trust slash command
with temp-only patterns/scopes:

| command | expected PTY evidence |
|---|---|
| `/mode conservative` | Renders `Mode switched to conservative` or equivalent daemon mode output. |
| `/mode permissive` | Renders `Mode switched to permissive`. |
| `/mode locked` | Renders `Mode switched to locked`. |
| `/mode sandbox` | Renders `Mode switched to sandbox`. |
| `/mode custom` | Renders `Mode switched to custom`. |
| `/trust list` | Renders `Mode:` and effective rules without real workspace/home paths. |
| `/trust allow "bash:go test ./..." --scope smoke` | Renders `Added allow rule: bash:go test ./...`; a follow-up `/trust list` proves the `smoke` scope through daemon state. |
| `/trust deny "bash:rm -rf /" --scope smoke` | Renders `Added deny rule: bash:rm -rf /`; a follow-up `/trust list` proves the `smoke` scope through daemon state. |
| `/trust persist allow "bash:go vet ./..." --scope smoke` | Renders `Persisted allow grant: bash:go vet ./...`; a follow-up `/trust grants` proves pattern/action/scope through persisted daemon state. |
| `/trust persist deny "bash:curl *" --scope smoke` | Renders `Persisted deny grant: bash:curl *`; a follow-up `/trust grants` proves pattern/action/scope through persisted daemon state. |
| `/trust grants` | Renders persistent grants including smoke-scope entries; failure logs redact grant bodies. |
| `/trust revoke "bash:go vet ./..." --scope smoke` | Renders `Revoked persistent trust grant: bash:go vet ./...`; a follow-up `/trust grants` proves the revoked smoke grant is absent while unrelated grants remain. |
| `/trust reset` | Renders reset to config defaults. |

If public docs or `internal/tui/commands/trust.go` add a new TUI mode/trust
slash command or mode value, the docs guard fails until this matrix and PTY
smoke include it or the public docs mark it out of scope.

### Help And Autocomplete Alignment

- Update `/help` output so the documented command surface includes:
  - `/mode <mode>` with `conservative|permissive|locked|sandbox|custom`;
  - `/trust list`;
  - `/trust allow`;
  - `/trust deny`;
  - `/trust persist allow`;
  - `/trust persist deny`;
  - `/trust grants`;
  - `/trust revoke`;
  - `/trust reset`;
  - `/tree`.
- Update autocomplete command entries to include `/tree`, `/mode`, and
  `/trust`.
- Update public `ratchet help` / `printUsage` slash-command section to match
  the same mode/trust/tree slash surface, or remove the stale partial slash
  section entirely. If retained, binary smoke must assert the built CLI help
  includes the aligned entries.
- Add focused tests proving submitted-command support, `/help`, and
  autocomplete do not drift for the mode/trust/tree slash surface; add a
  built-binary help assertion for `ratchet help`.
- This aligns existing discoverability surfaces only; it does not add new
  commands or change command semantics.

### Normal Launch Smoke

- Keep the untagged built-binary smoke for `version`, `help`, and
  `daemon status`.
- Add a non-race CI smoke job in `.github/workflows/ci.yml` so binary/startup
  smoke tests that skip under `-race` are actually exercised:
  `go test ./cmd/ratchet ./internal/tui -run 'HarnessSmoke|TUIBinarySmoke|StartupSmoke' -count=1`.
- Required test names:
  - release-shaped startup/onboarding smoke:
    `TestHarnessSmokeReleaseStartupOnboardingBoundary`;
  - smoke TUI binary PTY proof: `TestTUIBinarySmoke`;
  - existing version/help/daemon smoke remains
    `TestHarnessSmokeVersionHelpAndDaemonStatus`.
- Add one untagged temp-home smoke for default `ratchet` startup only up to the
  expected provider setup/onboarding boundary, not chat:
  - build release-shaped binary without tags;
  - set temp `HOME`/`XDG_STATE_HOME`;
  - set `cmd.Dir` to an empty temp workdir;
  - launch in PTY;
  - assert splash/onboarding or provider-setup boundary appears;
  - exit cleanly;
  - run `ratchet daemon stop` with the same temp env if a temp pid/socket exists,
    or read the temp pid file and terminate/wait;
  - assert temp `.ratchet/daemon.pid` and `.ratchet/daemon.sock` are gone after
    cleanup.
- Docs distinguish:
  - release-shaped binary: startup/help/daemon/onboarding boundary;
  - `ratchet-tui-smoke` binary: credential-free interactive TUI
    chat/commands/shortcuts against mock daemon.

### Portable Compile And CLI Proof

- Keep existing `cmd/ratchet` binary smoke for `version`, `help`, and
  `daemon status`.
- Add Windows cross-build verification to the plan, not PTY execution:
  `GOOS=windows GOARCH=amd64 go build ./cmd/ratchet` and
  `GOOS=windows GOARCH=arm64 go build ./cmd/ratchet`.
- Add a `windows-latest` CI smoke job that builds `ratchet.exe` and executes
  safe non-PTY commands:
  - `ratchet.exe version`;
  - `ratchet.exe help`;
  - output must include `ratchet`, `Commands:`, and the aligned slash-command
    help entries, without starting the TUI or daemon.
  - configure `GOPRIVATE`, `GONOSUMCHECK`, and the same private-module Git
    rewrite used by existing CI jobs before `go build`.
- If Go test cannot run Unix PTY on Windows CI, tests remain build-tagged by
  filename suffix and docs state that Windows interactive proof is not claimed.
- The smoke package/client use `tui_smoke && !windows`; Windows verification
  explicitly asserts release-shaped `./cmd/ratchet` builds and smoke package
  `go list`/`go build` are unavailable on Windows amd64 and arm64.

### Release Artifact Guard

- Add `scripts/check-release-artifacts.sh` as the executable release preflight:
  - default mode runs `goreleaser check`, `goreleaser release --snapshot
    --clean --skip=publish`, then the artifact-manifest guard;
  - `--manifest-only dist` mode inspects an already-generated `dist/` tree.
- Wire a PR/push CI job named `release-check` in `.github/workflows/ci.yml`:
  - checkout with `fetch-depth: 0` and setup Go `1.26`;
  - configure private-module Git rewrite like the existing jobs;
  - run `goreleaser/goreleaser-action@v7` with `version: "~> v2"` and
    `args: check`;
  - run `goreleaser/goreleaser-action@v7` with `version: "~> v2"` and
    `args: release --snapshot --clean --skip=publish`, with no publish tokens
    beyond the normal `GITHUB_TOKEN`;
  - run `scripts/check-release-artifacts.sh --manifest-only dist`.
- Keep `.github/workflows/release.yml` using the tag-only publishing command
  `goreleaser release --clean`, but add a publish-free preflight before it:
  - set `GOPRIVATE` and `GONOSUMCHECK` to `github.com/GoCodeAlone/*`;
  - configure the same private-module Git rewrite before GoReleaser preflight
    and publish steps;
  - run `goreleaser/goreleaser-action@v7` with `version: "~> v2"` and
    `args: check`;
  - run `goreleaser/goreleaser-action@v7` with `version: "~> v2"` and
    `args: release --snapshot --clean --skip=publish`;
  - run `scripts/check-release-artifacts.sh --manifest-only dist`;
  - only then run the existing publishing `goreleaser release --clean` step.
- After the publishing step creates a draft GitHub release and before the
  existing publish/undraft script flips `draft: false`, inspect the draft
  release assets and uploaded checksum file for the same forbidden names. This
  postcheck proves the uploaded GitHub asset set did not diverge from the
  snapshot guard.
- Homebrew/tap limitation: the current GoReleaser release step can push
  `homebrew_casks.repository` changes before any postcheck can run. This slice
  prechecks cask/tap config and snapshot material before publishing, then
  performs an after-the-fact tap/cask reference check after publishing. If the
  tap contains forbidden smoke names, rollback is deleting/reverting the bad tap
  commit and cutting a corrected patch release. Fully pre-public tap blocking
  requires a future split-publish workflow and is out of scope for this slice.
- Local verification can run the script when GoReleaser is installed; ordinary
  `go test ./...` must not silently claim release-artifact coverage if
  GoReleaser is absent.
- Build a normalized manifest from `dist/`:
  - every file path under `dist/`;
  - `checksums.txt` entries;
  - archive member lists from each `.tar.gz` and `.zip`;
  - generated Homebrew cask/tap files if present;
  - GoReleaser metadata files if present.
- Fail closed when expected artifact classes are missing:
  - at least one Linux archive, one Darwin archive, one Windows archive, and
    `checksums.txt` must be present;
  - if `.goreleaser.yaml` has `homebrew_casks`, snapshot output must either
    contain generated cask/tap material or the guard must parse `.goreleaser.yaml`
    as deterministic fallback proof;
  - fallback proof parses `.goreleaser.yaml` and fails unless every build
    `id`, archive `ids` entry, `homebrew_casks[].ids` entry,
    `homebrew_casks[].binaries` entry, and release-publish build surface is
    exactly `ratchet`.
- The fallback parser is intentionally strict:
  - it parses `.goreleaser.yaml` top-level keys into an explicit taxonomy;
  - current nonpublishable metadata keys are `version` and `changelog`;
  - current artifact/publish keys are `builds`, `archives`, `checksum`,
    `homebrew_casks`, and `release`;
  - any unknown top-level key fails until classified;
  - any future artifact-producing or publishing section, including examples
    like `publishers`, `nfpms`, `sboms`, `dockers`, `brews`, `scoops`, `nix`,
    `aurs`, `winget`, or `signs`, must be added to the taxonomy with explicit
    id/binary/artifact assertions before the guard can pass.
- Assert the manifest contains `ratchet` and never contains
  `ratchet-tui-smoke`.
- Assert archive manifests may contain allowlisted `RATCHET.md` but no smoke
  binary/package names.
- Snapshot output and manifest failures are redacted through the shared helper.

### Documentation

- Update `docs/harness-emulation.md` TUI row from manual to automated Unix PTY
  coverage split into two rows/phrases:
  - `ratchet`: release-shaped startup/onboarding boundary, no credential-free
    chat claim;
  - `ratchet-tui-smoke`: non-release Unix PTY proof for interactive
    chat/slash/shortcut behavior.
- Update `README.md` harness table with the same split wording.
- Extend `cmd/ratchet/harness_docs_test.go` so docs must mention TUI binary
  smoke, slash commands, shortcuts, temp home/workdir, mock provider, Windows
  compile proof, and the fact that release-shaped `ratchet` proof does not
  claim credential-free chat.
- Make the docs guard bidirectional:
  - public docs scanned: `README.md`, `RATCHET.md`,
    `docs/harness-emulation.md`, `docs/competitor-parity.md`, and
    `docs/policy-matrix.md`;
  - positive assertions require the split `ratchet` startup/onboarding proof
    and `ratchet-tui-smoke` interactive proof;
  - negative assertions scan each Markdown line and each table row as the
    deterministic claim unit;
  - each claim unit is normalized by lowercasing, collapsing whitespace, and
    treating punctuation separators (`/`, `-`, `:`, `;`, comma) as spaces;
  - a claim unit fails only when it is an evidence claim: it contains an exact
    release-target token (`` `ratchet` ``, `ratchet` command, `release-shaped`,
    `release binary`, or `untagged`), an evidence token (`proof`, `proves`,
    `proven`, `covered`, `coverage`, `smoke`, `automated`, `automation`,
    `verified`, `binary smoke`, or `CI`), and an interactive-surface token
    (`credential free chat`, `interactive chat`, `interactive tui`, `full tui`,
    `slash command`, `slash commands`, `shortcut`, `shortcuts`, or
    `slash shortcut`);
  - valid product-support statements such as "ratchet-cli supports trust slash
    commands" are allowed unless they also claim automation/evidence for the
    release-shaped binary;
  - same-line/same-row exceptions must negate the same release-target evidence
    claim (`not claimed for ratchet`, `does not claim release-shaped`, or
    equivalent), or must assign the evidence claim to `ratchet-tui-smoke` rather
    than release-shaped `ratchet`;
  - the guard reports the normalized claim unit and original file/line through
    the redaction helper.

## Security Review

| risk | control |
|---|---|
| Smoke mode becomes a user-facing bypass. | Compile it only with `tui_smoke`; release binaries do not contain the path. |
| Test leaks real home/provider/project state. | Set temp `HOME`/`XDG_STATE_HOME`/`cmd.Dir`/session `WorkingDir`; temp workdir contains no instruction files/dirs from `internal/agent/instructions.go` and no hook configs from `internal/hooks/hooks.go` (`~/.ratchet/hooks.yaml`, `.ratchet/hooks.yaml`); assert captured output excludes real workspace/home paths. |
| PTY test hangs in CI. | Per-read deadline, process kill cleanup, bounded waits, and no external network/provider dependency. |
| Shortcut proof misses broken layout. | Use fixed PTY size and assert representative full frames for chat/sidebar input-visible states and branch-tree/team/job panel states, then return to chat and assert input usability. |
| Shortcut hint drifts from handlers. | Treat `internal/tui/app.go` key handling plus `internal/tui/components/statusbar.go` hints as a checked contract; prove `ctrl+b`, `esc`, `ctrl+s`, `ctrl+j`, `ctrl+t`, `ctrl+c`, and `ctrl+d`, and fix/remove stale `ctrl+h`. |
| Sensitive prompts/log paths in logs. | Use harmless deterministic prompts and route every test failure payload through one redaction helper that removes real home/workspace/temp/socket/executable/artifact paths plus trust/prompt bodies; runtime outputs reject instruction surfaces from `internal/agent/instructions.go` and hook config surfaces from `internal/hooks/hooks.go`, release manifests allowlist expected `RATCHET.md`. |
| Release-shaped startup leaks daemon process. | Cleanup stops or kills the temp-home daemon and asserts pid/socket files are gone. |
| Binary smoke skipped under race. | Add non-race CI smoke job for `cmd/ratchet` and `internal/tui` smoke tests alongside the existing race suite. |
| Release artifact guard only runs manually. | Add the publish-free `release-check` CI job; CI uses GoReleaser action for snapshot generation and the same manifest guard script local runs use. |
| Tag release bypasses artifact guard. | Add the same publish-free `goreleaser check` + snapshot + manifest guard to `.github/workflows/release.yml` before the publishing step. |
| Release jobs cannot fetch private modules. | Mirror CI's `GOPRIVATE`/`GONOSUMCHECK` and private-module Git rewrite in release preflight/publish and Windows safe-command smoke jobs. |
| Snapshot guard differs from uploaded release. | Add a draft-release postcheck before undrafting that inspects uploaded GitHub assets/checksums for forbidden smoke names; explicitly treat Homebrew/tap postcheck as after-the-fact rollback under the current GoReleaser workflow. |
| Platform mismatch. | Unix PTY proof is explicitly Unix-only; Windows claim is limited to build/noninteractive smoke. |

## Infrastructure Impact

No cloud resources, secrets, migrations, registry entries, Homebrew publishing,
or production deploy. Infrastructure changes are limited to publish-free
GoReleaser preflight jobs on PR/push and before tag publishing, a draft-release
GitHub asset postcheck before undrafting, an after-the-fact Homebrew/tap
postcheck with rollback instructions, plus a `windows-latest` safe-command
smoke job. These workflow paths must mirror existing private-module environment
and Git rewrite setup. Local test-only process and temp files only. Runtime
behavior does not change in release builds because the smoke entrypoint is
build-tagged out.

## Multi-Component Validation

| boundary | proof |
|---|---|
| Release-shaped built binary to startup | Untagged PTY or subprocess smoke launches compiled `ratchet` with temp home/workdir, reaches help/daemon/onboarding boundary, then stops the temp daemon and verifies pid/socket cleanup. |
| Smoke built binary to TUI | `-tags tui_smoke` PTY test launches compiled `ratchet-tui-smoke` and reads rendered frames. |
| TUI to daemon gRPC | Smoke service uses real daemon service/client RPCs for provider list, session tree, trust mode/rules, and chat send; docs do not claim auto-daemon socket proof from this row. |
| TUI to mock provider | Chat prompt reaches built-in mock provider and streams a response. |
| Slash commands | PTY submits slash commands through the input widget and asserts rendered system output/navigation, including the full documented TUI mode/trust slash-command matrix. |
| Slash discoverability | Focused tests keep in-TUI `/help`, autocomplete, and public `ratchet help` aligned with submitted `/tree`, `/mode`, and `/trust` support. |
| Shortcuts | PTY sends control keys and asserts branch tree/pane transitions with fixed-size full-frame checks matching current behavior; focused tests reject advertised-but-unimplemented shortcuts. |
| Docs to tests | Docs guard fails if automated TUI smoke evidence is removed. |
| Binary smoke to CI | Non-race CI smoke job runs the built-binary/startup/TUI smoke tests that the race suite skips. |
| Release preflight to CI/release | `release-check` and tag release workflow both run `goreleaser check`, snapshot generation, and the same artifact-manifest guard before publish; release postcheck inspects uploaded GitHub draft assets before undrafting; Homebrew/tap postcheck is after-the-fact with rollback. |
| Windows build/smoke | Cross-build commands prove release-shaped `ratchet` still compiles for Windows; `windows-latest` executes `ratchet.exe version` and `ratchet.exe help`; `ratchet-tui-smoke` interactive PTY remains Unix-only. |

## Integration Matrix

| integration | classification | proof |
|---|---|---|
| Release-shaped `ratchet` binary | runtime-integrated | Existing and expanded smoke builds without tags and runs `version`, `help`, `daemon status`, plus startup/onboarding boundary. |
| `ratchet-tui-smoke` binary | runtime-integrated | Unix PTY test builds `./cmd/ratchet-tui-smoke` with `-tags tui_smoke`, launches binary, and drives TUI. |
| TUI Bubble Tea event loop | runtime-integrated | PTY frames prove splash, chat prompt, transcript, navigation, and exit. |
| Daemon gRPC service/client | runtime-integrated | Smoke service/client RPCs over a temp Unix socket execute provider list, trust commands, session tree, and chat send. |
| Built-in mock provider | runtime-integrated | Chat prompt streams deterministic mock response. |
| Slash commands and shortcuts | runtime-integrated | PTY sends input/control keys and asserts resulting UI states, including the full documented TUI mode/trust slash-command matrix, every advertised core shortcut, and fixed-size representative frames. |
| Slash help/autocomplete/CLI help | runtime-integrated | Focused tests invoke in-TUI `/help`, autocomplete models, and built `ratchet help` to prove submitted command support and discoverability stay aligned. |
| Non-race binary smoke CI | config-only | `.github/workflows/ci.yml` runs a focused non-race smoke job for subprocess/startup tests that are intentionally skipped under `-race`, with explicit test names for release startup and TUI binary smoke. |
| Docs guard | config-only | `cmd/ratchet/harness_docs_test.go` checks public docs mention exact evidence boundaries. |
| GoReleaser snapshot, GitHub draft assets, and Homebrew/tap | runtime-integrated | Publish-free `release-check` CI job, tag release preflight, local script, and release draft postcheck run `goreleaser check`/snapshot generation plus archive/upload inspection with deterministic `.goreleaser.yaml` fallback to prove `ratchet-tui-smoke` is absent from GitHub archives/checksums before undraft. Homebrew/tap is prechecked by snapshot/config and postchecked after publish, with rollback if the tap diverges. |
| Windows safe-command smoke | runtime-integrated | `windows-latest` builds `ratchet.exe` and runs `version` and `help` to prove non-PTY Windows CLI startup/help behavior. |
| Windows smoke package boundary | config-only | `GOOS=windows GOARCH=amd64/arm64 go list` and `go build -tags tui_smoke ./cmd/ratchet-tui-smoke` fail with the expected Unix-only no-buildable-files class. |
| Windows interactive PTY | deferred | No ConPTY runner in this slice; Windows coverage is build/noninteractive only. |

## Rollback

Revert `cmd/ratchet-tui-smoke`, the `tui_smoke` client constructor, smoke
helper, PTY test, help/autocomplete/CLI-help alignment, shortcut hint fix,
non-race smoke CI job, Windows safe-command smoke job, `release-check` CI job,
release-workflow preflight/postcheck, release-artifact script, and docs updates.
No data migration exists. Release binaries are unaffected because `cmd/ratchet`
does not contain a smoke command/flag. If a future GitHub release accidentally
includes `ratchet-tui-smoke`, keep the release draft, delete the bad
artifact/checksum where possible, fix the release config, and rerun/cut a patch
release. If a Homebrew tap commit includes a bad smoke reference, revert or
delete that tap commit/reference and cut a corrected patch release. If the CI
or release preflight/postcheck itself is bad, revert that job/script while
keeping the existing tag-only release workflow's publish step, then cut a
corrected preflight PR.

## Assumptions

| id | assumption | challenge | fallback |
|---|---|---|---|
| A1 | A dedicated build-tagged smoke binary is acceptable evidence for credential-free TUI proof. | It is not byte-for-byte the release binary. | Keep release-shaped startup smoke separate and document the boundary precisely. |
| A2 | Built-in mock provider response is stable enough for PTY assertions. | Mock wording may change. | Assert broad substrings plus stream completion markers, not exact full transcript. |
| A3 | Unix PTY proof plus Windows compile proof is sufficient cross-platform honesty. | User may require Windows interactive proof. | Add a separate Windows ConPTY design if a Windows runner is available. |
| A4 | Extracting daemon mock service construction is lower risk than seeding production DB state. | Helper extraction may touch daemon internals. | Keep helper private/internal and covered by existing daemon tests. |

## Self-Challenge

1. Laziest solution: update docs to cite existing `integration` PTY tests.
   Rejected because those require Ollama and do not close the CI-proof gap.
2. Fragile assumption: A1. Mitigation is separating release-shaped startup
   proof from build-tagged interactive proof.
3. YAGNI sweep: no visual snapshot framework, no new TUI features, no Windows
   ConPTY, no provider setup automation, no external agent CI.
4. First failure mode: PTY hang or off-screen pane after a shortcut. Design
   requires bounded waits, cleanup kill, deterministic mock daemon, fixed PTY
   size, and representative frame assertions.
5. Repo fit: follows existing PTY helpers, `cmd/ratchet` binary smoke shape,
   daemon mock-provider harness, and docs guard tests.

## Out Of Scope

- ACPX TypeScript flow runtime.
- Managed hooks or broad extension SDK.
- Daemon background queue drain.
- Local-first gateway/channels.
- Credentialed third-party agent CI.
- Windows interactive ConPTY proof.
- New user-facing TUI commands or changed default runtime behavior; help and
  autocomplete alignment for existing commands is in scope.

## Review Resolutions

| finding | resolution |
|---|---|
| D1 | Replaced hidden env-gated release-binary path with a dedicated `cmd/ratchet-tui-smoke` package behind `//go:build tui_smoke && !windows`; release builds do not contain the smoke entrypoint. |
| D2 | Split validation/docs claims into release-shaped startup smoke and build-tagged credential-free interactive TUI event-loop smoke. |
| D3 | Added explicit smoke helper contract: no `testing.T`, explicit context/cleanup, local-only listener, smoke-only mock seeding, unreachable from normal startup. |
| D4 | Added real-state isolation and redacted bounded failure-output requirements. |
| D5 | Added declared integration matrix with `runtime-integrated`, `config-only`, and `deferred` classifications. |
| D6 | Public docs must split release-shaped `ratchet` startup/onboarding proof from non-release `ratchet-tui-smoke` interactive proof, and docs guard must reject credential-free release chat wording. |
| D7 | Smoke binary must run with temp `HOME`, `XDG_STATE_HOME`, `cmd.Dir`, and session `WorkingDir`; daemon listens on a temp Unix socket only; output must exclude real workspace/home paths. |
| D8 | Added exact `tui_smoke`-only `client.ConnectSmokeUnix(ctx, tempRoot, socketPath)` contract; no general arbitrary-target constructor ships in release builds. |
| D9 | Added release-shaped guard that no smoke command/flag/help text is present and release artifacts do not include `ratchet-tui-smoke`. |
| D10 | Updated smoke client API to `ConnectSmokeUnix(ctx, tempRoot, socketPath)` and required absolute, symlink-aware containment plus socket mode validation. |
| D11 | Release-shaped startup smoke must stop/kill the temp-home daemon and assert pid/socket cleanup. |
| D12 | Release guard now requires `goreleaser check` plus snapshot/archive inspection for archives, checksums, Homebrew artifacts, and release assets. |
| D13 | Added untagged repo-root/build helper requirement for the non-integration PTY test. |
| D14 | Added single redaction helper for every test failure payload, covering real home/workspace/temp/socket/executable/artifact paths and prompt/trust bodies. |
| D15 | Added final socket `Lstat`, no-symlink, `ModeSocket`, `0600`, full resolved containment, and repeat-before-dial requirements. |
| D16 | Smoke command/client use `tui_smoke && !windows`; Windows smoke package has an expected negative `go list` check. |
| D17 | Release guard now defines exact snapshot command, `dist/` manifest contents, expected artifact classes, and fail-closed behavior for missing artifacts. |
| D18 | Redaction helper now covers every test failure payload, including build, GoReleaser, daemon cleanup, docs guard, artifact manifest, PTY, stdout/stderr, and command errors. |
| D19 | Added and rejected generated test-only smoke main alternative in the approaches table. |
| D20 | Docs guard now has positive split-proof checks and forbidden-phrase assertions against release-shaped credential-free/full-interactive `ratchet` claims. |
| D21 | Release guard now parses `.goreleaser.yaml` as deterministic fallback for snapshot-skipped cask/release surfaces and fails unless publishable ids/binaries are exactly `ratchet`. |
| D22 | Runtime output rejects instruction filenames; release artifact manifests use allowlist semantics and permit expected `RATCHET.md`. |
| D23 | Windows negative checks now cover amd64 and arm64 for both `go list` and `go build -tags tui_smoke ./cmd/ratchet-tui-smoke`. |
| D24 | Windows smoke package boundary is classified as `config-only` negative build-boundary proof, not runtime-integrated. |
| D25 | Added full documented TUI trust slash-command matrix to the PTY smoke scope instead of claiming only selected slash-command proof. |
| D26 | Docs guard now names scanned public docs and concrete forbidden line regexes for release-shaped credential-free/full-interactive/slash-shortcut claims. |
| D27 | Runtime leak assertions derive instruction surfaces from `internal/agent/instructions.go`, including global/project/model-specific files and instruction directories. |
| D28 | GoReleaser fallback parser is strict and fails on unrecognized publishable sections until allowed ids/binaries are explicitly checked. |
| D29 | Expanded the matrix from selected `/mode` values plus all `/trust` subcommands to every documented `/mode` value (`conservative`, `permissive`, `locked`, `sandbox`, `custom`) and tied drift checks to `internal/tui/commands/trust.go`. |
| D30 | Replaced phrase-fragile forbidden docs regexes with normalized line/table-row claim predicates and deterministic same-unit exceptions for `ratchet-tui-smoke`, `not claimed`, and `does not claim`. |
| D31 | Replaced ambiguous "publishable section" language with an explicit GoReleaser top-level taxonomy: current nonpublishable metadata, current artifact/publish keys, fail-unknown behavior, and examples of future artifact/publish sections that require explicit checks. |
| D32 | Split instruction and hook leak handling: instruction deny patterns derive from `internal/agent/instructions.go`; hook config deny patterns derive from `internal/hooks/hooks.go`. |
| D33 | Changed trust mutation evidence to match existing command output and require follow-up `/trust list` or `/trust grants` state-read assertions for scope/persistence/revoke proof. |
| D34 | Added fixed PTY sizing and representative full-frame assertions for chat, branch tree, sidebar, and job-panel states after shortcuts. |
| D35 | Added a publish-free release preflight: local `scripts/check-release-artifacts.sh` runs GoReleaser plus manifest guard, while CI uses `goreleaser/goreleaser-action@v7` for snapshot generation and the same script in `--manifest-only` mode. |
| D36 | Brought `/help` and autocomplete alignment into scope for existing `/tree`, `/mode`, and `/trust` slash surfaces, with focused drift tests. |
| D37 | Added shortcut source-of-truth matrix covering `/tree`, `ctrl+b`, `esc`, `ctrl+s`, `ctrl+j`, `ctrl+t`, `ctrl+c`, and `ctrl+d`; stale advertised `ctrl+h` must be implemented or removed with tests. |
| D38 | Added non-race CI smoke job for subprocess/startup/TUI smoke tests skipped by the race suite. |
| D39 | Added public `ratchet help` / `printUsage` to the slash discoverability contract and built-binary help assertions. |
| D40 | Added explicit `goreleaser check` steps to CI and release preflight paths and pinned the GoReleaser action to `~> v2`. |
| D41 | Added publish-free snapshot artifact guard to `.github/workflows/release.yml` before the publishing GoReleaser step so tag releases cannot bypass the manifest guard. |
| D42 | Added a `windows-latest` safe-command smoke job that builds `ratchet.exe` and runs `version` and `help`; Windows interactive PTY remains deferred. |
| D43 | Mirrored the release workflow's GoReleaser `version: "~> v2"` in preflight action steps. |
| D44 | Changed `ctrl+j`/`ctrl+t` evidence to match current full-panel behavior: panel opens, repeated shortcut returns to chat/input; no hidden layout redesign. |
| D45 | Narrowed docs negative guard to evidence claims only and tightened exceptions so product support statements are allowed and generic `not claimed` cannot mask contradictory release-binary proof claims. |
| D46 | Required `GOPRIVATE`/`GONOSUMCHECK` plus private-module Git rewrite in release preflight/publish and Windows safe-command smoke jobs. |
| D47 | Added draft-release postcheck before undrafting so uploaded assets/checksums/tap references are inspected, not only snapshot artifacts. |
| D48 | Reframed Homebrew/tap safety honestly: snapshot/config precheck plus after-the-fact tap postcheck/rollback under the current GoReleaser workflow; full pre-public tap blocking is deferred to a split-publish workflow. |
| D49 | Required `fetch-depth: 0` for GoReleaser preflight checkout so PR/push preflight has release-equivalent git tag/changelog state. |
| D50 | Broadened non-race smoke CI regex to include `StartupSmoke` and named the required startup/TUI smoke tests explicitly. |
