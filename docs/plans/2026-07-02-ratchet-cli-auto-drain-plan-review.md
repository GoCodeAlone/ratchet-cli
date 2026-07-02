### Adversarial Review Report

**Phase:** plan
**Artifact:** `docs/plans/2026-07-02-ratchet-cli-auto-drain.md`
**Status:** PASS

**Findings (Critical):**
- None.

**Findings (Important):**
- None.

**Findings (Minor):**
- `P1` [User-intent drift] [Scope Manifest/Task 6]: One PR is efficient for this contained slice but makes the feature/docs/closeout review boundary larger than a docs-only change. Recommendation: keep the PR body structured by task and avoid adding the daemon scheduler or extension hooks while implementing. _Resolution: accepted; manifest out-of-scope list is explicit and the design keeps daemon scheduling deferred._
- `P2` [Verification-class mismatch] [Task 4/Task 6]: CLI watch runtime proof depends on the binary smoke path, which can be slower and more brittle than unit tests. Recommendation: preserve both focused command tests and the binary smoke so failures isolate parser/executor bugs from process-level integration bugs. _Resolution: accepted; plan requires both._
- `P3` [Platform assumptions] [Task 6]: Windows build proof is compile-only and does not execute the binary on Windows. Recommendation: treat Windows execution as CI/future runner coverage and keep cancellation tests context-driven so compile proof is meaningful locally. _Resolution: accepted; local plan requires amd64/arm64 Windows builds and context-driven tests._

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Plan maps workspace guidance to existing repo patterns, Windows builds, and no duplicated policy engine. |
| Assumptions under attack | Clean | Foreground watch, explicit command authority, polling, and local owner locks are all covered by tasks and out-of-scope boundaries. |
| Repo-precedent conflicts | Clean | Plan edits the same command, store, docs, and smoke-test surfaces used by existing ACP client functionality. |
| Artifact-class precedent | Clean | Plan follows existing `cmd/ratchet` parser tests, ACP binary smoke tests, and docs guard tests. |
| YAGNI violations | Clean | No daemon scheduler, launch profile, hook SDK, or new policy store is planned. |
| Missing failure modes | Clean | Error return, busy owner, cancellation, empty queue, max cycles, and prompt-free output are all test targets. |
| Security / privacy at architecture level | Clean | Prompt persistence is documented and output is constrained to aggregate cycle data. |
| Infrastructure impact | Clean | No cloud resources, network endpoints, migrations, release tags, or registry changes. |
| Multi-component validation | Clean | Binary smoke exercises the built CLI and fixture ACP agent together. |
| Declared integration proof | Clean | Integration matrix classifies the ACP process client and local store as runtime-integrated with concrete proof. |
| Contributed UI rendering proof | Clean | No UI contribution. |
| Rollback story | Clean | Each task includes revert-only rollback; no schema/data migration. |
| Simpler alternative not considered | Clean | Design considered and rejected shell-loop docs only; plan implements the chosen explicit worker. |
| User-intent drift | Finding | One foreground-worker PR is narrower than daemon background scheduling; accepted because policy matrix requires this prerequisite first. |
| Existence / runtime-validity | Clean | Plan mutates existing files and requires real CLI help/invocation through command tests and binary smoke. |
| Over-decomposition / under-decomposition | Clean | Six tasks are reviewable and match the single-PR scope; TDD test tasks are separated before implementation. |
| Verification-class mismatch | Finding | Binary smoke is heavier than unit tests but appropriate for CLI boundary proof when paired with focused tests. |
| Auth/authz chain composition | Clean | No server-side auth chain is introduced; authorization remains explicit operator command execution. |
| Hidden serial dependencies | Clean | Tasks intentionally serialize shared files in one PR; no parallel execution assumption is made. |
| Missing rollback wiring | Clean | Rollback is included per task. |
| Missing integration proof | Clean | Binary smoke covers the CLI-to-agent boundary. |
| Missing declared integration matrix | Clean | Plan has an integration matrix with runtime-integrated/config-only rows. |
| Missing contributed UI route proof | Clean | No UI routes. |
| Infrastructure verification mismatch | Clean | No infrastructure changes. |
| Plugin-loader runtime layout | Clean | No plugin process layout changes. |
| Config-validation schema rules | Clean | No generated config schema artifacts. |
| Identifier / naming-convention match | Clean | Flag names follow existing dashed CLI flag style: `--max-per-cycle`, `--stop-when-empty`, `--json`. |
| Planned-code compile-validity | Clean | Plan does not embed compiled code snippets beyond shell commands. |

**Options the author may not have considered:**

1. Two PRs: core/library first, CLI/docs second. This would reduce review size but add overhead without isolating a deployable user-visible milestone.
2. Docs-first release note only: not enough because user asked to continue implementation and the previous matrix explicitly queued auto-drain as the next phase.
3. CI-only Windows proof: rejected because local cross-builds are cheap and the user specifically asked to build for Windows.

**Verdict reasoning:** PASS. The plan now preserves red-before-green ordering for internal, CLI, binary, and docs guards, and its verification matches a CLI feature that crosses a child-process boundary.
