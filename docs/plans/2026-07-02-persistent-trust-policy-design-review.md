### Adversarial Review Report

**Phase:** design
**Artifact:** `docs/plans/2026-07-02-persistent-trust-policy-design.md`
**Status:** PASS

**Findings (Critical):**
- None.

**Findings (Important):**
- None.

**Findings (Minor):**
- `D1` [User-intent drift] The design does not solve the full interactive permission prompt gap even though the prompt component has an "Always allow" option. Recommendation: keep this explicitly out of scope and add a follow-up after persistent grant editing ships. _Resolution: accepted; design out-of-scope section calls this out._
- `D2` [Failure modes] Revoking a missing grant could confuse scripts if treated as an error. Recommendation: make revoke idempotent and document it. _Resolution: design requires idempotent revoke._
- `D3` [Security] Listing grant patterns can reveal sensitive local paths or commands. Recommendation: document grant listings as sensitive local policy metadata. _Resolution: security review includes this._

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Design reuses workflow-plugin-agent policy storage and avoids new secret handling. |
| Assumptions under attack | Clean | Assumptions are explicit; the only fragile one is local daemon authorization and it matches existing trust RPCs. |
| Repo-precedent conflicts | Clean | Follows existing trust RPC, client wrapper, command, and TUI slash-command shapes. |
| Artifact-class precedent | Clean | Proto, client, daemon, CLI command, and TUI command files follow existing artifact locations. |
| YAGNI violations | Clean | Does not add new matcher semantics, config editing, or ACPX transport. |
| Missing failure modes | Clean | Store-unavailable and missing-revoke behavior are specified. |
| Security/privacy | Clean | No secrets or cloud credentials; grants are treated as sensitive local metadata. |
| Infrastructure impact | Clean | No new cloud or ratchet-owned migration. |
| Multi-component validation | Clean | Requires daemon reload persistence, CLI parsing, TUI parsing, and cross-builds. |
| Declared integration proof | Clean | The only integration is the existing workflow-plugin-agent PermissionStore, exercised through daemon RPCs. |
| Contributed UI rendering proof | Clean | No new UI route or plugin contribution. |
| Rollback story | Clean | Revert and patch release path is documented. |
| Simpler alternative | Clean | Config-YAML editing was considered and rejected. |
| User-intent drift | Minor | Full interactive prompt persistence remains out of scope for this slice. |
| Existence/runtime-validity | Clean | Existing proto/service/trust command surfaces and PermissionStore were confirmed in code. |

**Options the author may not have considered:**
1. Make runtime `/trust allow` persistent by default. This is simpler for users but contradicts v0.21.0 docs and makes temporary grants sticky.
2. Add `granted_by` as a caller-provided field. This may be useful later, but daemon-set `operator` is safer until authenticated principals exist.

**Verdict reasoning:** PASS. The design keeps persistence in the agent plugin, adds explicit durable controls without changing runtime rule semantics, and includes enough reload and cross-platform verification.
