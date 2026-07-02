### Adversarial Review Report

**Phase:** plan
**Artifact:** `docs/plans/2026-07-02-ratchet-cli-extension-hooks-profiles.md`
**Status:** PASS

**Findings (Critical):**
- None.

**Findings (Important):**
- None.

**Findings (Minor):**
- `P1` [verification-class mismatch] [Task 2]: Runtime hook error policy is subtle; plan says trusted hook execution errors may fail parent operations but does not require a test distinguishing trusted execution failure from skipped untrusted hooks. Recommendation: add an implementation-time test for both paths. _Resolution: accepted as implementation detail under Task 2 hook runtime tests._
- `P2` [hidden serial dependencies] [Task 5/Task 6]: Both tasks edit `cmd/ratchet/cmd_acp_client.go`; they are in the same PR row, so no branch collision, but the implementer must do Task 5 before Task 6. Recommendation: keep task order strict in PR2. _Resolution: manifest orders Task 5 before Task 6._
- `P3` [docs/release verification] [Task 8]: Fixed tag `v0.24.0` is correct if no concurrent release occurs; plan checks remote tag absence but does not name fallback. Recommendation: if tag exists, stop and reconcile instead of reusing or deleting it. _Resolution: release step says ensure remote tag absent; implementation must stop on conflict._

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Plan extends existing hooks/plugins/ACP client packages; no duplicate extension runtime. |
| Assumptions under attack | Clean | Design assumptions map to docs/tests: user-hook exception, built-in profile precedence, no managed hook config. |
| Repo-precedent conflicts | Clean | Follows existing command/test/doc/release plan patterns in repo. |
| Artifact-class precedent | Clean | Hook/plugin/acpclient tests are colocated with sibling package tests. |
| YAGNI violations | Clean | No TS SDK, daemon scheduler, gateway, raw ACPX replay, or managed hook system. |
| Missing failure modes | Clean | Covers path escape, untrusted/disabled/unsupported hooks, workdir absence, profile trust, built-in shadowing, tag conflict. |
| Security/privacy architecture | Clean | No secret values in profiles; command display truncation/redaction; local trust store only. |
| Infrastructure impact | Clean | Local state plus release tag only; no production deploy. |
| Multi-component validation | Clean | Plugin loader -> CLI -> runtime and profile -> ACP fixture proofs are planned. |
| Declared integration proof | Clean | Plugin `hooks` and `acpProfiles` get host loader and command proof. |
| Contributed UI rendering proof | Clean | No UI contribution. |
| Rollback story | Clean | Patch release/revert plus inert local stores. |
| Simpler alternative not considered | Clean | Design rejected docs-only and full SDK alternatives. |
| User-intent drift | Clean | Implements extension hooks/profile distribution next phase; leaves broader backlog deferred. |
| Existence/runtime-validity | Clean | Existing consumed files/commands are present; new commands get binary smoke/doc guard. |
| Over/under decomposition | Clean | 8 tasks across 4 PRs; each PR is reviewable and revertible. |
| Verification-class mismatch | Finding | Task 2 should ensure trusted hook failure vs untrusted skip behavior; P1. |
| Auth/authz chain composition | Clean | No server auth chain; local trust store gates execution. |
| Hidden serial dependencies | Finding | Task 5 and 6 share CLI file but are in same PR and ordered; P2. |
| Missing rollback wiring | Clean | Task 8 release/closeout includes rollback evidence and inert store behavior. |
| Missing integration proof | Clean | ACP fixture binary smoke and plugin loader proof included. |
| Missing declared integration matrix | Clean | Requirements trace plus task proofs cover plugin hook/profile integrations. |
| Missing contributed UI route proof | Clean | No UI route. |
| Infrastructure verification mismatch | Clean | Release workflow and Homebrew cask verification named; no infra apply. |
| Plugin-loader runtime layout | Clean | No external gRPC plugin process; plugin manifests are local directory fixtures. |
| Config-validation schema rules | Clean | No Workflow/wfctl config schema emitted. |
| Identifier/naming convention match | Clean | Uses existing camelCase JSON (`acpProfiles`) and existing CLI naming style. |
| Planned-code compile-validity | Clean | Plan does not embed compile-sensitive Go snippets beyond identifiers. |

**Options the author may not have considered:**

1. Merge hook trust and ACP profiles into one PR: faster but too large and harder to revert.
2. Make project/plugin hooks permanently list-only: safer but fails the extension-hooks functionality goal.
3. Make profiles a top-level `ratchet profile` command: clearer for future global config, but the current use case is ACP launch specs, so nesting under `acp client profiles` is more precise.

**Verdict reasoning:** PASS. The plan is structurally aligned and executable. Minor risks are noted for implementation discipline; none require redesign or manifest changes.
