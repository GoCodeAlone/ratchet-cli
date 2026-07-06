# Ratchet CLI Harness Onboarding Interop Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Add provider setup discovery, daemon session export bundles, and Zed ACP/MCP config writers.

**Architecture:** Extend existing command handlers and config helpers without adding new runtimes or provider SDKs. Keep all generated artifacts local and explicit, with sensitive session exports written using restrictive permissions.

**Tech Stack:** Go 1.26, existing ratchet-cli daemon client interfaces, existing MCP config helper package, standard-library JSON/file APIs.

**Base branch:** master

---

## Scope Manifest

**PR Count:** 1
**Tasks:** 6
**Estimated Lines of Change:** ~500

**Out of scope:**
- Zed ACP registry publication.
- Session import/share links.
- Credentialed external provider CI.
- New provider SDKs or background workers.

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|------|-------|-------|--------|
| 1 | feat: add harness onboarding interop utilities | Task 1, Task 2, Task 3, Task 4, Task 5, Task 6 | feat/harness-onboarding-interop |

**Status:** Locked 2026-07-06T08:08:45Z

## Tasks

### Task 1: Provider Setup Discovery

**Files:**
- Modify: `cmd/ratchet/cmd_provider.go`
- Modify: `cmd/ratchet/cmd_provider_test.go`
- Modify: `README.md`

**Steps:**
1. RED: add tests for `provider setup list`, `provider setup list --json`,
   `provider setup guide openai-chatgpt`, and unknown guide alias.
2. GREEN: add setup guide metadata and parser branches under existing
   `provider setup` handling.
3. Keep existing setup aliases unchanged.
4. Verify: `go test ./cmd/ratchet -run Provider -count=1`.
5. Rollback: revert provider command/test/docs changes.

### Task 2: Daemon Session Export

**Files:**
- Modify: `cmd/ratchet/cmd_sessions.go`
- Modify: `cmd/ratchet/cmd_sessions_test.go`
- Modify: `README.md`
- Modify: `docs/harness-emulation.md`
- Modify: `docs/policy-matrix.md`

**Steps:**
1. RED: add tests for `sessions export <id> --output file`, missing output
   usage, JSON summary output, and `0600` file mode.
2. GREEN: extend `sessionsClient` fake/real usage to fetch tree/history/
   compactions and write `ratchet.session-export.v1` JSON.
3. Ensure stdout success summaries do not print message content.
4. Verify: `go test ./cmd/ratchet -run Sessions -count=1`.
5. Rollback: revert session export command/test/docs changes; existing session
   list/history/tree remain.

### Task 3: Zed ACP Config Writer

**Files:**
- Create or modify: `internal/acp/config.go`
- Create or modify: `internal/acp/config_test.go`
- Modify: `cmd/ratchet/cmd_acp.go`
- Modify: `cmd/ratchet/cli_help_surface_test.go`
- Modify: `README.md`

**Steps:**
1. RED: add config writer tests for merge into `.zed/settings.json` preserving
   existing `agent_servers`.
2. RED: add command test for `ratchet acp config zed <path>`.
3. GREEN: implement Zed ACP settings structs/writer and command branch.
4. Verify: `go test ./internal/acp ./cmd/ratchet -run 'ACP|Help' -count=1`.
5. Rollback: revert ACP config writer and command; `ratchet acp` remains.

### Task 4: Zed MCP Config Writer

**Files:**
- Modify: `internal/mcp/config.go`
- Modify: `internal/mcp/config_test.go`
- Modify: `cmd/ratchet/cmd_mcp.go`
- Modify: `cmd/ratchet/cmd_mcp_test.go`
- Modify: `README.md`

**Steps:**
1. RED: add config tests for Zed `context_servers` merge preserving existing
   servers and writing command object path/args/env/settings.
2. RED: add command tests for `mcp config zed <path> daemon` and `blackboard`.
3. GREEN: implement `WriteZedMCPConfig`, command handling, and default path.
4. Verify: `go test ./internal/mcp ./cmd/ratchet -run 'MCP|Help' -count=1`.
5. Rollback: revert Zed MCP writer and command; existing MCP config formats remain.

### Task 5: Docs And Parity Matrix

**Files:**
- Modify: `README.md`
- Modify: `docs/harness-emulation.md`
- Modify: `docs/competitor-parity.md`
- Modify: `docs/policy-matrix.md`
- Modify: `cmd/ratchet/harness_docs_test.go`

**Steps:**
1. RED/GREEN: update docs guard tests for provider setup discovery, daemon
   session export sensitivity, and Zed ACP/MCP config commands.
2. Update competitor parity source snapshot to mention checked 2026-07-06 docs
   and the new ratchet-cli status.
3. Verify: `go test ./cmd/ratchet -run HarnessEmulationDocs -count=1`.
4. Rollback: revert docs/test changes.

### Task 6: Final Verification And PR

**Files:**
- Modify: implementation files above only.

**Steps:**
1. Run focused tests from Tasks 1-5.
2. Run `go test ./cmd/ratchet ./internal/acp ./internal/mcp -count=1`.
3. Run `go test ./... -count=1`.
4. Run `git diff --check`.
5. Build Windows command binary: `GOOS=windows GOARCH=amd64 go build -o /tmp/ratchet.exe ./cmd/ratchet`.
6. Create PR, add Copilot reviewer, monitor CI/reviews, admin merge when clean
   or when checks are delayed and local verification above is green.
