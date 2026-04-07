# Ratchet Quality & E2E Test Infrastructure — Design

**Date:** 2026-04-06
**Repo:** ratchet-cli
**Goal:** Fix all known bugs, add DB migration for stale secrets, build E2E test harness exercising real gRPC paths, and ensure every user journey works end-to-end.

## Bug Fixes (Before Tests)

### 1. DB Migration for Stale Secrets
On daemon startup in `initDB`, add:
```sql
UPDATE llm_providers SET secret_name = '' WHERE secret_name != '' AND type IN ('ollama', 'llama_cpp')
```
Fixes existing installs where keyless providers were registered with a secret_name that was never written.

### 2. Fix AddProvider Missing `settings` Column
The INSERT statement omits `settings`, which breaks provider types that need it (openai_azure, anthropic_foundry, anthropic_vertex, anthropic_bedrock). Add `settings` to the INSERT with default `'{}'`.

### 3. Fix Compression Silent Provider Failure
`handleCompact` and auto-compression in `handleChat` silently ignore provider resolution errors (`prov, _ = ...`). If the provider can't be resolved, `prov` is nil. The `Compress` function handles nil providers via fallback summary, but the silent error swallow should be logged.

## E2E Test Harness

### `testharness_test.go` — Shared Infrastructure

```go
type Harness struct {
    t       *testing.T
    svc     *Service
    server  *grpc.Server
    client  *client.Client
    db      *sql.DB
    dataDir string
}

func NewHarness(t *testing.T) *Harness
```

`NewHarness`:
1. Creates temp dir for secrets + data
2. Opens in-memory SQLite, calls `initDB`
3. Runs the stale secret migration
4. Creates real `EngineContext` with real `ProviderRegistry`, `ToolRegistry`, file-based `SecretsProvider`
5. Creates real `Service` with all managers (SessionBroadcaster, FleetManager, TeamManager, etc.)
6. Starts gRPC server on random TCP port
7. Creates real gRPC `Client` connected to it
8. Returns harness with cleanup via `t.Cleanup`

Helper methods:
```go
func (h *Harness) AddProvider(alias, provType, model, apiKey, baseURL string) *pb.Provider
func (h *Harness) CreateSession() string
func (h *Harness) SendMessage(sessionID, content string) []*pb.ChatEvent
func (h *Harness) ListProviders() []*pb.Provider
```

### E2E Test Scenarios

Each test creates a fresh `Harness` and exercises a complete user journey through real gRPC.

| Test | What it validates |
|---|---|
| `TestE2E_AddOllamaProvider_NoKey` | Register ollama with no API key → no secret created → GetByAlias succeeds |
| `TestE2E_AddProvider_WithKey` | Register with API key → secret file written → GetByAlias resolves key |
| `TestE2E_StaleSecretMigration` | Insert stale secret_name row → re-init → verify migration cleared it |
| `TestE2E_ChatRoundtrip` | Add mock provider → create session → send message → receive response events |
| `TestE2E_SessionAttach` | Create session → send message → attach from second stream → verify events fan out |
| `TestE2E_UpdateProviderModel` | Add provider → UpdateProviderModel → verify DB and cache updated |
| `TestE2E_Shutdown` | Set shutdown func → call Shutdown RPC → verify func called |
| `TestE2E_ListAgents_Empty` | No teams/fleets running → ListAgents returns empty |
| `TestE2E_CronTickInjection` | Add provider → create session → create cron → verify message appears in DB |
| `TestE2E_ProviderCRUD` | Add → list → test → update model → remove → verify each step |

Tests that require a real LLM (fleet execution, team execution, review sub-session) use mock provider responses — but exercise the real gRPC + DB + secret resolution stack.

## Files Changed

| File | Change |
|---|---|
| `internal/daemon/engine.go` | Add stale secret migration in initDB |
| `internal/daemon/service.go` | Fix AddProvider settings column |
| `internal/daemon/chat.go` | Log compression provider errors instead of silent swallow |
| `internal/daemon/testharness_test.go` | NEW — E2E test harness |
| `internal/daemon/e2e_test.go` | NEW — all E2E test scenarios |
