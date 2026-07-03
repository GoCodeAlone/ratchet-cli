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

## Design

### Build-Tagged Smoke Binary

- Add `cmd/ratchet-tui-smoke/main.go` compiled only with
  `//go:build tui_smoke`; release workflows and normal `go build ./cmd/ratchet`
  do not build this package.
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
    temp directories with no `.ratchet`, `AGENTS.md`, `CLAUDE.md`, `RATCHET.md`,
    `.cursorrules`, or `.windsurfrules`;
  - normal release binary has no path into this helper.
- Add negative assertions:
  - `go build ./cmd/ratchet` succeeds without `tui_smoke` and exposes no
    smoke command/flag/help text;
  - `goreleaser check` passes and a snapshot release/archive inspection finds
    no archive member, checksum entry, Homebrew artifact, or release asset named
    `ratchet-tui-smoke`;
  - captured PTY frames/logs do not contain the developer workspace path or
    real home path.

### Smoke Client Contract

- Add `internal/client/client_tui_smoke.go` with `//go:build tui_smoke`.
- API:
  `func ConnectSmokeUnix(ctx context.Context, tempRoot, socketPath string) (*Client, error)`.
- Contract:
  - `tempRoot` and `socketPath` are converted to absolute clean paths;
  - symlink-aware containment is checked by resolving the existing temp root and
    socket parent with `filepath.EvalSymlinks`;
  - `socketPath` must be under `tempRoot`;
  - socket file must exist and be mode `0600` before dialing;
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
  temp workdir, then starts the smoke binary in a PTY. This proves the TUI event
  loop, daemon RPCs, mock provider, slash commands, and shortcuts; it does not
  claim the untagged release binary can run credential-free chat without
  configured providers.
- Drive:
  - splash dismissal;
  - normal chat prompt with deterministic mock response;
  - `/help` renders "Available commands:";
  - `/provider list` renders `e2e-mock`;
  - `/mode conservative` and `/trust list` render daemon-backed trust output;
  - `/tree` opens branch tree and `esc` returns to chat;
  - `ctrl+b` opens branch tree and `esc` returns;
  - `ctrl+s` and `ctrl+j` toggle panes without swallowing text input;
  - `/exit` or `ctrl+c` exits cleanly.
- Assertions strip ANSI and bound line width for representative frames.
- Test has timeouts and cleanup that kills the child process if it hangs.
- Test includes an untagged repo-root discovery/build helper rather than
  reusing `internal/tui/pty_test.go`, which is behind the `integration` tag.
- All PTY/stdout/stderr failure logs pass through one redaction helper before
  `t.Log`/`t.Fatalf`; it redacts real home path, repo workspace path, temp
  roots, socket path, executable path, trust output body, and deterministic
  prompt frames down to bounded excerpts.
- Failure assertions reject output containing the repo workspace path, real home
  path, or known instruction filenames from the source checkout.

### Normal Launch Smoke

- Keep the untagged built-binary smoke for `version`, `help`, and
  `daemon status`.
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
- If Go test cannot run Unix PTY on Windows CI, tests remain build-tagged by
  filename suffix and docs state that Windows interactive proof is not claimed.

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

## Security Review

| risk | control |
|---|---|
| Smoke mode becomes a user-facing bypass. | Compile it only with `tui_smoke`; release binaries do not contain the path. |
| Test leaks real home/provider/project state. | Set temp `HOME`/`XDG_STATE_HOME`/`cmd.Dir`/session `WorkingDir`; temp workdir contains no instruction or hook files; assert captured output excludes real workspace/home paths. |
| PTY test hangs in CI. | Per-read deadline, process kill cleanup, bounded waits, and no external network/provider dependency. |
| Sensitive prompts/log paths in logs. | Use harmless deterministic prompts and route all failure output through one redaction helper that removes real home/workspace/temp/socket/executable paths plus trust/prompt bodies. |
| Release-shaped startup leaks daemon process. | Cleanup stops or kills the temp-home daemon and asserts pid/socket files are gone. |
| Platform mismatch. | Unix PTY proof is explicitly Unix-only; Windows claim is limited to build/noninteractive smoke. |

## Infrastructure Impact

No cloud resources, secrets, migrations, registry entries, Homebrew changes, or
production deploy. Local test-only process and temp files only. Runtime behavior
does not change in release builds because the smoke entrypoint is build-tagged
out.

## Multi-Component Validation

| boundary | proof |
|---|---|
| Release-shaped built binary to startup | Untagged PTY or subprocess smoke launches compiled `ratchet` with temp home/workdir, reaches help/daemon/onboarding boundary, then stops the temp daemon and verifies pid/socket cleanup. |
| Smoke built binary to TUI | `-tags tui_smoke` PTY test launches compiled `ratchet-tui-smoke` and reads rendered frames. |
| TUI to daemon gRPC | Smoke service uses real daemon service/client RPCs for provider list, session tree, trust mode/rules, and chat send; docs do not claim auto-daemon socket proof from this row. |
| TUI to mock provider | Chat prompt reaches built-in mock provider and streams a response. |
| Slash commands | PTY submits slash commands through the input widget and asserts rendered system output/navigation. |
| Shortcuts | PTY sends control keys and asserts branch tree/pane transitions. |
| Docs to tests | Docs guard fails if automated TUI smoke evidence is removed. |
| Windows build | Cross-build commands prove release-shaped `ratchet` still compiles for Windows; `ratchet-tui-smoke` interactive PTY remains Unix-only. |

## Integration Matrix

| integration | classification | proof |
|---|---|---|
| Release-shaped `ratchet` binary | runtime-integrated | Existing and expanded smoke builds without tags and runs `version`, `help`, `daemon status`, plus startup/onboarding boundary. |
| `ratchet-tui-smoke` binary | runtime-integrated | Unix PTY test builds `./cmd/ratchet-tui-smoke` with `-tags tui_smoke`, launches binary, and drives TUI. |
| TUI Bubble Tea event loop | runtime-integrated | PTY frames prove splash, chat prompt, transcript, navigation, and exit. |
| Daemon gRPC service/client | runtime-integrated | Smoke service/client RPCs over a temp Unix socket execute provider list, trust commands, session tree, and chat send. |
| Built-in mock provider | runtime-integrated | Chat prompt streams deterministic mock response. |
| Slash commands and shortcuts | runtime-integrated | PTY sends input/control keys and asserts resulting UI states. |
| Docs guard | config-only | `cmd/ratchet/harness_docs_test.go` checks public docs mention exact evidence boundaries. |
| GoReleaser snapshot artifacts | runtime-integrated | `goreleaser check` plus snapshot/archive inspection proves `ratchet-tui-smoke` is absent from archives/checksums/Homebrew artifacts/release assets. |
| Windows interactive PTY | deferred | No ConPTY runner in this slice; Windows coverage is build/noninteractive only. |

## Rollback

Revert `cmd/ratchet-tui-smoke`, the `tui_smoke` client constructor, smoke
helper, PTY test, and docs updates. No data migration exists. Release binaries
are unaffected because `cmd/ratchet` does not contain a smoke command/flag. If a
future release accidentally includes `ratchet-tui-smoke`, remove it from the
release config, delete the bad artifact/checksum/tap reference where possible,
and cut a patch release.

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
4. First failure mode: PTY hang waiting for a frame. Design requires bounded
   waits, cleanup kill, and deterministic mock daemon.
5. Repo fit: follows existing PTY helpers, `cmd/ratchet` binary smoke shape,
   daemon mock-provider harness, and docs guard tests.

## Out Of Scope

- ACPX TypeScript flow runtime.
- Managed hooks or broad extension SDK.
- Daemon background queue drain.
- Local-first gateway/channels.
- Credentialed third-party agent CI.
- Windows interactive ConPTY proof.
- New user-facing TUI commands or changed default runtime behavior.

## Review Resolutions

| finding | resolution |
|---|---|
| D1 | Replaced hidden env-gated release-binary path with `//go:build tui_smoke`; release builds do not contain the smoke entrypoint. |
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
| D14 | Added single redaction helper for all PTY/stdout/stderr failure logs, covering real home/workspace/temp/socket/executable paths and prompt/trust bodies. |
