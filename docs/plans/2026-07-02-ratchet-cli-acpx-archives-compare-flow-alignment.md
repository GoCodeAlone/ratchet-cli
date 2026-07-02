### Alignment Report

**Status:** PASS

**Coverage:**

| Design Requirement | Plan Task(s) | Status |
|---|---|---|
| ACPX-compatible raw JSON-RPC archive import/export. | Task 1, Task 2, Task 3 | Covered |
| Preserve imported ACPX raw `history` into sidecar NDJSON. | Task 1, Task 2, Task 3 | Covered |
| Keep ratchet summary archive default and support raw/summary/both modes. | Task 1, Task 2, Task 3 | Covered |
| Fail raw export when raw sidecar is unavailable. | Task 1, Task 2, Task 3 | Covered |
| Expose `sessions events` metadata/export without printing payloads in human output. | Task 3 | Covered |
| Use `acp-go-sdk`; avoid raw stdio transport reimplementation. | Scope Manifest, Task 2 | Covered |
| Persist event data from live ACP client runs for raw archive and downstream bundle use. | Task 2, Task 3 | Covered |
| Persist compare bundles with rows and per-agent events under `--save`. | Task 4, Task 5 | Covered |
| Preserve unsaved compare JSON compatibility. | Task 4, Task 5 | Covered |
| Add replay-grade JSON v1 flow bundles: manifest, trace, projections, artifacts, session events. | Task 6, Task 7, Task 8 | Covered |
| Add read-only `flow replay` command with path containment validation. | Task 6, Task 7, Task 8 | Covered |
| Keep ACPX TypeScript runtime deferred. | Scope Manifest, Task 9 | Covered |
| Treat archives/events/bundles as sensitive local metadata. | Task 3, Task 5, Task 7, Task 9 | Covered |
| Build/test for Windows and release versioned artifacts. | Task 8, Task 10 | Covered |
| Update README, harness, competitor parity, policy matrix, docs guard, retro, workspace state. | Task 9, Task 10 | Covered |

**Scope Check:**

| Plan Task | Design Requirement | Status |
|---|---|---|
| Task 1 | Raw archive/event-log tests for ACPX and client event capture. | Justified |
| Task 2 | Event-log primitives, client result events, archive raw/summary implementation. | Justified |
| Task 3 | CLI flags/subcommands, live sidecar persistence, binary smoke. | Justified |
| Task 4 | Compare bundle tests and JSON shape lock. | Justified |
| Task 5 | Compare bundle store/runtime/CLI implementation. | Justified |
| Task 6 | Flow replay bundle tests. | Justified |
| Task 7 | Flow replay bundle writer and replay CLI implementation. | Justified |
| Task 8 | Cross-surface focused/full verification and Windows builds. | Justified |
| Task 9 | Public docs and docs guard state. | Justified |
| Task 10 | Release, lock completion, retro, workspace state. | Justified |

**Manifest Check:**

`plan-scope-check.sh --plan docs/plans/2026-07-02-ratchet-cli-acpx-archives-compare-flow.md` -> PASS.

**Drift Items:** none.
