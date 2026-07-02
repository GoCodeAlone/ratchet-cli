# ratchet-cli Extension Hooks And ACP Launch Profiles Design

**Status:** Draft
**Date:** 2026-07-02

## Goal

Ship the next ratchet-cli harness slice after v0.23.0: reviewable lifecycle hooks plus reusable ACP launch profiles. This closes the immediate "extension hooks/profile distribution" gap without adding a broad TypeScript extension SDK, daemon-hidden background drain, raw ACPX replay, or local-first gateway/channel routing.

## Global Design Guidance

Source: workspace `AGENTS.md`; ratchet-cli repo docs and policy matrix.

| guidance | design response |
|---|---|
| Avoid duplicated plumbing; reuse local helper APIs. | Extend existing `internal/hooks`, `internal/plugins`, and `internal/acpclient.AgentSpec` instead of adding a second extension/profile runtime. |
| Respect repo build/test style; keep Go minimal. | Single Go repo, focused package tests, binary smoke, Windows cross-builds. |
| Policy matrix says hooks/extensions are partial/deferred and sensitive. | This slice adds explicit trust/review and local-only state; mutation-capable hooks remain opt-in and reviewable. |
| ACP watch/drain must stay explicit foreground execution. | Profiles provide reviewed launch specs for `exec`/`drain`/`watch`/`flow`; they do not create daemon scheduling. |

## Source Snapshot

Checked 2026-07-02 from primary sources:

| system | source signal | design consequence |
|---|---|---|
| Codex | Official hooks docs: non-managed hooks require review/trust; plugin-bundled hooks load from plugin roots; managed-only mode exists; Windows managed dir is distinct. | ratchet-cli hooks need hash-based review, plugin path containment, and Windows command fields. |
| Pi | `earendil-works/pi@21cb380...` docs: extensions can register tools/commands and intercept events; project-local extensions load only after trust; extensions run with full system permissions. | ratchet-cli should not jump to a TS SDK; command hooks must be trusted before project/plugin execution. |
| ACPX | `openclaw/acpx@1d88257...` README: persistent sessions, compare, flows, runtime-owned actions, workspace isolation, and persisted artifacts. | Profiles should be explicit ACP launch specs and feed existing ACP client commands; TypeScript runtime/replay remain deferred. |
| Zed | `zed-industries/zed@4aa8ad9...` docs: external agents own native auth/config; registry/custom agents have boundaries. | Plugin/profile distribution must not silently import provider credentials or Zed-like host config. |
| Hermes | `NousResearch/hermes-agent@a2d49de...` README: flexible tools, learning loop, gateway/backends, native Windows. | Keep Windows builds and avoid hardcoding one provider/backend. |
| OpenClaw | `openclaw/openclaw@a51b06f...` README: local-first gateway/channel routing and non-main sandboxing. | Gateway/channel routing is separate product scope; profile work is local CLI harness scope only. |

## Current Repo Baseline

- Hooks already exist: `internal/hooks` supports named events, YAML config, glob filters, command templates, and daemon wiring.
- Plugin loader already supports `capabilities.hooks` plus skills/agents/commands/tools/MCP.
- ACP client already has `AgentSpec{Name, Command, Args, EnvKeys}`, fingerprinting, built-in registry, queue/drain/watch/flow.
- Policy matrix marks broad extension hooks and daemon background drain as deferred pending trust/redaction boundaries.

## Approaches Considered

| option | trade-off | verdict |
|---|---|---|
| A. Full Pi-style TypeScript extension SDK | High leverage, but introduces runtime/dependency trust, tool registration API, hot reload, and Windows packaging now. | Reject for this slice; too broad. |
| B. Reviewable command hooks + ACP launch profile store | Uses existing Go code, adds trust before mutation, solves launch-profile follow-up, reviewable in 2-3 PRs. | Choose. |
| C. Docs-only policy update | Safe but does not advance functionality after v0.23.0. | Reject; user asked to execute next phases. |

## Design

### Hook Review And Trust

Add hook metadata and a local trust store:

- Hook identity: hash over event, command, command_windows, glob, source kind, source path/plugin, and optional timeout/status fields.
- Sources: `user`, `project`, `plugin`; future `managed` reserved.
- Default policy:
  - user hooks from `~/.ratchet/hooks.yaml`: active, listed as user-owned;
  - project hooks from `.ratchet/hooks.yaml`: skipped until trusted;
  - plugin hooks: skipped until trusted;
  - disabled hooks always skipped.
- CLI:
  - `ratchet hooks list [--json]` shows event, source, status, hash, command redacted/truncated;
  - `ratchet hooks trust <hash>` records the current hash;
  - `ratchet hooks disable <hash>` records an explicit local disable;
  - `ratchet hooks untrust <hash>` removes trust.
- Plugin hook paths must stay inside plugin root; manifest `hooks` entries using `..` or absolute paths fail load.
- Windows: add `command_windows`; on Windows, hooks without a Windows command are skipped with status `unsupported_platform`, not run through `sh`.

### ACP Launch Profiles

Add a local ACP launch profile store under ratchet state:

- Profile shape: name, command, args, env_keys, cwd default, source metadata, hash, trusted, created_at, updated_at.
- CLI:
  - `ratchet acp client profiles list [--json]`
  - `ratchet acp client profiles add <name> --command <cmd> [--arg x] [--env-key KEY] [--cwd DIR] [--trust]`
  - `ratchet acp client profiles trust <name>`
  - `ratchet acp client profiles remove <name>`
- Resolution:
  - existing `--agent <name>` first checks built-ins, then trusted local profiles;
  - untrusted profile execution fails with actionable text;
  - command override still wins for one-off explicit launches.
- Plugin-distributed profiles:
  - add manifest capability `acpProfiles` pointing to a JSON/YAML file or directory;
  - loader discovers templates but does not execute them;
  - `profiles install <plugin>/<profile> --as <name>` copies to the local profile store as untrusted unless `--trust` is given.
- Profiles are launch specs, not credentials. `env_keys` names allowed env vars but never stores values.

### Docs And Policy

Update README, harness docs, competitor parity, and policy matrix:

- hooks/extensions become "Supported with review/trust" for command hooks;
- broad SDK, tool registration, hot reload, and TypeScript extensions remain deferred;
- ACP launch profiles are supported for explicit foreground ACP client commands;
- daemon-hidden background drain remains deferred.

## Security Review

| risk | mitigation |
|---|---|
| Project/plugin hook executes arbitrary local command. | Project/plugin hooks skipped until exact hash is trusted; changed hooks require re-trust. |
| Plugin manifest escapes root to load arbitrary hook/profile file. | Clean/evaluate capability paths; reject absolute/outside-root paths. |
| Hook command injection via templates. | Preserve existing shell-escaping for data interpolation; add tests for quote/control characters. |
| Windows accidental POSIX shell fallback. | Use `command_windows`; skip unsupported hooks on Windows. |
| Profile stores secrets. | Store env key names only; no env values; redact/truncate profile command display. |
| Untrusted profile silently starts a long-running watch. | ACP commands refuse untrusted profiles and still require explicit foreground invocation. |

## Infrastructure Impact

No cloud resources, IAM, queues, network exposure, or production deploys. Local files only:

- hook trust store under ratchet state;
- ACP launch profile store under ratchet state;
- optional copied profile definitions from installed plugins.

Rollback: revert ratchet-cli PRs and publish a patch release. Local trust/profile JSON files become inert if older binaries do not read them.

## Multi-Component Validation

| boundary | proof |
|---|---|
| plugin manifest -> loader -> hook trust listing | fixture plugin with hook file; `ratchet hooks list --json` shows untrusted plugin hook; execution skipped until trust. |
| plugin manifest -> loader -> profile install -> ACP command | fixture plugin with `acpProfiles`; install profile; trust profile; `ratchet acp client exec --agent <profile>` against ACP fixture succeeds. |
| CLI -> profile store -> watch/drain | queued prompts drain using trusted profile without repeating command/args. |
| Windows | `GOOS=windows GOARCH=amd64/arm64 go build ./cmd/ratchet`; tests cover Windows command selection by injecting GOOS helper where feasible. |

## Assumptions

| id | assumption | challenge | fallback |
|---|---|---|---|
| A1 | Existing user hooks should remain active for compatibility. | Codex reviews even user hooks; compatibility may be less important than safety. | Add later config to require trust for user hooks; this slice gates project/plugin hooks first. |
| A2 | ACP launch profiles can reuse `--agent` resolution. | Name collision with built-ins can confuse users. | Built-ins win; profile add rejects built-in names unless a later explicit override policy is designed. |
| A3 | Local JSON/YAML stores are enough. | Teams may want managed policy distribution. | Reserve `managed` source; do not implement enterprise management in this slice. |

## Self-Challenge

1. Laziest solution: add docs for existing `hooks.yaml` and tell users to repeat `--command`. Rejected because it does not add review/trust or profile distribution.
2. Fragile assumption: user hooks active by default. This preserves compatibility but leaves user-level hook safety weaker than Codex; docs must state the distinction.
3. YAGNI risk: plugin-distributed profiles could be too much. Kept because profile distribution is in the requested next phase and solves the watch/drain launch-profile follow-up.

## Out Of Scope

- TypeScript extension runtime, hot reload, or custom LLM-callable tool SDK.
- Daemon background or scheduled drain.
- Raw ACPX JSON-RPC event-log import/export or replay UI.
- ACPX TypeScript flow runtime compatibility.
- Local-first gateway/channels.
- Enterprise managed hook distribution.
- Storing secrets or env values in profiles.

## Rollback

Revert the feature commits and release a patch. Local trust/profile stores are additive and can be ignored by older binaries. If a bad plugin hook/profile was trusted locally, `ratchet hooks disable <hash>` or `ratchet acp client profiles remove <name>` removes local execution authority before downgrade.
