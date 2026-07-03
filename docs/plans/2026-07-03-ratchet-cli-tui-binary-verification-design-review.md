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
