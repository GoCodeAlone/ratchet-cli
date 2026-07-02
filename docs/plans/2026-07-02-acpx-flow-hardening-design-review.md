### Adversarial Review Report

**Phase:** design
**Artifact:** `docs/plans/2026-07-02-acpx-flow-hardening-design.md`
**Status:** PASS

**Findings (Critical):**
- None.

**Findings (Important):**
- None.

**Findings (Minor):**
- `D1` [Security/privacy] [Permission Preflight]: Initial draft allowed a flow author to omit `requires:["shell"]` while still using `action` nodes, which would bypass explicit shell consent. Recommendation: make `shell` an implicit runtime requirement for every action node and check it before any node starts. _Resolution: resolved in design commit `893142e`; implicit `shell` and `outside-cwd` requirements were added._
- `D2` [Security/privacy] [Action Runner]: Environment inheritance can expose local secrets to action commands, even without runtime secret expansion. Recommendation: document that action commands inherit process env plus explicit overrides and that output/env handling remains sensitive local metadata. _Resolution: resolved in design commit `893142e`; environment behavior is explicit._
- `D3` [YAGNI] [Action Runner]: `node.input` could become a mini templating/selection language if overloaded in this slice. Recommendation: keep it static JSON for this PR and defer template/select expansion. _Resolution: resolved in design commit `893142e`; static input is explicit._

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Design follows workspace Windows, minimal duplication, and repo policy-matrix boundaries. |
| Assumptions under attack | Finding | A2 was weak until shell permission became implicit for action nodes. |
| Repo-precedent conflicts | Clean | Design extends existing `internal/acpclient` flow structs/runner/store and `cmd/ratchet` parser/smoke tests. |
| Artifact-class precedent | Clean | Flow feature shape matches existing archive/compare/flow tests and docs guard pattern. |
| YAGNI violations | Finding | Dynamic action input expansion was deferred to keep the slice bounded. |
| Missing failure modes | Clean | Non-zero action exit, failed-state persistence, cwd escape, malformed input, and output bounds are covered. |
| Security / privacy at architecture level | Finding | Shell consent and environment inheritance needed explicit treatment; resolved in design. |
| Infrastructure impact | Clean | No cloud, daemon protocol, migration, registry, release, or network impact. |
| Multi-component validation | Clean | Binary smoke through built CLI + fixture ACP agent + action flow proves the runtime boundary. |
| Declared integration proof | Clean | Runtime-integrated surfaces are existing ACP client flow runner and CLI; TypeScript runtime and Hermes profile/self-evolution are deferred. |
| Contributed UI rendering proof | Clean | No UI contribution. |
| Rollback story | Clean | Revert-only rollback is sufficient; added fields are optional. |
| Simpler alternative not considered | Clean | Docs-only and TypeScript-runtime alternatives are considered and rejected/deferred. |
| User-intent drift | Clean | User asked to continue archive/compare/flow work; design recognizes archive/compare/basic flow are already shipped and advances flow orchestration. |
| Existence / runtime-validity | Clean | Existing flow files and command/tests were inspected; emitted JSON remains consumed by `ratchet acp client flow run`. |

**Options the author may not have considered:**

1. Permission per command path: more expressive than `--allow shell`, but it becomes a policy matcher and overlaps with deferred sandbox/path/network expansion.
2. Node-free action prelude: a separate `--prepare-command` flag could prepare workspaces before flow execution, but it would not persist as a first-class step in run bundles.
3. Direct ACPX `.flow.ts` execution through `node`: closer to ACPX, but too much toolchain/security/release surface for this slice.

**Verdict reasoning:** PASS. The main security flaw was fixed before review report commit: shell and outside-cwd grants are now implicit runtime requirements, not author-declared optional metadata. Remaining trade-offs are intentionally scoped: static action stdin, process environment inheritance, and no TypeScript runtime compatibility.
