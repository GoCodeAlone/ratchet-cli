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
