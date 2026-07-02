### Adversarial Review Report

**Phase:** design
**Artifact:** `docs/plans/2026-07-02-ratchet-cli-extension-hooks-profiles-design.md`
**Status:** PASS

**Findings (Critical):**
- None.

**Findings (Important):**
- None.

**Findings (Minor):**
- `D1` [security/privacy] [Hook Review And Trust]: User hooks remain active without review, unlike stricter Codex-style trust. Recommendation: docs must state this compatibility exception and a future config can add user-hook review. _Resolution: accepted; design scopes trust-first behavior to project/plugin hooks to avoid breaking existing user hooks._
- `D2` [UX/discoverability] [ACP Launch Profiles]: Reusing `--agent <name>` for built-ins and profiles may hide profile resolution rules. Recommendation: command help and errors must explicitly say built-ins win and profile names cannot shadow built-ins. _Resolution: plan must include CLI help/error tests._
- `D3` [YAGNI/future-scope] [Hook Review And Trust]: Reserved `managed` source could be mistaken as implemented enterprise policy. Recommendation: docs must label managed hooks out of scope and not expose a config key for it. _Resolution: accepted; design reserves only source vocabulary._

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Reuses existing `internal/hooks`, `internal/plugins`, `internal/acpclient`; no duplicate SDK/runtime. |
| Assumptions under attack | Finding | A1 user-hook compatibility is a weaker trust posture; recorded as D1. |
| Repo-precedent conflicts | Clean | Existing hooks/plugins/ACP client registry are extended rather than replaced. |
| Artifact-class precedent | Clean | Hook tests, plugin manifest tests, and ACP client command tests already define the artifact shapes this design extends. |
| YAGNI violations | Finding | `managed` source is reserved but not implemented; D3 requires docs not to imply support. |
| Missing failure modes | Clean | Covers plugin path escape, untrusted hooks/profiles, unsupported Windows hook commands, changed hashes, and no workdir project events. |
| Security/privacy architecture | Finding | User hooks active by default are a deliberate compatibility risk; D1. |
| Infrastructure impact | Clean | Local files only; no cloud/IAM/network/production deployment. |
| Multi-component validation | Clean | Defines plugin manifest -> loader -> trust/profile CLI -> ACP fixture proofs. |
| Declared integration proof | Clean | Plugin hooks and acpProfiles are declared integrations with runtime host proofs. |
| Contributed UI rendering proof | Clean | No UI contribution. |
| Rollback story | Clean | Revert + patch release; local stores inert under older binaries. |
| Simpler alternative not considered | Clean | Docs-only/simple repeat-command alternative considered and rejected. |
| User-intent drift | Clean | Targets the queued extension hooks/profile distribution follow-up; keeps gateway/ACPX TS/deamon scheduling out. |
| Existence/runtime-validity | Clean | Existing hook, plugin, ACP client, and docs surfaces exist in repo; design does not invent a consumed external artifact without a proof. |

**Options the author may not have considered:**

1. Trust all hooks by default and only add `ratchet hooks list`: simpler, but leaves plugin/project hooks as arbitrary unreviewed code and fails the policy matrix boundary.
2. Add only ACP launch profiles and skip hooks: smaller, but leaves the extension-hooks follow-up open and does not improve plugin hook trust.
3. Jump to a Pi-style TypeScript SDK: higher parity, but far larger trust/runtime surface; should wait until command hooks/profile distribution prove the policy model.

**Verdict reasoning:** PASS. The design has three minor issues to carry into the plan, but no critical/important blocker after the project-workdir hook loading gap was backported into the design. The scope is strict enough for autonomous execution: command hooks with trust, ACP launch profiles, plugin-distributed templates, docs, tests, Windows builds, release.
