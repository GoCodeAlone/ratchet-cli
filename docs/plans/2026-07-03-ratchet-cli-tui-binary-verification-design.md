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
| Verify real boundaries, not demos. | Build the real `ratchet` binary, launch it in a PTY where supported, and drive the same Bubble Tea TUI path a user sees. |
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
| A. Hidden credential-free TUI smoke mode | Add a test-only hidden command/env path that starts a real in-process daemon with the mock provider, then runs the normal TUI against it. Built binary is driven by PTY. | Strong proof without credentials; adds a hidden harness surface that must stay undocumented for users. | Choose. |
| B. Seed production daemon state then launch normal `ratchet` | Use temp home and direct DB/config writes so the normal auto-daemon finds a mock provider. | Fewer hidden hooks, but brittle because tests depend on on-disk daemon internals and background process lifecycle. | Reject. |
| C. Keep in-process model tests only | Expand `internal/tui` unit tests for commands and shortcuts. | Portable and cheap, but does not satisfy binary execution proof. | Reject as insufficient. |

## Design

### Hidden Smoke Entrypoint

- Add an internal CLI path gated by `RATCHET_TUI_SMOKE=1`; no public README
  command is advertised.
- Entrypoint constructs a real daemon `Service` with temp local state and a
  default mock provider, starts gRPC on an isolated local listener, builds a
  `client.Client` against that listener, creates a session, and calls
  `tui.Run`.
- Reuse existing daemon test-harness assembly where possible by extracting a
  small non-test helper only for local mock service creation; avoid importing
  `_test.go` helpers into production.
- Normal `ratchet` behavior is unchanged when `RATCHET_TUI_SMOKE` is absent.

### PTY Binary Smoke

- Add `internal/tui/tui_binary_smoke_unix_test.go` without the `integration`
  tag.
- Test builds the real `./cmd/ratchet` binary, sets temp `HOME`,
  `XDG_STATE_HOME`, `TERM=xterm-256color`, and `RATCHET_TUI_SMOKE=1`, then
  starts the binary in a PTY.
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
  smoke plus Windows compile proof.
- Update `README.md` harness table with the same wording.
- Extend `cmd/ratchet/harness_docs_test.go` so docs must mention TUI binary
  smoke, slash commands, shortcuts, temp home, mock provider, and Windows
  compile proof.

## Security Review

| risk | control |
|---|---|
| Hidden smoke mode becomes a user-facing bypass. | Gate on explicit env var, keep command undocumented, use temp-local mock provider only, and avoid loading credentials. |
| Test leaks real home/provider state. | Set temp `HOME`/`XDG_STATE_HOME`; no live daemon socket or real provider config is used. |
| PTY test hangs in CI. | Per-read deadline, process kill cleanup, bounded waits, and no external network/provider dependency. |
| Sensitive prompts in logs. | Use harmless deterministic prompts; failure output is local test output only. |
| Platform mismatch. | Unix PTY proof is explicitly Unix-only; Windows claim is limited to build/noninteractive smoke. |

## Infrastructure Impact

No cloud resources, secrets, migrations, registry entries, Homebrew changes, or
production deploy. Local test-only process and temp files only. Runtime behavior
changes only when `RATCHET_TUI_SMOKE=1` is set.

## Multi-Component Validation

| boundary | proof |
|---|---|
| Built binary to TUI | PTY test launches compiled `ratchet` and reads rendered frames. |
| TUI to daemon gRPC | Smoke service uses real daemon service/client RPCs for provider list, session tree, trust mode/rules, and chat send. |
| TUI to mock provider | Chat prompt reaches built-in mock provider and streams a response. |
| Slash commands | PTY submits slash commands through the input widget and asserts rendered system output/navigation. |
| Shortcuts | PTY sends control keys and asserts branch tree/pane transitions. |
| Docs to tests | Docs guard fails if automated TUI smoke evidence is removed. |
| Windows build | Cross-build commands prove the hidden harness changes compile for Windows. |

## Rollback

Revert the hidden smoke entrypoint, extracted mock-service helper, PTY test, and
docs updates. No data migration exists. If a release includes the hidden
environment gate, rollback is a normal source revert and patch release.

## Assumptions

| id | assumption | challenge | fallback |
|---|---|---|---|
| A1 | A hidden env-gated smoke path is acceptable test plumbing. | It slightly expands the binary surface. | Move the entrypoint behind a build tag if review rejects runtime-gated test plumbing. |
| A2 | Built-in mock provider response is stable enough for PTY assertions. | Mock wording may change. | Assert broad substrings plus stream completion markers, not exact full transcript. |
| A3 | Unix PTY proof plus Windows compile proof is sufficient cross-platform honesty. | User may require Windows interactive proof. | Add a separate Windows ConPTY design if a Windows runner is available. |
| A4 | Extracting daemon mock service construction is lower risk than seeding production DB state. | Helper extraction may touch daemon internals. | Keep helper private/internal and covered by existing daemon tests. |

## Self-Challenge

1. Laziest solution: update docs to cite existing `integration` PTY tests.
   Rejected because those require Ollama and do not close the CI-proof gap.
2. Fragile assumption: A1. Mitigation is an explicit env gate plus no public
   docs; fallback is a build-tagged harness.
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
