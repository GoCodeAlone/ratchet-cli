# Competitor Parity

This snapshot was refreshed on 2026-07-02 and updated on 2026-07-04 for the
v0.25.0 release state after the permission-persistence, retro evidence,
Windows release, TUI binary verification, release artifact gates, ACP flow,
hook trust, ACP launch profile, raw archive, compare bundle, and flow replay
slices. Repository sources are pinned to the
reviewed commit where available; hosted documentation sources are recorded as
official docs checked on the review date. Because some upstream repositories
changed after the 2026-06-30 planning window, this document records the exact
revisions used for reproducibility.

## Source Snapshot

| System | Source | Checked revision or doc |
|---|---|---|
| Zed | https://github.com/zed-industries/zed | `bb48a42983f2a4bb9ac9d31c63abe02497088f67` |
| ACP | https://github.com/agentclientprotocol/agent-client-protocol | `a90d7e3a7a77bad4d9af35bbb08962daa0167453` |
| Pi | https://github.com/earendil-works/pi | `21cb3807e766fa97f752e08f96a40ee99b49a644` |
| Codex | https://github.com/openai/codex | `b35d4b6b9d80c800b9f5731e2eee8c86ef317d70` |
| Claude Code | https://code.claude.com/docs | Official docs checked 2026-07-02: subagents, hooks, settings, Agent SDK |
| Hermes | https://github.com/NousResearch/hermes-agent | `ab942330fc627e931577bc7c68ef0ec086e810e4` |
| Hermes meta-harness | https://github.com/howdymary/hermes-agent-metaharness | `a0179af552ab179e6967ab4a846a1bab2ca83206` |
| OpenClaw | https://github.com/openclaw/openclaw | `c20171ddfc1313aed920d230a54985fe213f19e7` |
| ACPX | https://github.com/openclaw/acpx | `1d882575e34e18621e59229f0e711723cef223ae` |

## Matrix

| Capability area | Current source signal | ratchet-cli status |
|---|---|---|
| ACP editor/agent boundary | Zed External Agents run separate agent processes over ACP; ACP publishes versioned schema artifacts and negotiated protocol versions. | Supported for initialize, new/load session, prompt, cancel, model, and in-process mode through `ratchet acp`; real stdio-style smoke is `TestACPStdioPromptSmoke`. Session list/resume/close/delete remain deferred because `acp-go-sdk v0.6.3` does not expose those agent methods yet. |
| ACP operational clients | ACPX is a headless ACP client with persistent/named sessions, prompt queueing, cancel, status, history, import/export, structured output, compare/flow commands, runtime-owned action steps, flow workspace isolation, and persisted run artifacts. | Supported foundation. ratchet-cli can drive external ACP agents with `ratchet acp client exec`, persist local ACP client session metadata, list/show/status sessions, append multi-prompt FIFO queue entries with `--no-wait`, inspect and explicitly watch/drain queued prompts, record cooperative cancel requests, export/import ratchet-cli archive v1 JSON with summary history or raw ACPX JSON-RPC event-log history, inspect/copy raw ACPX event logs with `sessions events`, run serial compare across multiple ACP agents with optional `compare --save` bundles, run JSON v1 ACP/compute/action flows with replay-grade bundles and `flow replay`, and reuse trusted ACP launch profiles from local or plugin-distributed templates. Built-ins win over profile names; untrusted profiles do not resolve through `--agent`. Action nodes require `--allow shell`; cwd escapes require `--allow outside-cwd`; action stdout/stderr, raw event logs, compare bundles, and replay bundles are sensitive local artifacts. Deferred ACPX parity: ACPX TypeScript flow runtime compatibility remains deferred, and credentialed third-party agent CI. |
| MCP and tool surfaces | Zed may forward configured MCP servers to External Agents; Codex and Claude Code both document MCP as a configurable tool extension surface. | Supported for stdio MCP blackboard plus daemon-backed session/project/blackboard/team tools through `ratchet mcp daemon`. `team_message` is daemon-backed for active teams and sends operator-originated messages to agents by team/agent id or name. |
| Permissions, sandbox, and trust | Zed tracks sandbox/tool permissions in agent threads; Codex publishes sandbox/approval configuration docs; Claude Code has hierarchical settings, permissions, sandbox settings, and managed policy precedence. | Partial. ratchet-cli has daemon-backed runtime `/mode` and `/trust` controls, permission prompts, persistent trust grants through `workflow-plugin-agent/policy.PermissionStore`, reviewable hook trust, and ACP launch profile trust. The Policy Matrix in [docs/policy-matrix.md](policy-matrix.md) documents supported layers and deferred gaps. Full Codex/Claude-style sandbox/path/network controls and Zed-style per-tool sandbox escalation UX remain deferred. |
| Hooks and extension points | Claude Code hooks and settings reload across scopes and documents managed-policy hook controls; Pi extensions can intercept lifecycle/tool events and register tools/commands/UI; Codex has lifecycle hook config docs. | Supported with review/trust for command hooks; broader SDK deferred. ratchet-cli now lists project/plugin hooks, skips them until exact descriptor hook trust is recorded with `ratchet hooks trust <hash>`, lets operators inspect with `ratchet hooks list --cwd .`, and preserves user hooks as trusted by default for compatibility. Managed hooks remain deferred, and TypeScript extension SDK remains deferred. Daemon background drain is also deferred until policy ownership and cancellation are designed. |
| Session history and compaction | Pi stores sessions as JSONL trees, supports `/tree`, `/fork`, `/clone`, `/compact`, branch summaries, and extension-customized compaction. ACPX stores session metadata and turn history. | Supported for daemon-backed branch navigation. ratchet-cli exposes `ratchet sessions history`, `clone`, `fork`, `tree`, `browse`, `summary`, and `compactions`; the TUI supports Pi-style in-place branch browsing through `ctrl+b` and `/tree`, selected-history reload, and stale-event protection. JSONL compatibility remains out of scope. |
| Multi-agent orchestration | Claude Code subagents have scoped tools, permissions, MCP, hooks, memory, optional worktree isolation, and background behavior; OpenClaw routes channels/accounts/peers to isolated agents and workspaces. | Partial. ratchet-cli has daemon team management and MCP team list/status/message. Worktree isolation, per-agent permission scopes, and channel routing remain future work. |
| Flexible harness composition | Hermes emphasizes a composable multi-agent harness pattern; the Hermes meta-harness layers additional orchestration around agent roles and workflows. | Partial. ratchet-cli has team orchestration, workflow integration, ACP/MCP surfaces, branch browser, reviewable hooks, and reusable ACP launch profiles, but it still lacks a runtime extension SDK for swapping harness behaviors without rebuilds. |
| Local-first gateway/channels | OpenClaw positions a local-first gateway for sessions, channels, tools, events, multi-channel inboxes, companion apps, and sandboxing for non-main sessions. | Deferred. Voice/mobile/canvas/channel gateway support is out of scope for this ratchet-cli release. Track as a separate product direction rather than expanding this release. |
| Windows distribution | OpenClaw documents Windows onboarding and a Windows Hub; Claude docs include Windows config path behavior. | Supported for release artifacts. ratchet-cli now cross-builds Windows amd64/arm64 binaries and GoReleaser emits Windows zip archives. GoReleaser snapshot release-check, draft release asset postcheck, tap preflight, and tap postcheck gates verify release and Homebrew cask outputs. Windows executable runtime remains deferred pending approved runner changes; no MSI, winget, or Windows service installer is claimed. |
| Self-improvement loop | Pi can share session data and customize compaction through extensions; Claude Code supports skills/subagents/hooks/memory; Codex supports repo instruction files and hooks. | Supported as opt-in reporting/evidence only. ratchet-cli records redacted evidence when `retro.enabled` is true and routes findings to local project, ratchet-cli upstream, workflow integration, or general process buckets. Automatic local mutation and upstream PR creation remain disabled by default. |

## Follow-ups

| Priority | Follow-up | Rationale |
|---|---|---|
| P1 | Add JSONL-compatible import/export for branch trees if Pi interoperability becomes a product requirement. | In-place navigation is supported through daemon sessions; JSONL storage compatibility remains explicitly out of scope for v0.24.0. |
| P1 | Define daemon background drain policy before unattended queue execution. | ACP client queue plus explicit foreground watch/drain are supported; daemon scheduling still needs owner/session scope, cancellation, prompt persistence warnings, and audit evidence. |
| P2 | Keep [docs/policy-matrix.md](policy-matrix.md) current as permissions, sandboxing, and trust evolve. | Codex, Claude Code, and Zed have clearer policy surfaces; ratchet-cli now documents the current supported, partial, and deferred layers. |
| P2 | Design managed hooks and a broader extension SDK only after the command-hook trust model has operational history. | Pi and Claude Code show high leverage from hooks, but ratchet-cli must keep mutation opt-in and redacted. Current command hooks are reviewable through hook trust; managed hooks remain deferred and TypeScript extension SDK remains deferred. |
| P3 | Extend ACP client/orchestrator mode only where product demand appears. | The ACP client foundation covers typed exec, status, history metadata, cooperative cancel, multi-prompt FIFO queue/watch/drain, ratchet-cli archive v1 export/import with raw ACPX event logs, `sessions events`, `compare --save` bundles, JSON v1 ACP/compute/action flow replay bundles, `flow replay`, and ACP launch profiles. ACPX TypeScript flow runtime compatibility remains deferred, and credentialed third-party agent CI remains deferred. |
| P3 | Track local-first gateway/channel work separately. | OpenClaw parity is valuable but much broader than ratchet-cli harness follow-ups. |

## Verification Links

- Zed External Agents source: https://github.com/zed-industries/zed/blob/bb48a42983f2a4bb9ac9d31c63abe02497088f67/docs/src/ai/external-agents.md
- Zed ACP thread source: https://github.com/zed-industries/zed/blob/bb48a42983f2a4bb9ac9d31c63abe02497088f67/crates/acp_thread/src/acp_thread.rs
- ACP README: https://github.com/agentclientprotocol/agent-client-protocol/blob/a90d7e3a7a77bad4d9af35bbb08962daa0167453/README.md
- Pi sessions: https://github.com/earendil-works/pi/blob/21cb3807e766fa97f752e08f96a40ee99b49a644/packages/coding-agent/docs/sessions.md
- Pi compaction: https://github.com/earendil-works/pi/blob/21cb3807e766fa97f752e08f96a40ee99b49a644/packages/coding-agent/docs/compaction.md
- Pi extensions: https://github.com/earendil-works/pi/blob/21cb3807e766fa97f752e08f96a40ee99b49a644/packages/coding-agent/docs/extensions.md
- Codex config docs: https://github.com/openai/codex/blob/b35d4b6b9d80c800b9f5731e2eee8c86ef317d70/docs/config.md
- Codex sandbox docs: https://github.com/openai/codex/blob/b35d4b6b9d80c800b9f5731e2eee8c86ef317d70/docs/sandbox.md
- Claude Code subagents: https://code.claude.com/docs/en/sub-agents
- Claude Code hooks: https://code.claude.com/docs/en/hooks
- Claude Code settings: https://code.claude.com/docs/en/settings
- Claude Code Agent SDK: https://code.claude.com/docs/en/agent-sdk/overview
- Hermes README: https://github.com/NousResearch/hermes-agent/blob/ab942330fc627e931577bc7c68ef0ec086e810e4/README.md
- Hermes meta-harness README: https://github.com/howdymary/hermes-agent-metaharness/blob/a0179af552ab179e6967ab4a846a1bab2ca83206/README.md
- OpenClaw README: https://github.com/openclaw/openclaw/blob/c20171ddfc1313aed920d230a54985fe213f19e7/README.md
- ACPX README: https://github.com/openclaw/acpx/blob/1d882575e34e18621e59229f0e711723cef223ae/README.md
