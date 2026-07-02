# Competitor Parity

This snapshot was refreshed on 2026-07-02 for the v0.19.0 release. Repository
sources are pinned to the reviewed commit where available; hosted documentation
sources are recorded as official docs checked on the review date. Because some
upstream repositories changed after the 2026-06-30 planning window, this
document records the exact revisions used for reproducibility.

## Source Snapshot

| System | Source | Checked revision or doc |
|---|---|---|
| Zed | https://github.com/zed-industries/zed | `995e56d2639eff49ebdb6e988d01f1593b14aff5` |
| ACP | https://github.com/agentclientprotocol/agent-client-protocol | `cb1db65db17e5db07dfb68865bcdd2ecedb1beee` |
| Pi | https://github.com/earendil-works/pi | `45c0fe78338f203f7645c4cdb1f7759ee5ec9012` |
| Codex | https://github.com/openai/codex | `129ea2aaf5fb426d8ba683ee53f290742f41dd31` |
| Claude Code | https://code.claude.com/docs | Official docs: subagents, hooks, settings, Agent SDK |
| Hermes | https://github.com/NousResearch/hermes-agent | `88d1d6206f399c134d1f4c0b7db27733aaa3c50c` |
| Hermes meta-harness | https://github.com/howdymary/hermes-agent-metaharness | `a0179af552ab179e6967ab4a846a1bab2ca83206` |
| OpenClaw | https://github.com/openclaw/openclaw | `2913d3aee90c79bc420129606b3912662359723d` |
| ACPX | https://github.com/openclaw/acpx | `1d882575e34e18621e59229f0e711723cef223ae` |

## Matrix

| Capability area | Current source signal | ratchet-cli status |
|---|---|---|
| ACP editor/agent boundary | Zed External Agents run separate agent processes over ACP; ACP publishes versioned schema artifacts and negotiated protocol versions. | Supported for initialize, new/load session, prompt, cancel, model, and in-process mode through `ratchet acp`; real stdio-style smoke is `TestACPStdioPromptSmoke`. Session list/resume/close/delete remain deferred because `acp-go-sdk v0.6.3` does not expose those agent methods yet. |
| ACP operational clients | ACPX is a headless ACP client with persistent/named sessions, prompt queueing, cancel, status, history, import/export, structured output, and compare/flow commands. | Supported foundation. ratchet-cli can drive external ACP agents with `ratchet acp client exec`, persist local ACP client session metadata, list/show/status sessions, append multi-prompt FIFO queue entries with `--no-wait`, inspect and explicitly drain queued prompts, record cooperative cancel requests, export/import ratchet-cli archive v1 JSON with ACPX-shaped metadata, and run serial compare across multiple ACP agents. Deferred ACPX parity: JSON flow commands, raw ACPX JSON-RPC event-log archives, ACPX TypeScript flow runtime compatibility, and credentialed third-party agent CI. |
| MCP and tool surfaces | Zed may forward configured MCP servers to External Agents; Codex and Claude Code both document MCP as a configurable tool extension surface. | Supported for stdio MCP blackboard plus daemon-backed session/project/blackboard/team tools through `ratchet mcp daemon`. `team_message` remains exposed but daemon-deferred because daemon `DirectMessage` is still unimplemented. |
| Permissions, sandbox, and trust | Zed tracks sandbox/tool permissions in agent threads; Codex publishes sandbox/approval configuration docs; Claude Code has hierarchical settings, permissions, sandbox settings, and managed policy precedence. | Partial. ratchet-cli has trust policy concepts and permission prompts, but it does not yet match Codex/Claude policy layering or Zed-style per-tool sandbox escalation UX. Follow-up: document and test a policy matrix before adding new controls. |
| Hooks and extension points | Claude Code hooks and settings reload across scopes; Pi extensions can intercept lifecycle/tool events and register tools/commands/UI; Codex has lifecycle hook config docs. | Partial. ratchet-cli has workflow/agent extensibility and retro recording, but not a broad runtime extension SDK. Follow-up: define optional extension hooks around session lifecycle, tool execution, and retro reporting without allowing unreviewed local mutation by default. |
| Session history and compaction | Pi stores sessions as JSONL trees, supports `/tree`, `/fork`, `/clone`, `/compact`, branch summaries, and extension-customized compaction. ACPX stores session metadata and turn history. | Supported for daemon-backed branch navigation. ratchet-cli exposes `ratchet sessions history`, `clone`, `fork`, `tree`, `browse`, `summary`, and `compactions`; the TUI supports Pi-style in-place branch browsing through `ctrl+b` and `/tree`, selected-history reload, and stale-event protection. JSONL compatibility remains out of scope. |
| Multi-agent orchestration | Claude Code subagents have scoped tools, permissions, MCP, hooks, memory, optional worktree isolation, and background behavior; OpenClaw routes channels/accounts/peers to isolated agents and workspaces. | Partial. ratchet-cli has daemon team management and MCP team list/status. Worktree isolation, per-agent permission scopes, and channel routing remain future work. |
| Flexible harness composition | Hermes emphasizes a composable multi-agent harness pattern; the Hermes meta-harness layers additional orchestration around agent roles and workflows. | Partial. ratchet-cli has team orchestration, workflow integration, ACP/MCP surfaces, and the new branch browser, but it still lacks a runtime extension SDK for swapping harness behaviors without rebuilds. |
| Local-first gateway/channels | OpenClaw positions a local-first gateway for sessions, channels, tools, events, multi-channel inboxes, companion apps, and sandboxing for non-main sessions. | Deferred. Voice/mobile/canvas/channel gateway support is out of scope for this ratchet-cli release. Track as a separate product direction rather than expanding this release. |
| Windows distribution | OpenClaw documents Windows onboarding and a Windows Hub; Claude docs include Windows config path behavior. | Supported for release artifacts. ratchet-cli now cross-builds Windows amd64/arm64 binaries and GoReleaser emits Windows zip archives. No MSI, winget, or Windows service installer is claimed. |
| Self-improvement loop | Pi can share session data and customize compaction through extensions; Claude Code supports skills/subagents/hooks/memory; Codex supports repo instruction files and hooks. | Supported as opt-in reporting/evidence only. ratchet-cli records redacted evidence when `retro.enabled` is true and routes findings to local project, ratchet-cli upstream, workflow integration, or general process buckets. Automatic local mutation and upstream PR creation remain disabled by default. |

## Follow-ups

| Priority | Follow-up | Rationale |
|---|---|---|
| P1 | Complete daemon direct team messaging or remove the MCP tool until backed by daemon behavior. | The current MCP schema surfaces the command but correctly returns daemon errors; full parity requires real daemon support. |
| P1 | Add JSONL-compatible import/export for branch trees if Pi interoperability becomes a product requirement. | In-place navigation is supported through daemon sessions; JSONL storage compatibility remains explicitly out of scope for v0.19.0. |
| P2 | Define a policy-layer matrix for permissions, sandboxing, and trust. | Codex, Claude Code, and Zed have clearer policy surfaces than ratchet-cli currently documents. |
| P2 | Design optional extension hooks around session/tool/retro lifecycle. | Pi and Claude Code show high leverage from hooks, but ratchet-cli must keep mutation opt-in and redacted. |
| P3 | Extend ACP client/orchestrator mode only where product demand appears. | The ACP client foundation covers typed exec, status, history metadata, cooperative cancel, multi-prompt FIFO queue/drain, ratchet-cli archive v1 export/import, and serial compare. ACPX flow and raw JSON-RPC event-log archive compatibility remain intentionally deferred. |
| P3 | Track local-first gateway/channel work separately. | OpenClaw parity is valuable but much broader than ratchet-cli harness follow-ups. |

## Verification Links

- Zed External Agents source: https://github.com/zed-industries/zed/blob/995e56d2639eff49ebdb6e988d01f1593b14aff5/docs/src/ai/external-agents.md
- Zed ACP thread source: https://github.com/zed-industries/zed/blob/995e56d2639eff49ebdb6e988d01f1593b14aff5/crates/acp_thread/src/acp_thread.rs
- ACP README: https://github.com/agentclientprotocol/agent-client-protocol/blob/cb1db65db17e5db07dfb68865bcdd2ecedb1beee/README.md
- Pi sessions: https://github.com/earendil-works/pi/blob/45c0fe78338f203f7645c4cdb1f7759ee5ec9012/packages/coding-agent/docs/sessions.md
- Pi compaction: https://github.com/earendil-works/pi/blob/45c0fe78338f203f7645c4cdb1f7759ee5ec9012/packages/coding-agent/docs/compaction.md
- Pi extensions: https://github.com/earendil-works/pi/blob/45c0fe78338f203f7645c4cdb1f7759ee5ec9012/packages/coding-agent/docs/extensions.md
- Codex config docs: https://github.com/openai/codex/blob/129ea2aaf5fb426d8ba683ee53f290742f41dd31/docs/config.md
- Codex sandbox docs: https://github.com/openai/codex/blob/129ea2aaf5fb426d8ba683ee53f290742f41dd31/docs/sandbox.md
- Claude Code subagents: https://code.claude.com/docs/en/sub-agents
- Claude Code hooks: https://code.claude.com/docs/en/hooks
- Claude Code settings: https://code.claude.com/docs/en/settings
- Claude Code Agent SDK: https://code.claude.com/docs/en/agent-sdk/overview
- Hermes README: https://github.com/NousResearch/hermes-agent/blob/88d1d6206f399c134d1f4c0b7db27733aaa3c50c/README.md
- Hermes meta-harness README: https://github.com/howdymary/hermes-agent-metaharness/blob/a0179af552ab179e6967ab4a846a1bab2ca83206/README.md
- OpenClaw README: https://github.com/openclaw/openclaw/blob/2913d3aee90c79bc420129606b3912662359723d/README.md
- ACPX README: https://github.com/openclaw/acpx/blob/1d882575e34e18621e59229f0e711723cef223ae/README.md
