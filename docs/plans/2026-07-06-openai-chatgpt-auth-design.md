# OpenAI ChatGPT Auth Design

**Goal:** ratchet users can use OpenAI/Codex models through a ChatGPT subscription account, not only an API key or external `codex` CLI process.

## Global Design Guidance

Source: workspace `docs/design-guidance.md`; project tracker: `docs/PROJECTS.md` `misc tools & libs`; portfolio: `docs/PORTFOLIO.md` `GoCodeAlone/ratchet-cli`.

| guidance | design response |
|---|---|
| Reuse over rebuild | Put reusable provider/auth logic in `workflow-plugin-agent`; `ratchet-cli` consumes released module. |
| Primary language Go; stdlib-first | Use Go HTTP/JSON/JWT helpers; no Rust bridge, no new daemon binary. |
| Secrets never logged | Store token bundle in ratchet secrets provider; redact secret JSON; errors omit token values. |
| Multi-component validation | Test provider package with mock ChatGPT endpoints; test ratchet setup/import path; build ratchet against released module. |
| Release hygiene | Merge provider PR first, tag release, update ratchet, then release ratchet. |

## Ecosystem Facts

| source | fact used |
|---|---|
| OpenAI Codex auth docs | Codex supports ChatGPT sign-in and API-key auth; ChatGPT sessions cache in `auth.json` or keyring; device-code login exists for headless cases. |
| Official Codex source | Device flow: `auth.openai.com/api/accounts/deviceauth/usercode` → poll token → `/oauth/token`; refresh: JSON `grant_type=refresh_token`; client id `app_EMoamEEZ73f0CkXaXp7hrann`; response endpoint `chatgpt.com/backend-api/codex/responses`. |
| Hermes docs | Hermes implements device-code auth and can import `~/.codex/auth.json`. |
| Zed blog/docs | Zed supports direct ChatGPT subscription sign-in plus external-agent paths; external Codex remains separate auth. |

## Approach Options

| option | trade-off | decision |
|---|---|---|
| Shell to `codex` CLI | fastest; already works via `codex_cli`; requires separate binary/process and weak direct ratchet UX | rejected for direct feature; retained as existing fallback |
| Ratchet-only provider | smaller immediate diff; duplicates provider layer and blocks other Workflow consumers | rejected |
| Reusable `workflow-plugin-agent` provider + ratchet setup UX | two PRs/releases; best reuse, registry compatibility, and future consumers | chosen |

## Design

- Add `openai_chatgpt` provider in `workflow-plugin-agent`.
- Auth material is a JSON token bundle stored as the provider secret value, not in config or DB.
- Provider accepts token bundle from either ratchet device-code setup or Codex `auth.json` import.
- Provider refreshes access token before expiry using the refresh token; it updates the in-memory token bundle. Ratchet setup writes the initial bundle; persistent refreshed-token write-back is deferred unless the provider registry exposes a safe secret-update hook.
- Provider calls the ChatGPT Codex Responses endpoint with Responses-style JSON; non-streaming and streaming map to existing `provider.Provider`.
- Ratchet CLI adds `ratchet provider setup openai-chatgpt [--model MODEL] [--from-codex PATH] [--no-browser]`, default alias `openai-chatgpt`, default model `gpt-5-codex`.
- Existing `codex-cli` setup remains available and documented as external-binary fallback.

## Security Review

| class | treatment |
|---|---|
| secret custody | tokens stored only in ratchet secrets provider; no token in provider rows, logs, docs, PR text |
| phishing | device-code prompt says never share code; opens official `auth.openai.com/codex/device` only |
| confused auth | new provider type `openai_chatgpt` distinct from API-key `openai`; no API-key fallback under same alias |
| refresh failure | terminal `invalid_grant`/reused/revoked errors surface as re-auth required; no retry loop flood |
| endpoint trust | default endpoint is `https://chatgpt.com/backend-api/codex/responses`; test override allowed only via base URL for local tests |

## Infrastructure Impact

- No cloud resources, migrations, or hosted services.
- Module release cascade: `workflow-plugin-agent` tag, then `ratchet-cli` dependency update and release tag.
- Rollback: revert ratchet dependency/setup commit and remove `openai_chatgpt` provider config; provider release remains additive.

## Multi-Component Validation

| boundary | proof |
|---|---|
| provider ↔ ChatGPT auth/token endpoints | mock HTTP server covers device-code, refresh, request headers, error paths |
| provider ↔ Responses endpoint | mock endpoint asserts URL, bearer header, account header, request body; returns text and SSE |
| ratchet CLI ↔ daemon/secrets/provider registry | focused tests for setup/import and AddProvider secret behavior |
| ratchet ↔ released provider module | `go test ./cmd/ratchet ./internal/provider ./internal/daemon` plus full `go test ./...` if time permits before PR |

## Assumptions

| id | assumption | challenge | fallback |
|---|---|---|---|
| A1 | ChatGPT Codex backend accepts standard Responses payload for simple chat. | endpoint may require Codex metadata for all requests | shelling to `codex_cli` remains fallback; add required headers if mock/live evidence shows gap |
| A2 | Device-code endpoints remain enabled for personal/workspace accounts that allow it. | admin may disable device-code login | support Codex `auth.json` import and clear re-auth error |
| A3 | `workflow-plugin-agent` can ship additive provider type without engine changes. | registry factory shape may hide secret persistence | keep persistence to initial secret; defer refresh write-back |

## Self-Challenge

1. Laziest solution: document `codex_cli`; insufficient because user asked ratchet to offer same direct account auth.
2. Fragile assumption: Responses endpoint compatibility. Mitigation: use official Codex source constants, mock request contract, keep CLI fallback.
3. YAGNI sweep: no token pools, workspace switching UI, keyring backend, managed enterprise access tokens, or cloud Codex features.

## Rollback

- Provider PR is additive; rollback by not selecting `openai_chatgpt`.
- Ratchet PR rollback: revert setup UX/dependency bump, release next patch.
- Local user rollback: `ratchet provider remove openai-chatgpt`; tokens remain in ratchet secrets dir until existing secret cleanup policy handles them.

## Non-Goals

- No direct Zed integration.
- No Codex cloud task API.
- No credential pooling.
- No OS keychain migration in this slice.
- No live ChatGPT smoke in CI.
