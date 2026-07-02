### Adversarial Review Report

**Phase:** plan
**Artifact:** `docs/plans/2026-07-02-acpx-flow-hardening.md`
**Status:** PASS

**Findings (Critical):**
- None.

**Findings (Important):**
- None.

**Findings (Minor):**
- `P1` [Platform assumptions] [Task 3]: Initial binary smoke wording could have led implementation to use shell syntax such as `sh -c` or `echo`, weakening Windows portability evidence. Recommendation: require a built test binary action command instead of platform shell syntax. _Resolution: resolved in plan commit `931cdfd`; Task 3 now forbids platform shell syntax in smoke._
- `P2` [Verification-class mismatch] [Task 2/Task 4]: Action runner tests can overfit to fake runners and miss real CLI process behavior. Recommendation: preserve both internal fake-runner tests and built CLI binary smoke with a real action process. _Resolution: accepted; plan includes both._
- `P3` [Security/privacy] [Task 5]: Docs must not imply cwd containment is a sandbox. Recommendation: explicitly document action output sensitivity and keep sandbox/path/network expansion deferred. _Resolution: accepted; Task 5 requires sensitive command output and deferred TypeScript/sandbox language._

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Plan maps Windows, no duplicate policy engines, and sensitive metadata guidance to tasks. |
| Assumptions under attack | Clean | `--allow shell`, cwd containment, and bounded output assumptions are covered by tests/docs. |
| Repo-precedent conflicts | Clean | Plan edits existing ACP client flow/runtime/CLI/docs files and follows sibling tests. |
| Artifact-class precedent | Clean | Tests follow existing `internal/acpclient/*_test.go`, `cmd_acp_client_test.go`, binary smoke, and docs guard shape. |
| YAGNI violations | Clean | TypeScript runtime, replay UI, branching DSL, profile distribution, and self-evolution stay out of scope. |
| Missing failure modes | Clean | Missing grants, cwd escape, non-zero action exit, failed-state persistence, and truncation are explicit test targets. |
| Security / privacy at architecture level | Finding | Docs must avoid sandbox overclaim; Task 5 covers this. |
| Infrastructure impact | Clean | No infra, release, registry, migration, daemon protocol, or network changes. |
| Multi-component validation | Clean | Binary smoke exercises built CLI, local process action, persisted bundle, and fixture ACP agent. |
| Declared integration proof | Clean | Integration matrix classifies runtime-integrated and deferred surfaces. |
| Contributed UI rendering proof | Clean | No UI contribution. |
| Rollback story | Clean | Runtime-affecting tasks include revert rollback notes. |
| Simpler alternative not considered | Clean | Docs-only and TypeScript runtime alternatives are handled in the design. |
| User-intent drift | Clean | Plan advances flow orchestration because archive/import/export and compare are already shipped. |
| Existence / runtime-validity | Clean | Existing flow command and tests were inspected; plan mutates real consumer surfaces. |
| Over-decomposition / under-decomposition | Clean | Six tasks separate internal tests/runtime, CLI tests/runtime, docs, and verification. |
| Verification-class mismatch | Finding | Fake-runner tests alone would be insufficient; binary smoke resolves this. |
| Auth/authz chain composition | Clean | No server auth chain; flow-local explicit `--allow` preflight only. |
| Hidden serial dependencies | Clean | One PR intentionally serializes shared files. |
| Missing rollback wiring | Clean | Rollback notes are present on runtime/docs tasks. |
| Missing integration proof | Clean | Task 4/6 include built CLI representative invocation. |
| Missing declared integration matrix | Clean | Plan includes an integration matrix. |
| Missing contributed UI route proof | Clean | No UI routes. |
| Infrastructure verification mismatch | Clean | No infrastructure change. |
| Plugin-loader runtime layout | Clean | No plugin process layout. |
| Config-validation schema rules | Clean | Flow JSON validation is covered by runtime tests, not external schema files. |
| Identifier / naming-convention match | Clean | Flags follow existing dashed CLI style: `--allow`, `outside-cwd`; JSON stays snake_case. |
| Planned-code compile-validity | Clean | No embedded Go snippets beyond identifiers and command examples. |

**Options the author may not have considered:**

1. Split into two PRs: action runtime first, CLI/docs second. This would reduce review size but leaves no user-visible milestone in PR1 and adds overhead.
2. Add only permission preflight before actions. Safer but low value; action nodes are the feature that makes the preflight useful.
3. Add shell-string action mode now. More ergonomic, but direct command+args is more portable and easier to reason about for the first slice.

**Verdict reasoning:** PASS. The plan now avoids shell-specific smoke tests, preserves both fake and real process validation, and keeps the policy/sandbox claims bounded to explicit flow preflight and cwd containment.
