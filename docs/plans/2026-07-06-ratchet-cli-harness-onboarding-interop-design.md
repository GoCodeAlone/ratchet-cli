# Ratchet CLI Harness Onboarding Interop Design

## Goal

Ship the next three ratchet-cli improvements that reduce user dead ends without
adding a new runtime:

1. Provider setup discovery: make `ratchet provider setup` self-describing and
   scriptable so `/model` and CLI users can see install/auth commands for known
   providers.
2. Daemon session export bundles: add local-only `ratchet sessions export` for
   portable handoff of daemon session metadata/history/tree without requiring
   ACP client state.
3. Zed config writers: add Zed-shaped ACP and MCP config output so users can
   connect ratchet as a custom ACP external agent and expose ratchet MCP tools
   without hand-editing JSON.

## Source Snapshot

- Zed Agent Panel docs checked 2026-07-06: Agent Panel exposes threads and a
  new-thread menu for agents, and some thread features depend on the external
  agent integration.
- Zed External Agents docs checked 2026-07-06: external agents run over ACP,
  own auth/model/tool config boundaries, support custom `agent_servers`, and may
  receive MCP forwarding.
- OpenCode site checked 2026-07-06: emphasizes multi-session, share links,
  existing subscriptions, broad provider support, and terminal/IDE/desktop
  reach.
- Hermes README / ACP internals checked 2026-07-06: emphasizes model switching,
  a terminal TUI, cross-channel continuity, closed learning loop, scheduled
  automation, terminal backends, and ACP session lifecycle/list/fork support.
- Existing ratchet-cli docs: `docs/competitor-parity.md`, `README.md`,
  `docs/harness-emulation.md`, `docs/policy-matrix.md`.

## Global Design Guidance

Source: repository README, `docs/competitor-parity.md`,
`docs/policy-matrix.md`, and workspace guidance.

| guidance | design response |
|---|---|
| Local-first harness; avoid credential leakage. | Provider guide prints command/env-key names only. Session export is explicit and warns/docs that bundles may contain conversation content. Zed config writers never store secrets. |
| Prefer existing plugin/framework APIs and avoid duplicated provider libraries. | Provider model data continues through `workflow-plugin-agent/provider`; no new AWS/provider SDKs or credential stores. |
| Keep external delivery/config boundaries explicit. | Zed ACP config writes only custom-agent launch JSON. Zed MCP config writes local `ratchet mcp <target>` command entries. Zed-native auth/model policy remains external. |
| Runtime claims need real command/boundary proof. | Plan includes command handler tests, config JSON tests, docs guard updates, and focused CLI invocations where possible. |

## Design

### Provider Setup Discovery

Add `ratchet provider setup list [--json]` and
`ratchet provider setup guide <provider> [--json]`. The list is static metadata
for currently supported setup aliases: `ollama`, `openai-chatgpt`,
`claude-code`, `copilot-cli`, `codex-cli`, `gemini-cli`, and `cursor-cli`.

Each guide row includes alias, provider type, install hint, auth hint,
setup command, model behavior, and credential boundary. This is intentionally a
guide, not an installer. Existing `provider setup <alias>` behavior remains.

### Daemon Session Export Bundles

Add `ratchet sessions export <id> --output <path> [--json]`. The exported JSON
is daemon-session focused and separate from ACP client archives:

```json
{
  "schema": "ratchet.session-export.v1",
  "exported_by": "ratchet-cli",
  "session": {},
  "tree": [],
  "messages": [],
  "compactions": []
}
```

The command reads through existing daemon RPCs only: session tree, history, and
compactions. It fails rather than inventing unavailable data. Output files are
written `0600` because messages and summaries are sensitive local conversation
data.

### Zed ACP/MCP Config Writers

Add:

- `ratchet acp config zed [path]`
- `ratchet mcp config zed [path] [blackboard|daemon]`

Default path is `.zed/settings.json` for project-local config. ACP config merges
a `ratchet` custom `agent_servers` entry:

```json
{
  "agent_servers": {
    "ratchet": {
      "type": "custom",
      "command": "ratchet",
      "args": ["acp"],
      "env": {}
    }
  }
}
```

MCP config merges a Zed `context_servers.ratchet` entry with Zed's custom
server fields: `command: "ratchet"`, `args: ["mcp", "<target>"]`, and `env`.
This follows Zed's native shape instead of reusing Claude/Copilot/generic MCP
shapes.

## Security Review

- Auth/secrets: no command writes secret values; provider guides mention env key
  names and native login commands only.
- Sensitive data: session export can include prompt/response content and local
  paths; write `0600`, document sensitivity, and never print payloads in success
  summaries.
- Abuse cases: malformed paths fail with ordinary file errors; config writers
  merge into existing JSON and reject invalid JSON rather than clobbering.
- Dependencies: no new runtime dependency is required.

## Infrastructure Impact

No cloud resources, network paths, migrations, services, or production deploys.
Release impact is normal ratchet-cli binary/archive publication after merge.

## Multi-Component Validation

- Provider guide: command tests assert human and JSON output.
- Session export: command tests use fake daemon client data and assert file mode,
  schema, message/tree/compaction inclusion, and no stdout payload leak.
- Zed config: config package tests assert merge behavior for both ACP and MCP
  Zed settings shapes; command tests assert correct target entries.
- Docs: README and harness parity docs update the user-facing matrix.

## Assumptions

- Zed custom ACP agent config remains `agent_servers.<id>` with `type:
  "custom"`, command, args, and env fields.
- Zed local MCP settings accept project `.zed/settings.json` with
  `context_servers.<id>.command`, `args`, and `env` for stdio custom servers.
- Daemon session export is useful without import in this slice; import/share
  links remain later work.
- Session export consumers can handle protobuf-shaped JSON field names if
  emitted by Go structs; no schema compatibility with ACPX is claimed.

## Rollback

Revert the feature commit. Existing provider setup, ACP client archives, and
Claude/Copilot/generic MCP config commands remain unchanged. Remove any
user-created `.zed/settings.json` entries manually if desired; ratchet-cli will
not edit them outside the explicit config command.

## Approaches Considered

1. Recommended: one small interop/onboarding PR with static guide metadata,
   daemon export, and Zed config writers. This is cohesive, low-risk, and fills
   three visible user gaps.
2. Provider-first only. Lower risk but leaves session handoff and Zed
   integration gaps untouched.
3. Full import/share/link system. Higher value later, but too broad for the
   next focused ratchet-cli slice and would require persistence/link policy.

## Self-Challenge

- The laziest solution is README-only. That would not help users in `/model`,
  scripts, or CI, and would not generate correct config JSON.
- The most fragile assumption is Zed settings shape. The design keeps this
  isolated in a writer and cites checked docs; if Zed changes, one writer/test
  updates.
- The likely YAGNI risk is daemon session export import. Import is explicitly
  out of scope; this slice exports for handoff/audit only.
