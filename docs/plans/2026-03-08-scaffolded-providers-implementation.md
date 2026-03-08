# Scaffolded Provider Modes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the 6 scaffolded provider modes in `workflow-plugin-agent/provider`, replacing the `"not yet implemented"` stubs with working code.

**Architecture:** Each scaffolded provider already has its config struct, AuthModeInfo, and interface stubs. Implementation adds HTTP request construction, authentication, and response parsing specific to each cloud platform's API.

**Tech Stack:** Go 1.26, AWS SDK v2 (Bedrock), Google Cloud Auth (Vertex), Azure REST API (Foundry/Azure OpenAI), GitHub REST API (Models)

---

## Research Prerequisites

Before implementing each provider, read the relevant API documentation:

| Provider | API Reference | Auth Mechanism |
|---|---|---|
| GitHub Models | https://docs.github.com/en/rest/models/inference | PAT with `models:read` scope, `Authorization: Bearer <token>` |
| Anthropic Bedrock | https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_Converse.html | AWS SigV4 via `aws-sdk-go-v2` |
| Anthropic Vertex | https://cloud.google.com/vertex-ai/generative-ai/docs/partner-models/use-claude | GCP ADC → OAuth2 bearer token |
| Anthropic Foundry | https://learn.microsoft.com/en-us/azure/ai-services/model-catalog/how-to/deploy-models-serverless | Azure API key or Entra ID token |
| OpenAI Azure | https://learn.microsoft.com/en-us/azure/ai-services/openai/reference | `api-key` header or Entra ID bearer |
| OpenRouter | https://openrouter.ai/docs/api/reference/authentication | `Authorization: Bearer <key>` (OpenAI-compatible) |

---

## Task 1: GitHub Models Provider (`copilot_models.go`)

**Files:**
- Modify: `workflow-plugin-agent/provider/copilot_models.go`
- Create: `workflow-plugin-agent/provider/copilot_models_test.go`

**Implementation Notes:**
- Endpoint: `POST https://models.github.ai/inference/chat/completions`
- Auth: `Authorization: Bearer <PAT>` (fine-grained PAT with `models:read`)
- Request/response format: OpenAI-compatible chat completions
- Model IDs include vendor prefix: `openai/gpt-4o`, `anthropic/claude-sonnet-4`
- Can likely reuse OpenAI provider's request/response parsing with different base URL and auth header
- Streaming: SSE format identical to OpenAI

**Test Strategy:**
- Mock HTTP server returning OpenAI-compatible responses
- Test PAT auth header is set correctly
- Test model ID passthrough (vendor-prefixed)

---

## Task 2: OpenRouter Dedicated Type (`openrouter.go`)

**Files:**
- Modify: `workflow-plugin-agent/provider/openrouter.go`
- Create: `workflow-plugin-agent/provider/openrouter_test.go`

**Implementation Notes:**
- Currently `NewOpenRouterProvider` returns `*OpenAIProvider` — create a wrapper type `OpenRouterProvider` that embeds `*OpenAIProvider` and overrides `Name()` and `AuthModeInfo()`
- Adds proper `X-Title` and `HTTP-Referer` headers (OpenRouter-specific)
- Everything else delegates to OpenAI provider

**Test Strategy:**
- Verify Name() returns "openrouter"
- Verify AuthModeInfo() returns openrouter-specific info
- Verify OpenRouter-specific headers are set

---

## Task 3: Anthropic Bedrock Provider (`anthropic_bedrock.go`)

**Files:**
- Modify: `workflow-plugin-agent/provider/anthropic_bedrock.go`
- Create: `workflow-plugin-agent/provider/anthropic_bedrock_test.go`
- Modify: `workflow-plugin-agent/go.mod` (add `aws-sdk-go-v2` dependencies)

**Implementation Notes:**
- Endpoint: `POST https://bedrock-runtime.{region}.amazonaws.com/model/{model}/converse`
- Auth: AWS SigV4 via `aws-sdk-go-v2/config` credential chain
- Request format: Bedrock Converse API (different from Anthropic Messages API)
  - Messages use `role`/`content` but content is structured blocks
  - Tools use Bedrock's tool schema format
- Response format: Bedrock Converse response (map to provider.Response)
- Streaming: Bedrock uses event stream encoding, not SSE

**Dependencies:**
```
github.com/aws/aws-sdk-go-v2
github.com/aws/aws-sdk-go-v2/config
github.com/aws/aws-sdk-go-v2/credentials
github.com/aws/aws-sdk-go-v2/service/bedrockruntime
```

**Test Strategy:**
- Mock Bedrock Converse API endpoint
- Test credential chain configuration
- Test request/response format conversion
- Test region in URL construction

---

## Task 4: Anthropic Vertex AI Provider (`anthropic_vertex.go`)

**Files:**
- Modify: `workflow-plugin-agent/provider/anthropic_vertex.go`
- Create: `workflow-plugin-agent/provider/anthropic_vertex_test.go`

**Implementation Notes:**
- Endpoint: `POST https://{region}-aiplatform.googleapis.com/v1/projects/{project}/locations/{region}/publishers/anthropic/models/{model}:streamRawPredict`
- Auth: GCP Application Default Credentials → OAuth2 bearer token
- Request format: Anthropic Messages API (same as direct, wrapped in Vertex envelope)
- Response format: Anthropic Messages response
- Key difference: URL structure and auth mechanism; message format is identical to direct Anthropic

**Dependencies:**
```
golang.org/x/oauth2/google (for ADC)
```

**Test Strategy:**
- Mock Vertex AI endpoint
- Test URL construction with project/region
- Test ADC credential resolution
- Test that request body matches Anthropic Messages format

---

## Task 5: Anthropic Azure Foundry Provider (`anthropic_foundry.go`)

**Files:**
- Modify: `workflow-plugin-agent/provider/anthropic_foundry.go`
- Create: `workflow-plugin-agent/provider/anthropic_foundry_test.go`

**Implementation Notes:**
- Endpoint: `POST https://{resource}.services.ai.azure.com/anthropic/v1/messages`
- Auth: `api-key` header (Azure API key) OR `Authorization: Bearer <token>` (Entra ID)
- Request format: Anthropic Messages API
- Response format: Anthropic Messages response
- Very similar to direct Anthropic — mainly URL and auth differ

**Test Strategy:**
- Mock Azure AI Foundry endpoint
- Test both API key and Entra ID auth paths
- Test URL construction with resource name

---

## Task 6: Azure OpenAI Provider (`openai_azure.go`)

**Files:**
- Modify: `workflow-plugin-agent/provider/openai_azure.go`
- Create: `workflow-plugin-agent/provider/openai_azure_test.go`

**Implementation Notes:**
- Endpoint: `POST https://{resource}.openai.azure.com/openai/deployments/{deployment}/chat/completions?api-version={version}`
- Auth: `api-key` header (Azure key) OR `Authorization: Bearer <token>` (Entra ID)
- Request format: OpenAI Chat Completions (identical)
- Response format: OpenAI Chat Completions (identical)
- Key differences: URL structure (deployment-based), API version query param, auth header name

**Test Strategy:**
- Mock Azure OpenAI endpoint
- Test URL construction with resource/deployment/api-version
- Test both API key and Entra ID auth
- Test that request/response format matches OpenAI

---

## Task 7: Integration into provider_registry.go

**Files:**
- Modify: `ratchet/ratchetplugin/provider_registry.go`

**Implementation Notes:**
- Add factory functions for each new provider type
- Register in `NewProviderRegistry()`:
  - `"copilot_models"` → `copilotModelsProviderFactory`
  - `"anthropic_bedrock"` → `anthropicBedrockProviderFactory`
  - `"anthropic_vertex"` → `anthropicVertexProviderFactory`
  - `"anthropic_foundry"` → `anthropicFoundryProviderFactory`
  - `"openai_azure"` → `openaiAzureProviderFactory`
- Update `LLMProviderConfig` if new fields needed (region, project_id, resource, deployment)

---

## Task 8: Update ListModels() for new providers

**Files:**
- Modify: `workflow-plugin-agent/provider/models.go`

**Implementation Notes:**
- Add cases for each new provider type in `ListModels()` switch
- Implement model listing for providers that support it:
  - GitHub Models: `GET https://models.github.ai/inference/models`
  - Azure OpenAI: `GET https://{resource}.openai.azure.com/openai/models?api-version={version}`
  - Bedrock: Use `ListFoundationModels` API
  - Others: fallback model lists

---

## Task 9: Tag and release

Tag `workflow-plugin-agent` v0.3.0, update ratchet and ratchet-cli dependencies, tag both.

---

## Priority Order

Recommended implementation order (easiest → hardest):
1. **OpenRouter** (Task 2) — wrapper type only, minutes of work
2. **GitHub Models** (Task 1) — OpenAI-compatible, just different URL + auth
3. **Azure OpenAI** (Task 6) — OpenAI-compatible, URL/auth differences
4. **Anthropic Foundry** (Task 5) — Anthropic Messages format, different URL/auth
5. **Anthropic Vertex** (Task 4) — Anthropic Messages + GCP ADC
6. **Anthropic Bedrock** (Task 3) — Different API format (Converse), AWS SDK dependency
