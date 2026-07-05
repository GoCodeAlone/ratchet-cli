### Adversarial Review Report

**Phase:** design
**Artifact:** `docs/plans/2026-07-05-ratchet-workflow-profile-ci-design.md`
**Status:** PASS

**Findings (Critical):**
- None.

**Findings (Important):**
- None.

**Findings (Minor):**
- `D1` [YAGNI] [Workflow Messaging Export Envelope]: A full Workflow pipeline fragment would be more than the user asked for and could imply executable delivery. Recommendation: emit only step contract metadata and input text. _Resolution: design uses a local envelope with `required_config` and no pipeline execution claim._
- `D2` [Security/privacy] [ACP Profile Verification]: Verification could become a response-leaking CI command. Recommendation: JSON/human output must exclude prompt and response text. _Resolution: design requires text length only._
- `D3` [Declared integration proof] [workflow-plugin-messaging-core]: The design names a Workflow plugin but does not execute Workflow. Recommendation: mark it config-only and prove only contract-shaped output. _Resolution: design calls this config-only and leaves runtime delivery downstream._

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Reuses messaging-core and ACP profile machinery; no duplicated provider SDKs or secret stores. |
| Assumptions under attack | Clean | A1/A2/A3 are stated with fallbacks. |
| Repo-precedent conflicts | Clean | Follows existing blackboard export and ACP profile command patterns. |
| Artifact-class precedent | Clean | CLI commands/tests stay in sibling `cmd_*` files and fixture proof uses existing ACP fixture. |
| YAGNI violations | Finding | Avoided full Workflow pipeline generation and direct provider send. |
| Missing failure modes | Clean | Unknown flags, untrusted profiles, timeout, and empty sections are named. |
| Security / privacy at architecture level | Finding | Redacted output requirement resolves response/prompt leakage. |
| Infrastructure impact | Clean | No cloud resources, IAM, migrations, queues, or production deploy. |
| Multi-component validation | Clean | Daemon export shape and ACP fixture process proof are required. |
| Declared integration proof | Finding | Messaging-core is config-only; ACP fixture is runtime-integrated; third-party agents deferred. |
| Contributed UI rendering proof | Clean | No UI contribution. |
| Rollback story | Clean | Additive CLI behavior; revert PRs and release patch. |
| Simpler alternative not considered | Clean | Plain JSONL piping and direct provider send are considered and rejected. |
| User-intent drift | Clean | Directly addresses next ratchet-cli features from follow-up queue. |
| Existence / runtime-validity | Clean | Existing blackboard export, profiles, fixture agent, and messaging-core contract were checked before design. |

**Options the author may not have considered:**
1. Add a `ratchet workflow bridge` command: clearer naming for Workflow, but it would create a new top-level surface before export envelopes prove useful.
2. Make `profiles verify` initialize-only: cheaper for third-party agents, but weaker than prompt round-trip and less useful for CI compatibility.

**Verdict reasoning:** PASS. The design advances two tracked gaps while preserving local-only secret boundaries and avoiding duplicated messaging/provider infrastructure.
