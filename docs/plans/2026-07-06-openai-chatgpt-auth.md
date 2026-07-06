# OpenAI ChatGPT Auth Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Add direct ChatGPT subscription authentication for OpenAI/Codex models in ratchet.

**Architecture:** Implement reusable `openai_chatgpt` provider/auth in `workflow-plugin-agent`, release it, then update `ratchet-cli` to expose setup/import UX and use the released provider. Existing API-key `openai` and PTY `codex_cli` flows stay unchanged.

**Tech Stack:** Go 1.26, stdlib HTTP/JSON/JWT helpers, `workflow-plugin-agent/provider.Provider`, ratchet daemon provider registry, GitHub Actions/Goreleaser.

**Base branch:** master

---

## Scope Manifest

**PR Count:** 2
**Tasks:** 4
**Estimated Lines of Change:** ~900

**Out of scope:**
- OS keychain storage.
- Credential pools/workspace switcher.
- Live ChatGPT CI smoke.
- Zed/IDE integrations.

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|------|-------|-------|--------|
| 1 | workflow-plugin-agent: add OpenAI ChatGPT provider | Task 1, Task 2 | feat/openai-chatgpt-auth |
| 2 | ratchet-cli: expose OpenAI ChatGPT setup | Task 3, Task 4 | feat/openai-chatgpt-auth |

**Status:** Locked 2026-07-06T04:04:12Z

## Integration Matrix

| item | kind | status | proof |
|---|---|---|---|
| `workflow-plugin-agent` provider type `openai_chatgpt` | runtime-integrated | new | provider tests + registry factory tests |
| ratchet provider setup command | runtime-integrated | new | command tests + daemon AddProvider test |
| Codex `auth.json` import | config-only local file | new | temp-file parser/import test |
| ChatGPT live endpoint | deferred | external credential | mock endpoint only; no CI secret |

### Task 1: Provider Auth + Responses Client

**Files:**
- Create: `workflow-plugin-agent/genkit/openai_chatgpt_provider.go`
- Create: `workflow-plugin-agent/genkit/openai_chatgpt_provider_test.go`

**Steps:**
1. RED: tests for token JSON parse, JWT claim extraction, refresh-window behavior, request headers, non-stream response mapping, SSE stream mapping.
2. Run: `go test ./genkit -run 'OpenAIChatGPT|ChatGPT' -count=1` → FAIL on missing symbols.
3. GREEN: implement token bundle, device-compatible auth bundle parser, refresh, Responses HTTP calls, stream parser.
4. Run same command → PASS.
5. Regression proof: temporarily bypass header/token assertion → test FAIL; restore → PASS.
6. Rollback: provider file is additive; revert commit.

### Task 2: Provider Registry Wiring

**Files:**
- Modify: `workflow-plugin-agent/orchestrator/provider_registry.go`
- Modify: `workflow-plugin-agent/orchestrator/provider_registry_test.go`
- Modify if present/needed: `workflow-plugin-agent/provider/auth_modes.go`, `workflow-plugin-agent/provider/models.go`

**Steps:**
1. RED: registry test for `openai_chatgpt` factory resolving secret JSON and constructing provider; auth mode list includes ChatGPT subscription.
2. Run: `go test ./orchestrator ./provider -run 'ProviderRegistry|AuthMode|Model' -count=1` → FAIL.
3. GREEN: register factory and metadata/model listing fallback.
4. Run same command → PASS.
5. Run: `go test ./genkit ./orchestrator ./provider -count=1` → PASS.
6. Release PR after full `go test ./...` if feasible. Tag next patch after merge.
7. Rollback: revert provider release consumer bump in ratchet; additive provider can remain unused.

### Task 3: Ratchet Setup/Import UX

**Files:**
- Modify: `ratchet-cli/internal/provider/oauth.go`
- Modify: `ratchet-cli/internal/provider/auth.go`
- Modify: `ratchet-cli/internal/provider/*_test.go`
- Modify: `ratchet-cli/cmd/ratchet/cmd_provider.go`
- Modify: `ratchet-cli/cmd/ratchet/cmd_provider_test.go`

**Steps:**
1. RED: tests for `LoadCodexAuth`, device-code client against mock server, setup command choosing `openai_chatgpt`, no token printed.
2. Run: `go test ./internal/provider ./cmd/ratchet -run 'ChatGPT|OpenAIChatGPT|Provider' -count=1` → FAIL.
3. GREEN: add `provider setup openai-chatgpt` with `--model`, `--from-codex`, auth base override for tests; store token JSON via `AddProviderReq.ApiKey`.
4. Run same command → PASS.
5. Regression proof: remove `openai_chatgpt` type in setup → test FAIL; restore → PASS.
6. Rollback: remove provider alias via `ratchet provider remove openai-chatgpt`; revert commit.

### Task 4: Ratchet Dependency, Docs, Verification, PR/Release

**Files:**
- Modify: `ratchet-cli/go.mod`, `ratchet-cli/go.sum`
- Modify: `ratchet-cli/README.md`, `ratchet-cli/docs/competitor-parity.md` if relevant

**Steps:**
1. Update `workflow-plugin-agent` to released tag containing Task 1-2.
2. RED/GREEN docs tests if existing help/docs tests expect provider setup list.
3. Run: `go test ./cmd/ratchet ./internal/provider ./internal/daemon -count=1` → PASS.
4. Run: `go test ./...` → PASS or record constrained failure.
5. Open ratchet PR with regression proof; monitor CI to green; admin merge.
6. Verify merged HEAD equals intended commit, tag next patch, monitor release workflow, publish if checks pass.
7. Rollback: revert dependency/setup/docs commit and tag follow-up patch.
