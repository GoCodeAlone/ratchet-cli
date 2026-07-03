### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
None.

**Findings (Important):**
- `D1` [Security/privacy at architecture level] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:55]: The hidden `RATCHET_TUI_SMOKE=1` path ships new runtime behavior in the production binary, and "undocumented" is not a security boundary. This conflicts with the policy posture that deferred automation and mutation-capable surfaces need explicit boundaries, and it makes any process/environment injection capable of launching a mock-backed daemon/TUI path. Recommendation: move the smoke entrypoint behind a non-release build tag, or make it a deliberately named internal test binary compiled only by tests; if it must ship, document exact authorization, listener binding, env inheritance, and abuse-case limits.
- `D2` [Multi-component validation] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:59]: The design claims to drive the same path a user sees, but the proposed in-process daemon on an isolated listener bypasses the normal `runInteractive` path through `client.EnsureDaemon`, auto-daemon start, Unix socket, pid handling, and version compatibility checks used in `cmd/ratchet/main.go:97` and `internal/client/client.go:41`. That can close TUI rendering proof while leaving the current default launch/manual gap partly intact. Recommendation: either add a second proof for normal `ratchet` launch against temp seeded state, or explicitly narrow the docs claim to "TUI event loop with mock daemon" and keep auto-daemon launch evidence separate.
- `D3` [Repo-precedent conflicts] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:63]: The design says to extract the daemon test harness into a non-test helper, but the current harness is deeply `_test.go` shaped: it takes `*testing.T`, registers `t.Cleanup`, directly inserts `e2e-mock`, and starts test gRPC plumbing in `internal/daemon/testharness_test.go:46`. Moving that into production code without a precise API risks importing test lifecycle assumptions and DB shortcuts into runtime packages. Recommendation: define the production-safe helper contract in the design: no `testing.T`, explicit `context`/cleanup function, no direct DB seeding unless named as smoke-only, local-only listener binding, and tests proving it is unreachable when the env/build tag is absent.

**Findings (Minor):**
- `D4` [Missing failure modes] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:111]: The security review handles temp homes, but not the failure where the hidden path accidentally inherits or connects to real daemon/provider state. `docs/policy-matrix.md:58` treats trust rules, grant patterns, prompts, and operational metadata as sensitive. Recommendation: require an assertion that smoke mode refuses non-temp state/socket paths and redacts captured PTY failure output for `/trust list` and prompt frames.
- `D5` [Declared integration proof] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:123]: The multi-component table lists boundaries, but it does not use the required declared-integration status vocabulary (`runtime-integrated`, `config-only`, `deferred`). This makes the Windows claim and hidden smoke daemon claim easier to overread. Recommendation: add an integration matrix marking TUI/daemon/mock provider/slash commands/shortcuts as `runtime-integrated`, docs as `config-only`, and Windows interactive PTY as `deferred`.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | The design follows the TUI-proof goal, but the shipped hidden runtime gate conflicts with the repo's policy-boundary caution around new automation/runtime surfaces. |
| Assumptions under attack | Finding | A1 and A3 are load-bearing: if hidden env gates are not acceptable or Windows interactive proof is required, the proposed proof and docs claim collapse. |
| Repo-precedent conflicts | Finding | Existing daemon harness code is `_test.go`/`testing.T` based, while the design proposes production extraction without specifying a safe runtime API. |
| Artifact-class precedent | Clean | Existing sibling artifacts support the general shape: `cmd/ratchet/harness_smoke_test.go` builds a real binary and `internal/tui/pty_test.go` drives PTY TUI tests. |
| YAGNI violations | Clean | No obvious future-only feature expansion; the slice avoids ConPTY, external agents, SDK work, and new TUI features. |
| Missing failure modes | Finding | The design covers hangs, but not accidental real-state/real-daemon connection or sensitive PTY failure output. |
| Security/privacy at architecture level | Finding | Hidden shipped smoke mode is a new trust boundary; env gating and lack of public docs are insufficient controls. |
| Infrastructure impact | Clean | No cloud, IAM, migrations, secrets, or deploy order impact; local temp files/processes only. |
| Multi-component validation | Finding | The proof validates built binary plus TUI plus daemon RPC, but bypasses normal auto-daemon startup and socket/version paths. |
| Declared integration proof | Finding | The design has a boundary table but not a declared integration matrix with `runtime-integrated`/`config-only`/`deferred` markings. |
| Contributed UI rendering proof | Clean | No plugin-contributed UI into a host shell is claimed; this is the primary TUI process itself. |
| Rollback story | Clean | Source revert plus patch release is adequate for this local runtime/test/docs change, assuming the hidden gate issue is resolved. |
| Simpler alternative not considered | Finding | The design rejects docs-only and DB seeding, but does not consider a non-release smoke binary/build-tag path that avoids shipping a hidden mode. |
| User-intent drift | Finding | The selected slice is correct, but the Windows/cross-platform mandate can be overread because Windows interactive proof remains explicitly deferred. |
| Existence/runtime-validity | Clean | Referenced docs and consumer surfaces exist: TUI slash commands, shortcuts, `cmd/ratchet/harness_docs_test.go`, and existing PTY/binary smoke tests are present. |

**Options the author may not have considered:**
1. Build-tagged smoke binary: compile `ratchet` with `-tags tui_smoke` only inside the PTY test. This keeps the proof close to the real binary and TUI packages while preventing release artifacts from carrying a hidden env-triggered runtime path. Trade-off: docs must honestly say the credential-free proof uses a smoke-tagged binary, not an unmodified release binary.
2. Public-path temp daemon seeding: start the normal daemon in a temp home, add the mock provider through the daemon RPC or a narrowly scoped test seeding command, then launch plain `ratchet` through the PTY. This better proves `EnsureDaemon`/socket/default launch behavior, but it is more brittle and needs careful cleanup.
3. Split proof rows: keep this design for TUI event-loop proof, and add a separate smaller smoke for default daemon startup/version/socket compatibility. This avoids overclaiming "full TUI launch" while still retiring the manual-only rendering gap.

**Verdict reasoning:** The design targets the right gap and uses real TUI/PTY mechanics, but it currently overclaims the normal user path and introduces a hidden shipped runtime surface without a strong trust boundary. Those are Important design issues, not implementation nits. Status is FAIL until the hidden entrypoint is made non-release or fully bounded, and the validation/docs matrix distinguishes real default-launch proof from smoke-mode TUI proof.

## Cycle 2

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
None.

**Findings (Important):**
- `D6` [Multi-component validation / User-intent drift] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:111,128]: D2 is improved but not fully closed. The design says docs should update the TUI row from manual to "automated Unix PTY smoke plus Windows compile proof", while the public row is for command `ratchet` and the interactive chat/shortcut proof is explicitly a `-tags tui_smoke` binary, not the normal `runInteractive`/`EnsureDaemon` path in `cmd/ratchet/main.go:97`. Recommendation: require docs and docs guard text to split `ratchet` release-shaped startup/onboarding proof from `tui_smoke` interactive proof, using wording that cannot be read as release-binary credential-free chat coverage.
- `D7` [Security/privacy at architecture level / Missing failure modes] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:62-64,80-98]: D4 is only partially resolved. Temp `HOME`/`XDG_STATE_HOME` protects daemon home state, but the design does not require a temp working directory; current TUI launch creates sessions from `os.Getwd()` and daemon chat discovers project instructions/hooks from session working dirs (`cmd/ratchet/main.go:125`, `internal/daemon/chat.go:151`, `internal/hooks/hooks.go:105`). It also permits `127.0.0.1:0` for an unauthenticated gRPC daemon. Recommendation: require `cmd.Dir` and session `WorkingDir` to be a temp directory with no project `.ratchet` or instruction files, assert no real workspace/home paths appear in captured frames/logs, and prefer a temp Unix socket or otherwise justify localhost TCP exposure.
- `D8` [Existence/runtime-validity / Repo-precedent conflicts] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:64-65]: The design depends on an "exported constructor seam" for `client.Client` against an arbitrary listener, but the current `client.Client` has unexported fields and only `Connect()` to `daemon.SocketPath()` (`internal/client/client.go:20-38`), while `tui.Run` requires a concrete `*client.Client` (`internal/tui/app.go:517`). This is a real implementation contract, not a detail. Recommendation: define the exact production-safe client API in the design, including target format, close ownership, credentials, context, and whether it is general-purpose or `tui_smoke`-tagged only.

**Findings (Minor):**
- `D9` [Rollback story] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:178-183]: The rollback section notes the risk of a future release accidentally including `tui_smoke`, but the design only requires a normal no-tag negative assertion, not a release/goreleaser artifact guard. Recommendation: add a release-shaped verification that the published binary does not expose the smoke command/flag or smoke symbol.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | The design follows Windows/minimal-duplication guidance, but docs wording still risks overstating release-binary TUI proof. |
| Assumptions under attack | Finding | A1 remains load-bearing: a build-tagged binary is acceptable only if public docs never imply byte-for-byte release interactive proof. |
| Repo-precedent conflicts | Finding | Current TUI and slash-command code take concrete `*client.Client`; the proposed arbitrary-listener client seam is not yet specified. |
| Artifact-class precedent | Clean | Existing artifact class matches: binary smoke in `cmd/ratchet`, PTY proof in `internal/tui`, docs guard in `harness_docs_test.go`. |
| YAGNI violations | Clean | The slice still avoids ConPTY, visual snapshots, external provider CI, and new TUI features. |
| Missing failure modes | Finding | Temp home is covered, but inherited working directory/project instruction/hook state and localhost daemon exposure are not. |
| Security/privacy at architecture level | Finding | Sensitive project instructions/hooks and unauthenticated local gRPC exposure need explicit smoke-mode containment. |
| Infrastructure impact | Clean | No cloud, IAM, migrations, secrets, registry, or deploy-order impact is introduced. |
| Multi-component validation | Finding | The smoke proves TUI + daemon RPC, but docs can still overclaim the normal `ratchet`/`EnsureDaemon` path. |
| Declared integration proof | Clean | D5 is resolved with `runtime-integrated`, `config-only`, and `deferred` rows. |
| Contributed UI rendering proof | Clean | No host-shell contributed UI/plugin route is claimed. |
| Rollback story | Finding | Source revert is adequate, but accidental release inclusion of `tui_smoke` lacks a release artifact guard. |
| Simpler alternative not considered | Finding | The design does not consider a separate `cmd/ratchet-tui-smoke` test binary that avoids adding any conditional command surface to `cmd/ratchet`. |
| User-intent drift | Finding | The selected slice is valid, but public wording can drift from "verified TUI" into stronger release-binary chat proof than this slice delivers. |
| Existence/runtime-validity | Finding | The required arbitrary-target `client.Client` constructor does not exist and must be specified before implementation. |

**Options the author may not have considered:**
1. Dedicated smoke command package: build a separate `./cmd/ratchet-tui-smoke` or `./internal/tui/smokecmd` binary only in tests. It can call the same TUI packages without adding any conditional command/flag to the release `cmd/ratchet` surface. Trade-off: slightly farther from the release binary, but simpler rollback and no accidental user-facing command path.
2. Interface-first TUI seam: define a narrow TUI daemon-client interface and adapt `*client.Client` to it. This makes smoke testing easier without exporting a broad arbitrary gRPC constructor, but it touches more TUI code and should be scoped carefully.

**Verdict reasoning:** Cycle 2 resolves the big D1/D3/D5 shape problems and mostly addresses D2/D4, but it still leaves Important ambiguity around what the public docs may claim, how the smoke path avoids real project state, and how the tagged binary can actually construct the concrete client required by `tui.Run`. Status remains FAIL until those contracts are tightened.
