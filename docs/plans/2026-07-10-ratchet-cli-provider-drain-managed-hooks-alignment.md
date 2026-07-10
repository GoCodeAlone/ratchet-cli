### Alignment Report

**Status:** PASS

**Artifacts:**
- Design: `docs/plans/2026-07-10-ratchet-cli-provider-drain-managed-hooks-design.md`
- Plan: `docs/plans/2026-07-10-ratchet-cli-provider-drain-managed-hooks.md`
- Manifest check: PASS via `plan-scope-check.sh`

**Coverage:**

| Design Requirement | Plan Task(s) | Status |
|---|---|---|
| Upstream registry exposes sorted defensive runtime type names | Task 1 | Covered |
| Ratchet catalog covers all non-test runtime providers and canonical aliases | Task 2 | Covered |
| Catalog owns setup/auth/base URL/settings/model/guide metadata | Task 2 | Covered |
| CLI consumes catalog and preserves settings-aware discovery/manual fallback | Task 3 | Covered |
| TUI consumes catalog with no second provider list | Task 4 | Covered |
| TUI supports categorized scrolling/filtering, stable framing, back navigation | Task 4, Task 5 | Covered |
| ChatGPT, Copilot, Anthropic, CLI-native, Ollama, cloud, compatible paths | Task 3, Task 4 | Covered |
| Provider secrets remain transient and use existing daemon secrets/Redactor | Task 2, Task 3, Task 4, Task 5 | Covered |
| CLI/TUI/daemon/provider real-boundary and PTY/ConPTY proof | Task 3, Task 4, Task 5 | Covered |
| Background command uses `acp client` session identity and four daemon RPCs | Task 7, Task 8 | Covered |
| Background start requires explicit unattended acknowledgement | Task 6, Task 7, Task 8 | Covered |
| Only built-ins or descriptor-bound trusted profiles; no persisted argv | Task 6, Task 7 | Covered |
| Background policy stores metadata only with atomic owner-only persistence | Task 6 | Covered |
| Start/resume revalidate trust/hash; drift blocks launch | Task 6, Task 7, Task 8 | Covered |
| Terminal agent errors stop retries; stop persists before cancel | Task 6, Task 8 | Covered |
| Existing WatchQueue/DrainQueue claim, cancel, recovery remain authoritative | Task 6, Task 8 | Covered |
| Daemon owns contexts; smoke/test constructors cannot launch host state | Task 7 | Covered |
| Background policy changes append metadata-only JSONL audit | Task 6 | Covered |
| Managed policy uses fixed secure Linux/macOS/Windows administrator paths | Task 9 | Covered |
| Windows uses Known Folder/DACL and Unix uses no-follow/root/mode validation | Task 9 | Covered |
| Missing policy is normal; present insecure/malformed policy fails closed | Task 9, Task 11 | Covered |
| Additive and managed-only preserve diagnostics and immutable managed source | Task 9, Task 10 | Covered |
| Final policy applies after user/plugin and late project-hook composition | Task 11 | Covered |
| Managed execution durably audits start before launch and terminal result | Task 10, Task 11 | Covered |
| Managed audit/log paths exclude command, env, data, output, errors, secrets | Task 10, Task 11 | Covered |
| Hooks policy/audit/list expose mode, source, suppression, audit metadata | Task 10 | Covered |
| Go-native, existing SDKs/secrets, no remote service/new redactor/TS | Task 1-Task 11 | Covered |
| Native/cross Windows, full tests/lint, runtime launch, PR monitoring/release | Task 1, Task 5, Task 8, Task 9, Task 11 | Covered |
| Per-feature docs transition and independently coordinated rollback | Task 5, Task 8, Task 9, Task 11 | Covered |
| VS Code-style optimization loop remains the next deferred cluster | Scope Manifest, Final Closeout | Covered |

**Scope Check:**

| Plan Task | Design Requirement | Status |
|---|---|---|
| Task 1 | Upstream runtime type introspection prerequisite | Justified |
| Task 2 | Shared complete provider setup catalog | Justified |
| Task 3 | Catalog-driven CLI and settings-aware setup | Justified |
| Task 4 | Catalog-driven usable TUI wizard | Justified |
| Task 5 | Real UI/daemon proof, docs, release | Justified |
| Task 6 | Trusted persisted manager and background audit | Justified |
| Task 7 | Daemon/proto/client lifecycle boundary | Justified |
| Task 8 | Explicit CLI, fixture runtime, docs, release | Justified |
| Task 9 | Secure managed policy loading/precedence and Windows CI | Justified |
| Task 10 | Metadata audit and operator inspection | Justified |
| Task 11 | Final all-source enforcement, runtime proof, docs, release | Justified |

**Manifest Trace:**

- PR count `4` equals four grouping rows.
- Task count `11` equals eleven task headings.
- Every task appears exactly once in PR grouping.
- PR 1 ships the upstream contract; PR 2 ships provider unification; PR 3 ships
  background drains; PR 4 ships managed hooks.
- Each PR is independently reviewable/releasable and serial dependencies are
  explicit.

**Drift Items:** None.
