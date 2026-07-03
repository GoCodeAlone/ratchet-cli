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

## Cycle 3

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
None.

**Findings (Important):**
- `D10` [Existence/runtime-validity / Repo-precedent conflicts] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:87-92]: D8 is not fully resolved. The proposed `ConnectSmokeUnix(ctx context.Context, socketPath string)` contract says the socket must be "under the caller-provided temp root," but the API does not accept that root, so the constructor cannot enforce its own stated containment rule. Existing `internal/client.Client` has unexported fields and `Connect()` hardcodes `daemon.SocketPath()` in `internal/client/client.go:20-38`, so this exact seam matters. Recommendation: change the contract to `ConnectSmokeUnix(ctx, tempRoot, socketPath string)` or an options struct, and require validation of `Abs`, `EvalSymlinks`/clean path containment, socket mode, and `unix://` only before constructing the client.
- `D11` [Missing failure modes / Infrastructure impact] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:127-137]: The release-shaped startup smoke launches normal `ratchet`, which enters `runInteractive` and calls `client.EnsureDaemon()`; that can fork a background daemon under the temp home. The design only says "exit cleanly" and does not require daemon stop/kill, pid/socket cleanup, or a post-test assertion that no temp-home daemon remains. This can leak local processes in CI and leave temp state active after the PTY exits. Recommendation: require the test to run `ratchet daemon stop` with the same temp env, or read the temp pid file and terminate/wait; assert the socket and pid file are gone.
- `D12` [Rollback story / Infrastructure impact] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:77-81,210-216]: D9 is only partially resolved. Checking that the GoReleaser target/package list does not mention `cmd/ratchet-tui-smoke` is weaker than proving release artifacts do not contain it. Current `.goreleaser.yaml` archives only build id `ratchet`, but the design should guard the artifact boundary, not just the config text. Recommendation: require `goreleaser check` plus a snapshot release/archive inspection that no archive, checksum entry, Homebrew cask binary, or release asset contains `ratchet-tui-smoke`.

**Findings (Minor):**
- `D13` [Artifact-class precedent / Existence/runtime-validity] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:100-106]: The new non-integration test is placed in `internal/tui`, but the existing repo-root discovery and PTY build helper live in `internal/tui/pty_test.go` behind the `integration` build tag, so the new test cannot reuse them in an untagged build. The design says to build `./cmd/ratchet-tui-smoke` but does not state that the command must run from repo root, unlike existing binary smoke precedent in `cmd/ratchet/acp_client_binary_test.go:17-28`. Recommendation: require an untagged shared helper or explicit repo-root discovery for the new test.
- `D14` [Missing failure modes / Security/privacy at architecture level] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:122-125,169-175]: Failure-output redaction excludes trust bodies and deterministic prompt frames, but it does not cover daemon logs from startup/onboarding or provider/setup text that may include temp paths, executable paths, or environment-derived metadata. Recommendation: require all PTY/stderr/stdout failure dumps to pass through one redaction helper before logging, then assert no real home, workspace, socket path, or instruction filename survives.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | The design follows the workspace/repo goal of real boundary proof, but process cleanup and release-artifact guards are not strong enough for cross-platform release hygiene. |
| Assumptions under attack | Finding | A1 is now bounded by non-release docs, but the client seam assumes containment it cannot enforce with the proposed signature. |
| Repo-precedent conflicts | Finding | Existing `client.Client` only connects to the default daemon socket, and existing untagged binary-smoke tests set repo root explicitly; the design misses both exact precedents. |
| Artifact-class precedent | Finding | Binary build tests already use repo-root-aware helpers, while the proposed untagged `internal/tui` test would need its own helper because current PTY helpers are integration-tagged. |
| YAGNI violations | Clean | The slice still avoids ConPTY, snapshots, new user-facing commands, external-provider CI, and broader policy work. |
| Missing failure modes | Finding | Normal startup smoke can leave a background daemon alive, and failure logs are not globally redacted. |
| Security/privacy at architecture level | Finding | D7 is mostly resolved for temp state and Unix socket exposure, but log redaction and process lifetime boundaries remain incomplete. |
| Infrastructure impact | Finding | No cloud/IAM impact exists, but the local daemon process/socket/pid lifecycle is real infrastructure for CI and is not cleaned up in the design. |
| Multi-component validation | Clean | D6 is resolved: the design splits release-shaped startup/onboarding proof from non-release interactive TUI proof. |
| Declared integration proof | Clean | The integration matrix uses `runtime-integrated`, `config-only`, and `deferred` classifications. |
| Contributed UI rendering proof | Clean | No host-shell contributed UI/plugin route is claimed; this is the primary TUI process. |
| Rollback story | Finding | Source rollback is described, but accidental release inclusion is guarded by config inspection rather than artifact inspection. |
| Simpler alternative not considered | Finding | The design considers a dedicated smoke binary, but not a narrower untagged internal helper package plus test-only `main` generated under `t.TempDir()` to avoid adding a persistent module package. |
| User-intent drift | Clean | The revised docs boundaries avoid overclaiming Windows interactive or release-binary credential-free chat proof. |
| Existence/runtime-validity | Finding | The smoke client API cannot validate its stated temp-root rule, and the proposed untagged `internal/tui` test lacks an untagged repo-root/PTTY helper contract. |

**Options the author may not have considered:**
1. Test-generated smoke main: have the untagged PTY test write a tiny `main.go` into `t.TempDir()` that imports internal smoke helper packages and build it with `-tags tui_smoke`. This avoids adding a permanent `cmd/ratchet-tui-smoke` package that broad package patterns might discover, while still keeping release `cmd/ratchet` untouched. Trade-off: the test is a little more complex and the generated source must stay small.
2. Interface-first TUI client seam: define the minimal daemon-client interface consumed by TUI and adapt `*client.Client` to it. Then the smoke path can use a test/smoke client wrapper without exporting another concrete constructor. Trade-off: broader TUI refactor, but it reduces pressure on `internal/client` to support arbitrary endpoints.

**Verdict reasoning:** Cycle 3 fixes the major Cycle 2 documentation split and removes the shipped hidden env gate, so D6 and most of D7 are resolved. But D8 still has a concrete API-contract bug, the release-shaped startup smoke can leak a background daemon, and D9's guard is weaker than an artifact-level release check. These are actionable Important findings, so the design remains FAIL.

## Cycle 4

### Adversarial Review Report

**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D15` [Security/privacy at architecture level / Missing failure modes] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:90-97]: `ConnectSmokeUnix` validates the temp root and socket parent with `EvalSymlinks`, but does not require `Lstat`/socket-type validation on the final socket path or full final-path symlink resolution. A symlinked final component can satisfy parent containment while dialing outside the temp root. Recommendation: require `Lstat(socketPath)` rejects symlinks, `ModeSocket` is present, permissions are `0600`, full resolved socket path remains under resolved `tempRoot`, and validation is repeated immediately before dialing.
- `D16` [Project-guidance conflicts / Existence/runtime-validity] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:57-60,88-90,161-165]: The design says production remains portable and Windows proof is honest, but the new smoke command/client files are only specified as `//go:build tui_smoke`, not `tui_smoke && !windows`. The Windows gate only cross-builds `./cmd/ratchet`, so smoke-tagged code can rot or accidentally compile/run ambiguously on Windows despite Unix-socket-only semantics. Recommendation: mark smoke command/client with explicit Unix-only build tags and add a negative or documented `GOOS=windows go list/build -tags tui_smoke` expectation.
- `D17` [Declared integration proof / Rollback story / Infrastructure impact] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:80-82,223,226-233]: D12 is asserted but still not fail-closed. The design says inspect snapshot archives/checksums/Homebrew artifacts/release assets, but does not define the snapshot command, produced artifact paths, how `homebrew_casks` are generated without publishing/token side effects, or what happens if GoReleaser produces no Homebrew/release artifact in snapshot mode. That can degrade back into config-text checking. Recommendation: specify exact command and artifact manifest checks, e.g. `goreleaser release --snapshot --clean --skip=publish`, enumerate `dist/*`, archive member lists, `checksums.txt`, generated cask/tap files if present, and fail if an expected artifact class is absent rather than silently clean.
- `D18` [Security/privacy at architecture level / Missing failure modes] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:77-83,129-134,140-150,176-179]: D14 is only partially resolved. The redaction requirement is scoped to PTY/stdout/stderr failure logs, but this design also adds build failures, GoReleaser snapshot output, daemon-stop output, and docs-guard output that can include repo root, temp dirs, executable paths, socket paths, and generated artifact paths. Recommendation: require one redaction helper for every test failure payload before `t.Log`/`t.Fatalf`, including build/snapshot/daemon cleanup/docs-guard outputs, not only PTY frames.

**Findings (Minor):**
- `D19` [Simpler alternative not considered / Artifact-class precedent] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:45-51,246-255]: Cycle 3's test-generated smoke `main.go` alternative is not carried into the revised approaches. A permanent `cmd/ratchet-tui-smoke` package is defensible, but the design does not document why a generated test-only main was rejected after it was raised. Recommendation: add a short rejection row comparing permanent command package vs generated test main.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | Windows honesty is mostly addressed, but smoke-tagged Unix-only code lacks explicit `!windows` boundaries. |
| Assumptions under attack | Finding | A1/A3 remain acceptable only if non-release and Unix-only limits are mechanically enforced, not just documented. |
| Repo-precedent conflicts | Finding | Existing binary smoke tests build from repo root and print raw build output; new universal redaction must cover that precedent rather than only PTY logs. |
| Artifact-class precedent | Finding | Existing GoReleaser config has concrete archives/checksums/Homebrew cask surfaces, but the design does not specify artifact-level enumeration/fail-closed checks. |
| YAGNI violations | Clean | No new user feature, ConPTY, visual snapshot, external provider CI, or broader policy work is added. |
| Missing failure modes | Finding | Final socket symlink/dial escape and non-PTY failure-output leakage remain under-specified. |
| Security/privacy at architecture level | Finding | Temp isolation is improved, but socket final-component validation and all-output redaction are incomplete. |
| Infrastructure impact | Finding | No cloud impact; local daemon cleanup is now covered, but release artifact/tap guard behavior remains ambiguous. |
| Multi-component validation | Clean | Release startup and smoke TUI/daemon/mock-provider boundaries are now split clearly. |
| Declared integration proof | Finding | Integration matrix exists, but GoReleaser snapshot/Homebrew/release-asset proof lacks executable artifact criteria. |
| Contributed UI rendering proof | Clean | No plugin-contributed host UI is involved; PTY-rendered TUI proof covers the relevant UI surface. |
| Rollback story | Finding | Source rollback is fine, but accidental release inclusion still depends on an underspecified artifact guard. |
| Simpler alternative not considered | Finding | Test-generated smoke main from Cycle 3 was not explicitly evaluated in the revised alternatives. |
| User-intent drift | Clean | The slice still targets the documented manual TUI proof gap without claiming Windows interactive proof. |
| Existence/runtime-validity | Finding | D10/D11/D13 are resolved at contract level; D12/D14 still need fail-closed artifact/output mechanics. |

**Options the author may not have considered:**
1. Generated smoke main under `t.TempDir()`: keep smoke daemon/client helpers internal, write a tiny test-only `main.go`, then build it with `-tags tui_smoke`. This avoids a permanent `cmd/ratchet-tui-smoke` package and reduces accidental package/release discovery risk. Trade-off: the test becomes more complex and must keep generated source minimal.
2. Artifact manifest test helper: after GoReleaser snapshot, build a normalized manifest of `dist/` files, archive members, checksum entries, cask/tap files, and release metadata, then assert allowed/forbidden names against that manifest. This makes D12 mechanical and reusable.

**Verdict reasoning:** Cycle 4 resolves the literal D10, D11, and D13 wording, and it largely fixes the docs overclaiming problem from earlier cycles. It still leaves Important design gaps around socket containment, Unix-only build boundaries, fail-closed release artifact inspection, and redaction scope. Because unresolved Important findings remain, Status is FAIL.

## Cycle 5

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
None.

**Findings (Important):**
- `D20` [User-intent drift / Multi-component validation] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:202-212,310]: The docs guard requirement only says public docs must "mention" the release-shaped `ratchet` boundary does not claim credential-free chat. That can still pass if docs also contain contradictory wording like "full TUI automated for `ratchet`" elsewhere. This reopens the D6 overclaim class. Recommendation: require explicit forbidden-phrase/assert-not checks for release-binary credential-free chat/full interactive coverage, alongside positive checks for the split `ratchet` vs `ratchet-tui-smoke` evidence.
- `D21` [Declared integration proof / Rollback story / Existence-runtime-validity] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:181-198; .goreleaser.yaml:44-56]: D17 is still not fully fail-closed for Homebrew/release asset proof. The design allows the guard to "record the exact GoReleaser snapshot behavior" when cask/tap material is unavailable, but `.goreleaser.yaml` declares `homebrew_casks`; recording absence is not the same as proving the cask/release boundary cannot include `ratchet-tui-smoke`. Recommendation: define a deterministic fallback check for skipped Homebrew/release classes, such as parsing `.goreleaser.yaml` cask `ids`/`binaries` and release build IDs, and fail if any publishable config surface can reference anything other than `ratchet`.
- `D22` [Security/privacy at architecture level / Missing failure modes] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:136-143,183-188; .goreleaser.yaml:30-31]: The universal redaction/assertion rule conflicts with release artifact inspection: GoReleaser intentionally packages `RATCHET.md`, but the design says all artifact-manifest failure payloads go through the helper and failure assertions reject known instruction filenames from the source checkout. That can either make the artifact guard fail on a valid release archive or force redaction that hides the very archive contents being inspected. Recommendation: scope "instruction filename must not appear" assertions to PTY/TUI/daemon output, and separately allow expected release archive members like `RATCHET.md` in artifact manifests.
- `D23` [Project-guidance conflicts / Existence-runtime-validity] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:78-83,170-177,257]: D16 is only partially resolved. The smoke command/client are tagged `tui_smoke && !windows`, but the negative Windows check is only `GOOS=windows GOARCH=amd64 go list`; it does not cover `arm64`, and it does not include the "go list/build expectation" requested in Cycle 4. Recommendation: add both `windows/amd64` and `windows/arm64` negative checks and specify whether `go build -tags tui_smoke ./cmd/ratchet-tui-smoke` must fail with the same no-buildable-files class.

**Findings (Minor):**
- `D24` [Declared integration proof] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:257]: The integration matrix classifies "Windows smoke package boundary" as `runtime-integrated`, but the proof is a negative `go list` failure, not runtime integration. Recommendation: classify this row as `config-only` or `negative-build-boundary` wording inside the proof column, while keeping Windows release binary builds as `runtime-integrated` or build-integrated evidence.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | Windows honesty is improved with `tui_smoke && !windows`, but the Windows negative check is narrower than the Cycle 4 requested build/list expectation. |
| Assumptions under attack | Finding | A1 remains acceptable only if docs guards reject contradictory release-chat claims, not merely mention the intended boundary. |
| Repo-precedent conflicts | Finding | GoReleaser precedent intentionally archives `RATCHET.md`, conflicting with the proposed universal instruction-filename rejection across artifact-manifest payloads. |
| Artifact-class precedent | Finding | Existing release config has concrete Homebrew cask and archive surfaces, but snapshot-missing cask output can still be accepted by "record behavior" rather than mechanically checked. |
| YAGNI violations | Clean | The design still avoids ConPTY, visual snapshot framework, external-provider CI, and new user-facing TUI behavior. |
| Missing failure modes | Finding | The redaction/assertion policy misses the valid-artifact-member case and can create false failure or blind redaction. |
| Security/privacy at architecture level | Finding | D15 socket containment is resolved on paper, but the all-output redaction rule is overbroad for release manifests and under-specific about where instruction filenames are sensitive. |
| Infrastructure impact | Finding | No cloud impact exists, but local release artifact/Homebrew guard behavior remains ambiguous enough to affect release rollback confidence. |
| Multi-component validation | Finding | TUI smoke boundaries are split, but docs validation can still pass contradictory public claims. |
| Declared integration proof | Finding | The GoReleaser/Homebrew proof and Windows negative boundary are not classified or enforced precisely enough. |
| Contributed UI rendering proof | Clean | No host-shell contributed UI/plugin route is claimed; this is the primary Bubble Tea TUI. |
| Rollback story | Finding | Rollback names bad cask/tap/checksum removal, but the guard still may not prove those publishable surfaces before release. |
| Simpler alternative not considered | Clean | D19 is resolved: generated test-only smoke main is explicitly considered and rejected. |
| User-intent drift | Finding | The slice targets the TUI proof gap, but weak docs negative assertions can drift back into overclaiming full release-binary TUI proof. |
| Existence/runtime-validity | Finding | The GoReleaser command exists locally, but the design still lacks a deterministic fallback for snapshot-skipped cask/release outputs and underspecifies Windows negative build coverage. |

**Options the author may not have considered:**
1. Split redaction policies by evidence class: use strict "no instruction filenames" only for runtime PTY/daemon outputs, and use an allowlisted archive-manifest policy for release artifacts where `RATCHET.md` is expected.
2. Make docs guard bidirectional: require positive text for both proof rows and explicit negative assertions that public docs do not contain forbidden release-chat/full-TUI phrases for untagged `ratchet`.

**Verdict reasoning:** Cycle 5 resolves much of D15-D19 in text, especially final socket checks, Unix-only build tags, and explicit rejection of generated smoke main. It still leaves Important gaps in the release artifact guard, docs overclaim prevention, and redaction scope. Because unresolved Important findings remain, Status is FAIL.

## Cycle 6

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
None.

**Findings (Important):**
- `D25` [Multi-component validation / User-intent drift] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:126-135,266-273; docs/harness-emulation.md:171-181; README.md:97-104]: The smoke only drives `/help`, `/provider list`, `/mode`, `/trust list`, `/tree`, exit, and shortcuts, but public docs expose broader TUI slash behavior including `/trust allow`, `/trust deny`, `/trust reset`, `/trust persist`, `/trust grants`, and `/trust revoke`. Claiming "slash commands" are binary-proven can still miss policy-sensitive slash-command paths that mutate or expose local trust metadata. Recommendation: either narrow docs to "selected slash commands" or require a command matrix that drives every publicly documented TUI trust/slash command through the PTY with temp-state assertions.
- `D26` [Existence/runtime-validity / User-intent drift] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:222-231,343]: D20 is only partially resolved. The design requires negative assertions against "phrases that imply" release-shaped credential-free chat/full interactive coverage, but does not define concrete forbidden strings/regexes or which public docs are scanned. A weak implementation could satisfy this with subjective checks and still allow contradictory table wording. Recommendation: list exact public-doc paths and forbidden regexes, including release-shaped `ratchet` + `credential-free chat`, `full TUI automated`, `slash/shortcut proof`, or `interactive chat` unless the same row/sentence names `ratchet-tui-smoke`.
- `D27` [Security/privacy at architecture level / Missing failure modes] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:74-76,146-150; internal/agent/instructions.go:31-77]: The instruction-file isolation list omits real discovery surfaces: `.github/copilot-instructions.md`, `.claude/`, `.github/instructions/`, `.cursor/rules/`, `.ratchet/instructions/`, `~/.ratchet/instructions.md`, and provider-specific `instructions.<provider>.md`. Temp `HOME` and temp `cmd.Dir` reduce risk, but the design's "known instruction filenames" rejection can still be implemented from the incomplete list and miss a future leak from the actual discovery contract. Recommendation: derive the forbidden runtime instruction patterns from `internal/agent/instructions.go` or name the complete file/dir set in the design.

**Findings (Minor):**
- `D28` [Artifact-class precedent / Infrastructure impact] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:187-211]: The GoReleaser guard is artifact-level now, but the deterministic fallback is still described as custom parsing of selected YAML fields. That is brittle against GoReleaser schema changes or future publish surfaces not in the named field list. Recommendation: require `goreleaser check` plus parsing through a YAML struct that fails on unknown publishable sections, or add an explicit "no unrecognized publish surfaces" check.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | The design follows Windows honesty and no hidden release path, but broad "slash commands" proof still drifts beyond what the selected PTY drive list proves. |
| Assumptions under attack | Finding | A1 remains acceptable only if docs guards are mechanical and the public claim is narrowed or full slash-command coverage is added. |
| Repo-precedent conflicts | Finding | `internal/agent/instructions.go` has a broader instruction discovery set than the design's isolation/rejection list. |
| Artifact-class precedent | Finding | Release guard shape follows GoReleaser artifacts, but fallback YAML parsing is under-specified for future publishable surfaces. |
| YAGNI violations | Clean | No ConPTY, visual snapshots, external-provider CI, new user-facing TUI features, or broader policy work is added. |
| Missing failure modes | Finding | Runtime leak checks omit several actual instruction file and directory names. |
| Security/privacy at architecture level | Finding | Trust/prompt redaction is addressed, but incomplete instruction-source rejection weakens the privacy boundary. |
| Infrastructure impact | Finding | No cloud/IAM impact exists, but local release artifact guard behavior still depends on brittle fallback parsing. |
| Multi-component validation | Finding | The PTY proof exercises real TUI/daemon/mock boundaries, but not the full documented slash-command surface. |
| Declared integration proof | Clean | D24 is resolved: Windows smoke package boundary is now config-only/negative-build proof, not runtime-integrated. |
| Contributed UI rendering proof | Clean | No plugin-contributed host-shell UI is claimed; this is the primary Bubble Tea TUI. |
| Rollback story | Clean | Source revert plus artifact/tap removal and patch release is adequate for this test/docs/smoke-binary slice. |
| Simpler alternative not considered | Finding | A table-driven TUI command matrix using existing public docs/help as source of truth is not considered. |
| User-intent drift | Finding | The user asked for shortcuts/slash commands broadly; selected-command proof may be over-reported as full slash-command proof. |
| Existence/runtime-validity | Finding | D21-D24 are mostly resolved, but docs negative checks and instruction-name checks are not concrete enough to be reliably implemented. |

**Options the author may not have considered:**
1. Source-of-truth slash matrix: generate or hand-maintain a small table from `internal/tui/commands` plus README/harness trust-command docs, then drive every command with safe temp-state inputs in the PTY smoke. This costs a little more test time, but prevents "slash commands verified" from meaning only a handpicked subset.
2. Narrowed claim path: keep the PTY smoke small and explicitly document "selected representative slash commands and core shortcuts." This is cheaper, but the harness gap remains for policy-sensitive trust mutation commands.

**Verdict reasoning:** Cycle 6 resolves the exact D20-D24 wording much better than Cycle 5: docs guards are bidirectional in intent, GoReleaser fallback now checks publishable ids/binaries, artifact manifests allow `RATCHET.md`, Windows negative checks cover amd64/arm64 list/build, and the Windows smoke boundary is no longer runtime-integrated. The revised design still has unresolved Important issues: it can overclaim slash-command proof, leaves docs-guard forbidden phrases too subjective, and uses an incomplete instruction-source list for privacy assertions. Status remains FAIL.

## Cycle 7

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
None.

**Findings (Important):**
- `D29` [Multi-component validation / User-intent drift] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:156-173; docs/harness-emulation.md:173; README.md:90-91; internal/tui/commands/trust.go:21-35]: D25 is only partially resolved. The matrix now covers every documented `/trust` subcommand, but it still does not cover the full documented TUI trust slash surface because `/mode <mode>` is documented and implemented for five modes (`conservative`, `permissive`, `locked`, `sandbox`, `custom`) while the PTY matrix only drives `conservative` and `locked`. Recommendation: either drive every documented `/mode` value through the PTY smoke or narrow the docs claim to "representative `/mode` values plus full `/trust` matrix."
- `D30` [Existence/runtime-validity / User-intent drift] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:255-270]: D26 is improved but still mechanically weak. The design names scanned docs and forbidden regexes, but the regexes are phrase-fragile: they catch `slash/shortcut proof` but not "slash commands and shortcuts are proven," catch `interactive chat proof` but not "interactive TUI chat," and most regexes do not encode the stated "unless the same row/sentence names `ratchet-tui-smoke`" exception. Recommendation: define normalized claim predicates rather than a few literal phrases, or add explicit regexes for common wording variants and a deterministic same-line/same-row `ratchet-tui-smoke` exception.
- `D31` [Artifact-class precedent / Infrastructure impact] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:211-235; .goreleaser.yaml:36-68]: D28 is not actually strict enough. The fallback parser's recognized top-level list omits current nonpublishable `changelog`, so a naive whitelist would fail this repo today, but "unrecognized publishable section" is undefined, leaving implementers to guess which unknown sections publish artifacts. GoReleaser also supports additional artifact/publish surfaces such as `publishers`, `nfpms`, and `sboms` in official docs, so a hand-picked list can miss future release outputs while still claiming fail-closed behavior. Recommendation: parse with GoReleaser's JSON schema/config loader if available, or maintain an explicit allowlist of all top-level sections classified as `publishable`, `artifact-producing`, or `nonpublishable`, failing unknown top-level keys until classified.

**Findings (Minor):**
- `D32` [Security/privacy at architecture level / Missing failure modes] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:277-279; internal/agent/instructions.go:31-77; internal/hooks/hooks.go:105-132]: The security review says the temp workdir contains no "instruction or hook files from `internal/agent/instructions.go`," but hook discovery lives in `internal/hooks/hooks.go`, not `instructions.go`. Temp home/workdir reduces practical risk, but the design conflates two discovery surfaces and could leave hook-path leak assertions out of the shared redaction helper. Recommendation: derive instruction deny patterns from `internal/agent/instructions.go` and hook deny patterns from `internal/hooks/hooks.go`, or explicitly state hooks are out of this test's runtime output assertions.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | The design follows Windows honesty and real-boundary proof, but D29/D30 still allow broader "verified slash commands" claims than the actual matrix proves. |
| Assumptions under attack | Finding | A1 remains load-bearing: a build-tagged smoke binary is acceptable only if docs and guards cannot overstate release-binary or full slash-surface proof. |
| Repo-precedent conflicts | Finding | Hook discovery precedent is in `internal/hooks/hooks.go`, while the design incorrectly ties hook-file isolation to `internal/agent/instructions.go`. |
| Artifact-class precedent | Finding | Release guard shape follows existing GoReleaser artifacts, but fallback parser strictness is under-specified against GoReleaser's broader artifact/publish section model. |
| YAGNI violations | Clean | No ConPTY, external provider CI, visual snapshots, new runtime commands, or broader policy work is added. |
| Missing failure modes | Finding | Missing `/mode` values and hook discovery redaction are not covered by current matrix/assertion language. |
| Security/privacy at architecture level | Finding | Instruction discovery is now source-derived, but hook discovery is conflated with instructions and should be separated. |
| Infrastructure impact | Finding | No cloud impact, but local release artifact confidence still depends on an ambiguous GoReleaser fallback parser. |
| Multi-component validation | Finding | PTY proof crosses TUI/daemon/mock boundaries, but not every documented trust mode path it can still imply. |
| Declared integration proof | Finding | GoReleaser snapshot artifacts are classified, but fallback parser semantics are not tied to all artifact-producing/publishing surfaces. |
| Contributed UI rendering proof | Clean | No plugin-contributed host-shell UI is claimed; this is direct Bubble Tea TUI rendering. |
| Rollback story | Clean | Source revert plus artifact/tap removal and patch release remains adequate if the release guard is made deterministic. |
| Simpler alternative not considered | Finding | The design does not consider deriving the PTY trust matrix directly from docs/code command tables to prevent missing `/mode` values. |
| User-intent drift | Finding | User asked for shortcuts/slash commands broadly; the design can still over-report partial mode coverage as full trust slash coverage. |
| Existence/runtime-validity | Finding | Docs paths and instruction source exist, but docs regexes and GoReleaser fallback parsing are not behaviorally precise enough. |

**Options the author may not have considered:**
1. Source-derived trust matrix: generate the smoke command matrix from `internal/tui/commands/trust.go` valid modes plus documented `/trust` rows, then compare it to public docs. This adds a small maintenance helper but prevents future mode/subcommand drift.
2. GoReleaser schema-backed guard: use GoReleaser's own config validation or published schema as the fallback parser source of truth, then layer ratchet-specific assertions on resolved artifact IDs/binaries. This avoids maintaining a partial top-level section taxonomy by hand.

**Verdict reasoning:** Cycle 7 resolves the obvious Cycle 6 text gaps for D25-D28 better than prior revisions, but not all of them are mechanically closed. The trust matrix still misses documented `/mode` values, the forbidden docs regexes remain easy to evade with ordinary wording, and the GoReleaser fallback parser is not strict in a well-defined way against real artifact/publish surfaces. These are Important design issues because they can let the implementation pass while still overclaiming TUI/slash proof or missing release artifact contamination. Status remains FAIL.

## Cycle 8

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D33` [Existence/runtime-validity] [design:157-177; `internal/tui/commands/trust.go`:82-129]: The trust slash-command matrix expects several mutation commands to render `smoke` scope, but current `/trust allow`, `/trust deny`, `/trust persist`, and `/trust revoke` responses only render the pattern, not the scope. The PTY assertions would either be impossible or would need an unstated follow-up `/trust list`/`/trust grants` after each mutation. Recommendation: change the matrix evidence to match current command output, or require a follow-up state-read assertion after each mutating command that proves the scope through daemon state.
- `D34` [Missing failure modes / multi-component validation] [design:121-140; `internal/tui/app.go`:420-464]: The PTY proof never requires a deterministic terminal size or frame-level assertions for the alt-screen layout. Substring checks can pass while `ctrl+s`/`ctrl+j` panes render off-screen, overlap, or leave the input unusable. Recommendation: set the PTY rows/cols explicitly and assert representative full frames for chat, sidebar, job panel, and tree states with header/status/input visible and bounded.
- `D35` [Infrastructure impact / rollback story] [design:216-250; `.github/workflows/ci.yml`:29-61; `.github/workflows/release.yml`:17-20]: The release artifact guard requires `goreleaser release --snapshot --clean --skip=publish`, but the design does not say where that guard runs or how GoReleaser is installed outside the tag-only release workflow. Current CI runs `go build`, Windows cross-build, and `go test -race ./...`, but no GoReleaser preflight. Recommendation: add an explicit CI/release-preflight job using `goreleaser/goreleaser-action`, or state that the guard is a required manual release gate and wire docs/tests so it cannot be mistaken for normal CI coverage.

**Findings (Minor):**
- `D36` [User-intent drift / artifact-class precedent] [design:129-180; `internal/tui/commands/commands.go`:134-170; `internal/tui/components/autocomplete.go`:37-60]: The design proves submitted slash commands but not TUI discoverability surfaces. `/help` omits persistent trust commands and `custom` mode, and autocomplete omits `/tree`, `/mode`, and `/trust`; the design's docs/code drift guard does not include those UI surfaces. Recommendation: either mark help/autocomplete out of scope, or add assertions that submitted-command support, `/help`, and autocomplete stay aligned.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | Release-safe/current proof is weakened because the GoReleaser guard has no declared CI or release-preflight execution surface. |
| Assumptions under attack | Finding | The design assumes substring PTY evidence is enough for pane/shortcut rendering and assumes mutation commands expose scope text they do not currently print. |
| Repo-precedent conflicts | Finding | Existing CI/Makefile default to `go test ./...` and do not install GoReleaser, while this design adds an unstated external-tool gate. |
| Artifact-class precedent | Finding | Existing binary smoke tests are explicit about process commands and output contracts; the new trust matrix has expected output that does not match command handlers. |
| YAGNI violations | Clean | The scope avoids ConPTY, new runtime features, external provider CI, and broader extension work. |
| Missing failure modes | Finding | PTY size/layout failure and pane overlap are not guarded, so shortcut proof can pass without real usable rendering. |
| Security/privacy at architecture level | Clean | Prior smoke-entrypoint, temp state, socket containment, and redaction concerns are addressed in the current design text. |
| Infrastructure impact | Finding | GoReleaser snapshot inspection is a real toolchain/CI impact but is described as if it were just local test logic. |
| Multi-component validation | Finding | TUI/daemon/mock proof is real, but visual state transitions are not proven beyond substrings. |
| Declared integration proof | Finding | The integration matrix classifies GoReleaser artifacts, but the design does not define the runner/environment that proves that integration before release. |
| Contributed UI rendering proof | Clean | No plugin-contributed host-shell UI is claimed; this is the primary Bubble Tea TUI. |
| Rollback story | Finding | Rollback names bad release artifacts, but the missing pre-release guard wiring means detection may occur only after tag publish. |
| Simpler alternative not considered | Finding | A smaller explicit `make release-check`/CI preflight using the existing GoReleaser action is not considered. |
| User-intent drift | Finding | "Slash commands" can be overread as full TUI slash UX while help/autocomplete discoverability remains stale. |
| Existence/runtime-validity | Finding | Several expected PTY strings do not exist in current trust command output. |

**Options the author may not have considered:**
1. Add a dedicated `release-check` CI job using `goreleaser/goreleaser-action` with `args: release --snapshot --clean --skip=publish`, then run the archive manifest guard as a script after it.
2. Split trust PTY assertions into command-output checks and state-read checks: mutation commands assert "Added/Persisted/Revoked ...", then `/trust list` or `/trust grants` proves scope and persistence.
3. Add a small TUI frame assertion helper that fixes PTY size and validates required regions rather than only searching stripped text.

**Verdict reasoning:** The design is much stronger than earlier cycles, but it still has open Important issues that can let implementation pass while proving the wrong thing: impossible trust-output expectations, weak shortcut/pane rendering proof, and an unwired release artifact guard. Status is FAIL until those contracts are made mechanically executable.

## Cycle 9

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D37` [Multi-component validation / User-intent drift] [design:41,136-139; `internal/tui/app.go`:169-185; `internal/tui/components/statusbar.go`:59]: The design names `ctrl+c`/`ctrl+d` as an existing binary-proof gap, but the PTY drive list only proves `/exit` or `ctrl+c`. It also omits `ctrl+t`, which is implemented and advertised in the status bar, and does not address advertised `ctrl+h` at all. Recommendation: define the shortcut source of truth and either prove every advertised/core shortcut (`ctrl+b`, `esc`, `ctrl+s`, `ctrl+j`, `ctrl+t`, `ctrl+c`, `ctrl+d`, plus fix/remove/prove `ctrl+h`) or narrow docs to the exact shortcuts covered.
- `D38` [Infrastructure impact / Artifact-class precedent] [design:207-222,247-253; `.github/workflows/ci.yml`:48-61; `cmd/ratchet/harness_smoke_test.go`:14-17]: The design relies on untagged built-binary startup smoke, but current CI runs `go test -race ./...` and existing binary smoke explicitly skips under race. The design adds `release-check`, but that only proves GoReleaser artifacts, not the release-shaped startup/onboarding PTY smoke. Recommendation: add a non-race focused CI step for the binary/TUI smoke tests or require the new startup smoke not to inherit the existing race skip.
- `D39` [Repo-precedent conflicts / Existence-runtime-validity] [design:187-205; `cmd/ratchet/main.go`:168-174; `internal/tui/commands/commands.go`:134-170; `internal/tui/components/autocomplete.go`:37-60]: Help/autocomplete alignment covers in-TUI `/help` and autocomplete, but leaves the public `ratchet help` "Slash commands" section stale. Since the release-shaped smoke already runs `ratchet help`, the design can still ship documented slash proof while the built CLI's own help omits `/tree`, `/mode`, and `/trust`. Recommendation: include `printUsage`/`ratchet help` in the slash discoverability contract and assert it matches the supported TUI slash surface or intentionally remove that stale section.

**Findings (Minor):**
- None.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | Workspace/repo guidance wants real, documented, Windows-honest proof; D37/D38 leave shortcut and CI proof overclaimable. |
| Assumptions under attack | Finding | The design assumes "core shortcuts" means the handpicked list, but current code/status UI advertises more. |
| Repo-precedent conflicts | Finding | Existing binary smoke skips under race and public `ratchet help` has its own slash surface not covered by the design. |
| Artifact-class precedent | Finding | Binary smoke precedent lives in CI's `go test -race` path but is skipped there; the design does not compensate. |
| YAGNI violations | Clean | No new runtime feature, ConPTY runner, external provider CI, or broad extension work is introduced. |
| Missing failure modes | Finding | Missing shortcut cases can pass while advertised keyboard paths remain broken or stale. |
| Security/privacy at architecture level | Clean | Current text addresses build-tag isolation, temp state, Unix socket containment, and redaction boundaries. |
| Infrastructure impact | Finding | New CI release preflight is specified, but CI execution for binary startup/TUI smoke remains ambiguous under race. |
| Multi-component validation | Finding | TUI/daemon/mock proof is strong, but not complete for the claimed shortcut surface. |
| Declared integration proof | Clean | Runtime/config/deferred classifications are present and mostly precise. |
| Contributed UI rendering proof | Clean | No plugin-contributed host UI is claimed; direct TUI rendering proof is the relevant surface. |
| Rollback story | Clean | Source revert plus artifact/tap removal and patch release is adequate for this slice. |
| Simpler alternative not considered | Finding | A single source-derived shortcut/help matrix would be simpler than manually maintaining separate PTY, `/help`, autocomplete, and CLI-help expectations. |
| User-intent drift | Finding | User asked for shortcut/slash proof broadly; current text can still prove only selected shortcuts while claiming "shortcuts." |
| Existence/runtime-validity | Finding | `ctrl+d`, `ctrl+t`, and public `ratchet help` surfaces exist now but are not covered mechanically. |

**Options the author may not have considered:**
1. Source-derived UI contract: build a small test table from `commands.Parse`, `helpCmd`, autocomplete entries, `printUsage`, and status-bar shortcut hints, then require every advertised item to be either PTY-proven, unit-proven, or explicitly documented out of scope.
2. Dedicated non-race smoke CI job: keep the main `go test -race ./...` job, but add `go test ./cmd/ratchet ./internal/tui -run 'HarnessSmoke|TUIBinarySmoke' -count=1` so binary subprocess tests are actually exercised in PR/push CI.

**Verdict reasoning:** The current design is much stronger than the earlier cycles, but it still leaves Important gaps where implementation could pass while proving less than the user asked for: not all advertised/core shortcuts, no guaranteed CI execution for release-shaped startup smoke, and stale built CLI help. Status is FAIL until those are mechanically covered or the claims are narrowed.

## Cycle 10

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D40` [Existence/runtime-validity / Declared integration proof] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:271-281,406]: CI snapshot generation does not run `goreleaser check`. The design's local script default runs `goreleaser check`, but the CI path runs `goreleaser release --snapshot --clean --skip=publish` then `--manifest-only`; local GoReleaser v2.16.0 help says `--snapshot` skips validation. Recommendation: add an explicit CI `goreleaser check` step or run the script's full default mode in CI after installing GoReleaser.
- `D41` [Rollback story / Infrastructure impact] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:282-283,410-420; .github/workflows/release.yml:17-20]: The release artifact guard is only PR/push preflight; the tag release workflow remains unchanged and can publish without running the new manifest guard. That is not release-safe unless tag publishing is formally gated elsewhere, which the design does not state. Recommendation: run `scripts/check-release-artifacts.sh --manifest-only dist` in `.github/workflows/release.yml` after GoReleaser snapshot/build and before publishing, or document/enforce an explicit protected release gate.
- `D42` [Project-guidance conflicts / User-intent drift] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:29,260-267,367,391,407; .github/workflows/ci.yml:32-46]: The design promises Windows "non-PTY CLI smoke" / "build/noninteractive smoke," but the concrete Windows proof is Linux-hosted cross-build plus negative smoke-package build checks. No Windows runner executes `ratchet.exe version` or `ratchet.exe help`. Recommendation: either add a `windows-latest` non-PTY smoke for safe commands, or change all wording to "Windows cross-build proof only."

**Findings (Minor):**
- `D43` [Repo-precedent conflicts / Artifact-class precedent] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:275-281; .github/workflows/release.yml:17-20]: The new `release-check` job does not say to pin GoReleaser action version `~> v2`, while the real release workflow does. Recommendation: mirror the release workflow's action version input so preflight and publishing use the same GoReleaser major.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | Windows-honest proof is overworded as noninteractive smoke while only cross-build proof is specified. |
| Assumptions under attack | Finding | The design assumes PR/push preflight is enough for release safety and assumes snapshot release validates config; both are false or unstated. |
| Repo-precedent conflicts | Finding | Existing release workflow pins GoReleaser `~> v2`; proposed preflight omits the same pin. |
| Artifact-class precedent | Finding | Release artifact precedent exists in `.goreleaser.yaml` and `.github/workflows/release.yml`, but the new preflight is not wired like the actual release path. |
| YAGNI violations | Clean | No new runtime user feature, ConPTY runner, visual snapshot framework, or external provider CI is added. |
| Missing failure modes | Finding | Tag-publish bypass and snapshot-validation skip are release failure modes not handled by the current text. |
| Security/privacy at architecture level | Clean | Build-tag isolation, temp state, Unix socket containment, and redaction boundaries are now explicit enough at design level. |
| Infrastructure impact | Finding | New CI release preflight changes release infrastructure but does not gate the actual tag release workflow. |
| Multi-component validation | Clean | TUI/daemon/mock provider, docs, CI smoke, and artifact inspection boundaries are otherwise split clearly. |
| Declared integration proof | Finding | GoReleaser integration is declared, but CI omits `goreleaser check` even though snapshot skips validation. |
| Contributed UI rendering proof | Clean | No plugin-contributed host-shell UI is claimed; direct Bubble Tea TUI rendering is the relevant UI surface. |
| Rollback story | Finding | Rollback describes bad artifact cleanup, but the guard may not run before a tag release publishes those artifacts. |
| Simpler alternative not considered | Finding | The design does not consider putting the manifest guard directly into the release workflow as a publish-blocking preflight. |
| User-intent drift | Finding | "Windows noninteractive smoke" wording drifts beyond the specified cross-build-only proof. |
| Existence/runtime-validity | Finding | Local GoReleaser help confirms `--snapshot` skips validation, so the CI command sequence does not prove what the design claims. |

**Options the author may not have considered:**
1. Release-workflow guard: run the artifact manifest check in `.github/workflows/release.yml` before assets are published. This is stricter than PR-only preflight and directly protects tag releases.
2. Windows runtime smoke: add a `windows-latest` job that builds and runs `ratchet.exe version` and `ratchet.exe help`, while keeping TUI PTY proof Unix-only.

**Verdict reasoning:** The design is strong on TUI isolation and prior slash/shortcut gaps, but it still has unresolved Important release and Windows-honesty issues. The biggest mechanical problem is that the proposed CI path can skip GoReleaser config validation and the actual tag release path can bypass the new artifact guard entirely. Status is FAIL.

## Cycle 11

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D44` [Existence/runtime-validity / Multi-component validation] [design:139-142,166-172; `internal/tui/app.go`:439-450]: The shortcut matrix says `ctrl+j` and `ctrl+t` keep chat input visible and usable, but current app rendering replaces the chat body with the job panel or team panel. That evidence is mechanically false unless the implementation changes existing TUI behavior, which the design does not explicitly scope. Recommendation: either assert the current behavior honestly, e.g. panel opens, `esc`/toggle returns to chat/input, or explicitly design the layout change and treat it as user-facing behavior.
- `D45` [User-intent drift / Existence-runtime-validity] [design:346-364; `docs/harness-emulation.md`:58-64; `README.md`:90-104]: The docs negative guard is too broad and too easy to evade. A valid product statement like "ratchet-cli supports trust slash commands" can match `ratchet` + `slash commands`, while a contradictory row can pass by adding "not claimed" anywhere in the same unit. Recommendation: make the predicate target evidence claims only: exact command token `ratchet` plus automation/proof/smoke/coverage tokens plus interactive/slash/shortcut terms, with exceptions that negate the exact same predicate.
- `D46` [Infrastructure impact / Repo-precedent conflicts] [design:281-297; `.github/workflows/ci.yml`:14-15,28,42,59,78,94; `.github/workflows/release.yml`:17-23; `go.mod`:9-10]: New release workflow preflight steps are not specified with the private-module environment and Git rewrite used by existing CI jobs. The repo's CI treats `GoCodeAlone/*` modules as private, and GoReleaser builds the same module graph. Recommendation: require `GOPRIVATE`/`GONOSUMCHECK` and the Git rewrite before both release preflight and publish steps; mirror this setup for any new `windows-latest` smoke job.

**Findings (Minor):**
- `D47` [Rollback story / Declared integration proof] [design:290-297,406,422]: The release workflow guard inspects snapshot artifacts, then the publishing step runs `goreleaser release --clean`, which rebuilds and publishes a fresh `dist`. This is probably acceptable for detecting config-level smoke-binary inclusion, but it is not proof of the exact uploaded asset set. Recommendation: either state this limitation, add a draft-release post-publish asset check before undrafting, or use a publish path that inspects the same generated artifacts later uploaded.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | Release-safe/current proof is weakened by missing release-workflow private-module setup. |
| Assumptions under attack | Finding | The design assumes `ctrl+j`/`ctrl+t` preserve input visibility, which current rendering contradicts. |
| Repo-precedent conflicts | Finding | Existing CI jobs configure private module access; planned release preflight text does not carry that into release workflow steps. |
| Artifact-class precedent | Finding | Release preflight follows GoReleaser artifact precedent, but snapshot inspection is not identical to final publish artifacts. |
| YAGNI violations | Clean | No new ConPTY, external provider CI, broad SDK, or new command surface is required. |
| Missing failure modes | Finding | Panel shortcuts can hide input while tests claim input visibility; release builds can fail from missing private-module setup. |
| Security/privacy at architecture level | Clean | Build-tag isolation, temp state, socket containment, and redaction boundaries are adequately specified at design level. |
| Infrastructure impact | Finding | CI/release workflow changes need private-module setup and exact release-asset caveat. |
| Multi-component validation | Finding | TUI shortcut proof does not match current app rendering for team/job panels. |
| Declared integration proof | Finding | GoReleaser proof is strong but snapshot-only relative to final publish artifacts. |
| Contributed UI rendering proof | Clean | No plugin-contributed host UI is claimed; direct Bubble Tea rendering is the relevant UI surface. |
| Rollback story | Finding | Rollback is adequate, but final release artifact verification is weaker than claimed. |
| Simpler alternative not considered | Finding | Simpler shortcut proof would assert current panel open/return behavior instead of requiring input visibility under every panel. |
| User-intent drift | Finding | Docs guard can both block valid product docs and allow contradictory overclaim phrasing. |
| Existence/runtime-validity | Finding | Current code contradicts planned `ctrl+j`/`ctrl+t` visible-input evidence; docs guard predicates are mechanically imprecise. |

**Options the author may not have considered:**
1. Current-behavior shortcut contract: prove `ctrl+s` preserves split chat/input, while `ctrl+j` and `ctrl+t` intentionally switch to panel views and must return cleanly to chat/input. This avoids smuggling a layout redesign into a verification slice.
2. Evidence-row docs guard: scan only harness/evidence/status rows for release-binary automation claims, not every public product sentence mentioning ratchet and slash commands.
3. Draft-release postcheck: after GoReleaser creates the draft release, inspect uploaded asset names/checksums before the existing publish script undrafts it.

**Verdict reasoning:** The design is close, but it still has unresolved Important issues where implementation could either fail mechanically or pass while proving something different from the claim. Status is FAIL until the shortcut evidence matches actual rendering, the docs guard is made precise, and release workflow setup mirrors the repo's private-module CI requirements.

## Cycle 12

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D48` [Rollback story / Infrastructure impact / Declared integration proof] [design:304-308,443; `.goreleaser.yaml`:44-69; `.github/workflows/release.yml`:17-51]: The draft-release postcheck happens after `goreleaser release --clean` publishes all configured side effects, including the Homebrew cask repo update via `homebrew_casks.repository`. A GitHub draft can be held before undrafting, but a bad tap/cask commit is already pushed to the external tap by then; "before public release" is therefore false for that integration. Recommendation: either make the release path inspect the exact Homebrew cask material before any tap push, split tap publishing into a post-guard step, or state and accept that Homebrew rollback is after-the-fact rather than release-blocking.
- `D49` [Repo-precedent conflicts / Artifact-class precedent / Existence/runtime-validity] [design:284-292; `.github/workflows/release.yml`:11-20]: The new PR/push `release-check` job does not say to checkout with `fetch-depth: 0`, while the existing release workflow does so before GoReleaser. GoReleaser derives versions/changelogs from git tags; a shallow PR checkout can make the preflight non-equivalent to the tag release path or fail for reasons the real release job avoids. Recommendation: require `actions/checkout@v4` with `fetch-depth: 0` in `release-check` and any other GoReleaser preflight job.

**Findings (Minor):**
- `D50` [Multi-component validation / Infrastructure impact] [design:233-239]: The non-race CI command is fixed to `-run 'HarnessSmoke|TUIBinarySmoke'`, but the design does not require the new release-shaped startup/onboarding smoke test to use a matching name. An implementer can add `TestReleaseStartupSmoke` or similar and still have CI skip the very proof that distinguishes untagged `ratchet` startup from the build-tagged TUI smoke. Recommendation: name the required tests explicitly or broaden the CI regex to include the startup smoke contract.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | The design targets real, Windows-honest, release-safe proof, but D48 means one release surface can publish before the guard can block it. |
| Assumptions under attack | Finding | The design assumes draft-release postcheck gates every publish surface, assumes GoReleaser preflight has release-equivalent git state, and assumes CI regex will catch future startup-smoke naming. |
| Repo-precedent conflicts | Finding | Existing release workflow uses full fetch before GoReleaser; the new `release-check` job omits that precedent. |
| Artifact-class precedent | Finding | GoReleaser jobs and binary-smoke tests need exact runner/test discovery wiring; D49 and D50 leave those mechanics underspecified. |
| YAGNI violations | Clean | No ConPTY runner, visual snapshot framework, external-provider CI, new runtime command, or broader policy work is added. |
| Missing failure modes | Finding | Bad Homebrew tap publication before postcheck and CI skipping a differently named startup smoke are unhandled failure modes. |
| Security/privacy at architecture level | Clean | Build-tag isolation, temp state/workdir, Unix-socket containment, and redaction boundaries are explicit enough at design level. |
| Infrastructure impact | Finding | Release workflow and CI changes affect GoReleaser/Homebrew surfaces; D48 and D49 make the guard less reliable than claimed. |
| Multi-component validation | Finding | TUI/daemon/mock proof is well scoped, but the untagged startup proof may not be guaranteed in CI if test naming drifts. |
| Declared integration proof | Finding | Homebrew cask/tap is a declared release integration, but current postcheck cannot block that external publish side effect. |
| Contributed UI rendering proof | Clean | No plugin-contributed host-shell UI is claimed; this is direct Bubble Tea TUI rendering. |
| Rollback story | Finding | GitHub release rollback is covered, but Homebrew tap rollback is after-the-fact unless the publish step is split or prechecked against exact cask output. |
| Simpler alternative not considered | Finding | A split release workflow that snapshots/builds, guards, then publishes GitHub assets and tap changes separately would make the guard easier to reason about. |
| User-intent drift | Finding | The user asked for release-safe proof; claiming "before public release" while tap publishing can already happen drifts from that mandate. |
| Existence/runtime-validity | Finding | The referenced release workflow and GoReleaser config exist and show full-fetch precedent plus Homebrew cask publishing side effects. |

**Options the author may not have considered:**
1. Split Homebrew publishing from GoReleaser asset publishing: generate and inspect the cask first, publish GitHub draft assets only after the guard passes, then push the tap update as a separate guarded step. Trade-off: more workflow plumbing, but the rollback story becomes genuinely pre-publication.
2. Make smoke CI discover tests by contract rather than name regex: put all smoke tests behind an explicit `TestHarnessSmoke...` naming convention and enforce it in the design, or use a broader regex such as `Smoke|Startup` for the focused non-race job. Trade-off: broader regex can run a little more test code, but it avoids silent proof gaps.

**Verdict reasoning:** The current text resolves most prior design failures, especially the build-tagged smoke binary, slash/mode/trust matrix, Windows safe-command smoke, and GoReleaser validation steps. It still has unresolved release-safety and CI-discovery gaps: the Homebrew tap can be modified before the proposed postcheck blocks public release, the GoReleaser PR preflight is not release-equivalent without full git history, and the startup smoke can be skipped by the fixed CI regex. Status is FAIL because open Important findings remain.

## Cycle 13

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D51` [Infrastructure impact / Rollback story / User-intent drift] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:315-321,416-426,457; .goreleaser.yaml:44-69]: The current text accepts after-the-fact Homebrew/tap rollback, but still says there is "No ... Homebrew publishing" and classifies the GoReleaser/Homebrew integration as `runtime-integrated`. That is mechanically inconsistent with the existing `homebrew_casks.repository` publish side effect and with the user's release-safe mandate: a bad tap commit can already be public before the postcheck runs. Recommendation: either split Homebrew publishing behind a pre-public guard, or downgrade the design claim explicitly to "GitHub release-safe; Homebrew/tap has precheck plus after-the-fact rollback" in Infrastructure Impact, Integration Matrix, Rollback, and docs/plan gates.
- `D52` [Existence/runtime-validity / Repo-precedent conflicts / Multi-component validation] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:161-181; internal/tui/pages/chat.go:174-178; internal/tui/components/statusbar.go:58-60]: The design says `ctrl+h` is advertised but has no handler, but current code handles `ctrl+h` in the chat page when the thinking panel has content. The stated shortcut source of truth also omits page-level key handling, so the plan could remove or ignore a real shortcut while claiming to align advertised shortcuts with handlers. Recommendation: include `internal/tui/pages/chat.go` in the shortcut source-of-truth scan and either prove `ctrl+h` with a thinking-panel fixture or document why conditional thinking-panel behavior is out of scope without calling it unimplemented.
- `D53` [Multi-component validation / User-intent drift / Artifact-class precedent] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:161-181; README.md:84-88; internal/tui/components/sessiontree.go:90-117; internal/tui/pages/session_tree.go:85-99]: The shortcut matrix claims every implemented or advertised core shortcut, but it only proves opening/closing the branch tree. README advertises tree navigation keys `j`/`k`, arrows, `h`/`l`, `Enter`, `r`, and `Esc`, and the tree components implement those keys. A PTY proof that only opens `/tree`/`ctrl+b` can still miss broken advertised tree navigation and branch switching. Recommendation: add a small branch-tree PTY/focused matrix for advertised tree navigation, refresh, collapse/expand, selection, and return-to-chat, or narrow the design/docs claim to "tree entry/exit only."
- `D54` [Declared integration proof / Release safety / Existence/runtime-validity] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:78-93,284-355,462-476]: The release artifact guard forbids archive/checksum/member names containing `ratchet-tui-smoke`, but it does not extract the GoReleaser-built `ratchet` binaries and prove they expose no smoke help text or smoke command/flag. The local `go build ./cmd/ratchet` negative check is not the same artifact that the release workflow uploads. Recommendation: after snapshot generation, extract at least one Unix archive and inspect/run the archived `ratchet help`; for Windows, inspect the zip and run or string-check `ratchet.exe help` in the `windows-latest` job. Fail if smoke command/flag/help text appears.

**Findings (Minor):**
- `D55` [Security/privacy at architecture level / Missing failure modes] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:196-200; docs/policy-matrix.md:58-75]: The trust-command PTY matrix uses policy patterns like `bash:rm -rf /` and `bash:curl *` as expected rendered output. They are not executed, but they are still sensitive-looking local policy metadata and will appear in normal passing snapshots or assertion strings unless every path is redacted perfectly. Recommendation: use harmless deterministic placeholders such as `smoke:deny-dangerous-command` and keep one parser-level unit test for shell-shaped patterns if needed.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | The design largely follows Windows honesty and real-boundary guidance, but the current Homebrew/tap wording still overstates release safety for a publish surface that can change before postcheck. |
| Assumptions under attack | Finding | The design assumes after-the-fact tap rollback is acceptable release safety, assumes `app.go` plus statusbar fully define shortcuts, and assumes artifact name checks catch release-binary smoke exposure. |
| Repo-precedent conflicts | Finding | Current code has page-level `ctrl+h` handling in `internal/tui/pages/chat.go`, while the design's shortcut source-of-truth excludes that layer. |
| Artifact-class precedent | Finding | README advertises branch-tree keyboard navigation and the tree components implement it, but the proposed binary proof only covers entry/exit. |
| YAGNI violations | Clean | The design still avoids ConPTY, visual snapshot frameworks, external provider CI, new runtime commands, and broader policy work. |
| Missing failure modes | Finding | Bad Homebrew tap publication remains after-the-fact, archived release binaries can expose smoke text despite clean artifact names, and trust-pattern output can leak if redaction misses a path. |
| Security/privacy at architecture level | Finding | Temp state and socket containment are strong, but using destructive-looking trust patterns as expected PTY output increases the redaction burden unnecessarily. |
| Infrastructure impact | Finding | The Infrastructure Impact section contradicts the actual GoReleaser Homebrew cask publish side effect and needs a precise release-surface statement. |
| Multi-component validation | Finding | TUI/daemon/mock proof is strong for chat and trust commands, but shortcut proof misses page-level `ctrl+h` and advertised branch-tree navigation. |
| Declared integration proof | Finding | GoReleaser artifact proof checks names and metadata but does not verify the actual archived release binaries have no smoke command/flag/help surface. |
| Contributed UI rendering proof | Clean | No plugin-contributed host-shell UI is claimed; direct Bubble Tea PTY rendering is the relevant UI surface. |
| Rollback story | Finding | GitHub draft rollback is covered, but Homebrew/tap safety is explicitly after-the-fact and should not be described as fully release-safe. |
| Simpler alternative not considered | Finding | A cheaper shortcut proof could add a focused tree-navigation test and a thinking-panel fixture instead of treating all shortcut proof as PTY-only. |
| User-intent drift | Finding | The user asked for TUI/slash/shortcut proof that is real/current/release-safe; current text still misses advertised shortcut behavior and overstates Homebrew release safety. |
| Existence/runtime-validity | Finding | The referenced code and workflows exist and contradict current text on `ctrl+h`, Homebrew side effects, and release artifact equivalence. |

**Options the author may not have considered:**
1. Split the release gate by publish surface: keep GoReleaser GitHub archives/checksums behind the current preflight and draft-asset postcheck, but move Homebrew/tap publication to a separate guarded step after generated cask material is inspected.
2. Treat shortcut proof as two layers: PTY proves global TUI shortcuts and rendering boundaries; focused component/page tests prove advertised branch-tree navigation and conditional thinking-panel `ctrl+h` behavior.
3. Verify archived release binaries directly: extract GoReleaser snapshot artifacts and run or inspect the packaged `ratchet` binaries, not just the locally built binary and artifact names.

**Verdict reasoning:** The current design is close on the original TUI smoke gap, but it still has unresolved Important issues where the text can either prove a narrower shortcut surface than advertised or claim release safety stronger than the Homebrew workflow actually provides. Status remains FAIL until the Homebrew release-safety wording or workflow is corrected, shortcut source-of-truth/proof includes current page and tree behavior, and release artifact checks verify the actual packaged binaries.

## Cycle 14

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D56` [Existence/runtime-validity / Windows honesty] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:121,288]: The design relies on `internal/tui/tui_binary_smoke_unix_test.go` and says Unix PTY tests can remain build-tagged by filename suffix, but Go does not treat `_unix.go` as a GOOS filename suffix. `unix` is a build constraint term, not a filename target; `go tool dist list | rg '^unix/'` has no such GOOS. A Windows `go test ./internal/tui` can still try to compile the PTY smoke test unless the file has an explicit build constraint. Recommendation: require `//go:build !windows` or `//go:build unix` on the PTY smoke test and any PTY-only helper, and keep the filename as documentation only.
- `D57` [Infrastructure impact / Missing failure modes] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:247-249,463,478; cmd/ratchet/race_enabled_test.go:1-5]: The design says the new non-race CI job exercises smoke tests that skip under `-race`, but the new `TestTUIBinarySmoke` lives in `internal/tui`, and only `cmd/ratchet` currently defines the `raceEnabled` build-tag helper. Without an explicit skip mechanism in `internal/tui`, the existing `go test -race ./...` job will run the expensive PTY binary smoke anyway, or an implementation that copies `raceEnabled` will not compile in package `tui`. Recommendation: add a concrete `internal/tui` race skip contract, such as package-local `race_enabled_test.go` / `race_disabled_test.go` files plus `if raceEnabled { t.Skip(...) }`, and require the focused non-race job to be the only CI path that executes the PTY smoke.
- `D58` [Infrastructure impact / Declared integration proof / Multi-component validation] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:300-308,341-349,480,482]: The release guard says `windows-latest` extracts the snapshot Windows zip and runs packaged `ratchet.exe version/help`, but the CI design only says the Windows smoke job builds `ratchet.exe`; it does not say how the GoReleaser-generated `dist/` from the Ubuntu `release-check` job reaches the Windows runner, nor that Windows regenerates the same snapshot locally. That makes the packaged Windows-zip proof non-executable as written. Recommendation: specify either upload/download of the `dist/` artifact from `release-check` into the `windows-latest` job, or run the GoReleaser snapshot step on `windows-latest` before extracting the Windows zip; then run the packaged executable from that archive.

**Findings (Minor):**
- `D59` [Missing failure modes / Rollback story] [docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md:264-267; internal/daemon/daemon.go:128-139]: The release-shaped startup smoke can call `ratchet daemon stop`, but current `daemon.Stop` only signals the process and returns; it does not wait for pid/socket cleanup. An immediate "pid/socket gone" assertion can flake or leave a daemon alive if shutdown is slow. Recommendation: require a bounded wait loop after `ratchet daemon stop`, with fallback terminate/wait by pid if the pid file or socket remains.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | The design follows the workspace mandate for real and Windows-honest proof, but D56/D58 leave Windows proof mechanics inaccurate. |
| Assumptions under attack | Finding | The design assumes `_unix_test.go` gates Windows, assumes `internal/tui` smoke tests already have a race-skip mechanism, and assumes Windows can run snapshot zip contents without artifact transfer. |
| Repo-precedent conflicts | Finding | Existing race skip precedent exists only in `cmd/ratchet`, while the planned PTY smoke is in `internal/tui`. |
| Artifact-class precedent | Finding | Existing PTY helpers use explicit build tags for integration-only tests; the new untagged PTY artifact needs explicit platform and race gating. |
| YAGNI violations | Clean | The design still avoids ConPTY, visual snapshot frameworks, external provider CI, new runtime commands, and broader policy work. |
| Missing failure modes | Finding | Windows compilation of a supposed Unix-only test, race-suite execution of expensive PTY smoke, missing Windows artifact transfer, and non-waiting daemon stop are not handled. |
| Security/privacy at architecture level | Clean | Build-tagged smoke entrypoint, temp state/workdir, Unix-socket containment, and redaction boundaries are adequate at design level. |
| Infrastructure impact | Finding | CI job topology is underspecified for both race-suite exclusion and Windows packaged-artifact execution. |
| Multi-component validation | Finding | The design proves many real boundaries, but the packaged Windows zip proof is not wired across CI components. |
| Declared integration proof | Finding | Windows release archive execution is declared runtime-integrated, but the design omits the concrete consumer path that supplies the snapshot archive to Windows. |
| Contributed UI rendering proof | Clean | No plugin-contributed host-shell UI is claimed; direct Bubble Tea rendering remains the relevant UI surface. |
| Rollback story | Finding | Source rollback is adequate, but startup smoke cleanup needs wait/fallback mechanics to avoid lingering daemon state in CI. |
| Simpler alternative not considered | Finding | A simpler platform gate is explicit `//go:build !windows` on the PTY test, and a simpler Windows packaged proof is artifact upload/download from the existing release-check job. |
| User-intent drift | Finding | The user asked for Windows-honest and release-safe proof; current text still has Windows gating and packaged-artifact proof gaps. |
| Existence/runtime-validity | Finding | `go tool dist list` has no `unix` GOOS, `internal/tui` has no `raceEnabled` helper, and existing release-check/windows jobs are separate without artifact sharing. |

**Options the author may not have considered:**
1. Explicit platform tag plus boring filename: name the test `tui_binary_smoke_test.go` and put `//go:build !windows` at the top. That removes the false signal that `_unix` is mechanically meaningful.
2. Artifact-passing release proof: have `release-check` upload `dist/` as a short-lived workflow artifact and make `windows-latest` download/extract the Windows zip. This proves the exact archive generated by the release guard without rerunning GoReleaser on Windows.
3. Package-local smoke skip helper: add `internal/tui/race_enabled_test.go` and `internal/tui/race_disabled_test.go` mirroring `cmd/ratchet`, so the untagged PTY smoke is discoverable but intentionally skipped in the race suite.

**Verdict reasoning:** The current design resolves prior major concerns around hidden release paths, docs overclaiming, shortcut coverage, Homebrew honesty, and packaged binary inspection. It still has open Important mechanical issues: the Unix-only PTY test is not actually Unix-only by filename, the new internal/tui smoke test lacks race-suite skip wiring, and the Windows packaged-zip proof has no CI artifact path. Status is FAIL until those mechanics are made explicit in the design.

## Cycle 15

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D60` [User-intent drift / Multi-component validation / Existence-runtime-validity] [design:135-142,202-223,466-492; `internal/tui/commands/commands.go`:49-126,134-171]: The design still says PTY proof covers "slash commands" while the executable TUI command surface includes many commands outside the planned `/help`, `/provider list`, `/mode`, `/trust`, `/tree`, and exit set. Recommendation: narrow claims to representative slash commands plus full mode/trust/tree proof, or define a source-of-truth command matrix from `commands.Parse`/`helpCmd` classifying every command as PTY-proven, focused-proven, deferred, or out-of-scope.
- `D61` [Infrastructure impact / Declared integration proof / Missing failure modes] [design:288-321,360-364; `.github/workflows/ci.yml`:48-51]: Windows packaged-zip proof depends on `ratchet-snapshot-dist`, but the design does not require the Windows job to `needs: release-check`. Recommendation: specify exact CI DAG: `windows-safe-command-smoke` needs `release-check`, downloads `ratchet-snapshot-dist`, fails if no Windows zip exists, and runs the extracted packaged executable.
- `D62` [Multi-component validation / Artifact-class precedent] [design:187-198; `README.md`:84-88; `internal/tui/app.go`:219-225,365-401; `internal/tui/pages/session_tree.go`:85-98]: Branch-tree proof only requires `Enter` to emit a branch-switch command, but docs promise switching rebuilds chat for the selected branch. Recommendation: add App-level proof that opens tree, selects child, presses `Enter`, asserts selected session/chat view changed, waits for history reload, and verifies chat input can submit against the selected branch.

**Findings (Minor):**
- `D63` [Missing failure modes / Repo-precedent conflicts] [design:143-156; `internal/tui/pty_test.go`:63-75,107-119]: New CI PTY smoke skips under `-race`, but the design does not require a concurrency-safe PTY capture helper. Recommendation: serialize output with a mutex/channel or single-reader snapshot API rather than copying the existing unsynchronized buffer pattern.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | No repo-local guidance file exists; fallback guidance is captured, but slash-command proof is overclaimed. |
| Assumptions under attack | Finding | Curated slash coverage, implicit artifact dependency, and component-level branch proof are load-bearing assumptions. |
| Repo-precedent conflicts | Finding | Existing CI uses `needs` for dependent jobs; existing PTY helper is integration-only and unsynchronized. |
| Artifact-class precedent | Finding | Binary smoke shape is mostly aligned, but Windows artifact transfer and branch-switch proof are weaker than their claimed artifact classes. |
| YAGNI violations | Clean | No ConPTY, visual snapshots, external provider CI, new commands, or ACPX/import-export/flow work. |
| Missing failure modes | Finding | Missing CI artifact dependency, partial slash proof, App-level branch-switch failure, and PTY capture flake modes. |
| Security/privacy at architecture level | Clean | Build-tag isolation, temp state/workdir, Unix socket containment, and redaction are adequate. |
| Infrastructure impact | Finding | Release-check to Windows packaged-smoke topology is underspecified. |
| Multi-component validation | Finding | Branch-tree selection is not proven across component → App → chat reload, and slash coverage is broader in claims than proof. |
| Declared integration proof | Finding | Windows packaged archive execution needs explicit workflow handoff to be executable. |
| Contributed UI rendering proof | Clean | No plugin-contributed UI is claimed. |
| Rollback story | Clean | Source revert plus release/tap rollback is adequate. |
| Simpler alternative not considered | Finding | Command-surface classification table would keep claims honest without one huge PTY test. |
| User-intent drift | Finding | User asked for TUI slash/shortcuts and Windows builds; current text can overclaim slash coverage and under-wire Windows packaged proof. |
| Existence/runtime-validity | Finding | Extra commands, App branch-switch consumer, and CI artifact mechanics exist and must be accounted for. |

**Options the author may not have considered:**
1. Command-surface classification table from `internal/tui/commands/commands.go`.
2. App-level branch-tree smoke proving `SessionTreeSelectedMsg` through chat reload.
3. Explicit `release-check` → Windows packaged-smoke CI DAG.

**Verdict reasoning:** D56-D59 are resolved, but new Important gaps remain: slash-command claims exceed the real proof surface, Windows packaged proof lacks an explicit artifact dependency, and branch switching is not proven through the App-level chat reload path. Status is FAIL.

## Cycle 16

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D64` [Existence/runtime-validity / User-intent drift] [design:246-265,289-291; `internal/tui/commands/commands.go`:49-126]: D60 is directionally addressed, but fail-closed command classification is not mechanically defined. `commands.Parse` is a hand-written switch, `helpCmd` is a separate string list, and autocomplete is a third list; no enumerable command registry exists. Recommendation: require a single declarative command registry or an AST-based guard that extracts switch cases from `Parse` and compares them to help/autocomplete/coverage rows.
- `D65` [Infrastructure impact / Repo-precedent conflicts] [design:300-302; `.github/workflows/ci.yml`:17-28,48-58]: The new non-race smoke CI job is specified only as a `go test` command; existing CI jobs do checkout, setup-go, and private-module Git rewrite. Recommendation: make the smoke command a step in a setup-equivalent job or explicitly require checkout, setup-go `1.26`, `GOPRIVATE`/`GONOSUMCHECK`, and the same Git rewrite.
- `D66` [Declared integration proof / Rollback story] [design:402-413,545-547; `.goreleaser.yaml`:4-18]: Release guard runs packaged `help/version` only for host-compatible Unix and Windows zip; non-host Linux/Darwin artifacts are not binary-content checked beyond archive/member names. Recommendation: extract every packaged `ratchet` binary and byte-scan/`strings`-scan forbidden tokens for all OS/arch artifacts; keep executable runs where practical.

**Findings (Minor):**
- `D67` [Artifact-class precedent / Infrastructure impact] [design:331-333; `.github/workflows/ci.yml`:36-39]: Windows cross-build commands omit `-o`, unlike existing CI, and can write `ratchet.exe` into repo root. Recommendation: require explicit temp outputs.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | `docs/design-guidance.md` absent; fallback guidance captured, but Windows/release proof still has CI and artifact-proof gaps. |
| Assumptions under attack | Finding | Design assumes non-declarative command surfaces can be enumerated and partial packaged-binary execution proves all release binaries. |
| Repo-precedent conflicts | Finding | New non-race smoke job omits existing CI setup pattern. |
| Artifact-class precedent | Finding | Existing Windows cross-build uses explicit `/tmp` outputs; proposed commands write into repo root. |
| YAGNI violations | Clean | No ConPTY, visual snapshot framework, new command surface, external-provider CI, or import/export/flow scope. |
| Missing failure modes | Finding | Manual command classification, CI private-module fetch failure, and non-host release binary contamination remain plausible. |
| Security/privacy at architecture level | Clean | Temp state/workdir, build-tag isolation, socket containment, hook/instruction leak checks, and redaction are explicit. |
| Infrastructure impact | Finding | CI topology for non-race smoke and cross-build artifact placement are underspecified. |
| Multi-component validation | Finding | D56-D63 resolved in text, but release validation still does not inspect every packaged binary surface. |
| Declared integration proof | Finding | GoReleaser/Windows handoff explicit, but non-host packaged binaries are not behaviorally checked for forbidden smoke surfaces. |
| Contributed UI rendering proof | Clean | No plugin-contributed host UI is claimed. |
| Rollback story | Finding | Source rollback is adequate, but guard can miss bad non-host release binary before publish. |
| Simpler alternative not considered | Finding | Shared declarative command registry would be stricter than testing three manual lists. |
| User-intent drift | Finding | Command-surface table can still drift without mechanical extraction. |
| Existence/runtime-validity | Finding | `Parse` is not enumerable as designed; D64-D66 remain executable-contract gaps. |

**Options the author may not have considered:**
1. Shared command registry driving `Parse`, `/help`, autocomplete, CLI help snippets, and coverage classification.
2. All-archive binary byte scan plus executable `help/version` runs where supported.

**Verdict reasoning:** D56-D63 are resolved on paper, but the proof remains non-fail-closed in three places: command classification is not mechanically tied to the parser, the new smoke CI job lacks existing private-module setup, and release artifact inspection does not cover all packaged binaries.

## Cycle 17

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D68` [Existence/runtime-validity / Missing failure modes] [design:267-279; `internal/tui/commands/commands.go`:49-126,134-171; `internal/tui/components/autocomplete.go`:39-60]: AST guard still conflates top-level `Parse` cases, help lines containing subcommands/examples, and autocomplete entries. Recommendation: define separate extracted surfaces: top-level commands from `Parse`, documented subcommands from parser switches or explicit test tables, and examples excluded from discovery.
- `D69` [Existence/runtime-validity / Declared integration proof] [design:299-305,363-371; `cmd/ratchet/main.go`:168-175]: Design allows public `ratchet help` slash-command section to be removed, while Windows packaged smoke requires slash entries in `ratchet.exe help`. Recommendation: choose one contract.
- `D70` [Declared integration proof / Rollback story] [design:399-414,431-447,580]: D66 covers snapshot artifacts but not actual uploaded draft release contents after `goreleaser release --clean` reruns. Recommendation: publish already-scanned `dist/` or download every draft release archive before undrafting and run the same archive extraction/all-binary byte scan.

**Findings (Minor):**
- `D71` [Project-guidance conflicts / Artifact-class precedent] [design:487-513; `docs/policy-matrix.md`:1-7,93-103]: Positive TUI smoke wording in policy matrix risks noisy docs. Recommendation: keep negative overclaim scanning across public docs, but require positive TUI smoke wording only in README/RATCHET/harness docs unless policy text actually claims TUI evidence.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | No repo-local guidance; D71 pushes evidence wording into a policy artifact class. |
| Assumptions under attack | Finding | Design assumes one AST extraction can unify parser/help/autocomplete and assumes snapshot artifacts equal uploaded draft assets. |
| Repo-precedent conflicts | Finding | CLI help optional removal conflicts with Windows packaged-help assertion. |
| Artifact-class precedent | Finding | Docs guard should keep positive evidence checks in harness/public evidence docs, not policy matrix by default. |
| YAGNI violations | Clean | No ConPTY, visual snapshots, production command registry refactor, external-provider CI, or import/export/flow scope. |
| Missing failure modes | Finding | Command-surface guard false-pass/false-fail modes remain. |
| Security/privacy at architecture level | Clean | Build-tag isolation, temp state/workdir, socket containment, redaction, and hook/instruction leak controls are explicit. |
| Infrastructure impact | Finding | Final tag-release asset gate remains weaker than snapshot CI gate. |
| Multi-component validation | Finding | Uploaded release asset content remains unproven. |
| Declared integration proof | Finding | Draft GitHub release assets are runtime-integrated but not byte-scanned after publish rerun. |
| Contributed UI rendering proof | Clean | No plugin-contributed UI is claimed. |
| Rollback story | Finding | Contaminated GitHub release archive may only be caught by name checks before undraft. |
| Simpler alternative not considered | Finding | Test-only command-spec table and release-from-scanned-artifacts alternatives not captured. |
| User-intent drift | Finding | D69 can drift by making Windows proof depend on optional help text. |
| Existence/runtime-validity | Finding | D64 remains as D68; D69-D70 are executable-contract gaps. |

**Options the author may not have considered:**
1. Test-only command-spec table with top-level commands, subcommands, examples, autocomplete visibility, and proof class.
2. Release from scanned artifacts instead of rerunning GoReleaser publish.

**Verdict reasoning:** D60-D63 and D65-D67 are resolved on paper, but D64 remains mechanically ambiguous, and final release proof has snapshot-vs-uploaded asset drift. Status FAIL.

## Cycle 18

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D72` [Existence/runtime-validity / Missing failure modes] [design:235-237,269-286; `internal/tui/commands/trust.go`:21-139]: Mechanical guard covers top-level parser/help/autocomplete, but not production-derived `modeCmd`, `trustCmd`, or provider subcommand drift. Recommendation: AST-extract mode keys, trust switch cases, and provider switch cases, then compare source-derived sets to typed spec and proof classifications.
- `D73` [Multi-component validation / User-intent drift] [design:363-386,585,602; `cmd/ratchet/harness_smoke_test.go`:14-34; `internal/daemon/daemon.go`:142-147]: Windows packaged smoke only runs `version`/`help`, while release-shaped proof elsewhere includes `daemon status`. Recommendation: add packaged `ratchet.exe daemon status` on `windows-latest` with temp Windows home/state env and assert `daemon is not running`, or defer Windows daemon CLI runtime explicitly.
- `D74` [Repo-precedent conflicts / Infrastructure impact] [design:417-421,455-463; `.github/workflows/release.yml`:24-39]: Draft-asset postcheck does not require the repo's existing `listReleases` retry/release-id lookup behavior, so a naive draft lookup can flake or miss assets. Recommendation: reuse `listReleases` retry by tag, pass release id to postcheck/undraft, and download assets by release id with explicit token.

**Findings (Minor):**
- `D75` [Project-guidance conflicts / Artifact-class precedent] [design:499-512; `README.md`:219-235; `RATCHET.md`:1-36; `docs/competitor-parity.md`:1-47]: Positive docs assertions still force detailed TUI smoke mechanics into `RATCHET.md` and parity docs. Recommendation: exact positive proof wording only in README harness table and `docs/harness-emulation.md`; RATCHET/parity get negative overclaim checks plus links unless they independently claim TUI evidence.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | No repo-local guidance; D75 still pushes detailed harness evidence into non-harness docs. |
| Assumptions under attack | Finding | Test-owned subcommand specs are not fail-closed without source extraction; Windows help/version may not be enough runtime proof. |
| Repo-precedent conflicts | Finding | D74 conflicts with existing draft-release lookup retry pattern. |
| Artifact-class precedent | Finding | RATCHET/parity doc shape differs from harness evidence docs. |
| YAGNI violations | Finding | Positive docs guard breadth is heavier than needed outside README/harness evidence surfaces. |
| Missing failure modes | Finding | Mode/trust/provider additions can bypass the claimed fail-closed matrix. |
| Security/privacy at architecture level | Clean | Build-tag isolation, temp state/workdir, socket containment, redaction, and hook/instruction leak controls are explicit. |
| Infrastructure impact | Finding | Draft postcheck lookup can flake or miss assets; Windows daemon-status runtime unproven. |
| Multi-component validation | Finding | Windows packaged proof skips safe daemon-status path. |
| Declared integration proof | Finding | Windows daemon CLI behavior is outside declared runtime-integrated release-shaped proof. |
| Contributed UI rendering proof | Clean | No plugin-contributed UI claimed. |
| Rollback story | Clean | Source revert, draft retention, asset deletion/patch release, and tap rollback described. |
| Simpler alternative not considered | Finding | Source-derived subcommand extractor and smaller docs-positive set not captured. |
| User-intent drift | Finding | Windows follow-through stops at help/version despite safe daemon-status precedent. |
| Existence/runtime-validity | Finding | D72 and D74 remain consumer-surface validity gaps. |

**Options the author may not have considered:**
1. Source-derived subcommand/mode guard.
2. Packaged Windows daemon-status smoke.
3. Harness-doc positive minimum with negative checks elsewhere.

**Verdict reasoning:** D68-D71 are mostly addressed, but subcommand/mode drift, Windows daemon-status runtime, and draft-release lookup mechanics remain tangible design blockers.

## Cycle 19

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D76` [Existence/runtime-validity / Missing failure modes] [design:281-289; `internal/tui/commands/trust.go`:101-115]: Source-derived trust guard extracts only top-level `trustCmd` cases, but `/trust persist allow` and `/trust persist deny` are nested `args[1]` behavior, not switch cases. Recommendation: source-derive nested trust action sets or add executable table tests proving every spec row maps to accepted behavior and rejected nested actions stay rejected.
- `D77` [Rollback story / Declared integration proof] [design:424-475; `.github/workflows/release.yml`:24-51; `.goreleaser.yaml`:68]: Draft postcheck assumes release remains draft, but no preflight checks `.goreleaser.yaml` `release.draft: true` and no post-publish assertion fails if resolved release is already public. Recommendation: fail preflight unless `release.draft` is true and fail postcheck if resolved release is not draft.
- `D78` [Infrastructure impact / Multi-component validation] [design:375-385,456-461,476-478; `.goreleaser.yaml`:8-13]: Windows packaged smoke can select the wrong architecture zip because both `windows_amd64` and `windows_arm64` exist. Recommendation: byte-scan all Windows archives, execute only `windows_amd64` on x64 `windows-latest`, and make arm64 inspection-only unless an arm64 runner exists.

**Findings (Minor):**
- `D79` [Artifact-class precedent / Project-guidance conflicts] [design:516-524,748]: Review resolution D71 contradicts the main docs section by still saying positive evidence is required in README/RATCHET/harness/parity. Recommendation: fix stale resolution row.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | No repo-local guidance; stale D71 row contradicts docs-scope guidance. |
| Assumptions under attack | Finding | Top-level extraction does not cover nested trust behavior, draft release state is assumed, and a generic Windows zip is assumed executable. |
| Repo-precedent conflicts | Finding | Existing release flow treats draft lookup carefully but does not enforce draft state; GoReleaser emits two Windows archives. |
| Artifact-class precedent | Finding | Release smoke should select architecture-specific artifacts; resolution rows must not contradict controlling sections. |
| YAGNI violations | Clean | No ConPTY, production registry refactor, visual snapshots, or later import/export/flow scope. |
| Missing failure modes | Finding | Nested trust drift and wrong Windows archive selection are plausible. |
| Security/privacy at architecture level | Finding | Trust command coverage is policy-sensitive; nested action drift can hide persistent grant behavior changes. |
| Infrastructure impact | Finding | Draft-state and artifact-selection mechanics affect release reliability. |
| Multi-component validation | Finding | Windows packaged proof needs runner-compatible archive selection. |
| Declared integration proof | Finding | GitHub release assets are pre-public gated only if draft state is asserted. |
| Contributed UI rendering proof | Clean | No host-shell/plugin UI. |
| Rollback story | Finding | If `release.draft` becomes false, rollback becomes after-publication. |
| Simpler alternative not considered | Finding | Explicit `windows_amd64` execution + all-Windows inspection and hard draft-state gate. |
| User-intent drift | Clean | Slice remains prerequisite TUI verification. |
| Existence/runtime-validity | Finding | D76-D78 are executable-contract gaps. |

**Options the author may not have considered:**
1. Behavior-derived command matrix for typed spec rows and rejected nested actions.
2. Architecture-aware release artifact contract: execute `windows_amd64`, inspect `windows_arm64`.

**Verdict reasoning:** D68-D75 are directionally resolved, but nested trust action drift, pre-public draft-state enforcement, and Windows archive selection remain tangible blockers.

## Cycle 20

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D80` [Existence/runtime-validity / Infrastructure impact] [design:404-407,492-505]: Release guard is specified as shell script but must strictly parse `.goreleaser.yaml`; shell parsing is brittle and no `yq` dependency is declared. Recommendation: implement guard as Go helper/test using existing `gopkg.in/yaml.v3`, with shell wrapper only for ergonomics.
- `D81` [Missing failure modes / Multi-component validation] [design:136-143,180-189]: One PTY process cannot prove `/exit`, `ctrl+c`, and `ctrl+d`; first exit terminates the process. Recommendation: separate PTY subprocess/subtests for each exit mechanism plus one non-exit interaction run.
- `D82` [Project-guidance conflicts / Missing failure modes] [design:525-557]: Docs overclaim guard requires release-target token, so generic "full TUI coverage is automated" can evade. Recommendation: treat TUI/interactive-surface + evidence tokens as suspicious even without release-target token unless assigned to `ratchet-tui-smoke` or release-binary chat proof is explicitly deferred.

**Findings (Minor):**
- `D83` [Existence/runtime-validity / Artifact-class precedent] [design:260,303-329]: Command-surface table says deferred commands keep help/autocomplete/parse entries covered, but `/mcp` is autocomplete-only today. Recommendation: define per-command required help surface explicitly.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | D82 weakens honest docs boundary. |
| Assumptions under attack | Finding | Shell YAML parsing and one-process exit proof are false assumptions. |
| Repo-precedent conflicts | Finding | Existing scripts are simple shell; new guard needs structured parsing. |
| Artifact-class precedent | Finding | Command surface guard must handle current help/autocomplete asymmetry explicitly. |
| YAGNI violations | Clean | Heavy gates are tied to concrete release-contamination findings. |
| Missing failure modes | Finding | Exit-path proof and docs-overclaim evasion remain. |
| Security/privacy at architecture level | Clean | Temp state, socket containment, redaction, trust output, and build-tag isolation explicit. |
| Infrastructure impact | Finding | CI/local parser dependency undeclared. |
| Multi-component validation | Finding | Process-exit behavior needs separate subprocesses. |
| Declared integration proof | Clean | D72-D78 are materially resolved. |
| Contributed UI rendering proof | Clean | No plugin UI. |
| Rollback story | Clean | Source revert, draft retention, asset deletion/patch release, and tap rollback described. |
| Simpler alternative not considered | Finding | Go release helper and process-per-exit PTY split. |
| User-intent drift | Clean | Prerequisite TUI/Windows verification slice remains aligned. |
| Existence/runtime-validity | Finding | D80-D83 are executable-contract gaps. |

**Options the author may not have considered:**
1. Small Go release-artifact guard using `gopkg.in/yaml.v3`, shell wrapper only.
2. One interaction PTY run plus three short exit-only subprocess proofs.
3. Context-aware docs guard keyed by TUI/interactive evidence claims even without `ratchet` token.

**Verdict reasoning:** D72-D79 appear resolved, but release-guard parser validity, exit-path separation, and docs-overclaim false negatives remain Important blockers.

## Cycle 21

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D84` [Existence/runtime-validity / Security/privacy at architecture level] [design:58-90; `.github/workflows/ci.yml`:29-30]: Design proves release artifacts omit smoke command and Windows cannot build it, but lacks no-tag Unix `go list`/`go build ./cmd/ratchet-tui-smoke` failure and source guard for exact `//go:build tui_smoke && !windows` on smoke files. Recommendation: add Linux/Darwin no-tag negative checks, source assertions for smoke build tags, and positive Unix `-tags tui_smoke` build.
- `D85` [Missing failure modes / User-intent drift] [design:552-566; `README.md`:84-88; `docs/harness-emulation.md`:3-6]: Docs overclaim guard uses line/table-row claim units; hard-wrapped prose can split evidence and TUI tokens across lines and evade. Recommendation: join adjacent nonblank non-table lines into paragraph claim units, keep table rows as units, and apply same predicate.

**Findings (Minor):**
- `D86` [Multi-component validation / Artifact-class precedent] [design:147-152; `internal/tui/components/statusbar.go`:60-66]: Frame width assertion does not specify display-cell width. Recommendation: use `lipgloss.Width` or `runewidth` on ANSI-stripped lines.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | D85 weakens honest harness evidence wording. |
| Assumptions under attack | Finding | Release-artifact absence alone does not prevent default-buildable smoke command; line-level docs scan misses wrapped claims. |
| Repo-precedent conflicts | Finding | Docs are hard-wrapped; TUI layout uses Lip Gloss display width. |
| Artifact-class precedent | Finding | Build-tag-only command package needs explicit no-tag boundary proof. |
| YAGNI violations | Clean | Heavy gates tied to concrete release/doc drift findings. |
| Missing failure modes | Finding | Wrapped overclaim prose and default-buildable smoke command remain plausible. |
| Security/privacy at architecture level | Finding | Default-buildable smoke command exposes mock-backed smoke behavior as installable. |
| Infrastructure impact | Clean | CI/release/Homebrew impacts declared. |
| Multi-component validation | Finding | Frame proof needs terminal display-width semantics. |
| Declared integration proof | Clean | D76-D78 and D80 materially resolved. |
| Contributed UI rendering proof | Clean | No plugin UI. |
| Rollback story | Clean | Source revert, draft retention, asset deletion/patch release, and tap rollback described. |
| Simpler alternative not considered | Finding | Source/build-tag guard tests and paragraph-level docs scanning. |
| User-intent drift | Finding | D85 can still let docs overstate automation. |
| Existence/runtime-validity | Finding | D84 leaves no-tag runtime boundary unproven. |

**Options the author may not have considered:**
1. Smoke-source boundary test for exact build tags and no-tag Unix failure.
2. Paragraph/table-row docs scanner.

**Verdict reasoning:** D80-D83 are resolved in shape, but smoke package no-tag boundary and docs line-wrap false negatives remain Important blockers.

## Cycle 22

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D87` [Security/privacy / Existence-runtime-validity] [design:84-86,94-96]: Smoke source guard names specific paths plus vague "smoke helper files"; smoke-only helper code elsewhere can compile into release builds and evade the guard. Recommendation: define checked smoke-source manifest or repository-wide guard for smoke-only symbols/constructors and require exact `//go:build tui_smoke && !windows` unless explicitly test-only allowlisted.
- `D88` [Missing failure modes / User-intent drift] [design:562-588]: Paragraph claim unit catches hard-wrapped false negatives but can merge unrelated sentences into false positives. Recommendation: unwrap prose, split into sentence claim units, scan table rows separately, and keep paragraph context only for reporting.
- `D89` [Declared integration proof / Infrastructure impact] [design:511-530]: GoReleaser fallback parser asserts selected ids/binaries but does not recursively scan scalar strings under artifact/publish sections. Recommendation: recursively scan all scalar strings under artifact/publish sections for forbidden smoke tokens, then layer id/binary assertions.

**Findings (Minor):**
- `D90` [Artifact-class precedent / YAGNI] [design:422-430]: `cmd/ratchet-release-guard` adds an untagged installable helper command. Recommendation: use `internal/releaseguard` and a Go test or `tools/` wrapper, or justify public `cmd`.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | D88/D89 weaken honest harness/release evidence boundaries. |
| Assumptions under attack | Finding | Named smoke globs, paragraph claim units, and id/binary-only fallback are not fail-closed. |
| Repo-precedent conflicts | Finding | Repo `cmd` surface is product command; release guards/scripts are internal tooling. |
| Artifact-class precedent | Finding | Internal release guard should not create user-facing command surface. |
| YAGNI violations | Finding | Public helper command shape is unnecessary. |
| Missing failure modes | Finding | Release-build leakage, docs false positives, nested GoReleaser config contamination. |
| Security/privacy at architecture level | Finding | Smoke-only code can compile into release builds outside named paths. |
| Infrastructure impact | Finding | D89 affects artifact guard reliability; D90 expands `go build ./...` command surface. |
| Multi-component validation | Finding | Docs-to-test validation can pass/fail based on claim-unit artifacts rather than evidence semantics. |
| Declared integration proof | Finding | GoReleaser fallback proof incomplete for nested publish config. |
| Contributed UI rendering proof | Clean | No plugin-contributed UI. |
| Rollback story | Clean | Detection gap, not rollback-text gap. |
| Simpler alternative not considered | Finding | Source manifest, sentence-aware scanner, internal releaseguard package. |
| User-intent drift | Finding | Docs guard could police wording artifacts instead of proof boundary. |
| Existence/runtime-validity | Finding | D87/D89 remain executable-contract gaps. |

**Options the author may not have considered:**
1. Smoke-source manifest with unlisted smoke symbol/file failures.
2. Sentence-aware docs scanner.
3. `internal/releaseguard` package driven by tests and shell wrapper.

**Verdict reasoning:** D80-D86 are materially resolved, but fail-closed smoke source isolation, docs claim modeling, and GoReleaser fallback scalar coverage remain Important blockers.

## Cycle 23

### Adversarial Review Report
**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D91` [Existence/runtime-validity / Repo-precedent conflicts] [design:86-93; existing `cmd/ratchet/harness_smoke_test.go`, `internal/acp/harness_smoke_test.go`, `internal/daemon/harness_smoke_test.go`, `internal/mcp/harness_smoke_test.go`]: Smoke-source guard still is not mechanically crisp: exact manifest is mixed with vague smoke helper constructor names and a broad unmanifested pathname rule. This can be noisy because existing smoke tests use smoke naming, or miss neutral-name smoke behavior. Recommendation: exact manifest schema and exact token/path scope: non-test Go files with `tui_smoke` build tags, exact exported helper names, explicit checked allowlist for existing `*_smoke_test.go`; remove vague constructor-name language.
- `D92` [User-intent drift / Missing failure modes] [design:555-560,657-659,267-279]: Docs may overclaim because public wording says `ratchet-tui-smoke` proves interactive chat/slash/shortcut behavior, but design PTY-proves only `pty-proven` slash rows and focused/deferred-proves the rest. Docs guard allows claims assigned to `ratchet-tui-smoke`, so broad "slash commands are smoke-proven" can pass though `/model`, `/sessions`, `/provider add`, `/jobs`, `/team`, `/mcp`, and others are not PTY-proven. Recommendation: docs must say selected/PTY-proven slash commands or enumerate `/help`, `/provider list`, `/tree`, `/mode`, `/trust`, `/exit`; fail broad slash-command evidence claims unless referencing the classification table.
- `D93` [Declared integration proof / Existence/runtime-validity] [design:335-345,676; `cmd/ratchet/main.go`:143-177]: Design includes public `ratchet help` / `printUsage`, but the mechanical guard only extracts `commands.Parse`, `helpCmd`, and autocomplete. Fixed built-binary help assertion proves current strings but can drift with command spec. Recommendation: add `printUsage` extractor tied to typed command spec, or narrow packaged-help contract to fixed mode/trust/tree entries.
- `D94` [Infrastructure impact / Missing failure modes] [design:427-442]: `internal/releaseguard` may be invoked through `go test`, but design does not require `-count=1` or non-cacheable invocation for generated `dist`. Cached test can pass without rereading fresh artifacts. Recommendation: wrapper uses `go test -count=1` with explicit manifest path/env, or small `go run` tool entrypoint that always reads supplied dist.

**Findings (Minor):**
- None.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | D92 weakens the honest harness-evidence boundary by allowing broad slash-command wording. |
| Assumptions under attack | Finding | Design assumes naming-based smoke scans, `ratchet-tui-smoke` docs assignment, and `go test`-driven guards are fail-closed. |
| Repo-precedent conflicts | Finding | Existing smoke tests use smoke naming, so D91 path-name scan needs exact scope/allowlist. |
| Artifact-class precedent | Finding | Release/artifact guard code is tool-like; cacheable tests are weaker than explicit smoke command execution. |
| YAGNI violations | Finding | Broad repo-wide smoke path scanning and top-level CLI-help alignment risk more machinery than needed. |
| Missing failure modes | Finding | D92 and D94 leave overclaim/stale-artifact-pass modes. |
| Security/privacy at architecture level | Finding | D91 source isolation is partly naming-based; release scans mitigate. |
| Infrastructure impact | Finding | D94 affects release guard reliability. |
| Multi-component validation | Finding | D92/D93 validate docs/help without proving exact command-runtime boundary. |
| Declared integration proof | Finding | D93 leaves `printUsage` outside the slash-command matrix. |
| Contributed UI rendering proof | Clean | No plugin-contributed UI. |
| Rollback story | Clean | Rollback story remains source revert plus draft retention/asset/tap correction. |
| Simpler alternative not considered | Finding | Generated temp smoke main or narrower docs wording; tool entrypoint for releaseguard. |
| User-intent drift | Finding | D92 can report broader slash-command proof than users get. |
| Existence/runtime-validity | Finding | D91-D94 are executable-contract gaps. |

**Options the author may not have considered:**
1. Generated smoke main plus manifest-free tag guard to reduce command/source-manifest surface.
2. Narrow docs language: "Unix PTY proof for chat, core shortcuts, and selected slash-command matrix."
3. Tool entrypoint for releaseguard so artifact inspection never has test-cache ambiguity.

**Verdict reasoning:** D87-D90 were addressed, but vague naming scans, broad docs claims, incomplete CLI-help drift checks, and cached artifact-guard execution remain Important blockers.

## Cycle 24

### Adversarial Review Report

**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D95` [Security/privacy / Missing failure modes] [design:253-260; `README.md`:90-99]: `/trust reset` is policy-sensitive, but the PTY evidence only requires reset text. README says reset clears runtime slash-command rules and does not delete persisted grants; the design does not require a follow-up `/trust list` and `/trust grants` check proving runtime smoke rules are gone while persisted grants remain as intended. Recommendation: after `/trust reset`, assert effective runtime allow/deny rules reset to config defaults and persistent grants are unchanged unless the design explicitly changes that contract.
- `D96` [User-intent drift / Existence-runtime-validity] [design:607-628]: The docs overclaim predicate can still miss common evidence wording because its evidence-token list omits `test`, `tested`, `tests`, `exercised`, and `asserted`. A public claim like "full TUI slash commands are tested by the binary harness" can evade even though it overstates the selected `pty-proven` matrix. Recommendation: include those evidence terms, or invert the guard so any interactive TUI/slash/shortcut evidence claim must name `ratchet-tui-smoke` plus selected/PTY-proven scope.

**Findings (Minor):**
- `D97` [Repo-precedent conflicts / Artifact-class precedent] [design:207-216,172-175; `internal/tui/components/jobpanel.go`:181; `internal/tui/app.go`:301-303]: The shortcut matrix names `ctrl+j` as the job-panel return path, while the rendered job panel advertises `Esc: close` and App handles that path. The prose later says panels "toggle/escape back," but the matrix itself is the source-of-truth-looking table and omits the advertised `Esc` job-panel shortcut. Recommendation: add a job-panel `Esc` row or explicitly say the frame-return assertion covers both `ctrl+j` and `Esc`.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | No repo-local `AGENTS.md`, `CLAUDE.md`, or `docs/design-guidance.md` exists; reviewed `RATCHET.md`, `README.md`, and referenced docs. |
| Assumptions under attack | Finding | The docs guard assumes a narrow evidence vocabulary; trust reset assumes rendered text is enough proof. |
| Repo-precedent conflicts | Finding | Job panel advertises `Esc: close`, but the matrix emphasizes only `ctrl+j` for that panel. |
| Artifact-class precedent | Finding | Shortcut matrices should include advertised key hints from rendered component UI. |
| YAGNI violations | Clean | Heavy release/docs guards are tied to prior concrete release-contamination and overclaim risks. |
| Missing failure modes | Finding | `/trust reset` could delete persisted grants or fail to clear runtime rules while still rendering expected text. |
| Security/privacy at architecture level | Finding | Trust rules/grants are sensitive local policy metadata; reset semantics need state proof, not only output proof. |
| Infrastructure impact | Clean | CI/release impact is explicitly described; no new cloud/IAM/secrets/migrations are introduced beyond existing release publishing. |
| Multi-component validation | Finding | Trust reset is not proven across TUI command, daemon state, and persisted grant store. |
| Declared integration proof | Finding | Runtime-integrated trust-command proof is incomplete for reset state semantics. |
| Contributed UI rendering proof | Clean | No plugin-contributed host UI is involved. |
| Rollback story | Clean | Source revert plus draft retention/asset/tap correction remains adequate. |
| Simpler alternative not considered | Finding | A state-after-reset assertion is cheaper than broadening the whole PTY suite. |
| User-intent drift | Finding | Docs can still overclaim full TUI/slash proof using unlisted evidence verbs. |
| Existence/runtime-validity | Finding | The negative docs predicate is not mechanically complete for likely public wording. |

**Options the author may not have considered:**
1. Trust-state invariant after each mutating slash command: after allow/deny/persist/revoke/reset, run the minimum follow-up state query needed to prove the daemon/store contract, not only command output.
2. Docs-claim allowlist: instead of scanning for forbidden phrasing, require every TUI evidence sentence/table row to match one of a few allowed claim templates for `ratchet` or `ratchet-tui-smoke`.

**Verdict reasoning:** The design is much tighter than earlier cycles, but two unresolved Important gaps remain: policy-sensitive reset behavior is not state-proven, and the docs guard can still miss realistic overclaim wording.

## Cycle 25

### Adversarial Review Report

**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D98` [Missing failure modes / Security/privacy] [design:249-261; `internal/daemon/trust.go`:62-70]: D95 added reset checks for runtime rules and persistent grants, but still does not explicitly require proving `/trust reset` restores the daemon mode to the config default after the PTY sequence has changed mode through `permissive`, `locked`, `sandbox`, and `custom`. `ResetTrust` resets both mode and rules, and mode is policy-sensitive. Recommendation: after `/trust reset`, require `/trust list` to assert `Mode:` equals the expected config-default mode, not just that runtime allow/deny rules are gone and grants remain.
- `D99` [Infrastructure impact / Rollback story] [design:494-500,660,672-675,727-728]: The design acknowledges Homebrew/tap safety is post-publish audit plus rollback, but never specifies the actual tap postcheck mechanics. The release workflow section defines draft GitHub asset postcheck in detail, while tap/cask checking remains "after-the-fact tap/cask reference check." That can degrade into a non-executable rollback story after GoReleaser has already pushed the tap. Recommendation: specify the release workflow step: fetch/clone `GoCodeAlone/homebrew-tap` after publish, identify the generated cask file and commit/branch, scan it for forbidden smoke names, fail the workflow on contamination, and print the exact rollback instruction or revert target.

**Findings (Minor):**
- `D100` [User-intent drift / Existence-runtime-validity] [design:610-617]: D96 expanded the docs overclaim evidence vocabulary, but common evidence words like `validated`, `validation`, `guarded`, and `e2e` still evade the predicate. A sentence such as "full TUI slash-command validation is in the binary harness" can still overclaim. Recommendation: invert to an allowlist for TUI evidence claim templates, or add these remaining evidence terms.
- `D101` [Declared integration proof] [design:707-708]: The integration matrix classifies "Slash help/autocomplete/CLI help" as `runtime-integrated`, but the proof is mixed: built `ratchet help` is runtime, while autocomplete and most command-surface alignment are focused model/AST tests, not host runtime proof. Recommendation: split the row into runtime CLI help and config/focused command-surface guard, or mark the row as mixed with exact subproof classifications.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | No repo-local `AGENTS.md`, `CLAUDE.md`, or `docs/design-guidance.md` exists; reviewed `README.md`, `RATCHET.md`, and referenced docs. |
| Assumptions under attack | Finding | Trust reset proof assumes rules/grants are the full reset contract, but daemon mode is also reset state. |
| Repo-precedent conflicts | Clean | Smoke tests, PTY integration tests, `printUsage`, command help/autocomplete, job panel shortcuts, and release workflows were spot-checked against the design. |
| Artifact-class precedent | Clean | The proposed artifact classes match existing locations: `cmd/ratchet` binary smoke, `internal/tui` PTY tests, docs guard tests, and GoReleaser config. |
| YAGNI violations | Finding | The docs overclaim guard remains token-list heavy; a claim-template allowlist may be simpler and less porous. |
| Missing failure modes | Finding | Reset mode-state proof and executable Homebrew/tap postcheck mechanics are missing. |
| Security/privacy at architecture level | Finding | Trust mode/rules/grants are sensitive local policy state; reset evidence should prove all affected state. |
| Infrastructure impact | Finding | Release asset gating is detailed, but Homebrew/tap post-publish audit is under-specified. |
| Multi-component validation | Finding | Trust reset still needs proof across TUI command, daemon mode/rule state, and persistent grant store. |
| Declared integration proof | Finding | The slash help/autocomplete/CLI help integration row mixes runtime and focused/static proof under one classification. |
| Contributed UI rendering proof | Clean | No plugin-contributed host UI is involved; this design covers the primary Bubble Tea TUI. |
| Rollback story | Finding | Homebrew/tap rollback is named, but the detection step that would trigger it is not concrete. |
| Simpler alternative not considered | Finding | Docs evidence allowlisting would be simpler than continuing to patch a growing forbidden-token predicate. |
| User-intent drift | Finding | Overclaim wording can still imply broader TUI/slash validation than the selected PTY matrix proves. |
| Existence/runtime-validity | Finding | Tap/cask postcheck and docs-overclaim detection remain insufficiently mechanical. |

**Options the author may not have considered:**
1. Claim-template docs guard: replace the negative token predicate with a small allowlist of permitted TUI evidence statements for `ratchet` and `ratchet-tui-smoke`.
2. Dedicated tap postcheck helper: add a `releaseguard` mode that takes a checked-out Homebrew tap path and cask name, then scans the actual generated cask after GoReleaser pushes it.

**Verdict reasoning:** The latest commit resolves D95-D97 in text, but the design still leaves two Important gaps: `/trust reset` does not explicitly prove mode reset after mode mutations, and the Homebrew/tap audit is not specified enough to be an executable release safety check.

## Cycle 26

### Adversarial Review Report

**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D102` [Infrastructure impact / Rollback story] [design:499-515; `.goreleaser.yaml`:44-56]: Homebrew tap postcheck assumes the tap branch `HEAD` is the GoReleaser cask commit. If another commit lands on `GoCodeAlone/homebrew-tap/main` between GoReleaser publish and postcheck, the audit can scan the wrong commit and print the wrong rollback SHA. Recommendation: resolve the exact cask-changing commit for the current release, e.g. `git log -1 -- Casks/ratchet-cli.rb` after clone, verify it contains the expected version/checksum context, scan that commit's cask content and changed-file list, and report that SHA as rollback target.

**Findings (Minor):**
- `D103` [Artifact-class precedent / Missing failure modes] [design:80-86; `cmd/ratchet/harness_smoke_test.go`:43-57]: Positive build assertions use `go build ./cmd/ratchet` and `go build -tags tui_smoke ./cmd/ratchet-tui-smoke` without `-o`, which writes binaries into the current working directory if implemented literally. Existing smoke tests build to `t.TempDir()`. Recommendation: require temp output paths for every positive build assertion.
- `D104` [Simpler alternative / Multi-component validation] [design:64-74; `internal/daemon/service.go`:499-575; `internal/daemon/testharness_test.go`:155-199]: The design keeps direct mock-provider DB seeding even though the production `AddProvider` RPC supports keyless `mock` providers. Direct seeding adds smoke-only daemon surface and bypasses provider validation/cache behavior. Recommendation: seed `e2e-mock` through `AddProvider` over the smoke gRPC client, or explicitly justify why DB seeding is required.
- `D105` [Existence/runtime-validity] [design:305-324]: The command-surface AST guard extracts only string-literal cases/entries, but does not require failure on nonliteral command cases, generated help rows, or computed autocomplete entries. A later refactor could hide a command from the guard. Recommendation: fail closed on any nonliteral command-like case/entry in guarded surfaces unless the typed spec explicitly marks it as generated and tests the runtime output.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | No repo-local `AGENTS.md`, `CLAUDE.md`, or `docs/design-guidance.md` exists; reviewed `README.md`, `RATCHET.md`, referenced docs, and workspace portfolio. |
| Assumptions under attack | Finding | Tap postcheck assumes branch HEAD is the relevant cask commit; command guard assumes literal-only command surfaces remain stable. |
| Repo-precedent conflicts | Finding | Existing smoke tests build to temp paths; design build checks omit `-o`. |
| Artifact-class precedent | Finding | Release/tap audit shape is close, but exact tap commit selection is not tied to the published cask artifact. |
| YAGNI violations | Finding | Direct DB seeding may be unnecessary because `AddProvider` already seeds keyless mock providers through the real daemon boundary. |
| Missing failure modes | Finding | Wrong tap HEAD, cwd binary pollution, and nonliteral command-surface drift are not covered. |
| Security/privacy at architecture level | Clean | Build-tag isolation, temp state/workdir, Unix socket permissions, and redaction boundaries are explicit. |
| Infrastructure impact | Finding | Homebrew/tap audit is after-the-fact and must identify the exact published tap commit to make rollback reliable. |
| Multi-component validation | Finding | Provider setup can be proven through daemon RPC instead of smoke-only DB seeding. |
| Declared integration proof | Finding | Homebrew/tap post-publish audit is declared but not pinned to the exact consumed cask commit. |
| Contributed UI rendering proof | Clean | No plugin-contributed host-shell UI is claimed; direct Bubble Tea TUI proof is the relevant surface. |
| Rollback story | Finding | Tap rollback target can be wrong if postcheck scans branch HEAD after another tap commit. |
| Simpler alternative not considered | Finding | Use `AddProvider` RPC for mock seeding and temp `-o` build outputs. |
| User-intent drift | Clean | Slice remains aligned with TUI binary verification while keeping Windows interactive PTY deferred. |
| Existence/runtime-validity | Finding | Some planned verification commands and AST extraction rules are not fail-closed enough as written. |

**Options the author may not have considered:**
1. Exact tap commit audit: scan `git show <cask-sha>:Casks/ratchet-cli.rb` where `<cask-sha>` is the latest commit touching the cask for the current release, not branch `HEAD`.
2. RPC-based smoke provider setup: start smoke daemon, connect via `ConnectSmokeUnix`, call `AddProvider(alias=e2e-mock,type=mock,isDefault=true)`, then create the session.
3. Temp-output build helper: one shared helper for positive build checks that always passes `-o <temp>` and never writes binaries into the checkout.

**Verdict reasoning:** The design has converged on most prior issues, but D102 is still an unresolved Important release-safety gap: the Homebrew tap audit can inspect the wrong commit and produce an unusable rollback target.

## Cycle 27

### Adversarial Review Report

**Phase:** design
**Artifact:** docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D106` [Infrastructure impact / Rollback story] [design:499-515; `.goreleaser.yaml`:44-56]: Homebrew postcheck is still cask-only, but the actual tap has formula surfaces. The design is framed around `homebrew_casks`, `RATCHET_RELEASE_GUARD_CASK`, `Casks/ratchet-cli.rb`, and a single cask-changing commit. A cask-only guard can pass while another user-visible install surface remains stale, or fail because the assumed cask path does not exist. Recommendation: discover all ratchet tap files from the tap checkout, verify each relevant install surface for version/checksum/forbidden smoke tokens, and record rollback SHA per changed path. If formula/root tap files are intentionally out of scope, mark that as accepted release risk.
- `D107` [User-intent drift / Multi-component validation] [design:617-672, shortcut matrix]: Docs guard wording can overclaim PTY proof for shortcuts that are only focused-test proven. The design says docs may claim `ratchet-tui-smoke` provides Unix PTY proof for "interactive chat, core shortcuts, selected/PTY-proven slash commands," but conditional `ctrl+h` and branch tree navigation/App switch behavior are focused tests. Recommendation: split docs language and guard templates into `PTY-proven shortcuts` and `focused-test-proven shortcuts`; reject broad PTY shortcut wording unless every named shortcut is actually exercised through the PTY harness.

**Findings (Minor):**
- `D108` [Security/privacy / Missing failure modes] [design:415-418]: Daemon cleanup fallback can kill by stale PID without proving process identity. The release-shaped startup smoke design allows terminating a temp pidfile process if cleanup leaves pid/socket behind, but does not require checking the PID still belongs to the ratchet daemon launched from the temp home/binary. Recommendation: before fallback termination, verify process identity using executable path, command line, start time, or socket/home ownership; if identity cannot be proven, fail with diagnostics instead of killing the PID.

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Finding | D106 conflicts with prior repo release-retro guidance to verify all tap install surfaces when present. |
| Assumptions under attack | Finding | The design assumes the tap artifact is cask-only and that shortcut evidence can be summarized as PTY-proven core shortcuts. |
| Repo-precedent conflicts | Finding | Current Homebrew tap shape and TUI shortcut behavior do not match the simplified claims. |
| Artifact-class precedent | Finding | Release artifact checks need to cover all generated install artifacts, not only the GoReleaser stanza name. |
| YAGNI violations | Clean | The harness pieces remain tied to manual-gap closure. |
| Missing failure modes | Finding | Stale PID/PID reuse and stale non-cask tap surfaces are not covered. |
| Security/privacy at architecture level | Finding | D108 is a local runner safety issue. |
| Infrastructure impact | Finding | D106 can break or under-verify release/tap automation. |
| Multi-component validation | Finding | D107 blurs PTY runtime proof with focused component/App tests. |
| Declared integration proof | Finding | D106 leaves Homebrew release integration proof incomplete. |
| Contributed UI rendering proof | Clean | No plugin-contributed host-shell UI is claimed. |
| Rollback story | Finding | D106 rollback is incomplete if only one cask-changing commit is tracked. |
| Simpler alternative not considered | Finding | Discover tap files from checkout and verify each path; split PTY/focused docs claims instead of broad allowlists. |
| User-intent drift | Finding | D107 can misrepresent the full TUI manual harness gap closure as broader than planned evidence. |
| Existence/runtime-validity | Finding | The expected `Casks/ratchet-cli.rb` path may not exist while root/formula files do. |

**Options the author may not have considered:**
1. Discover tap files from checkout and verify every `ratchet` install surface by path.
2. Shortcut evidence classification table parallel to slash-command proof classes.
3. PID identity check before fallback daemon termination.

**Verdict reasoning:** FAIL because D106 and D107 are unresolved Important issues: the tap audit can miss formula/root install surfaces, and docs can overclaim PTY proof for focused shortcut behavior.
