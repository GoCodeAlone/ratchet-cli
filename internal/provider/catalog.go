package providerauth

import (
	"fmt"
	"slices"
	"strings"
)

// Category groups provider setup entries for presentation.
type Category string

const (
	CategoryAPI          Category = "api"
	CategoryCompatible   Category = "compatible"
	CategorySubscription Category = "subscription"
	CategoryCloud        Category = "cloud"
	CategoryLocal        Category = "local"
	CategoryCLI          Category = "cli"
)

// AuthStrategy identifies the credential flow used during setup.
type AuthStrategy string

const (
	AuthAPIKey        AuthStrategy = "api_key"
	AuthAnthropic     AuthStrategy = "anthropic"
	AuthGitHubDevice  AuthStrategy = "github_device"
	AuthOpenAIChatGPT AuthStrategy = "openai_chatgpt"
	AuthNone          AuthStrategy = "none"
	AuthCLINative     AuthStrategy = "cli_native"
)

// SetupStrategy identifies provider-specific setup behavior.
type SetupStrategy string

const (
	SetupInteractive SetupStrategy = "interactive"
	SetupOllama      SetupStrategy = "ollama"
	SetupCLINative   SetupStrategy = "cli_native"
)

// ModelStrategy identifies how setup obtains a model.
type ModelStrategy string

const (
	ModelDynamic  ModelStrategy = "dynamic"
	ModelManual   ModelStrategy = "manual"
	ModelOllama   ModelStrategy = "ollama"
	ModelExternal ModelStrategy = "external"
)

// SettingField describes a non-secret provider setting.
type SettingField struct {
	Key         string
	Label       string
	Placeholder string
	Default     string
	Required    bool
	Choices     []string
}

// SetupEntry describes one user-visible provider setup path.
type SetupEntry struct {
	Type               string
	DisplayName        string
	Description        string
	Aliases            []string
	Category           Category
	Auth               AuthStrategy
	Setup              SetupStrategy
	Model              ModelStrategy
	APIKeyEnv          string
	CredentialLabel    string
	CredentialRequired bool
	PromptBaseURL      bool
	DefaultBaseURL     string
	BaseURLRequired    bool
	Settings           []SettingField
	AllowManualModel   bool
	SetupAlias         string
	SetupCommand       string
	DefaultAlias       string
	CLICommand         string
	InstallHint        string
	AuthHint           string
	ModelBehavior      string
	CredentialBoundary string
}

var setupCatalog = []SetupEntry{
	{
		Type: "anthropic", DisplayName: "Anthropic (Claude)",
		Description: "Claude models through the Anthropic API.", Aliases: []string{"claude-api"}, Category: CategoryAPI,
		Auth: AuthAnthropic, Setup: SetupInteractive, Model: ModelDynamic,
		APIKeyEnv: "ANTHROPIC_API_KEY", CredentialLabel: "Anthropic API key", CredentialRequired: true, AllowManualModel: true,
		SetupCommand: "ratchet provider add anthropic", InstallHint: "No separate CLI required.",
		AuthHint:           "Use an Anthropic API key in the CLI; the TUI also offers the supported Anthropic browser flow.",
		ModelBehavior:      "Lists Anthropic models when available; accepts a model ID manually.",
		CredentialBoundary: "Stores the API credential through the daemon secrets provider.",
	},
	{
		Type: "openai", DisplayName: "OpenAI (API)",
		Description: "OpenAI API models and optional compatible base URLs.", Aliases: []string{"openai-api"}, Category: CategoryAPI,
		Auth: AuthAPIKey, Setup: SetupInteractive, Model: ModelDynamic,
		APIKeyEnv: "OPENAI_API_KEY", CredentialLabel: "OpenAI API key", CredentialRequired: true,
		PromptBaseURL: true, DefaultBaseURL: "https://api.openai.com/v1", AllowManualModel: true,
		SetupCommand: "ratchet provider add openai", InstallHint: "No separate CLI required.", AuthHint: "Use an OpenAI API key.",
		ModelBehavior:      "Lists OpenAI models when available; accepts a model ID manually.",
		CredentialBoundary: "Stores the API credential through the daemon secrets provider.",
	},
	{
		Type: "openrouter", DisplayName: "OpenRouter",
		Description: "Multi-provider models through OpenRouter.", Aliases: []string{"open-router"}, Category: CategoryAPI,
		Auth: AuthAPIKey, Setup: SetupInteractive, Model: ModelDynamic,
		APIKeyEnv: "OPENROUTER_API_KEY", CredentialLabel: "OpenRouter API key", CredentialRequired: true,
		DefaultBaseURL: "https://openrouter.ai/api/v1", AllowManualModel: true,
		SetupCommand: "ratchet provider add openrouter", InstallHint: "No separate CLI required.", AuthHint: "Use an OpenRouter API key.",
		ModelBehavior:      "Lists OpenRouter models when available; accepts a model ID manually.",
		CredentialBoundary: "Stores the API credential through the daemon secrets provider.",
	},
	{
		Type: "cohere", DisplayName: "Cohere",
		Description: "Cohere language models.", Aliases: []string{"cohere-api"}, Category: CategoryAPI,
		Auth: AuthAPIKey, Setup: SetupInteractive, Model: ModelDynamic,
		APIKeyEnv: "COHERE_API_KEY", CredentialLabel: "Cohere API key", CredentialRequired: true,
		DefaultBaseURL: "https://api.cohere.ai/v1", AllowManualModel: true,
		SetupCommand: "ratchet provider add cohere", InstallHint: "No separate CLI required.", AuthHint: "Use a Cohere API key.",
		ModelBehavior:      "Lists Cohere models when available; accepts a model ID manually.",
		CredentialBoundary: "Stores the API credential through the daemon secrets provider.",
	},
	{
		Type: "copilot_models", DisplayName: "GitHub Models",
		Description: "Models served by the GitHub Models inference API.", Aliases: []string{"copilot-models", "github-models"}, Category: CategoryAPI,
		Auth: AuthAPIKey, Setup: SetupInteractive, Model: ModelDynamic,
		APIKeyEnv: "GITHUB_TOKEN", CredentialLabel: "GitHub token", CredentialRequired: true,
		DefaultBaseURL: "https://models.github.ai/inference", AllowManualModel: true,
		SetupCommand: "ratchet provider add copilot_models", InstallHint: "No separate CLI required.", AuthHint: "Use a GitHub token with Models access.",
		ModelBehavior:      "Lists GitHub Models when available; accepts a model ID manually.",
		CredentialBoundary: "Stores the GitHub token through the daemon secrets provider.",
	},
	{
		Type: "gemini", DisplayName: "Google Gemini",
		Description: "Gemini models through the Google AI API.", Aliases: []string{"google-gemini"}, Category: CategoryAPI,
		Auth: AuthAPIKey, Setup: SetupInteractive, Model: ModelDynamic,
		APIKeyEnv: "GEMINI_API_KEY", CredentialLabel: "Gemini API key", CredentialRequired: true, AllowManualModel: true,
		SetupCommand: "ratchet provider add gemini", InstallHint: "No separate CLI required.", AuthHint: "Use a Gemini API key.",
		ModelBehavior:      "Lists Gemini models when available; accepts a model ID manually.",
		CredentialBoundary: "Stores the API credential through the daemon secrets provider.",
	},
	{
		Type: "openai_compatible", DisplayName: "OpenAI-compatible endpoint",
		Description: "A server implementing the OpenAI chat completions API.", Aliases: []string{"openai-compatible"}, Category: CategoryCompatible,
		Auth: AuthAPIKey, Setup: SetupInteractive, Model: ModelDynamic,
		CredentialLabel: "API key", PromptBaseURL: true, BaseURLRequired: true, AllowManualModel: true,
		SetupCommand: "ratchet provider add openai_compatible", InstallHint: "Provide a reachable OpenAI-compatible endpoint.",
		AuthHint:           "Enter an API key only when the endpoint requires one.",
		ModelBehavior:      "Lists endpoint models when supported; accepts a model ID manually.",
		CredentialBoundary: "Stores a supplied API credential through the daemon secrets provider.",
	},
	{
		Type: "anthropic_compatible", DisplayName: "Anthropic-compatible endpoint",
		Description: "A server implementing the Anthropic messages API.", Aliases: []string{"anthropic-compatible"}, Category: CategoryCompatible,
		Auth: AuthAPIKey, Setup: SetupInteractive, Model: ModelDynamic,
		CredentialLabel: "API key", PromptBaseURL: true, BaseURLRequired: true, AllowManualModel: true,
		SetupCommand: "ratchet provider add anthropic_compatible", InstallHint: "Provide a reachable Anthropic-compatible endpoint.",
		AuthHint:           "Enter an API key only when the endpoint requires one.",
		ModelBehavior:      "Lists endpoint models when supported; accepts a model ID manually.",
		CredentialBoundary: "Stores a supplied API credential through the daemon secrets provider.",
	},
	{
		Type: "custom", DisplayName: "Custom endpoint",
		Description: "A custom OpenAI- or Anthropic-compatible server.", Aliases: []string{"custom-endpoint"}, Category: CategoryCompatible,
		Auth: AuthAPIKey, Setup: SetupInteractive, Model: ModelDynamic,
		CredentialLabel: "API key", PromptBaseURL: true, BaseURLRequired: true, AllowManualModel: true,
		Settings:     []SettingField{{Key: "api_compat", Label: "API compatibility", Required: true, Default: "openai", Choices: []string{"openai", "anthropic"}}},
		SetupCommand: "ratchet provider add custom", InstallHint: "Provide a reachable compatible endpoint.",
		AuthHint:           "Enter an API key only when the endpoint requires one.",
		ModelBehavior:      "Lists endpoint models when supported; accepts a model ID manually.",
		CredentialBoundary: "Stores a supplied API credential through the daemon secrets provider.",
	},
	{
		Type: "openai_chatgpt", DisplayName: "OpenAI ChatGPT subscription",
		Description: "ChatGPT subscription access using OpenAI device authorization.", Aliases: []string{"openai-chatgpt", "chatgpt"}, Category: CategorySubscription,
		Auth: AuthOpenAIChatGPT, Setup: SetupInteractive, Model: ModelDynamic,
		CredentialLabel: "OpenAI OAuth token bundle", CredentialRequired: true, AllowManualModel: true,
		SetupAlias: "openai-chatgpt", SetupCommand: "ratchet provider setup openai-chatgpt", InstallHint: "No separate CLI required.",
		AuthHint:           "Uses OpenAI device-code auth or a Codex auth import.",
		ModelBehavior:      "Lists subscription models when available; accepts a model ID manually.",
		CredentialBoundary: "Stores the OAuth token bundle through the daemon secrets provider; token values are never printed.",
	},
	{
		Type: "copilot", DisplayName: "GitHub Copilot subscription",
		Description: "Copilot subscription models using GitHub device authorization.", Aliases: []string{"github-copilot"}, Category: CategorySubscription,
		Auth: AuthGitHubDevice, Setup: SetupInteractive, Model: ModelDynamic,
		CredentialLabel: "GitHub Copilot token", CredentialRequired: true,
		DefaultBaseURL: "https://api.githubcopilot.com", AllowManualModel: true,
		SetupCommand: "ratchet provider add copilot", InstallHint: "GitHub CLI is optional for token discovery.",
		AuthHint:           "Uses GitHub device authorization.",
		ModelBehavior:      "Lists Copilot models when available; accepts a model ID manually.",
		CredentialBoundary: "Stores the GitHub token through the daemon secrets provider.",
	},
	{
		Type: "openai_azure", DisplayName: "Azure OpenAI",
		Description: "Azure-hosted OpenAI deployments.", Aliases: []string{"openai-azure", "azure-openai"}, Category: CategoryCloud,
		Auth: AuthAPIKey, Setup: SetupInteractive, Model: ModelManual,
		APIKeyEnv: "AZURE_OPENAI_API_KEY", CredentialLabel: "Azure OpenAI API key", CredentialRequired: true, AllowManualModel: true,
		Settings: []SettingField{
			{Key: "resource", Label: "Azure resource", Required: true},
			{Key: "deployment_name", Label: "Deployment name", Required: true},
			{Key: "api_version", Label: "API version", Default: "2024-10-21"},
		},
		SetupCommand: "ratchet provider add openai_azure", InstallHint: "Create or select an Azure OpenAI deployment.",
		AuthHint:           "Uses an Azure OpenAI API key; Entra tokens are not stored in provider settings.",
		ModelBehavior:      "Uses the deployment model ID entered during setup.",
		CredentialBoundary: "Stores the API key through the daemon secrets provider; resource settings remain non-secret.",
	},
	{
		Type: "anthropic_foundry", DisplayName: "Anthropic on Azure AI Foundry",
		Description: "Claude models deployed through Azure AI Foundry.", Aliases: []string{"anthropic-foundry", "azure-anthropic"}, Category: CategoryCloud,
		Auth: AuthAPIKey, Setup: SetupInteractive, Model: ModelManual,
		CredentialLabel: "Azure AI Foundry API key", CredentialRequired: true, AllowManualModel: true,
		Settings:     []SettingField{{Key: "resource", Label: "Azure resource", Required: true}},
		SetupCommand: "ratchet provider add anthropic_foundry", InstallHint: "Create or select an Anthropic deployment in Azure AI Foundry.",
		AuthHint:           "Uses an API key; Entra tokens are not stored in provider settings.",
		ModelBehavior:      "Uses the deployed model ID entered during setup.",
		CredentialBoundary: "Stores the API key through the daemon secrets provider; resource settings remain non-secret.",
	},
	{
		Type: "anthropic_vertex", DisplayName: "Anthropic on Vertex AI",
		Description: "Claude models served through Google Vertex AI.", Aliases: []string{"anthropic-vertex", "vertex-anthropic"}, Category: CategoryCloud,
		Auth: AuthAPIKey, Setup: SetupInteractive, Model: ModelManual,
		CredentialLabel: "Google credentials JSON", CredentialRequired: true, AllowManualModel: true,
		Settings: []SettingField{
			{Key: "project_id", Label: "Google Cloud project ID", Required: true},
			{Key: "region", Label: "Google Cloud region", Required: true, Default: "us-east5"},
		},
		SetupCommand: "ratchet provider add anthropic_vertex", InstallHint: "Enable Vertex AI and Anthropic models for the selected project.",
		AuthHint:           "Paste service-account credentials JSON into the secret credential field.",
		ModelBehavior:      "Uses the Vertex model ID entered during setup.",
		CredentialBoundary: "Stores credentials JSON through the daemon secrets provider; project and region remain non-secret.",
	},
	{
		Type: "bedrock", DisplayName: "Amazon Bedrock",
		Description: "AWS Bedrock models, including Anthropic Claude.",
		Aliases:     []string{"anthropic_bedrock", "anthropic-bedrock", "aws-bedrock"}, Category: CategoryCloud,
		Auth: AuthAPIKey, Setup: SetupInteractive, Model: ModelDynamic,
		APIKeyEnv: "AWS_SECRET_ACCESS_KEY", CredentialLabel: "AWS secret access key", CredentialRequired: true, AllowManualModel: true,
		Settings: []SettingField{
			{Key: "access_key_id", Label: "AWS access key ID", Required: true},
			{Key: "region", Label: "AWS region", Required: true, Default: "us-east-1"},
		},
		SetupCommand: "ratchet provider add bedrock", InstallHint: "Enable the desired Bedrock models in the selected AWS region.",
		AuthHint:           "Uses an AWS access key ID and secret access key; session tokens are not stored in provider settings.",
		ModelBehavior:      "Lists Bedrock models for the configured region; accepts a model ID manually.",
		CredentialBoundary: "Stores the secret access key through the daemon secrets provider; access key ID and region remain non-secret.",
	},
	{
		Type: "ollama", DisplayName: "Ollama",
		Description: "Local models served by Ollama.", Aliases: []string{"ollama-local"}, Category: CategoryLocal,
		Auth: AuthNone, Setup: SetupOllama, Model: ModelOllama,
		PromptBaseURL: true, DefaultBaseURL: "http://localhost:11434", AllowManualModel: true,
		Settings:     []SettingField{{Key: "context_window", Label: "Context window", Placeholder: "optional"}},
		SetupCommand: "ratchet provider setup ollama", InstallHint: "Install and start Ollama locally.", AuthHint: "No API key required.",
		ModelBehavior:      "Lists or pulls local Ollama models; accepts a model name manually.",
		CredentialBoundary: "Uses the local Ollama endpoint and stores no secret value.",
	},
	{
		Type: "llama_cpp", DisplayName: "llama.cpp server",
		Description: "A local llama.cpp OpenAI-compatible server.", Aliases: []string{"llama-cpp"}, Category: CategoryLocal,
		Auth: AuthNone, Setup: SetupInteractive, Model: ModelDynamic,
		PromptBaseURL: true, DefaultBaseURL: "http://127.0.0.1:8080/v1", BaseURLRequired: true, AllowManualModel: true,
		SetupCommand: "ratchet provider add llama_cpp", InstallHint: "Start llama-server with its OpenAI-compatible endpoint.", AuthHint: "No API key required.",
		ModelBehavior:      "Lists endpoint models when supported; accepts a model ID manually.",
		CredentialBoundary: "Uses the local endpoint and stores no secret value.",
	},
	cliSetupEntry("claude_code", "Claude Code", "claude-code", "claude", "claude", "Claude Code owns credentials and model selection."),
	cliSetupEntry("copilot_cli", "GitHub Copilot CLI", "copilot-cli", "copilot", "copilot", "Copilot CLI owns credentials and model selection."),
	cliSetupEntry("codex_cli", "Codex CLI", "codex-cli", "codex", "codex", "Codex CLI owns credentials and model selection."),
	cliSetupEntry("gemini_cli", "Gemini CLI", "gemini-cli", "gemini", "gemini", "Gemini CLI owns credentials and model selection."),
	cliSetupEntry("cursor_cli", "Cursor CLI", "cursor-cli", "agent", "cursor-cli", "Cursor CLI owns credentials and model selection."),
}

func cliSetupEntry(providerType, displayName, setupAlias, defaultAlias, command, boundary string) SetupEntry {
	return SetupEntry{
		Type: providerType, DisplayName: displayName,
		Description: fmt.Sprintf("Use %s as the provider runtime.", displayName),
		Aliases:     []string{setupAlias}, Category: CategoryCLI,
		Auth: AuthCLINative, Setup: SetupCLINative, Model: ModelExternal,
		SetupAlias:         setupAlias,
		SetupCommand:       fmt.Sprintf("ratchet provider setup %s", setupAlias),
		DefaultAlias:       defaultAlias,
		CLICommand:         command,
		InstallHint:        fmt.Sprintf("Install %s and ensure `%s` is on PATH.", displayName, command),
		AuthHint:           fmt.Sprintf("Run %s's native login flow.", displayName),
		ModelBehavior:      fmt.Sprintf("Model selection remains owned by %s.", displayName),
		CredentialBoundary: boundary,
	}
}

// Catalog returns a defensive copy of all user-visible setup entries.
func Catalog() []SetupEntry {
	return cloneSetupEntries(setupCatalog)
}

// LookupSetup resolves a canonical provider type or accepted setup alias.
func LookupSetup(name string) (SetupEntry, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, entry := range setupCatalog {
		if name == entry.Type || slices.Contains(entry.Aliases, name) {
			return cloneSetupEntry(entry), true
		}
	}
	return SetupEntry{}, false
}

// ValidateCatalog checks catalog structure and coverage of runtime types.
func ValidateCatalog(runtimeTypes []string) error {
	return validateCatalog(setupCatalog, runtimeTypes)
}

func validateCatalog(entries []SetupEntry, runtimeTypes []string) error {
	seenNames := make(map[string]string)
	runtimeSet := make(map[string]struct{}, len(runtimeTypes))
	for _, providerType := range runtimeTypes {
		runtimeSet[providerType] = struct{}{}
	}

	for _, entry := range entries {
		if entry.Type == "" || entry.DisplayName == "" {
			return fmt.Errorf("provider type and display name are required")
		}
		if prior, exists := seenNames[entry.Type]; exists {
			return fmt.Errorf("duplicate provider type %q (already owned by %q)", entry.Type, prior)
		}
		seenNames[entry.Type] = entry.Type
		if !validCategory(entry.Category) {
			return fmt.Errorf("provider %q has unknown category %q", entry.Type, entry.Category)
		}
		if !validAuthStrategy(entry.Auth) {
			return fmt.Errorf("provider %q has unknown auth strategy %q", entry.Type, entry.Auth)
		}
		if !validSetupStrategy(entry.Setup) {
			return fmt.Errorf("provider %q has unknown setup strategy %q", entry.Type, entry.Setup)
		}
		if !validModelStrategy(entry.Model) {
			return fmt.Errorf("provider %q has unknown model strategy %q", entry.Type, entry.Model)
		}
		if entry.BaseURLRequired && !entry.PromptBaseURL {
			return fmt.Errorf("provider %q requires a base URL without a prompt", entry.Type)
		}
		if entry.SetupCommand == "" || entry.InstallHint == "" || entry.AuthHint == "" ||
			entry.ModelBehavior == "" || entry.CredentialBoundary == "" {
			return fmt.Errorf("provider %q has incomplete setup guide metadata", entry.Type)
		}
		if entry.Model == ModelExternal && (entry.Setup != SetupCLINative || entry.Auth != AuthCLINative || entry.AllowManualModel) {
			return fmt.Errorf("provider %q has inconsistent external CLI strategy", entry.Type)
		}
		if entry.Setup == SetupCLINative && (entry.SetupAlias == "" || entry.DefaultAlias == "" || entry.CLICommand == "") {
			return fmt.Errorf("provider %q has incomplete external CLI metadata", entry.Type)
		}
		for _, alias := range entry.Aliases {
			alias = strings.ToLower(strings.TrimSpace(alias))
			if alias == "" {
				return fmt.Errorf("provider %q has empty alias", entry.Type)
			}
			if prior, exists := seenNames[alias]; exists {
				return fmt.Errorf("duplicate provider name %q (owned by %q and %q)", alias, prior, entry.Type)
			}
			seenNames[alias] = entry.Type
		}
		seenSettings := make(map[string]struct{}, len(entry.Settings))
		for _, field := range entry.Settings {
			key := strings.ToLower(strings.TrimSpace(field.Key))
			if key == "" || field.Label == "" {
				return fmt.Errorf("provider %q has invalid setting field", entry.Type)
			}
			if isSecretSettingKey(key) {
				return fmt.Errorf("provider %q declares secret setting %q", entry.Type, key)
			}
			if _, exists := seenSettings[key]; exists {
				return fmt.Errorf("provider %q has duplicate setting %q", entry.Type, key)
			}
			seenSettings[key] = struct{}{}
		}
		if len(runtimeSet) > 0 {
			if _, exists := runtimeSet[entry.Type]; !exists {
				return fmt.Errorf("catalog provider %q is not registered at runtime", entry.Type)
			}
		}
	}

	for _, providerType := range runtimeTypes {
		if providerType == "mock" || providerType == "test" {
			continue
		}
		if _, exists := seenNames[providerType]; !exists {
			return fmt.Errorf("runtime provider %q is missing from setup catalog", providerType)
		}
	}
	return nil
}

func validCategory(category Category) bool {
	return category == CategoryAPI || category == CategoryCompatible || category == CategorySubscription ||
		category == CategoryCloud || category == CategoryLocal || category == CategoryCLI
}

func validAuthStrategy(strategy AuthStrategy) bool {
	return strategy == AuthAPIKey || strategy == AuthAnthropic || strategy == AuthGitHubDevice ||
		strategy == AuthOpenAIChatGPT || strategy == AuthNone || strategy == AuthCLINative
}

func validSetupStrategy(strategy SetupStrategy) bool {
	return strategy == SetupInteractive || strategy == SetupOllama || strategy == SetupCLINative
}

func validModelStrategy(strategy ModelStrategy) bool {
	return strategy == ModelDynamic || strategy == ModelManual || strategy == ModelOllama || strategy == ModelExternal
}

func isSecretSettingKey(key string) bool {
	if key == "api_key" || strings.Contains(key, "secret") || strings.Contains(key, "password") ||
		strings.Contains(key, "token") || strings.Contains(key, "credential") {
		return true
	}
	return false
}

func cloneSetupEntries(entries []SetupEntry) []SetupEntry {
	cloned := make([]SetupEntry, len(entries))
	for i, entry := range entries {
		cloned[i] = cloneSetupEntry(entry)
	}
	return cloned
}

func cloneSetupEntry(entry SetupEntry) SetupEntry {
	entry.Aliases = slices.Clone(entry.Aliases)
	entry.Settings = slices.Clone(entry.Settings)
	for i := range entry.Settings {
		entry.Settings[i].Choices = slices.Clone(entry.Settings[i].Choices)
	}
	return entry
}
