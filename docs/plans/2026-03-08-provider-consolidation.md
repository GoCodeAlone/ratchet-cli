# Provider Consolidation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Eliminate the duplicated `provider/` package by making `workflow-plugin-agent/provider` the single canonical source, add `AuthModeInfo` to all providers, scaffold new auth modes, and update all consumers.

**Architecture:** Three-repo change. Phase 1: extend `workflow-plugin-agent/provider` with `AuthModeInfo`, `cohere.go`, `openrouter.go`, scaffolded modes, tag v0.2.0. Phase 2: delete `ratchet/provider/`, delete `provider_adapter.go`, rewrite 38 imports to point at workflow-plugin-agent, tag new ratchet. Phase 3: update ratchet-cli's 1 import and bump deps.

**Tech Stack:** Go 1.26, `github.com/GoCodeAlone/workflow-plugin-agent`, `github.com/GoCodeAlone/ratchet`, `github.com/GoCodeAlone/ratchet-cli`

---

## Phase 1: workflow-plugin-agent (canonical provider package)

### Task 1: Add AuthModeInfo type and registry

**Files:**
- Create: `/Users/jon/workspace/workflow-plugin-agent/provider/auth_modes.go`

**Step 1: Write the file**

```go
package provider

// AuthModeInfo describes an authentication/deployment mode for a provider backend.
type AuthModeInfo struct {
	Mode        string // e.g. "personal", "direct", "bedrock"
	DisplayName string // e.g. "GitHub Copilot (Personal/IDE)"
	Description string // What this mode does
	Warning     string // ToS/usage concerns (empty if none)
	DocsURL     string // Link to official documentation
	ServerSafe  bool   // Whether this mode is appropriate for server/service use
}

// AllAuthModes returns metadata for all known provider authentication modes,
// including both implemented and scaffolded providers.
func AllAuthModes() []AuthModeInfo {
	return []AuthModeInfo{
		// Anthropic
		{Mode: "direct", DisplayName: "Anthropic (Direct API)", Description: "Direct access to Anthropic's Claude models via API key.", DocsURL: "https://platform.claude.com/docs/en/api/getting-started", ServerSafe: true},
		{Mode: "bedrock", DisplayName: "Anthropic (Amazon Bedrock)", Description: "Access Claude models via Amazon Bedrock using AWS IAM SigV4 authentication.", DocsURL: "https://platform.claude.com/docs/en/build-with-claude/claude-on-amazon-bedrock", ServerSafe: true},
		{Mode: "vertex", DisplayName: "Anthropic (Google Vertex AI)", Description: "Access Claude models via Google Cloud Vertex AI using Application Default Credentials.", DocsURL: "https://platform.claude.com/docs/en/build-with-claude/claude-on-vertex-ai", ServerSafe: true},
		{Mode: "foundry", DisplayName: "Anthropic (Azure AI Foundry)", Description: "Access Claude models via Microsoft Azure AI Foundry.", DocsURL: "https://platform.claude.com/docs/en/build-with-claude/claude-in-microsoft-foundry", ServerSafe: true},
		// OpenAI
		{Mode: "direct", DisplayName: "OpenAI (Direct API)", Description: "Direct access to OpenAI models via API key.", DocsURL: "https://platform.openai.com/docs/api-reference/introduction", ServerSafe: true},
		{Mode: "azure", DisplayName: "OpenAI (Azure OpenAI Service)", Description: "Access OpenAI models via Azure OpenAI Service.", DocsURL: "https://learn.microsoft.com/en-us/azure/ai-services/openai/reference", ServerSafe: true},
		{Mode: "openrouter", DisplayName: "OpenRouter", Description: "Access multiple AI models via OpenRouter's unified API.", DocsURL: "https://openrouter.ai/docs/api/reference/authentication", ServerSafe: true},
		// GitHub Copilot
		{Mode: "personal", DisplayName: "GitHub Copilot (Personal/IDE)", Description: "Uses GitHub Copilot's chat API via OAuth token exchange. For IDE/CLI use only.", Warning: "This mode uses Copilot's internal API. Server use may violate ToS.", DocsURL: "https://docs.github.com/en/copilot", ServerSafe: false},
		{Mode: "github_models", DisplayName: "GitHub Models", Description: "Access AI models via GitHub's Models marketplace using a fine-grained PAT.", DocsURL: "https://docs.github.com/en/rest/models/inference", ServerSafe: true},
		// Cohere
		{Mode: "direct", DisplayName: "Cohere (Direct API)", Description: "Direct access to Cohere's Command models via API key.", DocsURL: "https://docs.cohere.com/reference/chat", ServerSafe: true},
	}
}
```

**Step 2: Verify it compiles**

Run: `cd /Users/jon/workspace/workflow-plugin-agent && go build ./provider/...`
Expected: PASS

**Step 3: Commit**

```bash
cd /Users/jon/workspace/workflow-plugin-agent
git add provider/auth_modes.go
git commit -m "feat: add AuthModeInfo type for provider auth mode awareness"
```

---

### Task 2: Add AuthModeInfo() to Provider interface and update existing providers

**Files:**
- Modify: `/Users/jon/workspace/workflow-plugin-agent/provider/provider.go` (add method to interface)
- Modify: `/Users/jon/workspace/workflow-plugin-agent/provider/anthropic.go` (implement AuthModeInfo)
- Modify: `/Users/jon/workspace/workflow-plugin-agent/provider/openai.go` (implement AuthModeInfo)
- Modify: `/Users/jon/workspace/workflow-plugin-agent/provider/copilot.go` (implement AuthModeInfo)
- Modify: `/Users/jon/workspace/workflow-plugin-agent/provider/test_provider.go` (implement AuthModeInfo)

**Step 1: Add AuthModeInfo() to Provider interface**

In `provider.go`, add to the `Provider` interface (after `Stream` method):

```go
	// AuthModeInfo returns metadata about this provider's authentication mode.
	AuthModeInfo() AuthModeInfo
```

**Step 2: Add AuthModeInfo() to AnthropicProvider**

In `anthropic.go`, add method:

```go
func (p *AnthropicProvider) AuthModeInfo() AuthModeInfo {
	return AuthModeInfo{
		Mode:        "direct",
		DisplayName: "Anthropic (Direct API)",
		Description: "Direct access to Anthropic's Claude models via API key.",
		DocsURL:     "https://platform.claude.com/docs/en/api/getting-started",
		ServerSafe:  true,
	}
}
```

**Step 3: Add AuthModeInfo() to OpenAIProvider**

In `openai.go`, add method:

```go
func (p *OpenAIProvider) AuthModeInfo() AuthModeInfo {
	return AuthModeInfo{
		Mode:        "direct",
		DisplayName: "OpenAI (Direct API)",
		Description: "Direct access to OpenAI models via API key.",
		DocsURL:     "https://platform.openai.com/docs/api-reference/introduction",
		ServerSafe:  true,
	}
}
```

**Step 4: Add AuthModeInfo() to CopilotProvider**

In `copilot.go`, add method:

```go
func (p *CopilotProvider) AuthModeInfo() AuthModeInfo {
	return AuthModeInfo{
		Mode:        "personal",
		DisplayName: "GitHub Copilot (Personal/IDE)",
		Description: "Uses GitHub Copilot's chat completions API via OAuth token exchange. Intended for IDE/CLI use under a Copilot Individual or Business subscription.",
		Warning:     "This mode uses Copilot's internal API intended for IDE integrations. Using it in server/service contexts may violate GitHub Copilot Terms of Service (https://docs.github.com/en/site-policy/github-terms/github-terms-for-additional-products-and-features).",
		DocsURL:     "https://docs.github.com/en/copilot",
		ServerSafe:  false,
	}
}
```

**Step 5: Add AuthModeInfo() to TestProvider**

In `test_provider.go`, add method:

```go
func (p *TestProvider) AuthModeInfo() AuthModeInfo {
	return AuthModeInfo{
		Mode:        "test",
		DisplayName: "Test Provider",
		Description: "Mock provider for testing.",
		ServerSafe:  true,
	}
}
```

**Step 6: Verify it compiles and tests pass**

Run: `cd /Users/jon/workspace/workflow-plugin-agent && go test ./provider/...`
Expected: PASS

**Step 7: Commit**

```bash
cd /Users/jon/workspace/workflow-plugin-agent
git add provider/provider.go provider/anthropic.go provider/openai.go provider/copilot.go provider/test_provider.go
git commit -m "feat: add AuthModeInfo() to Provider interface and all implementations"
```

---

### Task 3: Copy cohere.go from ratchet to workflow-plugin-agent

**Files:**
- Create: `/Users/jon/workspace/workflow-plugin-agent/provider/cohere.go`

**Step 1: Copy the file**

Copy `/Users/jon/workspace/ratchet/provider/cohere.go` to `/Users/jon/workspace/workflow-plugin-agent/provider/cohere.go`.

The file contains `CohereConfig`, `CohereProvider`, `NewCohereProvider()`, `Chat()`, `Stream()`, and the constant `defaultCohereBaseURL = "https://api.cohere.com"`.

**Step 2: Add AuthModeInfo() to CohereProvider**

Add to the copied file:

```go
func (p *CohereProvider) AuthModeInfo() AuthModeInfo {
	return AuthModeInfo{
		Mode:        "direct",
		DisplayName: "Cohere (Direct API)",
		Description: "Direct access to Cohere's Command models via API key.",
		DocsURL:     "https://docs.cohere.com/reference/chat",
		ServerSafe:  true,
	}
}
```

**Step 3: Verify it compiles**

Run: `cd /Users/jon/workspace/workflow-plugin-agent && go build ./provider/...`
Expected: PASS

**Step 4: Commit**

```bash
cd /Users/jon/workspace/workflow-plugin-agent
git add provider/cohere.go
git commit -m "feat: add Cohere provider (copied from ratchet, with AuthModeInfo)"
```

---

### Task 4: Extract OpenRouter as separate provider file

**Files:**
- Create: `/Users/jon/workspace/workflow-plugin-agent/provider/openrouter.go`

**Step 1: Create openrouter.go**

OpenRouter is an OpenAI-compatible API with a different base URL. Create a thin wrapper:

```go
package provider

const defaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"

// OpenRouterConfig configures the OpenRouter provider.
type OpenRouterConfig struct {
	APIKey    string
	Model     string
	BaseURL   string
	MaxTokens int
}

// NewOpenRouterProvider creates a provider that uses OpenRouter's OpenAI-compatible API.
func NewOpenRouterProvider(cfg OpenRouterConfig) *OpenAIProvider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultOpenRouterBaseURL
	}
	return NewOpenAIProvider(OpenAIConfig{
		APIKey:    cfg.APIKey,
		Model:     cfg.Model,
		BaseURL:   baseURL,
		MaxTokens: cfg.MaxTokens,
	})
}
```

Note: `NewOpenRouterProvider` returns `*OpenAIProvider` which already implements `AuthModeInfo()` with mode "direct". We need to override this — but since Go doesn't support method overriding on returned types, we'll add a dedicated OpenRouter auth mode via the scaffold approach (Task 6). For now, the constructor and config struct are the value.

**Step 2: Verify it compiles**

Run: `cd /Users/jon/workspace/workflow-plugin-agent && go build ./provider/...`
Expected: PASS

**Step 3: Commit**

```bash
cd /Users/jon/workspace/workflow-plugin-agent
git add provider/openrouter.go
git commit -m "feat: extract OpenRouter as separate provider with config struct"
```

---

### Task 5: Update models.go to include cohere and copilot token exchange

**Files:**
- Modify: `/Users/jon/workspace/workflow-plugin-agent/provider/models.go`

**Step 1: Check current models.go content**

The workflow-plugin-agent's `models.go` currently supports: anthropic, openai, openrouter, copilot, mock.
The ratchet version additionally supports: cohere, and has `exchangeCopilotToken` + `listCopilotModels` with full token exchange.

**Step 2: Add cohere case to ListModels switch**

Add after the copilot case:

```go
	case "cohere":
		return listCohereModels(ctx, apiKey, baseURL)
```

**Step 3: Copy missing functions from ratchet's models.go**

Copy these functions from `/Users/jon/workspace/ratchet/provider/models.go`:
- `listCohereModels(ctx, apiKey, baseURL)`
- `cohereFallbackModels()`
- `exchangeCopilotToken(ctx, oauthToken, tokenURL)` (if not already present)
- Update `listCopilotModels` to use token exchange (if not already present)

Check which functions already exist in workflow-plugin-agent's models.go and only add what's missing.

**Step 4: Verify it compiles**

Run: `cd /Users/jon/workspace/workflow-plugin-agent && go build ./provider/...`
Expected: PASS

**Step 5: Commit**

```bash
cd /Users/jon/workspace/workflow-plugin-agent
git add provider/models.go
git commit -m "feat: add Cohere model listing and Copilot token exchange to models.go"
```

---

### Task 6: Scaffold Copilot Models provider

**Files:**
- Create: `/Users/jon/workspace/workflow-plugin-agent/provider/copilot_models.go`

**Step 1: Create the scaffold**

```go
package provider

import (
	"context"
	"fmt"
)

// CopilotModelsConfig configures the GitHub Models provider.
// GitHub Models is a separate product from GitHub Copilot, available at models.github.ai.
// It uses fine-grained Personal Access Tokens with the models:read scope.
type CopilotModelsConfig struct {
	// Token is a GitHub fine-grained PAT with models:read permission.
	Token string
	// Model is the model identifier (e.g. "openai/gpt-4o", "anthropic/claude-sonnet-4").
	Model string
	// BaseURL overrides the default endpoint. Default: "https://models.github.ai/inference".
	BaseURL string
	// MaxTokens limits the response length.
	MaxTokens int
}

// copilotModelsProvider uses GitHub Models (models.github.ai) for inference.
type copilotModelsProvider struct {
	config CopilotModelsConfig
}

// NewCopilotModelsProvider creates a provider that uses GitHub Models for inference.
// GitHub Models provides access to various AI models via a fine-grained PAT.
//
// NOT YET IMPLEMENTED — scaffolded for future development.
//
// Docs: https://docs.github.com/en/rest/models/inference
// Billing: https://docs.github.com/billing/managing-billing-for-your-products/about-billing-for-github-models
func NewCopilotModelsProvider(_ CopilotModelsConfig) (*copilotModelsProvider, error) {
	return nil, fmt.Errorf("copilot_models provider not yet implemented: see https://docs.github.com/en/rest/models/inference")
}

func (p *copilotModelsProvider) Name() string { return "copilot_models" }

func (p *copilotModelsProvider) Chat(_ context.Context, _ []Message, _ []ToolDef) (*Response, error) {
	return nil, fmt.Errorf("copilot_models provider not yet implemented")
}

func (p *copilotModelsProvider) Stream(_ context.Context, _ []Message, _ []ToolDef) (<-chan StreamEvent, error) {
	return nil, fmt.Errorf("copilot_models provider not yet implemented")
}

func (p *copilotModelsProvider) AuthModeInfo() AuthModeInfo {
	return AuthModeInfo{
		Mode:        "github_models",
		DisplayName: "GitHub Models",
		Description: "Access AI models via GitHub's Models marketplace using a fine-grained PAT with models:read scope.",
		DocsURL:     "https://docs.github.com/en/rest/models/inference",
		ServerSafe:  true,
	}
}
```

**Step 2: Verify it compiles**

Run: `cd /Users/jon/workspace/workflow-plugin-agent && go build ./provider/...`
Expected: PASS

**Step 3: Commit**

```bash
cd /Users/jon/workspace/workflow-plugin-agent
git add provider/copilot_models.go
git commit -m "feat: scaffold GitHub Models provider with AuthModeInfo"
```

---

### Task 7: Scaffold Anthropic Bedrock provider

**Files:**
- Create: `/Users/jon/workspace/workflow-plugin-agent/provider/anthropic_bedrock.go`

**Step 1: Create the scaffold**

```go
package provider

import (
	"context"
	"fmt"
)

// AnthropicBedrockConfig configures the Anthropic provider for Amazon Bedrock.
// Uses AWS IAM SigV4 authentication against the Bedrock Runtime API.
type AnthropicBedrockConfig struct {
	// Region is the AWS region (e.g. "us-east-1").
	Region string
	// Model is the Bedrock model ID (e.g. "anthropic.claude-sonnet-4-20250514-v1:0").
	Model string
	// MaxTokens limits the response length.
	MaxTokens int
	// AccessKeyID is the AWS access key (optional if using instance/role credentials).
	AccessKeyID string
	// SecretAccessKey is the AWS secret key (optional if using instance/role credentials).
	SecretAccessKey string
	// SessionToken is the AWS session token for temporary credentials (optional).
	SessionToken string
	// Profile is the AWS config profile name (optional, for shared credentials).
	Profile string
}

// anthropicBedrockProvider accesses Anthropic models via Amazon Bedrock.
type anthropicBedrockProvider struct {
	config AnthropicBedrockConfig
}

// NewAnthropicBedrockProvider creates a provider that accesses Claude via Amazon Bedrock.
//
// NOT YET IMPLEMENTED — scaffolded for future development.
//
// Docs: https://platform.claude.com/docs/en/build-with-claude/claude-on-amazon-bedrock
func NewAnthropicBedrockProvider(_ AnthropicBedrockConfig) (*anthropicBedrockProvider, error) {
	return nil, fmt.Errorf("anthropic_bedrock provider not yet implemented: see https://platform.claude.com/docs/en/build-with-claude/claude-on-amazon-bedrock")
}

func (p *anthropicBedrockProvider) Name() string { return "anthropic_bedrock" }

func (p *anthropicBedrockProvider) Chat(_ context.Context, _ []Message, _ []ToolDef) (*Response, error) {
	return nil, fmt.Errorf("anthropic_bedrock provider not yet implemented")
}

func (p *anthropicBedrockProvider) Stream(_ context.Context, _ []Message, _ []ToolDef) (<-chan StreamEvent, error) {
	return nil, fmt.Errorf("anthropic_bedrock provider not yet implemented")
}

func (p *anthropicBedrockProvider) AuthModeInfo() AuthModeInfo {
	return AuthModeInfo{
		Mode:        "bedrock",
		DisplayName: "Anthropic (Amazon Bedrock)",
		Description: "Access Claude models via Amazon Bedrock using AWS IAM SigV4 authentication. Supports instance roles, access keys, and shared credential profiles.",
		DocsURL:     "https://platform.claude.com/docs/en/build-with-claude/claude-on-amazon-bedrock",
		ServerSafe:  true,
	}
}
```

**Step 2: Verify it compiles**

Run: `cd /Users/jon/workspace/workflow-plugin-agent && go build ./provider/...`
Expected: PASS

**Step 3: Commit**

```bash
cd /Users/jon/workspace/workflow-plugin-agent
git add provider/anthropic_bedrock.go
git commit -m "feat: scaffold Anthropic Bedrock provider with AuthModeInfo"
```

---

### Task 8: Scaffold Anthropic Vertex AI provider

**Files:**
- Create: `/Users/jon/workspace/workflow-plugin-agent/provider/anthropic_vertex.go`

**Step 1: Create the scaffold**

```go
package provider

import (
	"context"
	"fmt"
)

// AnthropicVertexConfig configures the Anthropic provider for Google Vertex AI.
// Uses GCP Application Default Credentials (ADC) or explicit OAuth2 tokens.
type AnthropicVertexConfig struct {
	// ProjectID is the GCP project ID.
	ProjectID string
	// Region is the GCP region (e.g. "us-east5", "europe-west1").
	Region string
	// Model is the Vertex model ID (e.g. "claude-sonnet-4@20250514").
	Model string
	// MaxTokens limits the response length.
	MaxTokens int
	// CredentialsJSON is the GCP service account JSON (optional if using ADC).
	CredentialsJSON string
}

// anthropicVertexProvider accesses Anthropic models via Google Vertex AI.
type anthropicVertexProvider struct {
	config AnthropicVertexConfig
}

// NewAnthropicVertexProvider creates a provider that accesses Claude via Google Vertex AI.
//
// NOT YET IMPLEMENTED — scaffolded for future development.
//
// Docs: https://platform.claude.com/docs/en/build-with-claude/claude-on-vertex-ai
func NewAnthropicVertexProvider(_ AnthropicVertexConfig) (*anthropicVertexProvider, error) {
	return nil, fmt.Errorf("anthropic_vertex provider not yet implemented: see https://platform.claude.com/docs/en/build-with-claude/claude-on-vertex-ai")
}

func (p *anthropicVertexProvider) Name() string { return "anthropic_vertex" }

func (p *anthropicVertexProvider) Chat(_ context.Context, _ []Message, _ []ToolDef) (*Response, error) {
	return nil, fmt.Errorf("anthropic_vertex provider not yet implemented")
}

func (p *anthropicVertexProvider) Stream(_ context.Context, _ []Message, _ []ToolDef) (<-chan StreamEvent, error) {
	return nil, fmt.Errorf("anthropic_vertex provider not yet implemented")
}

func (p *anthropicVertexProvider) AuthModeInfo() AuthModeInfo {
	return AuthModeInfo{
		Mode:        "vertex",
		DisplayName: "Anthropic (Google Vertex AI)",
		Description: "Access Claude models via Google Cloud Vertex AI using Application Default Credentials (ADC) or service account JSON.",
		DocsURL:     "https://platform.claude.com/docs/en/build-with-claude/claude-on-vertex-ai",
		ServerSafe:  true,
	}
}
```

**Step 2: Verify it compiles**

Run: `cd /Users/jon/workspace/workflow-plugin-agent && go build ./provider/...`
Expected: PASS

**Step 3: Commit**

```bash
cd /Users/jon/workspace/workflow-plugin-agent
git add provider/anthropic_vertex.go
git commit -m "feat: scaffold Anthropic Vertex AI provider with AuthModeInfo"
```

---

### Task 9: Scaffold Anthropic Azure Foundry provider

**Files:**
- Create: `/Users/jon/workspace/workflow-plugin-agent/provider/anthropic_foundry.go`

**Step 1: Create the scaffold**

```go
package provider

import (
	"context"
	"fmt"
)

// AnthropicFoundryConfig configures the Anthropic provider for Microsoft Azure AI Foundry.
// Uses Azure API keys or Entra ID (formerly Azure AD) tokens.
type AnthropicFoundryConfig struct {
	// Resource is the Azure AI Services resource name (forms the URL: {resource}.services.ai.azure.com).
	Resource string
	// Model is the model deployment name.
	Model string
	// MaxTokens limits the response length.
	MaxTokens int
	// APIKey is the Azure API key (use this OR Entra ID token, not both).
	APIKey string
	// EntraToken is a Microsoft Entra ID bearer token (optional, alternative to APIKey).
	EntraToken string
}

// anthropicFoundryProvider accesses Anthropic models via Azure AI Foundry.
type anthropicFoundryProvider struct {
	config AnthropicFoundryConfig
}

// NewAnthropicFoundryProvider creates a provider that accesses Claude via Azure AI Foundry.
//
// NOT YET IMPLEMENTED — scaffolded for future development.
//
// Docs: https://platform.claude.com/docs/en/build-with-claude/claude-in-microsoft-foundry
func NewAnthropicFoundryProvider(_ AnthropicFoundryConfig) (*anthropicFoundryProvider, error) {
	return nil, fmt.Errorf("anthropic_foundry provider not yet implemented: see https://platform.claude.com/docs/en/build-with-claude/claude-in-microsoft-foundry")
}

func (p *anthropicFoundryProvider) Name() string { return "anthropic_foundry" }

func (p *anthropicFoundryProvider) Chat(_ context.Context, _ []Message, _ []ToolDef) (*Response, error) {
	return nil, fmt.Errorf("anthropic_foundry provider not yet implemented")
}

func (p *anthropicFoundryProvider) Stream(_ context.Context, _ []Message, _ []ToolDef) (<-chan StreamEvent, error) {
	return nil, fmt.Errorf("anthropic_foundry provider not yet implemented")
}

func (p *anthropicFoundryProvider) AuthModeInfo() AuthModeInfo {
	return AuthModeInfo{
		Mode:        "foundry",
		DisplayName: "Anthropic (Azure AI Foundry)",
		Description: "Access Claude models via Microsoft Azure AI Foundry using Azure API keys or Microsoft Entra ID tokens.",
		DocsURL:     "https://platform.claude.com/docs/en/build-with-claude/claude-in-microsoft-foundry",
		ServerSafe:  true,
	}
}
```

**Step 2: Verify it compiles**

Run: `cd /Users/jon/workspace/workflow-plugin-agent && go build ./provider/...`
Expected: PASS

**Step 3: Commit**

```bash
cd /Users/jon/workspace/workflow-plugin-agent
git add provider/anthropic_foundry.go
git commit -m "feat: scaffold Anthropic Azure Foundry provider with AuthModeInfo"
```

---

### Task 10: Scaffold OpenAI Azure provider

**Files:**
- Create: `/Users/jon/workspace/workflow-plugin-agent/provider/openai_azure.go`

**Step 1: Create the scaffold**

```go
package provider

import (
	"context"
	"fmt"
)

// OpenAIAzureConfig configures the OpenAI provider for Azure OpenAI Service.
// Uses Azure API keys or Entra ID tokens. URLs follow the pattern:
// {resource}.openai.azure.com/openai/deployments/{deployment}/chat/completions?api-version={version}
type OpenAIAzureConfig struct {
	// Resource is the Azure OpenAI resource name.
	Resource string
	// DeploymentName is the model deployment name in Azure.
	DeploymentName string
	// APIVersion is the Azure API version (e.g. "2024-10-21").
	APIVersion string
	// MaxTokens limits the response length.
	MaxTokens int
	// APIKey is the Azure API key (use this OR Entra ID token, not both).
	APIKey string
	// EntraToken is a Microsoft Entra ID bearer token (optional, alternative to APIKey).
	EntraToken string
}

// openaiAzureProvider accesses OpenAI models via Azure OpenAI Service.
type openaiAzureProvider struct {
	config OpenAIAzureConfig
}

// NewOpenAIAzureProvider creates a provider that accesses OpenAI models via Azure.
//
// NOT YET IMPLEMENTED — scaffolded for future development.
//
// Docs: https://learn.microsoft.com/en-us/azure/ai-services/openai/reference
func NewOpenAIAzureProvider(_ OpenAIAzureConfig) (*openaiAzureProvider, error) {
	return nil, fmt.Errorf("openai_azure provider not yet implemented: see https://learn.microsoft.com/en-us/azure/ai-services/openai/reference")
}

func (p *openaiAzureProvider) Name() string { return "openai_azure" }

func (p *openaiAzureProvider) Chat(_ context.Context, _ []Message, _ []ToolDef) (*Response, error) {
	return nil, fmt.Errorf("openai_azure provider not yet implemented")
}

func (p *openaiAzureProvider) Stream(_ context.Context, _ []Message, _ []ToolDef) (<-chan StreamEvent, error) {
	return nil, fmt.Errorf("openai_azure provider not yet implemented")
}

func (p *openaiAzureProvider) AuthModeInfo() AuthModeInfo {
	return AuthModeInfo{
		Mode:        "azure",
		DisplayName: "OpenAI (Azure OpenAI Service)",
		Description: "Access OpenAI models via Azure OpenAI Service using Azure API keys or Microsoft Entra ID tokens. Uses deployment-specific URLs.",
		DocsURL:     "https://learn.microsoft.com/en-us/azure/ai-services/openai/reference",
		ServerSafe:  true,
	}
}
```

**Step 2: Verify it compiles**

Run: `cd /Users/jon/workspace/workflow-plugin-agent && go build ./provider/...`
Expected: PASS

**Step 3: Commit**

```bash
cd /Users/jon/workspace/workflow-plugin-agent
git add provider/openai_azure.go
git commit -m "feat: scaffold Azure OpenAI provider with AuthModeInfo"
```

---

### Task 11: Tag workflow-plugin-agent v0.2.0

**Files:**
- None (tag only)

**Step 1: Run all tests**

Run: `cd /Users/jon/workspace/workflow-plugin-agent && go test ./...`
Expected: ALL PASS

**Step 2: Tag and push**

```bash
cd /Users/jon/workspace/workflow-plugin-agent
git tag v0.2.0
git push origin main --tags
```

---

## Phase 2: ratchet (become consumer of workflow-plugin-agent/provider)

### Task 12: Update go.mod to use workflow-plugin-agent v0.2.0

**Files:**
- Modify: `/Users/jon/workspace/ratchet/go.mod`

**Step 1: Update dependency**

```bash
cd /Users/jon/workspace/ratchet
go get github.com/GoCodeAlone/workflow-plugin-agent@v0.2.0
go mod tidy
```

**Step 2: Verify it resolves**

Run: `cd /Users/jon/workspace/ratchet && go mod verify`
Expected: PASS

**Step 3: Commit**

```bash
cd /Users/jon/workspace/ratchet
git add go.mod go.sum
git commit -m "chore: bump workflow-plugin-agent to v0.2.0"
```

---

### Task 13: Update all 38 ratchet imports from ratchet/provider to workflow-plugin-agent/provider

**Files:**
- Modify: 38 files in `/Users/jon/workspace/ratchet/ratchetplugin/` and `/Users/jon/workspace/ratchet/plugin/`

The complete list of files (from grep):
```
ratchetplugin/plugin.go
ratchetplugin/provider_adapter.go
ratchetplugin/provider_registry.go
ratchetplugin/provider_registry_test.go
ratchetplugin/module_ai_provider.go
ratchetplugin/step_agent_execute.go
ratchetplugin/step_provider_models.go
ratchetplugin/test_provider.go
ratchetplugin/test_provider_test.go
ratchetplugin/test_provider_helpers_test.go
ratchetplugin/test_provider_scripted.go
ratchetplugin/tool_registry.go
ratchetplugin/ratchetplugin_test.go
ratchetplugin/context_manager.go
ratchetplugin/context_manager_test.go
ratchetplugin/transcript.go
ratchetplugin/secret_guard.go
ratchetplugin/memory.go
ratchetplugin/module_mcp_client.go
ratchetplugin/tools/approval.go
ratchetplugin/tools/browser.go
ratchetplugin/tools/code.go
ratchetplugin/tools/data.go
ratchetplugin/tools/db_external.go
ratchetplugin/tools/file.go
ratchetplugin/tools/git.go
ratchetplugin/tools/git_pr.go
ratchetplugin/tools/human_request.go
ratchetplugin/tools/k8s.go
ratchetplugin/tools/memory.go
ratchetplugin/tools/message.go
ratchetplugin/tools/security.go
ratchetplugin/tools/security_url.go
ratchetplugin/tools/shell.go
ratchetplugin/tools/sub_agent.go
ratchetplugin/tools/task.go
ratchetplugin/tools/web.go
plugin/plugin.go
```

**Step 1: Replace all imports**

In every file listed above, replace:
```go
"github.com/GoCodeAlone/ratchet/provider"
```
with:
```go
"github.com/GoCodeAlone/workflow-plugin-agent/provider"
```

For files that also import `agentprovider "github.com/GoCodeAlone/workflow-plugin-agent/provider"` (like `plugin.go` and `provider_adapter.go`), the aliased import will be removed in the next task — for now just do the mechanical replacement on the unaliased import.

**Step 2: Handle files with dual imports**

`ratchetplugin/plugin.go` currently has both:
```go
"github.com/GoCodeAlone/ratchet/provider"
agentprovider "github.com/GoCodeAlone/workflow-plugin-agent/provider"
```

After replacement, it would have two identical imports. Remove the aliased one and use the unaliased import everywhere. Any code using `agentprovider.XXX` should change to `provider.XXX`.

Similarly `ratchetplugin/provider_adapter.go` has dual imports — but this file is deleted in the next task.

**Step 3: Verify it compiles (expect errors from adapter)**

Run: `cd /Users/jon/workspace/ratchet && go build ./...`
Expected: May have errors related to provider_adapter.go — that's OK, it's deleted next.

**Step 4: Commit (even if not yet compiling)**

```bash
cd /Users/jon/workspace/ratchet
git add -A ratchetplugin/ plugin/
git commit -m "refactor: change all imports from ratchet/provider to workflow-plugin-agent/provider"
```

---

### Task 14: Delete provider_adapter.go and update plugin.go

**Files:**
- Delete: `/Users/jon/workspace/ratchet/ratchetplugin/provider_adapter.go`
- Modify: `/Users/jon/workspace/ratchet/ratchetplugin/plugin.go` (remove adapter calls, remove agentprovider alias)

**Step 1: Delete the adapter**

```bash
rm /Users/jon/workspace/ratchet/ratchetplugin/provider_adapter.go
```

**Step 2: Update plugin.go**

Remove the `agentprovider` import alias (now redundant since everything uses `provider` from workflow-plugin-agent).

Find all calls to `agentProviderToRatchet()` and replace with direct usage of the agent plugin's provider. The adapter wrapped `agentplugin.ProviderModule.Provider()` (which returns `agentprovider.Provider`) to satisfy `provider.Provider` — but now they're the same type.

Anywhere that called `agentProviderToRatchet(mod)`, replace with `mod.Provider()`.

**Step 3: Verify it compiles**

Run: `cd /Users/jon/workspace/ratchet && go build ./...`
Expected: May still fail if provider_registry.go references ratchet/provider constructors — fixed next task.

**Step 4: Commit**

```bash
cd /Users/jon/workspace/ratchet
git add -A
git commit -m "refactor: delete provider_adapter.go (155 lines), use provider types directly"
```

---

### Task 15: Update provider_registry.go factory functions

**Files:**
- Modify: `/Users/jon/workspace/ratchet/ratchetplugin/provider_registry.go`

**Step 1: Update factory functions**

The factory functions (lines 226-267) call `provider.NewAnthropicProvider(...)`, `provider.NewOpenAIProvider(...)`, etc. After the import change in Task 13, `provider` now points to `workflow-plugin-agent/provider`, so these constructor calls should just work — the config struct names and constructor signatures are the same.

Verify each factory function references the correct config type:
- `provider.AnthropicConfig` → exists in workflow-plugin-agent
- `provider.OpenAIConfig` → exists in workflow-plugin-agent
- `provider.CopilotConfig` → exists in workflow-plugin-agent
- `provider.CohereConfig` → now exists (Task 3)
- `provider.OpenRouterConfig` → now exists (Task 4)

Update `openrouterProviderFactory` to use the new `OpenRouterConfig`:

```go
func openrouterProviderFactory(apiKey string, cfg LLMProviderConfig) (provider.Provider, error) {
	return provider.NewOpenRouterProvider(provider.OpenRouterConfig{
		APIKey:    apiKey,
		Model:     cfg.Model,
		BaseURL:   cfg.BaseURL,
		MaxTokens: cfg.MaxTokens,
	}), nil
}
```

**Step 2: Verify it compiles**

Run: `cd /Users/jon/workspace/ratchet && go build ./...`
Expected: PASS

**Step 3: Commit**

```bash
cd /Users/jon/workspace/ratchet
git add ratchetplugin/provider_registry.go
git commit -m "refactor: update provider factories to use workflow-plugin-agent constructors"
```

---

### Task 16: Update step_provider_models.go

**Files:**
- Modify: `/Users/jon/workspace/ratchet/ratchetplugin/step_provider_models.go`

**Step 1: Verify imports**

This file calls `provider.ListModels(...)` and uses `provider.ModelInfo`. After the import change, these should resolve to workflow-plugin-agent's versions. Verify the function signatures match.

If `ListModels` or `ModelInfo` don't exist in workflow-plugin-agent's models.go, they need to be added there (should have been handled in Task 5).

**Step 2: Verify it compiles**

Run: `cd /Users/jon/workspace/ratchet && go build ./ratchetplugin/...`
Expected: PASS

**Step 3: Commit (if changes needed)**

```bash
cd /Users/jon/workspace/ratchet
git add ratchetplugin/step_provider_models.go
git commit -m "fix: update step_provider_models to use consolidated provider package"
```

---

### Task 17: Delete ratchet/provider/ directory

**Files:**
- Delete: `/Users/jon/workspace/ratchet/provider/` (entire directory — 10 files)

**Step 1: Delete the directory**

```bash
rm -rf /Users/jon/workspace/ratchet/provider/
```

**Step 2: Run go mod tidy**

```bash
cd /Users/jon/workspace/ratchet && go mod tidy
```

**Step 3: Verify it compiles and tests pass**

Run: `cd /Users/jon/workspace/ratchet && go test ./...`
Expected: ALL PASS

**Step 4: Commit**

```bash
cd /Users/jon/workspace/ratchet
git add -A
git commit -m "refactor: delete ratchet/provider/ package (consolidated into workflow-plugin-agent/provider)"
```

---

### Task 18: Tag ratchet

**Files:**
- None

**Step 1: Verify all tests pass**

Run: `cd /Users/jon/workspace/ratchet && go test ./...`
Expected: ALL PASS

**Step 2: Tag and push**

```bash
cd /Users/jon/workspace/ratchet
git tag v0.1.12
git push origin main --tags
```

---

## Phase 3: ratchet-cli (update single import)

### Task 19: Update ratchet-cli imports and dependencies

**Files:**
- Modify: `/Users/jon/workspace/ratchet-cli/internal/daemon/chat.go` (change import)
- Modify: `/Users/jon/workspace/ratchet-cli/go.mod` (bump ratchet + workflow-plugin-agent)

**Step 1: Update go.mod**

```bash
cd /Users/jon/workspace/ratchet-cli
go get github.com/GoCodeAlone/ratchet@v0.1.12
go get github.com/GoCodeAlone/workflow-plugin-agent@v0.2.0
go mod tidy
```

**Step 2: Update chat.go import**

In `/Users/jon/workspace/ratchet-cli/internal/daemon/chat.go`, replace:
```go
"github.com/GoCodeAlone/ratchet/provider"
```
with:
```go
"github.com/GoCodeAlone/workflow-plugin-agent/provider"
```

All usage of `provider.Provider`, `provider.Message`, `provider.RoleSystem`, `provider.RoleUser`, `provider.ToolCall` stays the same — the types are identical.

**Step 3: Verify it compiles and tests pass**

Run: `cd /Users/jon/workspace/ratchet-cli && go build ./... && go test ./...`
Expected: ALL PASS

**Step 4: Commit**

```bash
cd /Users/jon/workspace/ratchet-cli
git add go.mod go.sum internal/daemon/chat.go
git commit -m "refactor: use workflow-plugin-agent/provider as canonical provider package"
```

---

### Task 20: Tag ratchet-cli

**Files:**
- None

**Step 1: Tag and push**

```bash
cd /Users/jon/workspace/ratchet-cli
git tag v0.1.12
git push origin main --tags
```

---

## Phase 4: Follow-up plan document

### Task 21: Write and commit plan for implementing scaffolded provider modes

**Files:**
- Create: `/Users/jon/workspace/ratchet-cli/docs/plans/2026-03-08-scaffolded-providers-implementation.md`

**Step 1: Write the plan document**

Create a plan document covering the implementation of all 6 scaffolded provider modes:
1. GitHub Models (`copilot_models.go`) — OpenAI-compatible API at `models.github.ai/inference`
2. Anthropic Bedrock (`anthropic_bedrock.go`) — AWS SigV4 auth, Bedrock Runtime API
3. Anthropic Vertex (`anthropic_vertex.go`) — GCP ADC/OAuth2, Vertex AI endpoint
4. Anthropic Foundry (`anthropic_foundry.go`) — Azure API key/Entra ID, Azure AI Foundry
5. OpenAI Azure (`openai_azure.go`) — Azure API key/Entra ID, deployment-based URLs
6. OpenRouter dedicated type (`openrouter.go`) — separate type with proper AuthModeInfo

The plan should include:
- Research links for each provider's API format
- Config struct fields (already scaffolded)
- HTTP request construction (endpoint, headers, body format)
- Response parsing (each has slightly different response shapes)
- Token exchange or credential resolution logic
- Test strategy (mock HTTP servers per provider)
- Integration into `provider_registry.go` factories
- Update to `ListModels()` for each new provider type

**Step 2: Commit**

```bash
cd /Users/jon/workspace/ratchet-cli
git add docs/plans/2026-03-08-scaffolded-providers-implementation.md
git commit -m "docs: add implementation plan for scaffolded provider modes"
```

---

## Verification Checklist

After all tasks complete:

- [ ] `workflow-plugin-agent` has: provider.go (with AuthModeInfo in interface), anthropic.go, openai.go, copilot.go, cohere.go, openrouter.go, auth_modes.go, copilot_models.go, anthropic_bedrock.go, anthropic_vertex.go, anthropic_foundry.go, openai_azure.go, models.go, test_provider*.go
- [ ] `workflow-plugin-agent` tagged v0.2.0
- [ ] `ratchet/provider/` directory deleted
- [ ] `ratchet/ratchetplugin/provider_adapter.go` deleted
- [ ] All 38 ratchet files import from `workflow-plugin-agent/provider`
- [ ] `ratchet` tests pass, tagged
- [ ] `ratchet-cli` chat.go imports from `workflow-plugin-agent/provider`
- [ ] `ratchet-cli` tests pass, tagged
- [ ] Plan doc for scaffolded provider implementation committed
