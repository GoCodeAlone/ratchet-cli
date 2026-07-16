### Adversarial Review Report

**Phase:** plan
**Artifact:** `docs/plans/2026-07-15-ratchet-cli-provider-operation-contract.md`
**Status:** PASS

**Findings (Critical):**
- `P1` [verification-class mismatch] [Task 1]: changing `wantErr` in the existing table would not create a red test because the harness uses `strings.Contains`. Recommendation: add a separate exact-equality regression. _Resolution: Task 1 now requires an exact standalone test and names the expected current failure suffix._
- `P2` [scope-manifest mismatch] [Task 4/Scope Manifest]: one declared PR conflicted with a required post-merge closeout PR. Recommendation: model closeout as Task 5/PR2 or omit the unapproved merge. _Resolution: manifest is now 5 tasks/2 PRs; Task 5 owns retro, lock completion, green closeout merge, and release._

**Findings (Important):**
- `P3` [existence/runtime-validity] [Task 4]: `ratchet provider operation --help` is not a documented help surface and can be parsed as an invalid operation ID. Recommendation: use a real credential-free command. _Resolution: installed proof uses `ratchet provider setup list --json`._
- `P4` [verification under-specification] [Task 4]: “repository snapshot command” and `<snapshot-dist>` were not directly executable. Recommendation: name GoReleaser's exact snapshot invocation and `dist`. _Resolution: exact commands are now present._

**Bug-class scan transcript:**
| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Real command boundary, exact race selector, Windows build, settled checks, and exact-merge releases are tasks. |
| Assumptions under attack | Clean | Retry, operation result, daemon availability, and Unix-only IPC constraints have explicit tests/non-goals. |
| Repo-precedent conflicts | Clean | Uses existing catalog tests, provider manager, binary smoke, docs guard, release, and retro paths. |
| Artifact-class precedent | Clean | The real durable-provider binary smoke stays in `harness_smoke_unix_test.go`; closeout follows prior two-PR plans. |
| YAGNI violations | Clean | No enum/RPC/schema/UI/provider/cancellation expansion. |
| Missing failure modes | Clean | Failed secret read, unchanged durable state, retry success, unknown/failure fields, and secret/error leakage are asserted. |
| Security / privacy | Clean | Sentinel/raw-error negative assertions and existing secret custody are explicit. |
| Infrastructure impact | Clean | No infrastructure or migration work; release publication is approved and non-destructive. |
| Multi-component validation | Clean | Built CLI → production daemon → SQLite/file-secret lifecycle is exercised. |
| Declared integration proof | Clean | Runtime/config-only integrations match the design matrix; no new dependency is declared. |
| Contributed UI route proof | Clean | No UI contribution exists. |
| Rollback story | Clean | Every runtime/release task has compatible rollback or repair-forward guidance. |
| Simpler alternative not considered | Clean | Mapping-only and schema-redesign alternatives remain rejected for concrete reasons. |
| User-intent drift | Clean | Covers the three review followups, README, Windows builds, green merges, releases, and retro. |
| Existence / runtime-validity | Important, resolved | P3/P4 replaced assumed commands with existing, exact commands. |
| Over/under-decomposition | Clean | Three implementation slices, one comprehensive gate, and one post-merge closeout task match two PR boundaries. |
| Verification-class mismatch | Critical, resolved | P1 now guarantees RED; command and multi-component classes have representative invocations. |
| Auth/authz chain composition | Clean | No auth/authz chain changes. |
| Hidden serial dependencies | Clean | Tasks 1-4 share one feature branch serially; Task 5 begins only from PR1 merge. |
| Missing rollback wiring | Clean | Rollback is present per task and for both releases. |
| Missing integration proof | Clean | Task 3 is the real boundary, with unit checks supporting it. |
| Missing declared integration matrix | Clean | Design matrix is fully represented by Tasks 1-4. |
| Missing contributed UI route proof | Clean | Not applicable. |
| Infrastructure verification mismatch | Clean | No IaC; release publication uses existing pre/post guards. |
| Plugin-loader runtime layout | Clean | No plugin process is built or loaded. |
| Config-validation schema rules | Clean | No Workflow config is emitted. |
| Identifier / naming-convention match | Clean | Existing protobuf enum, SQL states, command, branches, and Go test names are used. |
| Planned-code compile-validity | Clean | No embedded implementation snippet relies on invalid Go syntax or comparability. |

**Options the author may not have considered:**
1. Fold retro into PR1: impossible to score merge/CI/release evidence before PR1 merges.
2. Skip closeout release: conflicts with the standing requirement to release every merge.

**Verdict reasoning:** PASS after resolving P1-P4. The manifest now honestly represents both merge/release boundaries, and every behavioral claim has an executable proof.
