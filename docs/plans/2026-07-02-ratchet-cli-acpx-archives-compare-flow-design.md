# ACPX Archives, Compare, And Flow Replay Design

**Status:** Draft
**Date:** 2026-07-02
**Project:** `GoCodeAlone/ratchet-cli`

## Goal

Close the next ratchet-cli harness gaps after v0.24.0: ACPX-compatible raw event-log archive import/export, compare run artifacts, replay-grade flow bundles, docs, Windows proof, and a versioned release.

## User Intent

Latest ask: "tackle the import/export archives, compare, flow orchestration as well." Prior constraints still apply: use current source as of 2026-07-01/2026-07-02, build for Windows, do not reimplement protocol/provider SDKs, keep secrets/prompt data local and redacted in shared evidence, and continue autonomously through green PRs/merge/release.

## Global Design Guidance

Source: workspace `AGENTS.md`, repo `README.md`, `docs/harness-emulation.md`, `docs/policy-matrix.md`, `docs/competitor-parity.md`. No repo-local `AGENTS.md`, `CLAUDE.md`, or `docs/design-guidance.md` exists.

| guidance | design response |
|---|---|
| Build for Windows. | Use portable Go file/path APIs; no shell-specific archive/compare/flow smoke commands; verify Windows amd64/arm64 builds. |
| Avoid duplicated SDKs. | Continue using `github.com/coder/acp-go-sdk`; do not parse stdio transport directly or embed ACPX's TypeScript runtime. |
| Sensitive local metadata stays local. | Event logs, archives, compare bundles, prompts, responses, stdout/stderr, and paths are treated as local-only sensitive artifacts; docs warn and shared evidence uses filenames/counts only. |
| Defer policy-heavy automation. | No daemon background drain, managed hooks, broad extension SDK, self-mutation, or credentialed third-party CI in this slice. |
| Current-source parity. | Design cites ACPX `1d882575e34e18621e59229f0e711723cef223ae` and ACP `a90d7e3a7a77bad4d9af35bbb08962daa0167453`, checked 2026-07-02. |

## Current State

| surface | shipped in v0.24.0 | gap |
|---|---|---|
| Session archive | `format_version:1` JSON with ACPX-shaped metadata and ratchet summary `history`. | ACPX archive `history` is raw ACP JSON-RPC messages; ratchet loses imported raw history. |
| ACP client run | `Result` exposes SDK notifications/text, not raw transport. | Need replayable event sidecars without replacing `acp-go-sdk`. |
| Compare | Serial rows, JSON/table output. | No persisted run bundle tying rows to prompt, timings, errors, and per-agent event logs. |
| Flow | JSON v1 `acp`/`compute`/`action`, run dir with `flow.json`, `input.json`, `state.json`, `steps/*.json`. | ACPX flow replay bundles define `manifest.json`, `trace.ndjson`, projections, artifacts, and session event links. |
| TypeScript ACPX flows | Deferred. | Still deferred; this design adds replay-grade JSON-v1 bundles, not `.flow.ts` execution. |

## Source Snapshot

| source | revision/doc | signal | ratchet response |
|---|---|---|---|
| ACPX | `openclaw/acpx@1d882575e34e18621e59229f0e711723cef223ae` | `export.ts` writes `history: AcpJsonRpcMessage[]`; `event-log.ts` stores `.stream.ndjson`; output docs define raw ACP JSON-RPC NDJSON. | Import/export ACPX archive v1 raw histories and preserve event logs as sidecars. |
| ACPX flow docs | same revision | Replay bundle requires `manifest.json`, `trace.ndjson`, projections, `sessions/*/events.ndjson`, artifact refs. | Extend ratchet flow store toward that layout for JSON v1 runs. |
| ACP protocol | `agentclientprotocol/agent-client-protocol@a90d7e3a7a77bad4d9af35bbb08962daa0167453` | Versioned JSON-RPC schema, session lifecycle additions in progress. | Stay on `acp-go-sdk v0.6.3`; do not claim lifecycle methods the SDK lacks. |
| Existing ratchet docs | `docs/competitor-parity.md`, `docs/policy-matrix.md` | Raw ACPX event logs and TypeScript runtime are deferred. | This slice removes raw event-log archive deferral; TypeScript runtime remains deferred. |

## Approaches

| option | summary | trade-off | decision |
|---|---|---|---|
| A. Sidecar/event-bundle compatibility | Preserve/import raw ACPX history; add ratchet event sidecars and replay-grade flow/compare bundles around existing SDK results. | Accurate about SDK limits, portable, reviewable. Some ratchet-generated events are normalized, not wire-tap exact. | Choose. |
| B. Raw stdio transport tap | Wrap child stdio to capture exact bytes before `acp-go-sdk`. | Closer to wire, but risks duplicating protocol framing/transport behavior and destabilizing SDK use. | Reject. |
| C. Embed ACPX TypeScript runtime | Execute `.flow.ts` with Node/ACPX packages. | Higher parity, but adds toolchain, extension/security policy, release complexity. | Defer. |

## Design

### Archive/Event Log Compatibility

- Add `internal/acpclient` event-log primitives:
  - ACP JSON-RPC validation: `jsonrpc:"2.0"` plus request/notification/response shape.
  - Sidecar path under the ACP client state directory: `events/<escaped-session-id>.ndjson`.
  - Event line schema for ratchet-owned logs: `{seq, at, direction, message}`.
- `ImportSession`:
  - Accept ACPX archive v1 where `history` entries are raw JSON-RPC messages.
  - Preserve raw history into sidecar NDJSON.
  - Continue accepting existing ratchet summary history archives.
  - Reject invalid JSON-RPC history with `ErrInvalidSessionArchive`; tolerate empty history.
- `ExportSession`:
  - Default remains current summary history for backward compatibility.
  - New `--history raw|summary|both` selects raw ACPX-compatible history, existing summary history, or both (`history` raw + `summary_history`).
  - If raw requested and sidecar is absent, export an empty raw history plus metadata warning in JSON/human output; do not synthesize false wire history.
- `sessions events <id>`:
  - Print event count and sidecar path by default; `--json` returns structured metadata.
  - `--output <path>` copies raw NDJSON for scripting/replay.

### Compare Run Artifacts

- Extend compare with optional persisted bundle:
  - flags: `--run-id <id>`, `--run-root <dir>`, `--save`.
  - default root: sibling of ACP client state, `compare/<run-id>`.
  - `compare.json`: prompt digest, agents, rows, started/finished timestamps, status.
  - `agents/<safe-agent>/events.ndjson`: per-agent event logs when the runner returns them.
- JSON/stdout rows remain compatible; `run_dir` is additive only when saved.
- Human output stays table-oriented and never prints raw event payloads.

### Flow Replay Bundles

- Keep JSON v1 flow runtime and existing files; add ACPX-replay-compatible files:
  - `manifest.json` with schema `acpx.flow-run-bundle.v1`.
  - `trace.ndjson` append-only events for run/node/action/acp outcomes.
  - `projections/run.json`, `projections/live.json`, `projections/steps.json`.
  - `artifacts/sha256-*.json|txt` for prompt text, action stdout/stderr, node outputs.
  - `sessions/<handle>/binding.json` and `sessions/<handle>/events.ndjson` for ACP node runs when event data is available.
- Add `ratchet acp client flow replay <run-dir> [--json]`:
  - validates manifest paths stay inside the run dir;
  - reads manifest/projections/trace counts;
  - emits a summary without contacting agents or executing actions.
- No TypeScript `.flow.ts` parser/executor; docs must keep that deferral explicit.

### Data Model Compatibility

| artifact | compatibility rule |
|---|---|
| `history` in archives | Raw JSON-RPC for ACPX mode; existing summary events supported on import/export summary mode. |
| `summary_history` | Ratchet-only additive field when `--history both`. |
| Event sidecar | NDJSON envelope with raw message field; readers ignore unknown fields. |
| Flow manifest | ACPX-compatible names/schema where feasible; ratchet-specific fields additive. |
| Compare bundle | Ratchet schema v1; not claimed as ACPX standard because ACPX compare JSON is summarized rows. |

## Security Review

| risk | control |
|---|---|
| Prompt/response leak through archives/events/bundles. | Local files only, `0600`, docs classify as sensitive, PR evidence uses counts/paths not payloads. |
| Invalid archive smuggles arbitrary files/paths. | No archive path extraction; import writes only sidecar under store dir; flow replay validates bundle-relative paths stay under run dir. |
| Raw JSON-RPC replay confused with live execution. | Replay commands are read-only; no agent process launch, no action execution. |
| Protocol reimplementation drift. | Use `acp-go-sdk` for live ACP; raw JSON-RPC handling is validation/preservation/export only. |
| Windows path/filename issues. | Escape session/agent/run ids as safe path segments; verify Windows builds. |

## Infrastructure Impact

No cloud/IAM/secrets/migrations/network exposure. Local disk layout expands under ratchet ACP client state:

- `events/*.ndjson`
- `compare/<run-id>/...`
- richer `flows/<run-id>/...`

No production deploy. Release is a Git tag + GoReleaser assets + Homebrew tap update after PRs merge and checks pass.

## Multi-Component Validation

| boundary | proof |
|---|---|
| Archive import/export ↔ store/event sidecars | Unit tests with ACPX-shaped raw archive fixture; binary smoke import/export through built CLI. |
| ACP client ↔ fixture ACP agent | Existing fixture agent plus new smoke verifies saved event sidecar metadata/counts. |
| Compare ↔ artifact bundle | Unit tests and binary smoke run two fixture agents, save bundle, read `compare.json` and event files. |
| Flow runtime ↔ bundle replay | Unit tests and binary smoke run action+ACP JSON flow, then `flow replay --json` reads manifest/projections without launching agents. |
| Docs ↔ policy/parity | `HarnessEmulationDocs` test asserts raw ACPX event logs no longer listed as deferred while TypeScript runtime remains deferred. |

## Rollback

Revert feature PRs and release tag if needed. Old archives/flow bundles remain readable because new fields/files are additive. If sidecar event files exist, rollback leaves inert local data; no migration/down command required.

## Assumptions

| id | assumption | challenge | fallback |
|---|---|---|---|
| A1 | Preserving ACPX raw `history` is enough to remove the raw archive deferral. | Ratchet-generated sessions may not have wire-exact history yet. | Export raw only when sidecar exists; state generated logs as normalized event logs, not wire captures. |
| A2 | Compare bundles should be ratchet schema, not ACPX schema. | Users may expect ACPX compare JSON exactly. | Keep row JSON compatible and document bundle as ratchet v1; add translator later if needed. |
| A3 | Replay-grade JSON v1 flow bundles are the right flow-orchestration next step. | User may prefer `.flow.ts` execution first. | Keep TypeScript runtime as explicit follow-up with separate security/toolchain design. |
| A4 | Sidecar NDJSON under state dir will not create unacceptable disk growth. | Long sessions can be large. | Add count/path metadata now; later add rotation/prune policy under separate design. |

## Self-Challenge

1. Laziest solution: docs-only update claiming existing archive/compare/flow support. Rejected because raw ACPX histories and replay bundles are real behavioral gaps.
2. Fragile assumption: A1. The design mitigates by preserving imported raw histories and refusing to synthesize raw wire logs when absent.
3. YAGNI sweep: no TypeScript runtime, replay viewer, daemon scheduler, SDK transport tap, managed hooks, credentialed agent CI.
4. First failure mode: malformed archive with path-like ids. Import writes only store-owned sidecars and uses safe escaped filenames.
5. Repo fit: follows existing `internal/acpclient` archive/compare/flow store pattern, existing `cmd/ratchet` parser/smoke tests, existing docs guard.

## Out Of Scope

- ACPX TypeScript `.flow.ts` execution.
- Exact stdio byte tap around `acp-go-sdk`.
- Daemon background drain or unattended execution.
- Managed hooks or broad TypeScript extension SDK.
- Credentialed third-party agent CI.
- Pi JSONL branch-tree interoperability.
- Local-first gateway/channels.
- Full sandbox/path/network enforcement.
