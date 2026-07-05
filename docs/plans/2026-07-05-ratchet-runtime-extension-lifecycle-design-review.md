# Runtime Extension Lifecycle Design Review

**Verdict:** PASS with constraints

## Findings

1. **Avoid full workflow runtime in first slice.** A JavaScript workflow engine needs sandboxing, permission propagation, resumability, and cost bounds. The first implementation must stop at persistent definitions/runs or a declarative graph stub.
2. **Do not inject every skill.** Autodev-style plugins can carry many large skills. Full prompt injection by default would create context bloat and cache churn. Explicit match plus compact index is the right first boundary.
3. **Marketplace trust is not hook trust.** Catalog update and plugin install cannot implicitly trust plugin hooks. Hash trust must remain separate.
4. **Raw prompts in hooks are a leak.** Hook payloads should use prompt hashes/lengths unless a later JSON hook design adds explicit opt-in.
5. **Dynamic reload needs daemon ownership.** CLI-only reload would not update long-running sessions. The daemon must own reload state and old daemon shutdown.

## Required Guardrails

- First PR must not implement hidden background scheduling.
- First PR must not add direct Slack/Discord/Teams credentials.
- Plugin reload must stop old plugin daemon processes.
- Plugin skill names should be namespaced to avoid collisions.
- Tests must prove hook callsites fire on actual chat/tool/permission/compact paths, not only direct `RunHooks` calls.

