# Provider Consolidation Design

## Overview

Eliminate the duplicated `provider/` package by making `workflow-plugin-agent/provider` the single canonical source. Add auth mode awareness with scaffolded constructors for all known provider modes, each with documentation links and ToS warnings.

## Problem

`github.com/GoCodeAlone/ratchet/provider` and `github.com/GoCodeAlone/workflow-plugin-agent/provider` define identical types (`Provider`, `Message`, `ToolDef`, `ToolCall`, `Response`, `StreamEvent`, etc.) in separate packages. This caused a real bug: the Copilot token exchange fix was applied to the wrong repo. A 155-line `provider_adapter.go` exists solely to convert between the two identical type systems.

## Solution

### 1. Canonical Package: `workflow-plugin-agent/provider`

`workflow-plugin-agent` is the reusable library; `ratchet` is the application. The library owns shared interfaces.

### 2. Auth Mode Architecture

Each provider backend has multiple auth modes with genuinely different endpoints, headers, URL structures, and config fields. These are modeled as **separate constructors** returning the same `Provider` interface.

Every provider exposes `AuthModeInfo()`:

```go
type AuthModeInfo struct {
    Mode        string // e.g. "personal", "direct", "bedrock"
    DisplayName string // e.g. "GitHub Copilot (Personal/IDE)"
    Description string // What this mode does
    Warning     string // ToS/usage concerns (empty if none)
    DocsURL     string // Link to official documentation
    ServerSafe  bool   // Whether this mode is appropriate for server/service use
}
```

### 3. Provider Modes

#### GitHub Copilot

| Mode | Constructor | Auth | Base URL | Server Safe |
|---|---|---|---|---|
| Personal/IDE | `NewCopilotPersonalProvider` | OAuth token → exchange at `copilot_internal/v2/token` | `api.githubcopilot.com` | **No** |
| GitHub Models | `NewCopilotModelsProvider` | Fine-grained PAT (`models:read`) | `models.github.ai/inference` | **Yes** |

**Documentation:**
- Personal: https://docs.github.com/en/copilot/how-tos/copilot-cli/set-up-copilot-cli/authenticate-copilot-cli
- GitHub Models: https://docs.github.com/en/rest/models/inference
- GitHub Models billing: https://docs.github.com/billing/managing-billing-for-your-products/about-billing-for-github-models
- Copilot ToS: https://docs.github.com/en/site-policy/github-terms/github-terms-for-additional-products-and-features

**Notes:** Copilot SDK and Copilot Extensions are excluded — the SDK is a higher-level agent framework (not a raw chat completions provider), and Extensions are an inbound architecture (GitHub calls your endpoint).

#### Anthropic

| Mode | Constructor | Auth | Base URL | Server Safe |
|---|---|---|---|---|
| Direct API | `NewAnthropicProvider` | `x-api-key` header | `api.anthropic.com` | **Yes** |
| Amazon Bedrock | `NewAnthropicBedrockProvider` | AWS IAM SigV4 | `bedrock-runtime.{region}.amazonaws.com` | **Yes** |
| Google Vertex AI | `NewAnthropicVertexProvider` | GCP ADC / OAuth2 | `{region}-aiplatform.googleapis.com` | **Yes** |
| Azure Foundry | `NewAnthropicFoundryProvider` | Azure API key or Entra ID | `{resource}.services.ai.azure.com/anthropic` | **Yes** |

**Documentation:**
- Direct: https://platform.claude.com/docs/en/api/getting-started
- Bedrock: https://platform.claude.com/docs/en/build-with-claude/claude-on-amazon-bedrock
- Vertex: https://platform.claude.com/docs/en/build-with-claude/claude-on-vertex-ai
- Foundry: https://platform.claude.com/docs/en/build-with-claude/claude-in-microsoft-foundry

#### OpenAI

| Mode | Constructor | Auth | Base URL | Server Safe |
|---|---|---|---|---|
| Direct API | `NewOpenAIProvider` | `Authorization: Bearer sk-proj-...` | `api.openai.com/v1` | **Yes** |
| Azure OpenAI | `NewOpenAIAzureProvider` | `api-key` header or Entra ID | `{resource}.openai.azure.com/openai/deployments/{deploy}/...` | **Yes** |
| OpenRouter | `NewOpenRouterProvider` | `Authorization: Bearer ...` | `openrouter.ai/api/v1` | **Yes** |

**Documentation:**
- Direct: https://platform.openai.com/docs/api-reference/introduction
- Azure: https://learn.microsoft.com/en-us/azure/ai-services/openai/reference
- OpenRouter: https://openrouter.ai/docs/api/reference/authentication

#### Cohere

| Mode | Constructor | Auth | Base URL | Server Safe |
|---|---|---|---|---|
| Direct API | `NewCohereProvider` | `Authorization: Bearer ...` | `api.cohere.com` | **Yes** |

**Documentation:**
- Direct: https://docs.cohere.com/reference/chat

### 4. Scaffolding Approach

For the initial consolidation:
- **Fully implemented:** `NewCopilotPersonalProvider`, `NewAnthropicProvider`, `NewOpenAIProvider`, `NewOpenRouterProvider`, `NewCohereProvider` (these exist today)
- **Scaffolded:** All other constructors return `error` with a "not yet implemented" message, plus complete `AuthModeInfo` and config structs with documented fields

### 5. File Changes

#### workflow-plugin-agent (gains ownership)

| Action | File | Description |
|---|---|---|
| Keep | `provider/provider.go` | Core interfaces (add `AuthModeInfo` type + method to `Provider` interface) |
| Keep | `provider/anthropic.go` | Direct Anthropic (add `AuthModeInfo()`) |
| Keep | `provider/openai.go` | Direct OpenAI (add `AuthModeInfo()`) |
| Keep | `provider/copilot.go` | Rename to Personal mode (add `AuthModeInfo()`) |
| Add | `provider/cohere.go` | Copy from ratchet |
| Add | `provider/models.go` | Copy from ratchet (with token exchange + cohere support) |
| Add | `provider/openrouter.go` | Extract from openai.go (currently uses OpenAI with custom baseURL) |
| Add | `provider/auth_modes.go` | `AuthModeInfo` type, registry of all modes |
| Add | `provider/anthropic_bedrock.go` | Scaffold: config + constructor + AuthModeInfo |
| Add | `provider/anthropic_vertex.go` | Scaffold: config + constructor + AuthModeInfo |
| Add | `provider/anthropic_foundry.go` | Scaffold: config + constructor + AuthModeInfo |
| Add | `provider/openai_azure.go` | Scaffold: config + constructor + AuthModeInfo |
| Add | `provider/copilot_models.go` | Scaffold: config + constructor + AuthModeInfo |
| Keep | `provider/test_provider*.go` | Test providers (unchanged) |

#### ratchet (becomes consumer)

| Action | File | Description |
|---|---|---|
| Delete | `provider/` | Entire directory (all files) |
| Delete | `ratchetplugin/provider_adapter.go` | 155-line adapter (dead code after consolidation) |
| Modify | 38 files | Change import `ratchet/provider` → `workflow-plugin-agent/provider` |
| Modify | `ratchetplugin/plugin.go` | Remove adapter calls, use provider types directly |
| Modify | `ratchetplugin/step_agent_execute.go` | Simplify — no more type conversion |
| Modify | `go.mod` | Bump workflow-plugin-agent to v0.2.0 |

#### ratchet-cli (1 import change)

| Action | File | Description |
|---|---|---|
| Modify | `internal/daemon/chat.go` | Change import to `workflow-plugin-agent/provider` |
| Modify | `go.mod` | Bump both ratchet and workflow-plugin-agent |

### 6. Provider Interface Change

```go
type Provider interface {
    Name() string
    Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error)
    Stream(ctx context.Context, messages []Message, tools []ToolDef) (<-chan StreamEvent, error)
    AuthModeInfo() AuthModeInfo
}
```

### 7. What Doesn't Change

- All type names and interfaces stay identical
- All existing provider behavior stays identical
- Test providers (`TestProvider`, `ChannelTestProvider`, etc.) unchanged
- No behavioral changes for existing users

### 8. Deliverables

1. Consolidated provider package in workflow-plugin-agent v0.2.0
2. Updated ratchet with all imports changed, adapter deleted
3. Updated ratchet-cli with import changed
4. Scaffolded constructors for all provider modes with AuthModeInfo + docs links
5. Plan doc for implementing scaffolded modes (committed, not executed)
