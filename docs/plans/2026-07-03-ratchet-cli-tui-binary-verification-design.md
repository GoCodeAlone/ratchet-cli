# TUI Binary Verification Design

**Status:** Draft
**Date:** 2026-07-03
**Project:** `GoCodeAlone/ratchet-cli`

## Goal

Close the ratchet-cli harness proof gap where `docs/harness-emulation.md` still
marks the full TUI as manual by adding credential-free binary execution proof
for interactive TUI launch, chat input, PTY-proven shortcuts, focused shortcut
coverage, and the selected PTY-proven slash-command matrix.

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
- PTY tests build `./cmd/ratchet-tui-smoke` with `-tags tui_smoke -o
  <tempdir>/ratchet-tui-smoke`; there is no conditional command, flag, or
  environment gate added to `cmd/ratchet`.
- Smoke path constructs a daemon `Service` with temp local state and a temp Unix
  socket through a smoke-only service option/constructor that reuses core
  DB/provider/session/trust/chat wiring but disables unrelated host-dependent
  subsystems: MCP CLI discovery, external plugin loading/daemon tools,
  autoresponder loading, cron/background work, and any scan of host `PATH` or
  user plugin directories. It builds a `client.Client` through a
  `tui_smoke`-only constructor against that socket, calls the production
  `AddProvider` RPC to register keyless default `mock` provider `e2e-mock`,
  creates a session whose `WorkingDir` is an empty temp directory through the
  daemon/client boundary, and calls `tui.Run`.
- Helper contract:
  - no `testing.T` in non-test code;
  - explicit `context.Context` and `cleanup func()`;
  - listener bound to a temp Unix socket with `0600` permissions;
  - smoke service options must be explicit fail-closed booleans, not implicit
    "best effort" environment behavior; default production daemon construction
    remains unchanged;
  - focused tests assert MCP discovery, plugin loading, daemon tool/plugin
    registration, autoresponder file loading, and unrelated background workers
    are not started in smoke mode, and captured logs/frames contain no host
    `PATH`, real plugin directory, or real home/plugin path;
  - mock provider setup goes through the real daemon `AddProvider` RPC; no
    smoke-only direct DB seeding path is added;
  - `cmd.Dir`, session `WorkingDir`, `HOME`, and `XDG_STATE_HOME` are all
    temp directories with no instruction files or directories from
    `internal/agent/instructions.go`;
  - normal release binary has no path into this helper.
- Add negative assertions:
  - `go build -o <tempdir>/ratchet ./cmd/ratchet` succeeds without
    `tui_smoke` and exposes no smoke command/flag/help text;
  - on Unix hosts, no-tag `go list ./cmd/ratchet-tui-smoke` and
    `go build ./cmd/ratchet-tui-smoke` fail with no buildable Go files or an
    equivalent build-constraint class;
  - on Unix hosts, `go build -tags tui_smoke -o
    <tempdir>/ratchet-tui-smoke ./cmd/ratchet-tui-smoke` succeeds to prove the
    smoke package is intentionally buildable only under the tag;
  - source guard uses a test-owned typed smoke-source manifest with rows
    `{path, buildConstraint, exportedSymbols, exactTokens}` for every
    persistent non-test smoke-only Go file, initially
    `cmd/ratchet-tui-smoke/main.go` and
    `internal/client/client_tui_smoke.go`, plus the smoke-only daemon helper
    file that exposes the explicit smoke service option/constructor, e.g.
    `internal/daemon/service_tui_smoke.go`;
  - each manifest row must exist, must contain the exact build constraint
    `//go:build tui_smoke && !windows`, and must contain only the declared
    smoke-only exported symbols/tokens for that file;
  - the guard scans every non-test `.go` file outside the manifest and fails
    only on exact smoke-surface tokens: the build tag token `tui_smoke`, the
    package/path token `ratchet-tui-smoke`, manifest exported symbol names such
    as `ConnectSmokeUnix`, or any additional exact source token declared in the
    manifest row;
  - verification tooling gets a separate exact-token allowlist, not a smoke
    runtime-file exemption: `internal/releaseguard` may contain declared
    forbidden-token constants such as `ratchet-tui-smoke` and `tui_smoke` only
    for artifact scanning, and the source guard fails if those tooling files
    contain smoke build tags, smoke exported constructors, or call paths into
    smoke runtime helpers;
  - the guard does not fail on broad pathname-only `smoke` matches in non-test
    files; existing `*_smoke_test.go` files are explicitly allowlisted as
    test-only precedent and are not part of the non-test source leak scan;
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

- Add `internal/tui/tui_binary_smoke_test.go` without the `integration` tag
  and with explicit `//go:build !windows`. Do not rely on a `_unix_test.go`
  filename suffix; Go has no `unix` GOOS filename suffix.
- Add package-local `internal/tui/race_enabled_test.go` and
  `internal/tui/race_disabled_test.go`, mirroring `cmd/ratchet`, so
  `TestTUIBinarySmoke` can skip under `-race` while the focused non-race CI job
  still executes it.
- Test builds `./cmd/ratchet-tui-smoke` with `-tags tui_smoke`, sets temp
  `HOME`, `XDG_STATE_HOME`, `TERM=xterm-256color`, and `cmd.Dir` to an empty
  temp workdir, then starts the smoke binary in a fixed-size PTY
  (`40x120`). This proves the TUI event loop, daemon RPCs, mock provider, slash
  representative slash commands, shortcuts, and representative rendered frames;
  it does not claim the untagged release binary can run credential-free chat
  without configured providers, and it does not claim PTY runtime coverage for
  every slash command in `internal/tui/commands/commands.go`.
- Drive:
  - splash dismissal;
  - normal chat prompt with deterministic mock response;
  - `/help` renders "Available commands:";
  - `/provider list` renders `e2e-mock`;
  - documented mode/trust slash-command matrix listed below;
  - shortcut matrix listed below;
  - one long interaction run exits with `/exit`.
- Add separate short PTY subprocess subtests for `ctrl+c` and `ctrl+d` so every
  terminal exit mechanism is proven independently; one process cannot prove
  more than one exit path after it terminates.
- Assertions strip ANSI and bound display-cell width for representative frames
  with `lipgloss.Width` or `runewidth`, matching the TUI rendering width model
  rather than byte or rune count.
- Frame assertions require header/status/input anchors to be simultaneously
  visible in normal chat and sidebar states; branch tree, team panel, and job
  panel states assert their panel-specific anchors plus status framing, then
  toggle/escape back to chat and assert the message input is visible and usable.
  Each representative frame must keep lines within the PTY width.
- Test has timeouts and cleanup that kills the child process if it hangs; if
  `raceEnabled` is true, it calls `t.Skip` with a message that names the
  focused non-race smoke CI job as the execution path.
- Test includes an untagged repo-root discovery/build helper rather than
  reusing `internal/tui/pty_test.go`, which is behind the `integration` tag.
- Test uses a new untagged PTY capture helper with synchronized output
  snapshots (mutex/channel/single-reader API). It must not copy the existing
  integration helper's unsynchronized `bytes.Buffer` read/write pattern.
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

The PTY smoke and focused view tests cover every implemented or advertised TUI
shortcut without collapsing the evidence class. The source of truth is
`internal/tui/app.go` key handling,
`internal/tui/pages/chat.go` page-level key handling,
`internal/tui/components/sessiontree.go` branch-tree key handling, README TUI
navigation docs, and `internal/tui/components/statusbar.go` hints.

| shortcut | proof class | expected evidence |
|---|---|---|
| `/tree` | `pty-proven` | Opens branch tree; `esc` returns to chat; frame keeps status/input anchors bounded. |
| `ctrl+b` | `pty-proven` | Opens branch tree; `esc` returns to chat; same frame checks as `/tree`. |
| `ctrl+s` | `pty-proven` | Toggles sidebar; chat input remains visible and usable. |
| `ctrl+j` | `pty-proven` | Opens job panel view; pressing `ctrl+j` again returns to chat with input visible and usable. |
| `esc` in job panel | `pty-proven` | Closes job panel, matching the advertised `Esc: close` hint, and returns to chat with input visible and usable. |
| `ctrl+t` | `pty-proven` | Opens team panel view; pressing `ctrl+t` again returns to chat with input visible and usable. |
| `ctrl+h` | `focused-proven` | Focused `ChatModel` test seeds thinking-panel content and proves `ctrl+h` toggles collapse/expand; if no thinking content exists, it is a no-op and no docs claim PTY proof. |
| `ctrl+c` | `pty-proven` | Exits cleanly. |
| `ctrl+d` | `pty-proven` | Exits cleanly. |

Branch-tree navigation proof uses focused component/page tests with a fixture
tree containing at least one root and one child branch:

| branch-tree key | expected evidence |
|---|---|
| `j` / `down` | Moves selection down and loads preview for the new selected session. |
| `k` / `up` | Moves selection up and loads preview for the new selected session. |
| `h` / `left` | Collapses an expanded branch; hidden child cannot be selected. |
| `l` / `right` | Expands a collapsed branch; child becomes visible/selectable. |
| `Enter` | Emits branch-switch command for the selected session. |
| `r` | Reloads the session tree through the page client. |
| `Esc` | PTY/app proof returns from tree page to chat with input visible. |

Add one App-level branch-switch proof above the component/page tests:

- open the branch tree with a fixture containing root and child sessions;
- select the child and press `Enter`;
- assert the App consumes `SessionTreeSelectedMsg`, updates the selected
  session, transitions back to chat, and rebuilds chat history for the child;
- wait for the history reload completion path, not only command emission;
- submit a deterministic chat prompt after the switch and assert it is sent
  against the selected child session.

Docs may claim PTY proof only for shortcut rows classified `pty-proven`.
Conditional `ctrl+h`, detailed branch-tree navigation (`j`/`k`, arrows, `h`/`l`,
`Enter`, `r`), and App-level child-session switching are focused/App-level
proof in this slice and must not be described as PTY-proven.

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
| `/trust allow "smoke:allow-unit-tests" --scope smoke` | Renders `Added allow rule: smoke:allow-unit-tests`; a follow-up `/trust list` proves the `smoke` scope through daemon state. |
| `/trust deny "smoke:deny-dangerous-command" --scope smoke` | Renders `Added deny rule: smoke:deny-dangerous-command`; a follow-up `/trust list` proves the `smoke` scope through daemon state. |
| `/trust persist allow "smoke:persist-allow-vet" --scope smoke` | Renders `Persisted allow grant: smoke:persist-allow-vet`; a follow-up `/trust grants` proves pattern/action/scope through persisted daemon state. |
| `/trust persist deny "smoke:persist-deny-network" --scope smoke` | Renders `Persisted deny grant: smoke:persist-deny-network`; a follow-up `/trust grants` proves pattern/action/scope through persisted daemon state. |
| `/trust grants` | Renders persistent grants including smoke-scope entries; failure logs redact grant bodies. |
| `/trust revoke "smoke:persist-allow-vet" --scope smoke` | Renders `Revoked persistent trust grant: smoke:persist-allow-vet`; a follow-up `/trust grants` proves the revoked smoke grant is absent while unrelated grants remain. |
| `/trust reset` | Renders reset to config defaults; a follow-up `/trust list` proves `Mode: conservative` matches the smoke config default after prior `/mode` mutations, runtime smoke allow/deny rules are gone, and effective rules are rebuilt from config defaults; a follow-up `/trust grants` proves persisted smoke grants that were not explicitly revoked remain unchanged. |

If public docs or `internal/tui/commands/trust.go` add a new TUI mode/trust
slash command or mode value, the docs guard fails until this matrix and PTY
smoke include it or the public docs mark it out of scope.

### TUI Slash Command Coverage Contract

The design intentionally proves selected representative slash commands through
PTY and keeps all other TUI commands honest through classification and focused
parse/help/autocomplete tests. Public docs and harness docs may claim automated
PTY proof only for the `pty-proven` rows below.

Source of truth: `internal/tui/commands/commands.go` (`Parse`, `helpCmd`) plus
autocomplete command models.

| command family | classification | required evidence |
|---|---|---|
| `/help` | `pty-proven` | PTY submits command and sees `Available commands:`. |
| `/provider list` | `pty-proven` | PTY submits command and sees `e2e-mock`; other `/provider` subcommands are classified separately. |
| `/tree` | `pty-proven` | PTY opens branch tree and returns to chat; App-level branch-switch proof covers `Enter`. |
| `/mode <all documented modes>` | `pty-proven` | PTY matrix covers every documented mode value. |
| `/trust <documented subcommands>` | `pty-proven` | PTY matrix covers list, allow, deny, persist allow, persist deny, grants, revoke, and reset. |
| `/exit` | `pty-proven` | PTY exits cleanly. |
| `/clear`, `/plan` | `focused-proven` | Focused command tests assert parse result/help text only; no PTY runtime claim. |
| `/model`, `/cost`, `/agents`, `/sessions`, `/compact`, `/review`, `/jobs`, `/login` | `focused-proven` | Focused command tests with fake/stub clients assert parse behavior, help/autocomplete presence, and no docs overclaim of PTY proof. |
| `/provider add`, `/provider remove`, `/provider default`, `/provider test` | `focused-proven` | Focused command tests assert usage/dispatch behavior; provider setup wizard/runtime side effects are not PTY-proven in this slice. |
| `/loop`, `/cron`, `/fleet`, `/team`, `/approve`, `/reject` | `deferred-runtime` | Parse/help/autocomplete presence is checked where the current code exposes it; full runtime orchestration is out of scope for this TUI binary-proof slice and must not be documented as PTY-proven here. |
| `/mcp` | `deferred-runtime` | Parse and autocomplete entries are checked; `/help` is allowed to omit `/mcp` until product docs choose to expose it. Full runtime orchestration is out of scope and must not be documented as PTY-proven here. |

The docs guard scans public harness evidence claims and fails if a
`focused-proven` or `deferred-runtime` row is described as PTY/binary-smoke
runtime proof. If `commands.Parse` or `helpCmd` gains a new slash command, the
focused coverage table/test must classify it before CI passes.

Mechanical fail-closed check:

- Add `internal/tui/commands/command_surface_test.go` for TUI parser/help and
  autocomplete surfaces, plus a `cmd/ratchet` CLI-help guard test for
  `printUsage`/built `ratchet help`.
- Put the shared test-owned typed command-spec table in a test helper package or
  internal test fixture importable by both packages; do not make production
  dispatch depend on it.
- Use the test-owned typed command-spec table, not production dispatch metadata.
  Each row contains: top-level command, proof class (`pty-proven`,
  `focused-proven`, `deferred-runtime`), whether autocomplete should list it,
  required in-TUI help surface, required public CLI help surface, and optional
  subcommands/modes. Examples are a separate field and are never treated as
  commands.
- Use Go `parser`/`ast` to extract only top-level string literal `case` labels
  from the `switch cmd` in `Parse`; fail if a guarded command switch contains a
  nonliteral command-like case unless the typed spec marks that surface as
  generated and a focused runtime-output test covers it.
- Use separate extractors for help and autocomplete:
  - help extractor reads command tokens only from the leading command portion of
    `helpCmd` output lines; examples inside descriptions are ignored; fail if
    help rows are computed/generated in a way the extractor cannot enumerate
    unless the typed spec explicitly marks the generated surface and tests the
    runtime help output;
  - autocomplete extractor reads `CommandEntry{Name: "/..."}` literals only;
    fail on nonliteral/computed autocomplete command entries unless the typed
    spec explicitly marks the generated surface and tests the runtime
    autocomplete output;
  - the public CLI help extractor lives under `cmd/ratchet` so it can capture
    unexported `printUsage` directly and also run the built `ratchet help`;
    extracted leading slash tokens must match the command spec rows where
    `requiresCLIHelp` or `cliHelpEntry` is true;
  - source-derived subcommand extractors parse production code, not help prose:
    `modeCmd` valid mode map keys, `trustCmd` `switch args[0]` cases, and
    `providerCmd` `switch sub` cases must match the typed spec table and the
    PTY/focused classification rows;
  - nested trust action coverage is behavior-derived: table tests execute every
    typed `/trust` spec row against fake clients, including `/trust persist
    allow`, `/trust persist deny`, and rejected nested actions such as
    `/trust persist maybe`, proving accepted/rejected behavior matches the
    spec;
  - examples in help descriptions are never command discovery inputs.
- Compare top-level parser cases, in-TUI help rows, and autocomplete entries in
  `internal/tui/commands`; compare public `printUsage` slash rows and built
  `ratchet help` output in `cmd/ratchet`. Both packages read the same typed
  spec fixture. Compare source-derived mode/trust/provider subcommand sets
  against the matching spec fields. Fail on unclassified parser commands,
  help-only commands, CLI-help-only commands, unclassified source-derived
  subcommands/modes, missing required in-TUI help rows, missing required public
  CLI help rows, missing required autocomplete entries, or docs claims that
  assign PTY proof to a non-`pty-proven` row.
- Do not introduce a production shared command registry in this slice; that
  would be a broader command-dispatch refactor. The test-owned spec table plus
  AST guard is narrower and keeps behavior unchanged.

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
- Update public `ratchet help` / `printUsage` slash-command section to retain
  and match the same mode/trust/tree slash surface. Binary smoke must assert the
  built CLI help includes the aligned entries; removing the section is out of
  scope for this slice because Windows packaged-help smoke depends on it.
- Add focused tests proving submitted-command support, `/help`, and
  autocomplete do not drift for the mode/trust/tree slash surface. Add a
  `cmd/ratchet` built-binary help assertion for `ratchet help`; that CLI test
  extracts `printUsage` slash rows and compares them to the typed command spec
  so the public CLI help section cannot silently drift from the discoverable
  TUI command contract.
- Add focused tests that enumerate every slash command returned by
  the AST command-surface guard for `commands.Parse`, `helpCmd`, and the
  autocomplete model, compare that list to the coverage contract above, and
  fail closed on unclassified additions.
- The command spec has per-command `requiresHelp` and `requiresAutocomplete`
  booleans. A command such as `/mcp` can be parser/autocomplete-covered while
  intentionally help-omitted; the guard fails only when the source surface
  diverges from the explicit per-command requirements.
- This aligns existing discoverability surfaces only; it does not add new
  commands or change command semantics, and it does not turn
  `focused-proven`/`deferred-runtime` commands into PTY runtime claims.

### Normal Launch Smoke

- Keep the untagged built-binary smoke for `version`, `help`, and
  `daemon status`.
- Add a non-race CI smoke job in `.github/workflows/ci.yml` so binary/startup
  smoke tests that skip under `-race` are actually exercised:
  `go test ./cmd/ratchet ./internal/tui -run 'HarnessSmoke|TUIBinarySmoke|StartupSmoke' -count=1`.
- The non-race smoke job must be setup-equivalent to existing CI jobs:
  - `runs-on: ubuntu-latest`;
  - `needs: build`;
  - `actions/checkout@v4`;
  - `actions/setup-go@v5` with `go-version: "1.26"` and cache enabled;
  - job/env includes `GOPRIVATE` and `GONOSUMCHECK` for
    `github.com/GoCodeAlone/*`;
  - configure the same private-module Git rewrite with `GITHUB_TOKEN` before
    running the smoke command.
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
    then wait boundedly for pid/socket cleanup; if either remains after the
    wait, read the temp pid file and verify the PID still belongs to the
    ratchet daemon launched by this smoke test before terminating it;
  - fallback termination must prove process identity by matching at least the
    built `ratchet` executable path or command line plus temp `HOME`/state
    directory and daemon socket path; if identity cannot be proven, fail with
    redacted diagnostics instead of killing the PID;
  - assert temp `.ratchet/daemon.pid` and `.ratchet/daemon.sock` are gone after
    cleanup.
- Docs distinguish:
  - release-shaped binary: startup/help/daemon/onboarding boundary;
  - `ratchet-tui-smoke` binary: credential-free interactive TUI chat, selected
    PTY-proven commands, and PTY-proven shortcuts against mock daemon;
  - focused TUI tests: conditional shortcut and branch-selection behavior that
    is not claimed as PTY-proven.

### Portable Compile And CLI Proof

- Keep existing `cmd/ratchet` binary smoke for `version`, `help`, and
  `daemon status`.
- Add Windows cross-build verification to the plan, not PTY execution:
  `GOOS=windows GOARCH=amd64 go build -o "$TMPDIR/ratchet-windows-amd64.exe" ./cmd/ratchet`
  and
  `GOOS=windows GOARCH=arm64 go build -o "$TMPDIR/ratchet-windows-arm64.exe" ./cmd/ratchet`
  locally; CI uses `$RUNNER_TEMP` instead of `$TMPDIR`.
- Add a `windows-safe-command-smoke` job on `windows-latest` that declares
  `needs: release-check`, builds source `ratchet.exe`, downloads the
  `ratchet-snapshot-dist` workflow artifact from `release-check`, fails unless
  both `ratchet_windows_amd64.zip` and `ratchet_windows_arm64.zip` are present,
  byte-scans both zips/binaries for forbidden smoke tokens, extracts the
  runner-compatible `ratchet_windows_amd64.zip`, and executes safe non-PTY
  commands from that packaged executable:
  - `ratchet.exe version`;
  - `ratchet.exe help`;
  - `ratchet.exe daemon status` with temp Windows `HOME`/`USERPROFILE` and
    `XDG_STATE_HOME`;
  - output must include `ratchet`, `Commands:`, the aligned slash-command help
    entries, and `daemon is not running`, without starting the TUI or daemon.
  - configure `GOPRIVATE`, `GONOSUMCHECK`, and the same private-module Git
    rewrite used by existing CI jobs before `go build`.
- If Go test cannot run Unix PTY on Windows CI, PTY tests and PTY-only helpers
  remain explicitly build-constrained with `//go:build !windows`, and docs
  state that Windows interactive proof is not claimed.
- The smoke package/client use `tui_smoke && !windows`; Windows verification
  explicitly asserts release-shaped `./cmd/ratchet` builds and smoke package
  `go list`/`go build` are unavailable on Windows amd64 and arm64.

### Release Artifact Guard

- Add `internal/releaseguard` as the structured release guard implementation and
  `scripts/check-release-artifacts.sh` as a thin shell wrapper:
  - the Go helper code lives under `internal/releaseguard` and is exercised by
    Go tests; the shell wrapper invokes it through explicit non-cacheable test
    commands, not a product `cmd/*` package;
  - every artifact-reading invocation sets explicit environment such as
    `RATCHET_RELEASE_GUARD_DIST=<dist>` and
    `RATCHET_RELEASE_GUARD_MODE=<manifest|draft-assets>` and runs
    `go test -count=1 ./internal/releaseguard -run <guard-test>` so a cached Go
    test result can never stand in for reading the supplied artifact tree;
  - the Go helper uses existing Go module dependencies, including
    `gopkg.in/yaml.v3`, to parse `.goreleaser.yaml` and inspect generated
    artifact directories; do not implement YAML parsing in shell or add an
    unpinned external `yq` dependency;
  - the shell wrapper only runs `goreleaser` when needed and invokes the
    `internal/releaseguard` test entrypoint without adding a product command;
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
  - run `scripts/check-release-artifacts.sh --manifest-only dist`;
  - upload the generated `dist/` directory as a short-lived
    `ratchet-snapshot-dist` artifact with `actions/upload-artifact@v4`, for the
    dependent `windows-latest` packaged-zip proof.
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
  - clone `GoCodeAlone/homebrew-tap` before publishing and run a pre-publish
    tap drift audit with
    `RATCHET_RELEASE_GUARD_MODE=tap-preflight
    RATCHET_RELEASE_GUARD_TAP=<tap-checkout>
    RATCHET_RELEASE_GUARD_VERSION=<tag-or-version>
    go test -count=1 ./internal/releaseguard -run TestTapPreflight`; this
    discovers every relevant `ratchet` root/Formula/Casks install file and
    fails before publish if any stale legacy tap surface would make the
    post-publish audit fail for reasons unrelated to the current release;
  - only then run the existing publishing `goreleaser release --clean` step.
- After the publishing step creates a draft GitHub release and before the
  existing publish/undraft script flips `draft: false`, inspect the draft
  release assets and uploaded checksum file for the same forbidden names. This
  postcheck proves the uploaded GitHub asset set did not diverge from the
  snapshot guard.
- Homebrew/tap limitation: the current GoReleaser release step can push
  `homebrew_casks.repository` changes before any postcheck can run. This slice
  prechecks cask/tap config and snapshot material before publishing, then
  performs an executable after-the-fact tap/cask reference check after
  publishing:
  - after `goreleaser release --clean`, clone the tap identified by
    `.goreleaser.yaml` `homebrew_casks[].repository` (currently
    `GoCodeAlone/homebrew-tap`, branch `main`) with `HOMEBREW_TAP_TOKEN`;
  - parse `.goreleaser.yaml` to derive expected tap names from every
    Homebrew-related release surface (`homebrew_casks` today, plus any
    `brews`/formula section added later);
  - discover all relevant tap install files in the checkout instead of assuming
    a cask-only layout: candidates include root `ratchet*.rb`,
    `Casks/ratchet*.rb`, `Formula/ratchet*.rb`, and any path changed by the
    current release commit whose basename contains `ratchet`;
  - fail if no relevant tap file exists; verify each discovered install surface
    for the current release's expected version/checksum context and forbidden
    smoke tokens;
  - resolve the exact path-changing commit for each relevant tap file with
    `git log -1 -- <tap-file>` in the tap checkout, not raw branch `HEAD`;
    verify each commit's file content contains the expected release tag or
    version/checksum context before scanning;
  - run `RATCHET_RELEASE_GUARD_MODE=tap-postcheck
    RATCHET_RELEASE_GUARD_TAP=<tap-checkout>
    RATCHET_RELEASE_GUARD_TAP_NAMES=ratchet-cli,ratchet
    RATCHET_RELEASE_GUARD_TAP_COMMITS=<path=sha,...>
    RATCHET_RELEASE_GUARD_VERSION=<tag-or-version>
    go test -count=1 ./internal/releaseguard -run TestTapPostcheck`;
  - the helper scans `git show <path-changing-sha>:<tap-file>` for every
    discovered install file, each path-changing commit's metadata, and each
    commit's changed-file list for forbidden smoke tokens
    (`ratchet-tui-smoke`, `tui_smoke`, smoke command/flag/help markers);
  - on contamination, fail the release workflow and print the exact tap owner,
    branch, per-path commit SHA, tap file path, and rollback command shape
    grouped by unique SHA (`git revert <tap-sha>` once per contaminated commit
    on the tap branch, then cut a corrected patch release); if one commit mixes
    contaminated and clean tap paths, diagnostics must name the clean paths that
    would also be reverted so the operator can choose a path-specific corrective
    commit instead.
  Fully pre-public tap blocking requires a future split-publish workflow and is
  out of scope for this slice.
- Local verification can run the script when GoReleaser is installed; ordinary
  `go test ./...` must not silently claim release-artifact coverage if
  GoReleaser is absent.
- Build a normalized manifest from `dist/`:
  - every file path under `dist/`;
  - `checksums.txt` entries;
  - archive member lists from each `.tar.gz` and `.zip`;
  - generated Homebrew cask/tap files if present;
  - GoReleaser metadata files if present.
- Inspect packaged release binaries from the snapshot artifacts:
  - extract at least the host-compatible Unix archive and run packaged
    `ratchet version` and `ratchet help`;
  - fail if packaged help/version output contains `ratchet-tui-smoke`,
    `tui_smoke`, a smoke command, a smoke flag, or smoke help text;
  - inspect every archive member list and binary filename for smoke names;
  - extract every packaged `ratchet`/`ratchet.exe` binary from every archive,
    including non-host Linux/Darwin/Windows OS/arch combinations, and byte-scan
    each binary for forbidden smoke tokens (`ratchet-tui-smoke`, `tui_smoke`,
    smoke command/flag/help markers); this is content proof even when the
    binary cannot execute on the current runner;
  - for Windows, the `windows-safe-command-smoke` job on `windows-latest`
    declares `needs: release-check`, downloads `ratchet-snapshot-dist` with
    `actions/download-artifact@v4`, requires `ratchet_windows_amd64.zip` and
    `ratchet_windows_arm64.zip`, byte-scans both, extracts the snapshot
    `ratchet_windows_amd64.zip` produced by `release-check`, and runs packaged
    `ratchet.exe version`, `ratchet.exe help`, and `ratchet.exe daemon status`
    with the same forbidden-output checks.
- Add a release draft asset postcheck script path before undrafting:
  - preflight fails unless `.goreleaser.yaml` has `release.draft: true`;
  - after `goreleaser release --clean` creates the draft release, reuse the
    existing release workflow's `listReleases` retry-by-tag behavior because
    `getReleaseByTag` can 404 for drafts;
  - resolve a single draft release id with an explicit token
    (`RELEASES_TOKEN || GITHUB_TOKEN`), pass that release id to both the
    postcheck and undraft steps, and download assets by release id into a temp
    directory;
  - fail immediately if the resolved release is not draft before inspection;
  - run the same archive manifest, all-packaged-binary byte scan, packaged
    help/version where executable, checksum-name, and forbidden-token checks
    used for snapshot `dist/`;
  - only then run the existing undraft step;
  - if the postcheck fails, leave the GitHub release draft, delete or supersede
    contaminated assets per rollback, and do not undraft.
- Fail closed when expected artifact classes are missing:
  - parse `.goreleaser.yaml` and derive the complete expected archive matrix
    from each `builds[].id`, `goos`, `goarch`, archive `ids`, archive format
    overrides, and archive `name_template`; for the current config this means
    `ratchet_linux_amd64.tar.gz`, `ratchet_linux_arm64.tar.gz`,
    `ratchet_darwin_amd64.tar.gz`, `ratchet_darwin_arm64.tar.gz`,
    `ratchet_windows_amd64.zip`, and `ratchet_windows_arm64.zip`;
  - every expected archive must exist in `dist/`, have a `checksums.txt` entry,
    and be included in member-list plus packaged-binary byte scans; one archive
    per OS is not enough;
  - `checksums.txt` must be present and must not contain unexpected smoke
    artifact names;
  - if `.goreleaser.yaml` has `homebrew_casks`, snapshot output must either
    contain generated cask/tap material or the guard must parse `.goreleaser.yaml`
    as deterministic fallback proof;
  - fallback proof parses `.goreleaser.yaml` and fails unless every build
    `id`, archive `ids` entry, `homebrew_casks[].ids` entry,
    `homebrew_casks[].binaries` entry, and release-publish build surface is
    exactly `ratchet`.
- In fallback mode, recursively scan every scalar string under artifact/publish
  sections (`builds`, `archives`, `checksum`, `homebrew_casks`, `release`, and
  any future artifact/publish taxonomy entry) for forbidden smoke tokens before
  applying the id/binary assertions. This catches nested hooks/templates/custom
  blocks/signing/publisher fields that reference smoke artifacts without
  changing ids.
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
    chat, selected/PTY-proven shortcuts, and selected/PTY-proven slash commands
    (`/help`, `/provider list`, `/tree`, `/mode`, `/trust`, and `/exit`);
  - focused TUI tests: conditional `ctrl+h`, detailed branch-tree navigation,
    and App-level child-session switching.
- Update `README.md` harness table with the same split wording.
- Extend `cmd/ratchet/harness_docs_test.go` so docs must mention TUI binary
  smoke, selected/PTY-proven slash commands, selected/PTY-proven shortcuts,
  focused shortcut coverage, temp home/workdir, mock provider, Windows compile
  proof, and the fact that release-shaped `ratchet` proof does not claim
  credential-free chat.
- Make the docs guard bidirectional:
  - exact positive evidence assertions scan `README.md` harness table and
    `docs/harness-emulation.md`;
  - `RATCHET.md` and `docs/competitor-parity.md` need only link/point to
    harness evidence unless they independently claim TUI binary-smoke evidence;
  - negative overclaim assertions scan `README.md`, `RATCHET.md`,
    `docs/harness-emulation.md`, `docs/competitor-parity.md`, and
    `docs/policy-matrix.md`;
  - `docs/policy-matrix.md` only needs positive TUI smoke wording if it itself
    starts making TUI binary-smoke evidence claims;
  - positive assertions require the split `ratchet` startup/onboarding proof
    and `ratchet-tui-smoke` interactive proof;
  - negative assertions scan each table row as one deterministic claim unit;
  - negative assertions unwrap adjacent nonblank non-table lines into
    paragraphs, split those paragraphs into sentence claim units, and apply the
    overclaim predicate to each sentence; paragraph context is retained only for
    diagnostics so hard-wrapped prose is caught without merging unrelated
    sentences into false positives;
  - each claim unit is normalized by lowercasing, collapsing whitespace, and
    treating punctuation separators (`/`, `-`, `:`, `;`, comma) as spaces;
  - a claim unit fails only when it is an evidence claim: it contains an exact
    evidence token (`proof`, `proves`, `proven`, `covered`, `coverage`,
    `smoke`, `automated`, `automation`, `verified`, `binary smoke`, `CI`,
    `test`, `tests`, `tested`, `testing`, `exercised`, `exercise`,
    `asserted`, `asserts`, `validated`, `validation`, `guarded`, `guard`,
    `e2e`, `end to end`, or `end-to-end`)
    and an interactive-surface token (`credential free chat`,
    `interactive chat`, `interactive tui`, `full tui`, `slash command`,
    `slash commands`, `shortcut`, `shortcuts`, or `slash shortcut`);
  - release-target tokens (`` `ratchet` ``, `ratchet` command,
    `release-shaped`, `release binary`, or `untagged`) make a failure more
    specific, but are not required for suspicious interactive-evidence claims;
    generic claims such as "full TUI coverage is automated" fail unless the
    same claim unit assigns the proof to `ratchet-tui-smoke` or explicitly
    says release-shaped `ratchet` does not claim that proof;
  - valid product-support statements such as "ratchet-cli supports trust slash
    commands" are allowed unless they also claim automation/evidence for the
    release-shaped binary;
  - a `ratchet-tui-smoke` evidence claim may describe slash-command proof only
    as selected/PTY-proven slash-command coverage, enumerate `/help`,
    `/provider list`, `/tree`, `/mode`, `/trust`, and `/exit`, or link to the
    command-surface classification table; broad claims such as "slash commands
    are smoke-proven" fail even when assigned to `ratchet-tui-smoke`;
  - positive TUI evidence statements in `README.md` and
    `docs/harness-emulation.md` must match one of the allowed claim templates:
    release-shaped `ratchet` startup/onboarding proof with no credential-free
    chat claim, or `ratchet-tui-smoke` Unix PTY proof for interactive chat,
    selected/PTY-proven shortcuts, and selected/PTY-proven slash commands; any
    other TUI evidence wording must link to the command-surface or shortcut
    classification table;
  - broad claims such as "PTY proof for core shortcuts" or "binary smoke tests
    all shortcuts" fail unless the claim enumerates only `pty-proven` shortcut
    rows or points to the shortcut matrix and distinguishes focused shortcut
    proof;
  - same-line/same-row exceptions must negate the same release-target evidence
    claim (`not claimed for ratchet`, `does not claim release-shaped`, or
    equivalent), assign the evidence claim to `ratchet-tui-smoke` within the
    selected/PTY-proven wording above, or explicitly classify the command family
    through the command-surface table;
  - the guard reports the normalized claim unit and original file/line through
    the redaction helper.

## Security Review

| risk | control |
|---|---|
| Smoke mode becomes a user-facing bypass. | Compile it only with `tui_smoke`; release binaries do not contain the path. |
| Smoke package accidentally becomes default-buildable. | Unix no-tag `go list`/`go build ./cmd/ratchet-tui-smoke` fail; tagged Unix build succeeds; test-owned smoke-source manifest requires exact `//go:build tui_smoke && !windows` on every non-test smoke-only file and scans non-test Go files for exact smoke-surface tokens instead of broad pathname matches. |
| Test leaks real home/provider/project state. | Set temp `HOME`/`XDG_STATE_HOME`/`cmd.Dir`/session `WorkingDir`; temp workdir contains no instruction files/dirs from `internal/agent/instructions.go` and no hook configs from `internal/hooks/hooks.go` (`~/.ratchet/hooks.yaml`, `.ratchet/hooks.yaml`); assert captured output excludes real workspace/home paths. |
| PTY test hangs or flakes in CI. | Per-read deadline, process kill cleanup, bounded waits, synchronized PTY output snapshots, and no external network/provider dependency. |
| PTY exit path is only partly tested. | Use one long interaction PTY run ending with `/exit`, plus separate short PTY subprocess subtests for `ctrl+c` and `ctrl+d`. |
| Shortcut proof misses broken layout. | Use fixed PTY size and assert representative full frames for chat/sidebar input-visible states and branch-tree/team/job panel states, then return to chat and assert input usability. |
| Shortcut hint drifts from handlers. | Treat app/page/component key handling plus README/statusbar hints as a checked contract; classify each shortcut as `pty-proven` or `focused-proven`, and reject docs that collapse focused shortcut proof into PTY proof. |
| Slash-command proof overclaims broad coverage. | Maintain a command-surface classification table from `commands.Parse`/`helpCmd`; docs may claim PTY proof only for `pty-proven` rows. |
| Sensitive prompts/log paths in logs. | Use harmless deterministic prompts and route every test failure payload through one redaction helper that removes real home/workspace/temp/socket/executable/artifact paths plus trust/prompt bodies; runtime outputs reject instruction surfaces from `internal/agent/instructions.go` and hook config surfaces from `internal/hooks/hooks.go`, release manifests allowlist expected `RATCHET.md`. |
| Release-shaped startup leaks daemon process. | Cleanup runs `ratchet daemon stop`, waits boundedly for temp pid/socket removal, proves PID identity before any fallback terminate/wait, and asserts pid/socket files are gone. |
| Binary smoke skipped under race. | Add package-local `internal/tui` race helper files; `TestTUIBinarySmoke` skips under `-race`, and a focused non-race CI smoke job for `cmd/ratchet` and `internal/tui` smoke tests runs alongside the existing race suite. |
| Non-race smoke CI cannot fetch private modules. | Make the smoke job setup-equivalent to existing CI: checkout, setup-go `1.26`, `GOPRIVATE`/`GONOSUMCHECK`, and private-module Git rewrite before `go test`. |
| Release artifact guard only runs manually. | Add the publish-free `release-check` CI job; CI uses GoReleaser action for snapshot generation and the same manifest guard script local runs use. |
| Shell release guard misparses GoReleaser YAML. | Implement the artifact/config guard under `internal/releaseguard` in Go with `gopkg.in/yaml.v3`; shell wrapper only orchestrates GoReleaser and invokes Go tests/helper logic without adding a product `cmd/*` package. |
| Tag release bypasses artifact guard. | Add the same publish-free `goreleaser check` + snapshot + manifest guard to `.github/workflows/release.yml` before the publishing step. |
| Release jobs cannot fetch private modules. | Mirror CI's `GOPRIVATE`/`GONOSUMCHECK` and private-module Git rewrite in release preflight/publish and Windows safe-command smoke jobs. |
| Snapshot guard differs from uploaded release. | Add a draft-release postcheck before undrafting that downloads uploaded GitHub release archives/checksums and runs the same archive manifest plus all-packaged-binary byte-scan guard against actual uploaded assets; add Homebrew tap preflight before publish to catch stale root/Formula/Casks drift, then an executable tap postcheck after publish that discovers every relevant install file, resolves each path-changing commit for the current release, scans each file content and commit metadata, and prints exact rollback targets on contamination. |
| Draft release lookup flakes or misses assets. | Reuse the existing `listReleases` retry-by-tag workflow, pass the resolved release id to postcheck and undraft, and download assets by release id. |
| Release is public before postcheck. | Preflight fails unless `.goreleaser.yaml` keeps `release.draft: true`; postcheck fails if the resolved release is not draft before inspection. |
| Platform mismatch. | Unix PTY proof is explicitly Unix-only; Windows claim is limited to build/noninteractive smoke. |

## Infrastructure Impact

No cloud resources, secrets, migrations, new registries, or production deploy.
The existing tag release workflow already publishes GitHub release assets and a
Homebrew cask/tap update through GoReleaser. This slice adds publish-free
GoReleaser preflight jobs on PR/push and before tag publishing, a GitHub draft
asset postcheck before undrafting, packaged-binary inspection, an executable
Homebrew/tap preflight that clones `GoCodeAlone/homebrew-tap` before publish,
an after-the-fact Homebrew/tap postcheck that scans every relevant `ratchet`
root/Formula/Casks tap install file at its exact path-changing commit for the
current release, plus a
`windows-safe-command-smoke` job on `windows-latest`. GitHub release artifacts
are pre-public gated; Homebrew/tap remains prechecked plus
postchecked/rollback under the current GoReleaser workflow. Fully pre-public
Homebrew/tap gating is deferred to a split-publish workflow. These workflow
paths must mirror existing
private-module environment and Git rewrite setup. Local test-only process and
temp files only. Runtime behavior does not change in release builds because the
smoke entrypoint is build-tagged out.

## Multi-Component Validation

| boundary | proof |
|---|---|
| Release-shaped built binary to startup | Untagged PTY or subprocess smoke launches compiled `ratchet` with temp home/workdir, reaches help/daemon/onboarding boundary, then stops the temp daemon and verifies pid/socket cleanup. |
| Smoke built binary to TUI | `-tags tui_smoke` PTY test launches compiled `ratchet-tui-smoke` and reads rendered frames. |
| TUI to daemon gRPC | Smoke service uses real daemon service/client RPCs for provider list, session tree, trust mode/rules, and chat send; docs do not claim auto-daemon socket proof from this row. |
| TUI to mock provider | Chat prompt reaches built-in mock provider and streams a response. |
| Slash commands | PTY submits representative slash commands through the input widget and asserts rendered system output/navigation for `pty-proven` rows: `/help`, `/provider list`, `/tree`, `/mode`, `/trust`, and `/exit`. |
| TUI exit paths | `/exit`, `ctrl+c`, and `ctrl+d` each run in their own PTY subprocess/subtest; the long interaction run exits with `/exit`, while control-key runs prove clean termination independently. |
| Slash discoverability | Focused tests keep in-TUI `/help`, autocomplete, public `ratchet help`/`printUsage`, and the command-surface classification table aligned with `commands.Parse`/`helpCmd`; docs cannot claim PTY proof for `focused-proven` or `deferred-runtime` commands. |
| Shortcuts | PTY sends `pty-proven` shortcut rows and asserts branch tree/pane transitions with fixed-size full-frame checks matching current behavior; focused tests prove conditional `ctrl+h`, advertised branch-tree navigation, and App-level branch switch into child chat history. Docs cannot claim PTY proof for focused shortcut rows. |
| Docs to tests | Docs guard fails if automated TUI smoke evidence is removed. |
| Binary smoke to CI | Non-race CI smoke job runs the built-binary/startup/TUI smoke tests that the race suite skips; `internal/tui` owns its race skip helper so package-local tests compile in both race and non-race suites. |
| Release preflight to CI/release | `release-check` and tag release workflow both run `goreleaser check`, snapshot generation, and the same artifact-manifest guard before publish; guard extracts and byte-scans every packaged `ratchet` binary from every archive, runs executable help/version checks where practical, and release postcheck downloads uploaded GitHub draft release assets and repeats the same checks before undrafting; Homebrew/tap preflight catches stale tap drift before publish, while postcheck remains after-the-fact with rollback for the current GoReleaser tap push. |
| Windows build/smoke | Cross-build commands prove release-shaped `ratchet` still compiles for Windows; `windows-safe-command-smoke` on x64 `windows-latest` declares `needs: release-check`, builds source `ratchet.exe`, downloads the `release-check` snapshot `dist/` artifact, requires both Windows zips, byte-scans amd64 and arm64 archives, extracts `ratchet_windows_amd64.zip`, then executes packaged `ratchet.exe version`, `ratchet.exe help`, and `ratchet.exe daemon status` with temp Windows home/state env; `ratchet-tui-smoke` interactive PTY remains Unix-only. |

## Integration Matrix

| integration | classification | proof |
|---|---|---|
| Release-shaped `ratchet` binary | runtime-integrated | Existing and expanded smoke builds without tags and runs `version`, `help`, `daemon status`, plus startup/onboarding boundary. |
| `ratchet-tui-smoke` binary | runtime-integrated | Unix PTY test builds `./cmd/ratchet-tui-smoke` with `-tags tui_smoke`, launches binary, and drives TUI. |
| TUI Bubble Tea event loop | runtime-integrated | PTY frames prove splash, chat prompt, transcript, navigation, and exit. |
| Daemon gRPC service/client | runtime-integrated | Smoke service/client RPCs over a temp Unix socket execute provider list, trust commands, session tree, and chat send. |
| Built-in mock provider | runtime-integrated | Chat prompt streams deterministic mock response. |
| Slash commands and PTY-proven shortcuts | runtime-integrated | PTY sends input/control keys and asserts resulting UI states for representative `pty-proven` slash commands, the full documented TUI mode/trust matrix, `pty-proven` shortcut rows, and fixed-size representative frames. |
| Focused shortcut coverage | config-only + focused proof | Focused tests prove conditional `ctrl+h`, advertised branch-tree navigation, and App-level branch switch into child chat history; this is not claimed as PTY runtime proof. |
| Public CLI slash help | runtime-integrated | Built `ratchet help` is executed from the release-shaped binary, and the `printUsage` extractor ties its slash rows to the command-surface classification table. |
| TUI help/autocomplete command-surface guard | config-only + focused proof | Focused tests invoke in-TUI `/help`, inspect autocomplete models, parse command surfaces, and compare them to the typed command spec; this prevents discoverability drift but is not claimed as host-runtime proof for every non-`pty-proven` command. |
| Non-race binary smoke CI | config-only | `.github/workflows/ci.yml` runs a focused non-race smoke job for subprocess/startup tests that are intentionally skipped under `-race`, with explicit test names for release startup and TUI binary smoke. |
| Docs guard | config-only | `cmd/ratchet/harness_docs_test.go` checks public docs mention exact evidence boundaries. |
| GoReleaser snapshot and GitHub draft assets | runtime-integrated | Publish-free `release-check` CI job, tag release preflight, local script, and release draft postcheck run `goreleaser check`/snapshot generation plus full `.goreleaser.yaml`-derived OS/arch archive matrix inspection, checksum-entry checks, all-archive packaged-binary byte scans, host/Windows packaged-binary help/version checks, and deterministic `.goreleaser.yaml` fallback. The postcheck downloads draft GitHub release assets and repeats the same checks before undraft, proving uploaded archives/checksums do not contain `ratchet-tui-smoke`. |
| Homebrew/tap install files | config-only + preflight + post-publish audit | Snapshot/config precheck proves Homebrew ids/binaries do not reference smoke artifacts; before publish, tap preflight discovers root/Formula/Casks drift that would make postcheck fail; after publish, the guard discovers and scans every relevant tap file at its exact path-changing commit. Current GoReleaser workflow can still push the tap before postcheck, so current-release tap safety is after-the-fact audit plus rollback, not pre-public gating. Split-publish pre-public gating is deferred. |
| Windows safe-command smoke | runtime-integrated | `windows-safe-command-smoke` on x64 `windows-latest` declares `needs: release-check`, builds source `ratchet.exe`, downloads `ratchet-snapshot-dist` from `release-check`, requires `ratchet_windows_amd64.zip` and `ratchet_windows_arm64.zip`, byte-scans both, extracts amd64 only for execution, and runs packaged `ratchet.exe version`, `ratchet.exe help`, and `ratchet.exe daemon status` with temp Windows home/state env to prove non-PTY Windows CLI startup/help/daemon-status behavior and packaged-archive absence of smoke surfaces. |
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
release. If the Homebrew tap postcheck fails, use the reported
`GoCodeAlone/homebrew-tap` branch, per-path tap SHA, and tap install file path
to revert each unique contaminated tap commit once (`git revert <tap-sha>` on
`main`) or delete the bad reference, then cut a corrected patch release. If a
mixed commit also changed clean tap paths, use the reported path list to choose
a path-specific corrective commit instead of a blind revert. If the CI or release
preflight/postcheck itself is bad, revert that job/script while keeping the
existing tag-only release workflow's publish step, then cut a corrected
preflight PR.

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
- Fully pre-public Homebrew/tap split-publish gating; this slice keeps
  precheck plus after-the-fact tap audit/rollback under the existing
  GoReleaser workflow.
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
| D51 | Removed the contradictory "no Homebrew publishing" claim and split release safety into GitHub pre-public gating versus Homebrew/tap precheck plus after-the-fact audit/rollback. |
| D52 | Added `internal/tui/pages/chat.go` to the shortcut source of truth and required focused proof for conditional `ctrl+h` thinking-panel behavior. |
| D53 | Added focused branch-tree navigation proof for README-advertised keys (`j`/`k`, arrows, `h`/`l`, `Enter`, `r`, `Esc`). |
| D54 | Added snapshot archive extraction and packaged release-binary `version`/`help` checks, including Windows zip execution on `windows-latest`, to catch smoke command/flag/help text in uploaded artifacts. |
| D55 | Replaced destructive-looking trust patterns with harmless deterministic `smoke:*` placeholders. |
| D56 | Replaced the `_unix_test.go` reliance with explicit `//go:build !windows` on the PTY smoke test and PTY-only helpers. |
| D57 | Added `internal/tui` package-local race helper files and required `TestTUIBinarySmoke` to skip under `-race`; the non-race CI job is the execution path. |
| D58 | Added `release-check` upload of the snapshot `dist/` artifact and `windows-latest` download/extraction of that artifact before packaged `ratchet.exe` checks. |
| D59 | Added bounded daemon cleanup wait plus terminate/wait fallback before asserting temp pid/socket removal. |
| D60 | Added a command-surface coverage contract from `commands.Parse`/`helpCmd`, narrowed PTY claims to `pty-proven` rows, and required docs guard failure on overclaims. |
| D61 | Made `windows-safe-command-smoke` explicitly depend on `release-check`, download `ratchet-snapshot-dist`, fail without a Windows zip, and run the packaged executable. |
| D62 | Added App-level branch-switch proof through `SessionTreeSelectedMsg`, selected-session update, child chat-history reload, and post-switch chat submit. |
| D63 | Required synchronized PTY capture helper output instead of copying the unsynchronized integration helper pattern. |
| D64 | Added an AST-based command-surface guard that extracts `Parse` switch cases, help output, and autocomplete literals, compares them to the coverage classification map, and fails on unclassified drift without adding a command registry refactor. |
| D65 | Made the non-race smoke CI job setup-equivalent to existing CI with checkout, setup-go `1.26`, private-module env, and Git rewrite. |
| D66 | Required extraction and byte-scanning of every packaged `ratchet` binary from every GoReleaser archive, with executable help/version checks kept where practical. |
| D67 | Changed Windows cross-build proof to write explicit temp output paths instead of repo-root `ratchet.exe`. |
| D68 | Replaced broad command regex extraction with a test-owned typed command-spec table plus separate top-level parser, help-row, autocomplete, and subcommand checks. |
| D69 | Chose the retained public `ratchet help` slash-section contract and required CLI help alignment because Windows packaged-help smoke asserts it. |
| D70 | Required draft-release postcheck to download actual uploaded GitHub release assets and run the same archive/all-binary byte-scan guard before undrafting. |
| D71 | Split docs guard behavior: exact positive TUI smoke evidence required only in README harness table and `docs/harness-emulation.md`; RATCHET/parity/policy get negative overclaim scanning unless they claim TUI binary evidence. |
| D72 | Added source-derived AST checks for `modeCmd` mode keys, `trustCmd` subcommands, and `providerCmd` subcommands, compared to the typed command spec and proof classifications. |
| D73 | Added packaged Windows `ratchet.exe daemon status` smoke with temp Windows home/state env and expected `daemon is not running` output. |
| D74 | Required draft postcheck to reuse the existing `listReleases` retry-by-tag lookup, pass release id to postcheck/undraft, and download assets by release id. |
| D75 | Narrowed exact positive docs assertions to README harness table and `docs/harness-emulation.md`; RATCHET/parity get links plus negative overclaim checks unless they claim TUI evidence. |
| D76 | Added behavior-derived `/trust` table tests for nested persist allow/deny and rejected nested actions, tied to the typed command spec. |
| D77 | Added hard draft-state gates: `.goreleaser.yaml` must keep `release.draft: true`, and postcheck fails if the resolved release is not draft. |
| D78 | Made Windows archive handling architecture-aware: require amd64 and arm64 zips, byte-scan both, execute only `ratchet_windows_amd64.zip` on x64 `windows-latest`. |
| D79 | Corrected stale D71 resolution wording to match the narrowed positive-docs guard scope. |
| D80 | Changed release artifact/config guard to internal Go helper logic (`internal/releaseguard`) using `gopkg.in/yaml.v3`; shell script is only a wrapper and no product `cmd/*` helper is added. |
| D81 | Split PTY exit proof into one long `/exit` interaction run plus separate short `ctrl+c` and `ctrl+d` subprocess subtests. |
| D82 | Made docs overclaim guard context-aware: interactive-surface evidence claims fail even without a release-target token unless assigned to `ratchet-tui-smoke` or explicitly deferred for release-shaped `ratchet`. |
| D83 | Added per-command help/autocomplete requirements so commands such as `/mcp` can be parser/autocomplete-covered while intentionally help-omitted. |
| D84 | Added Unix no-tag `go list`/`go build` negative checks, tagged Unix positive build, and exact smoke-file build-tag source guard. |
| D85 | Changed docs overclaim scanner to use paragraph claim units plus table rows, not physical lines only. |
| D86 | Required display-cell width assertions with `lipgloss.Width` or `runewidth` for ANSI-stripped PTY frames. |
| D87 | Replaced vague smoke helper globs with a test-owned smoke-source manifest plus repo-wide non-test smoke identifier/path scan that fails unmanifested smoke code. |
| D88 | Changed docs scanner to unwrap paragraphs but scan sentence claim units plus table rows, keeping paragraph context only for diagnostics. |
| D89 | Added recursive forbidden-token scalar scanning under all GoReleaser artifact/publish sections in fallback mode before id/binary assertions. |
| D90 | Moved release guard implementation shape from product `cmd/*` to `internal/releaseguard` plus tests/shell wrapper to avoid a public helper binary surface. |
| D91 | Replaced broad smoke-source path/name scanning with a typed manifest schema, exact token/symbol scan, and allowlist for existing `*_smoke_test.go` precedent only. |
| D92 | Narrowed docs wording and guard behavior: `ratchet-tui-smoke` may claim chat, selected/PTY-proven shortcuts, and selected/PTY-proven slash matrix only; focused shortcut proof stays separately labeled. |
| D93 | Added `printUsage` extraction and public CLI-help surface checks tied to the typed command spec. |
| D94 | Required releaseguard wrapper paths to use non-cacheable `go test -count=1` with explicit env/dist path for artifact inspection. |
| D95 | Added post-reset `/trust list` and `/trust grants` assertions proving runtime rules reset to config defaults while unreverted persisted grants remain unchanged. |
| D96 | Expanded docs evidence tokens to include test/tested/tests/testing/exercised/exercise/asserted/asserts so common overclaim wording cannot evade the guard. |
| D97 | Added advertised job-panel `Esc` close behavior to the shortcut matrix. |
| D98 | Required `/trust reset` follow-up proof that `Mode: conservative` matches the smoke config default after prior mode mutations. |
| D99 | Specified executable Homebrew tap postcheck mechanics: clone configured tap, discover relevant root/Formula/Casks install files, scan exact path-changing commit metadata/content, fail with rollback targets. |
| D100 | Added validated/validation/guarded/guard/e2e/end-to-end evidence terms and allowed TUI evidence claim templates. |
| D101 | Split the integration matrix row into runtime public CLI help and focused/static TUI help/autocomplete command-surface proof. |
| D102 | Changed Homebrew tap postcheck to resolve and scan the exact path-changing commit for every relevant tap install file for the current release, not branch HEAD. |
| D103 | Required positive build assertions to write binaries to temp `-o` paths instead of the checkout. |
| D104 | Replaced smoke-only direct DB mock-provider seeding with production `AddProvider` RPC setup through the smoke client. |
| D105 | Made command-surface guards fail closed on nonliteral/generated command cases, help rows, or autocomplete entries unless explicitly spec-marked and runtime-tested. |
| D106 | Expanded Homebrew tap postcheck from cask-only to discovery/scanning of every relevant root, Formula, and Casks install file with per-path rollback SHAs. |
| D107 | Split shortcut docs/proof into `pty-proven` and `focused-proven` classifications and rejected broad PTY shortcut overclaims. |
| D108 | Required daemon cleanup fallback to prove PID identity before termination, otherwise fail with redacted diagnostics. |
| D109 | Required a smoke-only daemon service option/constructor that disables MCP discovery, plugin loading/daemon tools, autoresponder loading, cron/background work, and host `PATH`/plugin scans, with focused inertness assertions. |
| D110 | Split command-surface guards by artifact class: TUI parser/help/autocomplete under `internal/tui/commands`, public `printUsage`/built help under `cmd/ratchet`, sharing only a test fixture spec. |
| D111 | Added pre-publish Homebrew tap drift audit so stale root/Formula/Casks install surfaces fail before GoReleaser publishes. |
| D112 | Changed tap rollback diagnostics to group contaminated paths by unique SHA and warn when a mixed commit would revert clean paths. |
| D113 | Split smoke-source scanning into a smoke runtime manifest and verification-tooling exact-token allowlist so `internal/releaseguard` can hold forbidden artifact tokens without becoming a smoke runtime path. |
| D114 | Added the smoke-only daemon helper file to the smoke-source manifest with exact build-tag/exported-token constraints. |
| D115 | Required releaseguard to derive and require every expected GoReleaser OS/arch archive and checksum entry from `.goreleaser.yaml`, including Linux/Darwin/Windows amd64 and arm64. |
