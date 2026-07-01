# Competitor Parity

This snapshot was refreshed on 2026-07-01 for the follow-up release. Repository
sources are pinned to the reviewed commit where available; hosted documentation
sources are recorded as official docs checked on the review date. Because some
upstream repositories changed after the 2026-06-30 planning window, this
document records the exact revisions used for reproducibility.

## Source Snapshot

| System | Source | Checked revision or doc |
|---|---|---|
| Zed | https://github.com/zed-industries/zed | `40d20036af34343a09f0ce6a2eb38c9e5a60e9ae` |
| ACP | https://github.com/agentclientprotocol/agent-client-protocol | `703f42901c9ccd48d775c61aa8053e944be0b4b2` |
| Pi | https://github.com/earendil-works/pi | `dd87c02cbf2681c9301cf809146651483ff16030` |
| Codex | https://github.com/openai/codex | `db887d03e1f907467e33271572dffb73bceecd6b` |
| Claude Code | https://code.claude.com/docs | Official docs: subagents, hooks, settings, Agent SDK |
| OpenClaw | https://github.com/openclaw/openclaw | `6495358f179911e7297ee092b342f633b5856960` |
| ACPX | https://github.com/openclaw/acpx | README on 2026-07-01 |

## Matrix

| Capability area | Current source signal | ratchet-cli status |
|---|---|---|
| ACP editor/agent boundary | Zed External Agents run separate agent processes over ACP; ACP publishes versioned schema artifacts and negotiated protocol versions. | Supported for initialize, new/load session, prompt, cancel, model, and in-process mode through `ratchet acp`; real stdio-style smoke is `TestACPStdioPromptSmoke`. Session list/resume/close/delete remain deferred because `acp-go-sdk v0.6.3` does not expose those agent methods yet. |
| ACP operational clients | ACPX is a headless ACP client with persistent/named sessions, prompt queueing, cancel, status, history, import/export, structured output, and compare/flow commands. | Partially supported. ratchet-cli can act as an ACP agent, but it is not yet an ACPX-equivalent ACP client/orchestrator. Follow-up: typed session queue/status/history/export surfaces if ratchet-cli becomes a headless client for other agents. |
| MCP and tool surfaces | Zed may forward configured MCP servers to External Agents; Codex and Claude Code both document MCP as a configurable tool extension surface. | Supported for stdio MCP blackboard plus daemon-backed session/project/blackboard/team tools through `ratchet mcp daemon`. `team_message` remains exposed but daemon-deferred because daemon `DirectMessage` is still unimplemented. |
| Permissions, sandbox, and trust | Zed tracks sandbox/tool permissions in agent threads; Codex publishes sandbox/approval configuration docs; Claude Code has hierarchical settings, permissions, sandbox settings, and managed policy precedence. | Partial. ratchet-cli has trust policy concepts and permission prompts, but it does not yet match Codex/Claude policy layering or Zed-style per-tool sandbox escalation UX. Follow-up: document and test a policy matrix before adding new controls. |
| Hooks and extension points | Claude Code hooks and settings reload across scopes; Pi extensions can intercept lifecycle/tool events and register tools/commands/UI; Codex has lifecycle hook config docs. | Partial. ratchet-cli has workflow/agent extensibility and retro recording, but not a broad runtime extension SDK. Follow-up: define optional extension hooks around session lifecycle, tool execution, and retro reporting without allowing unreviewed local mutation by default. |
| Session history and compaction | Pi stores sessions as JSONL trees, supports `/tree`, `/fork`, `/clone`, `/compact`, branch summaries, and extension-customized compaction. ACPX stores session metadata and turn history. | Partial. ratchet-cli now exposes daemon-backed `ratchet sessions history`, `ratchet sessions clone`, `ratchet sessions fork`, `ratchet sessions tree`, `ratchet sessions summary`, and `ratchet sessions compactions` for separate branch sessions, persisted branch summaries, compaction records, and linked pre-compaction archive sessions. Full Pi-style in-place tree navigation remains deferred. |
| Multi-agent orchestration | Claude Code subagents have scoped tools, permissions, MCP, hooks, memory, optional worktree isolation, and background behavior; OpenClaw routes channels/accounts/peers to isolated agents and workspaces. | Partial. ratchet-cli has daemon team management and MCP team list/status. Worktree isolation, per-agent permission scopes, and channel routing remain future work. |
| Local-first gateway/channels | OpenClaw positions a local-first gateway for sessions, channels, tools, events, multi-channel inboxes, companion apps, and sandboxing for non-main sessions. | Deferred. Voice/mobile/canvas/channel gateway support is out of scope for this ratchet-cli release. Track as a separate product direction rather than expanding this release. |
| Windows distribution | OpenClaw documents Windows onboarding and a Windows Hub; Claude docs include Windows config path behavior. | Supported for release artifacts. ratchet-cli now cross-builds Windows amd64/arm64 binaries and GoReleaser emits Windows zip archives. No MSI, winget, or Windows service installer is claimed. |
| Self-improvement loop | Pi can share session data and customize compaction through extensions; Claude Code supports skills/subagents/hooks/memory; Codex supports repo instruction files and hooks. | Supported as opt-in reporting/evidence only. ratchet-cli records redacted evidence when `retro.enabled` is true and routes findings to local project, ratchet-cli upstream, workflow integration, or general process buckets. Automatic local mutation and upstream PR creation remain disabled by default. |

## Follow-ups

| Priority | Follow-up | Rationale |
|---|---|---|
| P1 | Complete daemon direct team messaging or remove the MCP tool until backed by daemon behavior. | The current MCP schema surfaces the command but correctly returns daemon errors; full parity requires real daemon support. |
| P1 | Add Pi-style in-place message tree navigation. | Branch summaries, separate session fork/clone, compaction records, and archive session links are implemented; interactive navigation across turns remains deferred. |
| P2 | Define a policy-layer matrix for permissions, sandboxing, and trust. | Codex, Claude Code, and Zed have clearer policy surfaces than ratchet-cli currently documents. |
| P2 | Design optional extension hooks around session/tool/retro lifecycle. | Pi and Claude Code show high leverage from hooks, but ratchet-cli must keep mutation opt-in and redacted. |
| P3 | Evaluate ACP client/orchestrator mode. | ACPX demonstrates demand for headless ACP clients that drive other agents; ratchet-cli currently focuses on being an agent/server. |
| P3 | Track local-first gateway/channel work separately. | OpenClaw parity is valuable but much broader than ratchet-cli harness follow-ups. |

## Verification Links

- Zed External Agents source: https://github.com/zed-industries/zed/blob/40d20036af34343a09f0ce6a2eb38c9e5a60e9ae/docs/src/ai/external-agents.md
- Zed ACP thread source: https://github.com/zed-industries/zed/blob/40d20036af34343a09f0ce6a2eb38c9e5a60e9ae/crates/acp_thread/src/acp_thread.rs
- ACP README: https://github.com/agentclientprotocol/agent-client-protocol/blob/703f42901c9ccd48d775c61aa8053e944be0b4b2/README.md
- Pi sessions: https://github.com/earendil-works/pi/blob/dd87c02cbf2681c9301cf809146651483ff16030/packages/coding-agent/docs/sessions.md
- Pi compaction: https://github.com/earendil-works/pi/blob/dd87c02cbf2681c9301cf809146651483ff16030/packages/coding-agent/docs/compaction.md
- Pi extensions: https://github.com/earendil-works/pi/blob/dd87c02cbf2681c9301cf809146651483ff16030/packages/coding-agent/docs/extensions.md
- Codex config docs: https://github.com/openai/codex/blob/db887d03e1f907467e33271572dffb73bceecd6b/docs/config.md
- Codex sandbox docs: https://github.com/openai/codex/blob/db887d03e1f907467e33271572dffb73bceecd6b/docs/sandbox.md
- Claude Code subagents: https://code.claude.com/docs/en/sub-agents
- Claude Code hooks: https://code.claude.com/docs/en/hooks
- Claude Code settings: https://code.claude.com/docs/en/settings
- Claude Code Agent SDK: https://code.claude.com/docs/en/agent-sdk/overview
- OpenClaw README: https://github.com/openclaw/openclaw/blob/6495358f179911e7297ee092b342f633b5856960/README.md
- ACPX README: https://github.com/openclaw/acpx/blob/main/README.md
