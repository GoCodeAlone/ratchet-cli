# ACPX Flow Hardening Design

**Status:** Draft
**Date:** 2026-07-02
**Project:** `GoCodeAlone/ratchet-cli`

## Goal

Harden `ratchet acp client flow run` toward current ACPX flow ergonomics without adding a TypeScript runtime: runtime-owned action steps, per-node working directories, and permission preflight for flows.

## User Intent

Continue the ratchet-cli harness backlog after archive import/export, compare, JSON v1 flows, policy matrix, and foreground auto-drain. The user asked to tackle import/export archives, compare, and flow orchestration; current `master` already ships those in PRs #48-#50 and v0.20.0. This slice advances the remaining flow orchestration gap.

## Global Design Guidance

Source: workspace `AGENTS.md`, repo `README.md`, `docs/harness-emulation.md`, `docs/policy-matrix.md`, `docs/competitor-parity.md`. No repo-local `AGENTS.md`, `CLAUDE.md`, or `docs/design-guidance.md` exists.

| Guidance | Design response |
|---|---|
| Build for Windows. | Use portable Go; action nodes execute direct command+args without shell syntax, and local verification includes Windows compile. |
| Avoid duplicated policy engines. | Permission preflight is flow-local allowlist validation, not a new trust engine; trust remains in `workflow-plugin-agent/policy`. |
| Treat prompts, archives, and run bundles as sensitive local metadata. | Persist action stdout/stderr only inside existing flow run bundles with `0600` JSON files; docs warn about command output sensitivity. |
| Keep daemon background drain and broad extension SDK separate. | No daemon scheduler, hook SDK, profile distribution, or local mutation loop is added. |
| Current-source competitor research, not memory. | Sources checked on 2026-07-02: ACPX README/CLI/examples, Hermes README/profile distributions/self-evolution docs, existing ratchet-cli parity docs. |

## Current State

- `internal/acpclient` already supports archive v1 import/export, serial compare, JSON v1 flows, flow run bundles, `acp` nodes, `compute` nodes, shared ACP sessions, prompt templating, and binary smoke.
- Current flow gaps vs ACPX source:
  - no `action`/shell-owned step;
  - no per-node `cwd`;
  - no permission preflight before a flow starts;
  - no flow-level permission declaration;
  - TypeScript-authored ACPX flow modules remain deferred.

## Source Snapshot

| Source | Signal | ratchet-cli implication |
|---|---|---|
| `openclaw/acpx` README, main, checked 2026-07-02 | ACPX advertises flow run, runtime-owned shell actions, flow workspace isolation, and compare/session primitives. | Add only the Go/JSON subset that improves current flow runs: action nodes and per-node cwd. |
| `openclaw/acpx docs/CLI.md`, main, checked 2026-07-02 | `flow run` executes user-authored modules, persists run artifacts, supports per-step cwd, default agent, permission requirements, and fail-fast grants. | Mirror permission preflight semantics in JSON v1 without claiming TypeScript compatibility. |
| `openclaw/acpx examples/flows/README.md`, main, checked 2026-07-02 | Examples include `decision()`, `decisionEdge()`, a PR-triage flow, replay viewer, and `shell.flow.ts`. | Branching/replay UI are future work; shell action + run bundle hardening are this slice. |
| `NousResearch/hermes-agent`, main, checked 2026-07-02 | Hermes has CLI, gateway, skills, cron, MCP, profile distributions, and setup/update flows. | Useful for later extension/profile work, not for this flow-runtime slice. |
| `NousResearch/hermes-agent-self-evolution`, main, checked 2026-07-02 | Self-evolution is a separate DSPy/GEPA project with no release. | Do not add self-mutating behavior here; keep retro loop opt-in and separate. |

## Approach

| Approach | Summary | Trade-off | Decision |
|---|---|---|---|
| A. JSON v1 hardening | Add `action` nodes, per-node `cwd`, `requires`, `--allow`, and bundle outputs. | Small, portable, immediately useful; not ACPX TypeScript compatible. | Choose. |
| B. Embed Node/TypeScript ACPX runtime | Run `.flow.ts` files directly. | Better ACPX parity, but adds Node toolchain, runtime security surface, and release complexity. | Defer. |
| C. Docs-only parity note | Document existing JSON v1 limitations. | Accurate, but does not advance flow orchestration. | Reject. |

## Design

### JSON v1 Additions

Extend `FlowDefinition` and `FlowNode`:

| Field | Owner | Meaning |
|---|---|---|
| `requires []string` | flow | Permission names the operator must grant before execution. |
| `cwd string` | node | Optional node working directory; relative paths resolve under CLI `--cwd`. |
| `command string` + `args []string` | `action` node | Runtime-owned command to execute, not an ACP agent launch. |
| `env map[string]string` | `action` node | Optional explicit environment additions; no implicit secret expansion. |
| `input any` | `action` node | Optional JSON payload written to stdin. |

Add `FlowNodeTypeAction = "action"`. Action nodes imply the built-in `shell`
permission even when the flow author omits `requires:["shell"]`.

### Permission Preflight

- CLI adds repeated `--allow <permission>`.
- `RunFlow` checks `def.Requires` plus implicit runtime requirements before starting any node.
- Any `action` node adds implicit `shell`.
- Any node whose resolved `cwd` leaves the flow base cwd adds implicit `outside-cwd`.
- Missing grant error: `flow requires permission <name>; pass --allow <name>`.
- Names are exact strings; no wildcard, no policy precedence, no persistent grants.
- This is not a trust engine; it is flow-local user intent confirmation for known risk classes such as `shell`.

### Action Runner

- New injectable `ActionRunner` interface for tests.
- Default runner uses `exec.CommandContext`.
- Environment defaults to the current process environment plus explicit `node.env`
  overrides. No config/secret expansion is performed by the flow runtime.
- Output JSON shape:

```json
{"exit_code":0,"stdout":"...","stderr":"","duration_ms":123,"cwd":"..."}
```

- Non-zero exit returns an error, marks step failed, and persists failed state.
- Stdout/stderr are truncated to a bounded rune count in in-memory result and persisted bundle to avoid unbounded files.
- Action stdin is JSON encoding of static `node.input`; template/select expansion is deliberately deferred.

### Per-Node CWD

- For `acp` and `action` nodes, `node.cwd` overrides flow `--cwd`.
- Relative `node.cwd` resolves under flow base cwd.
- Resolved cwd must stay under flow base cwd unless `--allow outside-cwd` is present.
- This is path containment for flow runtime mechanics, not a full sandbox.

### Docs

Update README, harness emulation, competitor parity, and policy matrix:
- JSON v1 flows now support `acp`, `compute`, and `action`;
- action output can contain sensitive local command data;
- ACPX TypeScript runtime compatibility remains deferred;
- extension/profile/self-evolution work remains separate.

## Security Review

| Risk | Control |
|---|---|
| Shell actions execute local commands. | Implicit `shell` requirement + `--allow shell` preflight; no action node runs before preflight passes even if the flow omits `requires`. |
| CWD escape via `../` or absolute paths. | Resolve/clean path; require `--allow outside-cwd` to run outside base cwd. |
| Secret leakage in stdout/stderr. | Treat run bundles as sensitive; persist `0600`; bounded output; no public logs. |
| Policy duplication. | No persistent permission store; no wildcard matcher; no trust precedence. |
| Windows command behavior. | Execute direct command+args with `exec.CommandContext`; no `cmd /C` or `sh -c` string shell mode is added in this slice. |

## Infrastructure Impact

No cloud resources, migrations, registry publish, release tag, or daemon protocol changes. Local flow bundles gain additional JSON fields. Windows impact is process execution/path handling only.

## Multi-Component Validation

- Unit: validation rejects malformed action nodes, missing grants, cwd escapes, unsafe IDs.
- Unit: action runner success/failure persists state and truncates output.
- CLI: parser accepts repeated `--allow`; JSON output includes action node output.
- Binary smoke: built `ratchet` runs a JSON flow containing action + ACP nodes through fixture agent and verifies persisted bundle files.
- Gates: focused tests, full `go test`, `go vet`, lint, Windows amd64/arm64 build, docs guard, machine-path scan.

## Rollback

Revert the feature PR. Existing flow bundles remain readable because added fields are optional; no persisted schema migration is required.

## Assumptions

| ID | Assumption | Challenge | Fallback |
|---|---|---|---|
| A1 | JSON v1 action nodes are the right next flow step. | User may prefer ACPX TypeScript compatibility first. | Keep TypeScript runtime deferred and document exact remaining gap. |
| A2 | `--allow shell` is enough for first action-node policy. | More granular command/path grants may be needed. | Add finer permission names later; do not add a policy matcher now. |
| A3 | CWD containment under flow base cwd is useful without full sandboxing. | It may create false confidence. | Docs label it path containment only; sandbox expansion remains separate. |
| A4 | Bounded stdout/stderr preserves enough debug value. | Long build logs may be truncated too aggressively. | Record truncation flag/limits and allow future configurable limits. |

## Self-Challenge

1. Laziest solution: docs-only "ACPX TypeScript deferred" note. Rejected because action nodes are a compact runtime improvement with clear tests.
2. Fragile assumption: `--allow shell` may be too coarse. Keep it explicit and non-persistent; defer granular trust matching to policy work.
3. YAGNI sweep: no TypeScript runtime, no replay viewer, no branching DSL, no profile distribution, no self-evolution.
4. First failure: action exits non-zero after prior steps. Persist failed state with completed prior steps and return error.
5. Repo pattern fit: add to existing `internal/acpclient` flow structs/runner/store, `cmd/ratchet` parser tests, binary smoke, and docs guard.

## Out of Scope

- ACPX TypeScript flow runtime compatibility.
- Raw ACPX JSON-RPC event-log archive compatibility.
- Branching/decision DSL and replay viewer.
- Daemon background drain.
- Broad extension hooks/profile distributions.
- Self-improving local mutation or upstream PR automation.
- Full sandbox/path/network enforcement.
- Release tag/Homebrew publish.
