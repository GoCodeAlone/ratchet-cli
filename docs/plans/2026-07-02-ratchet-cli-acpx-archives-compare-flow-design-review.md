### Adversarial Review Report

**Phase:** design
**Artifact:** `docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow-design.md`
**Status:** FAIL

**Findings (Critical):**
- None.

**Findings (Important):**
- `D1` [Missing failure modes / multi-component validation] `## Design / Compare Run Artifacts`, `## Design / Flow Replay Bundles`: compare and flow bundles depend on "when the runner returns" event logs, but the design does not define how `Result`/`FlowPromptRunner` exposes those events. Implementation could ship bundle files with no real ACP boundary evidence. Recommendation: require a typed `EventLog`/`EventLogProvider` path from ACP client results into compare and flow, plus tests proving saved bundles contain events from fixture-agent prompt execution.
- `D2` [User-intent drift / security] `## Design / Archive/Event Log Compatibility`: raw export with absent sidecar writes an empty raw history plus a warning. That can be mistaken for a complete ACPX raw archive and silently loses replay data. Recommendation: raw export must fail when raw history is unavailable unless an explicit future lossy mode is designed; `summary` remains the backwards-compatible default.
- `D3` [Declared integration proof] `## Multi-Component Validation`: docs say raw ACPX event logs are no longer deferred, but validation does not require an ACPX-shaped fixture copied from current upstream archive shape with raw `history`. Recommendation: add a fixture test that imports `exported_by:"acpx"` archive with raw JSON-RPC `history`, preserves sidecar events, exports `--history raw`, and asserts JSON-RPC messages round-trip.

**Findings (Minor):**
- `D4` [YAGNI] `## Design / Flow Replay Bundles`: `sessions/<handle>/binding.json` and `sessions/<handle>/events.ndjson` may be overkill for PR1/PR2. Recommendation: keep them, but put them in the flow PR only and make archive/compare PRs independently useful.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Guidance favors Windows, local-sensitive data, and SDK reuse; design follows those. |
| Assumptions under attack | Finding | A1 is fragile because generated raw logs are not defined; D1/D2 address it. |
| Repo-precedent conflicts | Clean | Design follows existing `internal/acpclient` archive/compare/flow surfaces and docs guard pattern. |
| Artifact-class precedent | Clean | Existing archive/compare/flow tests live in `internal/acpclient` and `cmd/ratchet`; design targets same artifact class. |
| YAGNI violations | Minor | Replay session bundle pieces are heavier but trace to ACPX replay requirements. |
| Missing failure modes | Finding | Raw-history absence would silently produce lossy raw exports; D2. |
| Security/privacy architecture | Finding | Empty raw export could hide data loss; sensitive local artifact handling otherwise covered. |
| Infrastructure impact | Clean | Local state layout only; no cloud/prod changes. |
| Multi-component validation | Finding | Missing typed event propagation proof; D1. |
| Declared integration proof | Finding | Needs upstream-shaped ACPX archive fixture proof; D3. |
| Contributed UI rendering proof | Clean | No UI contribution. |
| Rollback story | Clean | Additive local files and revert path documented. |
| Simpler alternative not considered | Clean | Docs-only and raw stdio tap alternatives considered. |
| User-intent drift | Finding | Raw archive compatibility could be claimed without real raw round-trip; D2/D3. |
| Existence/runtime-validity | Clean | Design edits existing repo surfaces and plans CLI/binary smoke; no external consumed manifest emitted. |

**Options the author may not have considered:**
1. Fail `--history raw` when unavailable: less convenient but prevents false ACPX replayability.
2. Two-tier event capture: preserve imported raw history exactly; generate normalized session/update/result events for new ratchet runs through typed SDK callbacks without claiming byte-for-byte stdio capture.

**Verdict reasoning:** FAIL until the design explicitly wires event logs from ACP client results into compare/flow bundles and rejects unavailable raw export instead of emitting a misleading empty archive.

## Cycle 2

### Adversarial Review Report

**Phase:** design
**Artifact:** `docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow-design.md`
**Status:** PASS

**Findings (Critical):**
- None.

**Findings (Important):**
- None.

**Findings (Minor):**
- `D5` [YAGNI] `## Design / Flow Replay Bundles`: replay-grade flow bundles are a sizable addition. Acceptable because they directly trace to the user's "flow orchestration" ask and ACPX replay contract; keep TypeScript runtime deferred.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Windows, SDK reuse, local-sensitive data, and deferred automation boundaries are explicit. |
| Assumptions under attack | Clean | A1 now fails closed for missing raw sidecar and distinguishes imported raw histories from normalized generated event logs. |
| Repo-precedent conflicts | Clean | Uses existing `internal/acpclient` archive/compare/flow and `cmd/ratchet` patterns. |
| Artifact-class precedent | Clean | Tests/docs targets match sibling archive/compare/flow tests and docs guard. |
| YAGNI violations | Minor | Flow replay bundle breadth is justified by ACPX source contract and user ask. |
| Missing failure modes | Clean | Missing raw history now errors instead of exporting lossy empty raw archives. |
| Security/privacy architecture | Clean | Sensitive prompt/response/stdout metadata is local-only with docs warnings and read-only replay. |
| Infrastructure impact | Clean | Only local state layout and release tag/assets; no cloud/prod change. |
| Multi-component validation | Clean | Fixture ACP agent, sidecar event counts, compare bundles, flow replay command, and docs guard are required. |
| Declared integration proof | Clean | Upstream-shaped ACPX raw archive fixture round-trip is now required. |
| Contributed UI rendering proof | Clean | No UI contribution. |
| Rollback story | Clean | Additive files and revert path documented. |
| Simpler alternative not considered | Clean | Docs-only, raw stdio tap, and TypeScript runtime alternatives considered. |
| User-intent drift | Clean | Archive, compare, and flow orchestration are all in scope; TypeScript runtime deferral is explicit. |
| Existence/runtime-validity | Clean | Existing CLI/test surfaces verified; no assumed nonexistent external consumer command. |

**Options the author may not have considered:**
1. Exact stdio byte tap: still rejected because it duplicates SDK transport behavior.
2. Raw-only archives by default: rejected because existing ratchet summary archive compatibility should remain stable.

**Verdict reasoning:** PASS. Prior Important findings D1-D3 are resolved in design commit `fdc2d30`; remaining issue is a scope-size caution, not a blocker.
