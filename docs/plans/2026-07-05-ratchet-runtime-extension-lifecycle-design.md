# ratchet Runtime Extension Lifecycle Design

**Status:** Approved by user preauthorization, 2026-07-05
**Scope:** ratchet-cli extension lifecycle, marketplace management, plugin/skill/hook reload, workflow/routine primitives, and blackboard-to-Workflow handoff boundaries.

## Goal

Make ratchet-cli a credible host for plugin bundles like `autodev`: install and update plugins from marketplaces, reload runtime extensions without daemon restart, expose skills to the agent loop, fire lifecycle hooks at the same practical breadth as Claude/Codex-style hooks, and lay the primitives for scheduled routines and dynamic workflow orchestration.

## Current Baseline

- `internal/plugins` can load installed plugins from `~/.ratchet/plugins` and already supports skills, agents, commands, tools, hooks, MCP, and ACP profiles.
- `cmd/ratchet plugin` only lists/installs/removes direct GitHub or local plugins. It has no marketplace catalog, no update command, no enable/disable state, and no autoupdate policy.
- `cmd/ratchet skill` only sees global/project skills. Plugin skills are loaded into `EngineContext.PluginSkills` but are not shown by the CLI and are not injected into normal chat prompts.
- Hooks have a trust store and many ratchet-native event names, but only a subset is fired from the daemon. Missing runtime surfaces include prompt submit, pre/post tool use, permission request, stop/failure, pre/post compact, and session start/end.
- Plugin reload requires daemon restart. Plugin daemons are loaded but not retained on `EngineContext`, so shutdown cannot reliably stop them.
- Blackboard export already emits local notification-event JSON/JSONL. External delivery remains delegated to `workflow-plugin-messaging-core`, `workflow-plugin-slack`, `workflow-plugin-discord`, and `workflow-plugin-teams`.

## External Reference Snapshot

Checked 2026-07-05 from primary documentation:

- Claude Code hooks expose session, turn, tool, permission, compaction, agent, notification, config, worktree, file, and display lifecycle events. Hooks can gate tool use and permissions.
- Claude Code plugin marketplaces support catalog add/list/update/remove, plugin install/update/enable/disable/uninstall, `autoUpdate`, and reload without process restart.
- Claude Code dynamic workflows move orchestration into a JavaScript script that can fan out many subagents, track progress, pause/resume, and save reusable commands.
- Claude Code routines support schedule, API, and GitHub triggers, with CLI-created schedules and web-managed external triggers.
- Claude Code MCP integration supports dynamic list-changed updates and channel-style pushed messages.

## Design Principles

- Reuse existing ratchet primitives first: sessions, fleet/team managers, cron ticks, hook trust, plugin loader, ACP profiles, blackboard, and provider/tool registries.
- Keep external messaging in Workflow plugins. Ratchet should export or trigger local Workflow handoffs, not own Slack/Discord/Teams credentials.
- Treat marketplaces as metadata and update policy, not trust. Installing a plugin still requires user action; project/plugin hooks still require hash trust before execution.
- Make reload explicit and observable. Autoupdate may fetch catalogs and plugin archives, but loaded runtime changes should announce counts and changed components.
- Do not introduce hidden daemon background agent work in this slice. Scheduled routines and workflows must be visible, auditable, cancellable, and bounded.

## Marketplace And Plugin Lifecycle

Add a marketplace registry under ratchet state:

- `ratchet plugin marketplace add <source> [--name NAME] [--auto-update]`
- `ratchet plugin marketplace list [--json]`
- `ratchet plugin marketplace update [NAME|--all]`
- `ratchet plugin marketplace remove NAME`
- `ratchet plugin install <plugin[@marketplace]> [--version VERSION]`
- `ratchet plugin update [NAME|--all]`
- `ratchet plugin enable|disable NAME`
- `ratchet plugin reload`

Marketplace sources:

- GitHub shorthand `owner/repo`, git URLs, local directory or catalog path, and direct `marketplace.json` URL.
- Catalog location: `.ratchet-plugin/marketplace.json`, then `.claude-plugin/marketplace.json`, then direct file path.
- Catalog entries include `name`, `description`, `version`, `source`, optional `sha256`, optional `relevance`, and optional `autoUpdate`.

Autoupdate policy:

- Marketplace-level `auto_update` controls catalog refresh.
- Plugin-level `auto_update` controls installed plugin replacement.
- Official/internal marketplaces may opt in by default only when explicitly configured in ratchet settings. Third-party/local defaults remain off.
- Updates are staged then atomically swapped to avoid half-installed plugin dirs.

Reload:

- Daemon reload stops old plugin daemons, reloads enabled plugins, re-registers tools, refreshes plugin skills/agents/commands/hooks/MCP/profiles, and emits a reload summary.
- CLI `ratchet plugin reload` asks the daemon to reload if it is running; otherwise it validates that the next daemon start will load the current registry.

## Skill Runtime

Skill discovery becomes one merged view:

- plugin skills, then global user skills, then project skills;
- later scopes override same-name content;
- plugin skills are namespaced as `plugin-name:skill-name` while retaining legacy unqualified names only when no collision exists.

Prompt behavior:

- Every chat turn gets a compact skill index with name, source, path, and parsed description/frontmatter.
- Full skill content is injected only when explicitly requested (`$name`, `/name`, `plugin:name`) or when a conservative matcher finds a direct name/description match.
- Explicit `$autodev:using-autodev` must load that plugin skill before the first model response.

CLI:

- `ratchet skill list [--json] [--all]`
- `ratchet skill show <name>`
- plugin skills appear with source metadata.

## Hooks

Keep ratchet-native names for compatibility and add canonical aliases/events:

- session: `session-start`, `session-end`
- prompt/turn: `user-prompt-submit`, `stop`, `stop-failure`
- tools: `pre-tool-use`, `post-tool-use`, `post-tool-use-failure`
- permission: `permission-request`, `permission-denied`
- compaction: `pre-compact`, `post-compact`
- agents/workflows: `subagent-start`, `subagent-stop`, `workflow-start`, `workflow-stop`, `workflow-failure`
- notification/config/file: `notification`, `config-change`, `file-changed`

Existing names (`pre-session`, `post-session`, `on-tool-call`, `on-permission-request`, `on-error`, etc.) remain aliases or parallel events for compatibility.

Data contract:

- Command template data stays map-based for compatibility.
- Sensitive fields should prefer hashes/counts/paths over raw prompts or secrets. Raw prompt text is not passed to hooks by default.
- Future JSON stdin hooks can extend this without breaking YAML command hooks.

Runtime callsites for this slice:

- session create/resume/close;
- user prompt submit before provider call;
- provider/tool errors through `on-error` and stop-failure aliases;
- pre/post tool use around all `ToolRegistry.Execute` calls;
- permission request and denied decisions;
- manual and automatic pre/post compact;
- final stop after assistant turn completion.

## Dynamic Workflows

Ratchet should model dynamic workflows as script-owned orchestration over existing agents, not as a new privileged execution surface.

First primitives:

- `ratchet workflows list|show|run|stop|resume`
- workflows stored in `~/.ratchet/workflows` and `.ratchet/workflows`;
- workflow file contains metadata plus a declarative JSON/YAML graph initially, with a later JavaScript runtime only after sandboxing and permission boundaries are designed;
- runtime can spawn bounded ratchet team/fleet/session workers, collect outputs into run state, and emit progress events/hooks.

Initial caps:

- configurable concurrency, default 4;
- hard agent cap per run;
- no direct filesystem/shell access from workflow coordinator;
- all mutating work happens through normal agent/tool permission checks.

## Routines And Scheduling

Scheduled routines should use visible ratchet state and existing cron tick surfaces:

- `ratchet routines add --schedule CRON --prompt TEXT [--cwd DIR] [--provider NAME]`
- `ratchet routines list|show|run|pause|resume|remove`
- `on-cron-tick` fires before a due routine starts.
- API/GitHub triggers remain later; first slice persists definitions and allows explicit/manual execution plus daemon-visible due-run checks.

Routine runs create normal sessions with branch summaries and visible history. No hidden background drain.

## Blackboard Messaging Bridge

Next slice after `ratchet blackboard export`:

- define the ratchet notification-event schema in `workflow-plugin-messaging-core`;
- add parser/projection helpers for `messaging.text`;
- keep channel routing, credentials, rate limits, redaction, and delivery in Slack/Discord/Teams/Teams-compatible Workflow plugins;
- scenario proof must show no external post happens by default and that explicit routing redacts configured sensitive values.

## Security And Operations

| Risk | Mitigation |
|---|---|
| Marketplace update installs unreviewed code. | Update catalog separately from plugin install; plugin replacement is explicit unless plugin autoupdate is enabled; hooks still require hash trust. |
| Plugin reload leaves old daemons running. | Track `PluginDaemons` and stop them before replacing runtime capability state. |
| Skills flood prompt context. | Inject compact index by default; inject full content only for explicit or high-confidence matches. |
| Raw prompt leaks through hooks. | Hook data excludes raw prompt by default; include hashes/counts and session/workdir metadata. |
| Workflow/routine creates hidden autonomous work. | Runs are visible in session/workflow/routine lists, cancellable, bounded, and use normal permission gates. |
| Direct messaging secrets enter ratchet config. | Keep credentials in Workflow plugin secrets; ratchet only exports local event records or invokes explicit Workflow pipelines later. |

## Validation

- Unit tests for marketplace registry parse/update source resolution, enable/disable/autoupdate flags, and atomic install/update paths.
- Unit tests for skill merge, namespacing, and explicit `$plugin:skill` injection.
- Daemon tests for reload replacing skills/hooks/tools and stopping plugin daemons.
- Hook wiring tests for prompt submit, pre/post tool, permission, compact, stop, and error events.
- CLI tests for `plugin marketplace`, `plugin update`, `plugin reload`, and plugin skills in `skill list/show`.
- Runtime smoke for daemon reload and one chat turn with a plugin skill explicitly requested.
- Later scenario proof for ratchet notification events through Workflow messaging plugins.

## Out Of Scope For First Implementation Slice

- Full JavaScript workflow runtime.
- HTTP/prompt/agent-based hooks.
- Managed enterprise marketplace allowlists.
- LSP plugin support.
- Direct Slack/Discord/Teams adapters inside ratchet-cli.
- Hidden daemon background agent scheduling.

