package pages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	providerauth "github.com/GoCodeAlone/ratchet-cli/internal/provider"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

func TestOnboardingUsesCompleteProviderCatalogWithoutLocalTable(t *testing.T) {
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	entries := providerauth.Catalog()
	if len(model.providers) != len(entries) {
		t.Fatalf("providers = %d, want %d", len(model.providers), len(entries))
	}
	for _, entry := range entries {
		if !slices.ContainsFunc(model.providers, func(candidate providerauth.SetupEntry) bool { return candidate.Type == entry.Type }) {
			t.Errorf("TUI providers missing %q", entry.Type)
		}
	}

	source, err := os.ReadFile("onboarding.go")
	if err != nil {
		t.Fatalf("read onboarding.go: %v", err)
	}
	for _, forbidden := range []string{"var providerTypes", "type providerTypeInfo", "removeProvider"} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("onboarding.go retains local provider definition %q", forbidden)
		}
	}
	if !strings.Contains(string(source), "providerauth.Catalog()") {
		t.Error("onboarding.go does not load the shared provider catalog")
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, "onboarding.go", source, 0)
	if err != nil {
		t.Fatalf("parse onboarding.go: %v", err)
	}
	for _, declaration := range parsed.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if ok && general.Tok == token.VAR {
			t.Errorf("onboarding.go declares package-owned variable state at %s", fset.Position(general.Pos()))
		}
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		array, ok := literal.Type.(*ast.ArrayType)
		if !ok {
			return true
		}
		selector, ok := array.Elt.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == "SetupEntry" {
			t.Errorf("onboarding.go declares a local SetupEntry table at %s", fset.Position(literal.Pos()))
		}
		return true
	})
}

func TestOnboardingEveryCatalogEntryReachesDeclaredFirstStep(t *testing.T) {
	for _, entry := range providerauth.Catalog() {
		t.Run(entry.Type, func(t *testing.T) {
			model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
			model.cursor = onboardingProviderIndex(t, model, entry.Type)
			model, cmd := model.advanceFromProvider()
			switch {
			case entry.Setup == providerauth.SetupCLINative:
				if model.step != stepCLISetup || cmd == nil {
					t.Fatalf("CLI setup step/cmd = %v, %v", model.step, cmd)
				}
			case entry.Auth == providerauth.AuthAnthropic:
				if model.step != stepAnthropicAuthChoice {
					t.Fatalf("step = %v, want Anthropic auth choice", model.step)
				}
			case entry.Auth == providerauth.AuthGitHubDevice || entry.Auth == providerauth.AuthOpenAIChatGPT:
				if model.step != stepBrowserAuth || cmd == nil {
					t.Fatalf("device setup step/cmd = %v, %v", model.step, cmd)
				}
			case entry.Auth == providerauth.AuthAPIKey:
				if model.step != stepEnterAPIKey {
					t.Fatalf("step = %v, want API key", model.step)
				}
			case entry.Type == "ollama":
				if model.step != stepOllamaChoice {
					t.Fatalf("step = %v, want Ollama choice", model.step)
				}
			case len(entry.Settings) > 0:
				if model.step != stepEnterSettings {
					t.Fatalf("step = %v, want settings", model.step)
				}
			case entry.PromptBaseURL:
				if model.step != stepEnterBaseURL {
					t.Fatalf("step = %v, want base URL", model.step)
				}
			default:
				if model.step != stepFetchModels && model.step != stepSelectModel {
					t.Fatalf("step = %v, want model flow", model.step)
				}
			}
		})
	}
}

func TestOnboardingProviderFilterEscClearsBeforeLeavingSelection(t *testing.T) {
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	model.filtering = true
	model.filterInput.SetValue("bedrock")
	if got := model.filteredProviderIndices(); len(got) != 1 || model.providers[got[0]].Type != "bedrock" {
		t.Fatalf("filtered providers = %v", got)
	}

	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if model.step != stepSelectProvider || model.filtering || model.filterInput.Value() != "" {
		t.Fatalf("filter Esc state = step:%v filtering:%v value:%q", model.step, model.filtering, model.filterInput.Value())
	}
}

func TestOnboardingProviderFilterEscClearsAfterApplyingFilter(t *testing.T) {
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	model.filtering = true
	model.filterInput.SetValue("bedrock")
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.filtering || model.filterInput.Value() != "bedrock" {
		t.Fatalf("applied filter state = filtering:%v value:%q", model.filtering, model.filterInput.Value())
	}

	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if model.step != stepSelectProvider || model.filterInput.Value() != "" {
		t.Fatalf("cleared filter state = step:%v value:%q", model.step, model.filterInput.Value())
	}
	model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if model.step != stepSelectProvider || cmd == nil {
		t.Fatalf("root Esc state = step:%v cmd:%v", model.step, cmd)
	}
	msg := runOnboardingCmd(t, cmd)
	if _, ok := msg.(OnboardingCancelledMsg); !ok {
		t.Fatalf("root Esc message = %T", msg)
	}
}

func TestOnboardingIgnoresStaleAsyncResultAfterLeavingFlow(t *testing.T) {
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	model.cursor = onboardingProviderIndex(t, model, "openai_chatgpt")
	model, _ = model.advanceFromProvider()
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if model.step != stepSelectProvider {
		t.Fatalf("cancelled step = %v", model.step)
	}

	model, cmd := model.Update(deviceCodeMsg{result: &providerauth.DeviceCodeResult{
		DeviceCode: "stale-device", UserCode: "STALE", VerificationURI: "https://auth.example/device", ExpiresIn: 60, Interval: 1,
	}})
	if cmd != nil || model.step != stepSelectProvider || model.deviceUserCode != "" {
		t.Fatalf("stale result applied = step:%v code:%q cmd:%v", model.step, model.deviceUserCode, cmd)
	}
	staleFlowID := model.flowID - 1
	model, cmd = model.Update(modelsListMsg{
		models: []providerauth.ModelInfo{{ID: "stale-model", Name: "Stale model"}},
		flowID: staleFlowID,
	})
	if cmd != nil || len(model.fetchedModels) != 0 || model.step != stepSelectProvider {
		t.Fatalf("stale model result applied = step:%v models:%v cmd:%v", model.step, model.fetchedModels, cmd)
	}
	model, cmd = model.Update(cliCheckMsg{path: "/stale/bin", workingDir: "/stale/work", flowID: staleFlowID})
	if cmd != nil || model.cliCommandPath != "" || model.step != stepSelectProvider {
		t.Fatalf("stale CLI result applied = step:%v path:%q cmd:%v", model.step, model.cliCommandPath, cmd)
	}
}

func TestOnboardingFilteredSelectionClearsFilterBeforeReturning(t *testing.T) {
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	model.filterInput.SetValue("bedrock")
	model.cursor = 0
	model, _ = model.advanceFromProvider()
	if model.step != stepEnterAPIKey {
		t.Fatalf("selected step = %v, want API key", model.step)
	}

	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if model.step != stepSelectProvider || model.filterInput.Value() != "" || model.cursor != onboardingProviderIndex(t, model, "bedrock") {
		t.Fatalf("returned selection = step:%v filter:%q cursor:%d", model.step, model.filterInput.Value(), model.cursor)
	}
}

func TestOnboardingBedrockSettingsReachModelDiscovery(t *testing.T) {
	var discoveredSettings map[string]string
	deps := testOnboardingDeps()
	deps.listModels = func(_ context.Context, providerType, apiKey, baseURL string, settings map[string]string) ([]providerauth.ModelInfo, error) {
		if providerType != "bedrock" || apiKey != "SECRET-SENTINEL" || baseURL != "" {
			t.Fatalf("discovery args = %q %q %q", providerType, apiKey, baseURL)
		}
		discoveredSettings = settings
		return []providerauth.ModelInfo{{ID: "anthropic.claude-test", Name: "Claude Test"}}, nil
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "bedrock")
	model.authToken = "SECRET-SENTINEL"
	model, _ = model.advanceAfterCredential()
	if model.step != stepEnterSettings {
		t.Fatalf("step = %v, want settings", model.step)
	}

	model.settingInput.SetValue("AKIAEXAMPLE")
	model, _ = model.updateEnterSettings(tea.KeyPressMsg{Code: tea.KeyEnter})
	model.settingInput.SetValue("us-west-2")
	model, cmd := model.updateEnterSettings(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.step != stepFetchModels || cmd == nil {
		t.Fatalf("step/cmd = %v, %v", model.step, cmd)
	}
	model, _ = model.Update(runOnboardingCmd(t, cmd))
	if discoveredSettings["access_key_id"] != "AKIAEXAMPLE" || discoveredSettings["region"] != "us-west-2" {
		t.Fatalf("discovered settings = %#v", discoveredSettings)
	}
	if model.step != stepSelectModel || len(model.fetchedModels) != 1 {
		t.Fatalf("model state = step:%v models:%v", model.step, model.fetchedModels)
	}
	model, _ = model.updateSelectModel(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.step != stepReview {
		t.Fatalf("review step = %v", model.step)
	}
}

func TestOnboardingManualModelFallbackPreservesSettings(t *testing.T) {
	deps := testOnboardingDeps()
	deps.listModels = func(context.Context, string, string, string, map[string]string) ([]providerauth.ModelInfo, error) {
		return nil, errors.New("listing unavailable")
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "custom")
	model.authToken = "secret"
	model.baseURLInput.SetValue("https://models.example/v1")
	model.settings = map[string]string{"api_compat": "anthropic"}
	model, cmd := model.transitionToModelStrategy()
	model, _ = model.Update(runOnboardingCmd(t, cmd))
	if model.step != stepSelectModel || !model.enteringManualModel {
		t.Fatalf("fallback state = step:%v manual:%v", model.step, model.enteringManualModel)
	}
	model.manualModelInput.SetValue("manual-model")
	model, _ = model.updateSelectModel(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.step != stepReview || model.selectedModelID() != "manual-model" {
		t.Fatalf("review state = step:%v model:%q", model.step, model.selectedModelID())
	}
	if model.settings["api_compat"] != "anthropic" || model.baseURLInput.Value() != "https://models.example/v1" {
		t.Fatalf("setup state lost: settings=%v base=%q", model.settings, model.baseURLInput.Value())
	}
}

func TestOnboardingEmptyModelDiscoveryOffersManualEntry(t *testing.T) {
	deps := testOnboardingDeps()
	deps.listModels = func(context.Context, string, string, string, map[string]string) ([]providerauth.ModelInfo, error) {
		return nil, nil
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "openai")
	model.authToken = "secret"
	model.baseURLInput.SetValue("https://api.openai.com/v1")
	model, cmd := model.transitionToModelStrategy()
	model, _ = model.Update(runOnboardingCmd(t, cmd))
	if model.step != stepSelectModel || !model.enteringManualModel {
		t.Fatalf("empty discovery state = step:%v manual:%v", model.step, model.enteringManualModel)
	}
}

func TestOnboardingModelDiscoveryEscCancelsRequest(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	deps := testOnboardingDeps()
	deps.listModels = func(ctx context.Context, _, _, _ string, _ map[string]string) ([]providerauth.ModelInfo, error) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		return nil, ctx.Err()
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "openai")
	model, cmd := model.transitionToModelStrategy()
	batch := cmd().(tea.BatchMsg)
	go batch[len(batch)-1]() //nolint:errcheck
	<-started
	model, _ = model.updateFetchModels(tea.KeyPressMsg{Code: tea.KeyEsc})
	<-cancelled
	if model.step != stepSelectProvider || model.modelFetchCancel != nil {
		t.Fatalf("cancelled model fetch = step:%v cancel:%v", model.step, model.modelFetchCancel)
	}
}

func TestOnboardingChatGPTDeviceFlowTransitionsToModelDiscovery(t *testing.T) {
	deps := testOnboardingDeps()
	deps.startOpenAIDevice = func(context.Context) (*providerauth.DeviceCodeResult, error) {
		return &providerauth.DeviceCodeResult{DeviceCode: "device-id", UserCode: "USER-CODE", VerificationURI: "https://auth.openai.com/device", ExpiresIn: 60, Interval: 1}, nil
	}
	deps.pollOpenAIDevice = func(context.Context, string, string, int) (string, error) {
		return "CHATGPT-TOKEN-BUNDLE", nil
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.cursor = onboardingProviderIndex(t, model, "openai_chatgpt")
	model, cmd := model.advanceFromProvider()
	model, cmd = model.Update(runOnboardingCmd(t, cmd))
	if model.deviceUserCode != "USER-CODE" || cmd == nil {
		t.Fatalf("device state = code:%q cmd:%v", model.deviceUserCode, cmd)
	}
	model, cmd = model.Update(runOnboardingCmd(t, cmd))
	if model.authToken != "CHATGPT-TOKEN-BUNDLE" || model.step != stepFetchModels || cmd == nil {
		t.Fatalf("auth state = token:%q step:%v cmd:%v", model.authToken, model.step, cmd)
	}
	model, _ = model.Update(runOnboardingCmd(t, cmd))
	model, _ = model.updateSelectModel(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.step != stepReview {
		t.Fatalf("review step = %v", model.step)
	}
}

func TestOnboardingDeviceStartFailureUsesProviderCompatibleRecovery(t *testing.T) {
	chatGPTDeps := testOnboardingDeps()
	chatGPTDeps.startOpenAIDevice = func(context.Context) (*providerauth.DeviceCodeResult, error) {
		return nil, errors.New("device authorization unavailable")
	}
	model := newOnboarding(nil, theme.Dark(), chatGPTDeps)
	model.cursor = onboardingProviderIndex(t, model, "openai_chatgpt")
	model, cmd := model.advanceFromProvider()
	model, _ = model.Update(runOnboardingCmd(t, cmd))
	if model.step != stepBrowserAuth || model.authing || model.authError == "" {
		t.Fatalf("ChatGPT recovery = step:%v authing:%v error:%q", model.step, model.authing, model.authError)
	}
	model, cmd = model.updateBrowserAuth(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if model.step != stepBrowserAuth || !model.authing || cmd == nil {
		t.Fatalf("ChatGPT retry = step:%v authing:%v cmd:%v", model.step, model.authing, cmd)
	}

	githubDeps := testOnboardingDeps()
	githubDeps.startGitHubDevice = func(context.Context) (*providerauth.DeviceCodeResult, error) {
		return nil, errors.New("GitHub device authorization unavailable")
	}
	model = newOnboarding(nil, theme.Dark(), githubDeps)
	model.cursor = onboardingProviderIndex(t, model, "copilot")
	model, cmd = model.advanceFromProvider()
	model, _ = model.Update(runOnboardingCmd(t, cmd))
	if model.step != stepEnterAPIKey || model.apiKeyInput.Placeholder != "ghp_..." {
		t.Fatalf("GitHub recovery = step:%v placeholder:%q", model.step, model.apiKeyInput.Placeholder)
	}
}

func TestOnboardingDeviceStartEscCancelsRequest(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	deps := testOnboardingDeps()
	deps.startOpenAIDevice = func(ctx context.Context) (*providerauth.DeviceCodeResult, error) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		return nil, ctx.Err()
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.cursor = onboardingProviderIndex(t, model, "openai_chatgpt")
	model, cmd := model.advanceFromProvider()
	batch := cmd().(tea.BatchMsg)
	go batch[len(batch)-1]() //nolint:errcheck
	<-started
	model, _ = model.updateBrowserAuth(tea.KeyPressMsg{Code: tea.KeyEsc})
	<-cancelled
	if model.step != stepSelectProvider || model.authCancel != nil {
		t.Fatalf("cancelled device start = step:%v cancel:%v", model.step, model.authCancel)
	}
}

func TestOnboardingDeviceRetryClearsExpiredCode(t *testing.T) {
	deps := testOnboardingDeps()
	deps.pollOpenAIDevice = func(context.Context, string, string, int) (string, error) {
		return "", errors.New("authorization expired")
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.cursor = onboardingProviderIndex(t, model, "openai_chatgpt")
	model, cmd := model.advanceFromProvider()
	model, cmd = model.Update(runOnboardingCmd(t, cmd))
	if model.deviceUserCode == "" {
		t.Fatal("device flow did not expose initial code")
	}
	model, _ = model.Update(runOnboardingCmd(t, cmd))
	model, cmd = model.updateBrowserAuth(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil || model.deviceUserCode != "" || model.deviceVerificationURI != "" {
		t.Fatalf("retry state = code:%q URI:%q cmd:%v", model.deviceUserCode, model.deviceVerificationURI, cmd)
	}
}

func TestOnboardingAnthropicBrowserAuthUsesInjectedDependency(t *testing.T) {
	var started bool
	deps := testOnboardingDeps()
	deps.startAnthropic = func(context.Context) (string, error) {
		started = true
		return "ANTHROPIC-OAUTH-TOKEN", nil
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.cursor = onboardingProviderIndex(t, model, "anthropic")
	model, _ = model.advanceFromProvider()
	model.cursor = int(anthropicChoiceConsoleOAuth)
	model, cmd := model.updateAnthropicAuthChoice(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, cmd = model.Update(runOnboardingCmd(t, cmd))
	if !started || model.authToken != "ANTHROPIC-OAUTH-TOKEN" || model.step != stepFetchModels || cmd == nil {
		t.Fatalf("Anthropic auth = started:%v token:%q step:%v cmd:%v", started, model.authToken, model.step, cmd)
	}
	model, _ = model.Update(runOnboardingCmd(t, cmd))
	model, _ = model.updateSelectModel(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.step != stepReview {
		t.Fatalf("review step = %v", model.step)
	}
}

func TestOnboardingCopilotDeviceStrategyReachesReview(t *testing.T) {
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	model.cursor = onboardingProviderIndex(t, model, "copilot")
	model, cmd := model.advanceFromProvider()
	model, cmd = model.Update(runOnboardingCmd(t, cmd))
	model, cmd = model.Update(runOnboardingCmd(t, cmd))
	if model.authToken != "github-token" || model.step != stepFetchModels || cmd == nil {
		t.Fatalf("Copilot auth = token:%q step:%v cmd:%v", model.authToken, model.step, cmd)
	}
	model, _ = model.Update(runOnboardingCmd(t, cmd))
	model, _ = model.updateSelectModel(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.step != stepReview {
		t.Fatalf("review step = %v", model.step)
	}
}

func TestOnboardingOllamaExistingServerStrategyReachesReview(t *testing.T) {
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	model.cursor = onboardingProviderIndex(t, model, "ollama")
	model, _ = model.advanceFromProvider()
	model.ollamaChoiceCursor = 0
	model, _ = model.updateOllamaChoice(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.step != stepEnterSettings {
		t.Fatalf("settings step = %v", model.step)
	}
	model, _ = model.updateEnterSettings(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.step != stepEnterBaseURL {
		t.Fatalf("base URL step = %v", model.step)
	}
	model, cmd := model.updateEnterBaseURL(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, _ = model.Update(runOnboardingCmd(t, cmd))
	model, _ = model.updateSelectModel(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.step != stepReview {
		t.Fatalf("review step = %v", model.step)
	}
}

func TestOnboardingOllamaSetupEscCancelsOperation(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	deps := testOnboardingDeps()
	deps.setupOllama = func(ctx context.Context, _ string) (string, error) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		return "failed", ctx.Err()
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "ollama")
	model, cmd := model.beginOllamaSetup()
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) < 2 {
		t.Fatalf("setup command = %T", cmd())
	}
	go batch[len(batch)-1]() //nolint:errcheck
	<-started
	model, _ = model.updateOllamaSetup(tea.KeyPressMsg{Code: tea.KeyEsc})
	<-cancelled
	if model.step != stepOllamaChoice || model.ollamaSetupCancel != nil {
		t.Fatalf("cancelled setup = step:%v cancel:%v", model.step, model.ollamaSetupCancel)
	}
}

func TestOnboardingManualModelStrategyReachesReview(t *testing.T) {
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	model.providerIdx = onboardingProviderIndex(t, model, "openai_azure")
	model.authToken = "azure-secret"
	model, _ = model.advanceAfterCredential()
	for _, value := range []string{"resource", "deployment", "2024-10-21"} {
		model.settingInput.SetValue(value)
		model, _ = model.updateEnterSettings(tea.KeyPressMsg{Code: tea.KeyEnter})
	}
	if model.step != stepSelectModel || !model.enteringManualModel {
		t.Fatalf("manual model state = step:%v entering:%v", model.step, model.enteringManualModel)
	}
	model.manualModelInput.SetValue("deployment-model")
	model, _ = model.updateSelectModel(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.step != stepReview {
		t.Fatalf("review step = %v", model.step)
	}
}

func TestOnboardingCLINativeProviderUsesResolvedCommand(t *testing.T) {
	var healthChecked bool
	deps := testOnboardingDeps()
	deps.lookPath = func(command string) (string, error) {
		if command != "codex" {
			t.Fatalf("lookPath command = %q", command)
		}
		return "/test/bin/codex", nil
	}
	deps.checkCLI = func(_ context.Context, providerType, path string) error {
		healthChecked = true
		if providerType != "codex_cli" || path != "/test/bin/codex" {
			t.Fatalf("health check args = %q %q", providerType, path)
		}
		return nil
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.cursor = onboardingProviderIndex(t, model, "codex_cli")
	model, cmd := model.advanceFromProvider()
	model, _ = model.Update(runOnboardingCmd(t, cmd))
	if model.step != stepReview || model.cliCommandPath != "/test/bin/codex" || !healthChecked {
		t.Fatalf("CLI state = step:%v path:%q", model.step, model.cliCommandPath)
	}
}

func TestOnboardingCLICheckEscCancelsProcess(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	deps := testOnboardingDeps()
	deps.checkCLI = func(ctx context.Context, _, _ string) error {
		close(started)
		<-ctx.Done()
		close(cancelled)
		return ctx.Err()
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.cursor = onboardingProviderIndex(t, model, "codex_cli")
	model, cmd := model.advanceFromProvider()
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) < 2 {
		t.Fatalf("CLI check command = %T", cmd())
	}
	go batch[len(batch)-1]() //nolint:errcheck
	<-started
	model, _ = model.updateCLISetup(tea.KeyPressMsg{Code: tea.KeyEsc})
	<-cancelled
	if model.step != stepSelectProvider || model.cliCheckCancel != nil {
		t.Fatalf("cancelled CLI check = step:%v cancel:%v", model.step, model.cliCheckCancel)
	}
}

func TestOnboardingCLINativeSubmitUsesCatalogAliasCommandAndWorkingDirectory(t *testing.T) {
	var request *pb.AddProviderReq
	deps := testOnboardingDeps()
	deps.lookPath = func(command string) (string, error) {
		if command != "agent" {
			t.Fatalf("lookPath command = %q, want agent", command)
		}
		return "/test/bin/agent", nil
	}
	deps.workingDir = func() (string, error) { return "/test/workspace", nil }
	deps.addProvider = func(_ context.Context, req *pb.AddProviderReq) (*pb.Provider, error) {
		request = req
		return &pb.Provider{Alias: req.Alias, Type: req.Type}, nil
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.cursor = onboardingProviderIndex(t, model, "cursor_cli")
	model, cmd := model.advanceFromProvider()
	model, _ = model.Update(runOnboardingCmd(t, cmd))
	if model.step != stepReview || model.cliCommandPath != "/test/bin/agent" {
		t.Fatalf("CLI review state = step:%v command:%q", model.step, model.cliCommandPath)
	}
	model, cmd = model.updateReview(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = runOnboardingCmd(t, cmd)
	if request == nil || request.Alias != "cursor-cli" || request.Type != "cursor_cli" || request.BaseUrl != "/test/workspace" {
		t.Fatalf("CLI add request = %+v", request)
	}
}

func TestOnboardingReviewSuppressesSecretAndSubmitsSettingsOnce(t *testing.T) {
	const secret = "SECRET-REVIEW-SENTINEL"
	var requests []*pb.AddProviderReq
	deps := testOnboardingDeps()
	deps.addProvider = func(_ context.Context, req *pb.AddProviderReq) (*pb.Provider, error) {
		requests = append(requests, req)
		return &pb.Provider{Alias: req.Alias, Type: req.Type, Model: req.Model}, nil
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "bedrock")
	model.step = stepReview
	model.authToken = secret
	model.settings = map[string]string{"access_key_id": "AKIAEXAMPLE", "region": "us-west-2"}
	model.selectedModel = "anthropic.claude-test"
	view := model.View(theme.Dark(), 100, 36)
	if strings.Contains(view, secret) || !strings.Contains(view, "us-west-2") || !strings.Contains(view, "Credential configured") {
		t.Fatalf("review view leaked or omitted state:\n%s", view)
	}

	model, cmd := model.updateReview(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.step != stepTestConnection || cmd == nil {
		t.Fatalf("submit state = step:%v cmd:%v", model.step, cmd)
	}
	msg := runOnboardingCmd(t, cmd)
	if _, ok := msg.(providerAddedMsg); !ok {
		t.Fatalf("submit message = %T", msg)
	}
	if len(requests) != 1 {
		t.Fatalf("AddProvider calls = %d", len(requests))
	}
	var settings map[string]string
	if err := json.Unmarshal([]byte(requests[0].Settings), &settings); err != nil {
		t.Fatalf("settings JSON: %v", err)
	}
	if requests[0].ApiKey != secret || settings["region"] != "us-west-2" || strings.Contains(requests[0].Settings, secret) {
		t.Fatalf("request = %+v settings=%v", requests[0], settings)
	}
}

func TestOnboardingProviderBoundaryErrorsRedactCredential(t *testing.T) {
	const secret = "SECRET-PROVIDER-BOUNDARY-SENTINEL"
	for _, tc := range []struct {
		name string
		deps func() onboardingDeps
	}{
		{"add error", func() onboardingDeps {
			deps := testOnboardingDeps()
			deps.addProvider = func(context.Context, *pb.AddProviderReq) (*pb.Provider, error) {
				return nil, errors.New("add echoed " + secret)
			}
			return deps
		}},
		{"test result", func() onboardingDeps {
			deps := testOnboardingDeps()
			deps.testProvider = func(context.Context, string) (*pb.TestProviderResult, error) {
				return &pb.TestProviderResult{Success: false, Message: "test echoed " + secret}, nil
			}
			return deps
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := newOnboarding(nil, theme.Dark(), tc.deps())
			model.providerIdx = onboardingProviderIndex(t, model, "bedrock")
			model.authToken = secret
			model.selectedModel = "test-model"
			model, cmd := model.startTest()
			model, cmd = model.Update(runOnboardingCmd(t, cmd))
			if cmd != nil {
				model, _ = model.Update(runOnboardingCmd(t, cmd))
			}
			view := model.View(theme.Dark(), 80, 24)
			if strings.Contains(model.testError, secret) || strings.Contains(view, secret) {
				t.Fatalf("provider error leaked credential: error=%q\n%s", model.testError, view)
			}
			if !strings.Contains(model.testError, "REDACTED") {
				t.Fatalf("provider error lacks redaction marker: %q", model.testError)
			}
		})
	}
}

func TestOnboardingProviderListFitsCommonTerminalWidths(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 24}, {120, 40}} {
		for cursor := range providerauth.Catalog() {
			model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
			model.cursor = cursor
			view := model.View(theme.Dark(), size.width, size.height)
			lines := strings.Split(view, "\n")
			if len(lines) > size.height {
				t.Errorf("%dx%d cursor %d rendered %d lines", size.width, size.height, cursor, len(lines))
			}
			for lineNo, line := range lines {
				if got := lipgloss.Width(line); got > size.width {
					t.Errorf("%dx%d cursor %d line %d width = %d: %q", size.width, size.height, cursor, lineNo+1, got, line)
				}
			}
			if !strings.Contains(view, "filter") {
				t.Errorf("%dx%d cursor %d view missing navigation", size.width, size.height, cursor)
			}
		}
		model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
		model.cursor = len(model.providers) - 1
		view := model.View(theme.Dark(), size.width, size.height)
		if !strings.Contains(view, "filter") || !strings.Contains(view, "External CLI agents") || !strings.Contains(view, "Cursor CLI") {
			t.Errorf("%dx%d view missing navigation/selection:\n%s", size.width, size.height, view)
		}
	}
}

func TestOnboardingFilteredProviderListFitsShortTerminal(t *testing.T) {
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	model.filtering = true
	model.filterInput.SetValue("i")
	indices := model.filteredProviderIndices()
	for cursor := range indices {
		model.cursor = cursor
		view := model.View(theme.Dark(), 80, 24)
		if lines := len(strings.Split(view, "\n")); lines > 24 {
			t.Errorf("filtered cursor %d rendered %d lines", cursor, lines)
		}
		if !strings.Contains(view, "Esc:") || !strings.Contains(view, "filter") {
			t.Errorf("filtered cursor %d missing filter recovery", cursor)
		}
	}
}

func TestOnboardingProviderHelpMatchesFilterState(t *testing.T) {
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	view := model.View(theme.Dark(), 80, 24)
	if !strings.Contains(view, "Esc: cancel") || strings.Contains(view, "Esc: clear filter") {
		t.Fatalf("root provider help is inaccurate:\n%s", view)
	}
	model.filterInput.SetValue("bedrock")
	view = model.View(theme.Dark(), 80, 24)
	if !strings.Contains(view, "Esc: clear") || !strings.Contains(view, "filter") {
		t.Fatalf("filtered provider help is inaccurate:\n%s", view)
	}
}

func TestOnboardingReviewWrapsLongValuesWithinTerminalWidth(t *testing.T) {
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	model.providerIdx = onboardingProviderIndex(t, model, "custom")
	model.selectedModel = strings.Repeat("model-segment-", 10)
	model.baseURLInput.SetValue("https://" + strings.Repeat("endpoint-segment-", 8) + ".example/v1")
	model.settings = map[string]string{"api_compat": strings.Repeat("compatibility-", 10)}
	longError := strings.Repeat("unbroken-error-segment-", 10)

	for _, state := range []struct {
		name  string
		apply func(*OnboardingModel)
	}{
		{"review", func(model *OnboardingModel) { model.step = stepReview }},
		{"manual model error", func(model *OnboardingModel) {
			model.step = stepSelectModel
			model.enteringManualModel = true
			model.modelsError = longError
		}},
		{"CLI error", func(model *OnboardingModel) {
			model.providerIdx = onboardingProviderIndex(t, *model, "codex_cli")
			model.step = stepCLISetup
			model.cliError = longError
		}},
		{"discovered model", func(model *OnboardingModel) {
			model.step = stepSelectModel
			model.enteringManualModel = false
			model.fetchedModels = []providerauth.ModelInfo{{ID: longError, Name: longError}}
			model.modelCursor = 0
		}},
		{"successful test", func(model *OnboardingModel) {
			model.step = stepTestConnection
			model.testResult = &pb.TestProviderResult{Success: true}
			model.selectedModel = longError
		}},
	} {
		t.Run(state.name, func(t *testing.T) {
			stateModel := model
			state.apply(&stateModel)
			view := stateModel.View(theme.Dark(), 80, 24)
			for lineNo, line := range strings.Split(view, "\n") {
				if got := lipgloss.Width(line); got > 80 {
					t.Errorf("line %d width = %d: %q", lineNo+1, got, line)
				}
			}
			if (state.name == "discovered model" || state.name == "successful test") && !strings.Contains(view, "...") {
				t.Errorf("long model value lacks an explicit truncation marker:\n%s", view)
			}
		})
	}
}

func TestOnboardingPullAndCLICheckViewsBoundStatusAndShowRecovery(t *testing.T) {
	longValue := strings.Repeat("unbroken-status-segment-", 10)
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	model.providerIdx = onboardingProviderIndex(t, model, "ollama")
	model.step = stepPullModel
	model.recommendedModels = recommendedOllamaModels()
	model.pullingModel = true
	model.pullModelName = longValue
	view := model.View(theme.Dark(), 80, 24)
	if !strings.Contains(view, "...") {
		t.Fatalf("pull progress lacks truncation marker:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if width := lipgloss.Width(line); width > 80 {
			t.Fatalf("pull progress width = %d: %q", width, line)
		}
	}

	model.providerIdx = onboardingProviderIndex(t, model, "codex_cli")
	model.step = stepCLISetup
	model.pullingModel = false
	model.cliError = ""
	view = model.View(theme.Dark(), 80, 24)
	if !strings.Contains(view, "Esc: cancel") {
		t.Fatalf("CLI check view missing recovery:\n%s", view)
	}

	model.step = stepTestConnection
	model.testing = true
	view = model.View(theme.Dark(), 80, 24)
	if !strings.Contains(view, "Esc: cancel") {
		t.Fatalf("provider test view missing recovery:\n%s", view)
	}
}

func TestOnboardingReviewEscReturnsToModelSelection(t *testing.T) {
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	model.providerIdx = onboardingProviderIndex(t, model, "bedrock")
	model.step = stepReview
	model.selectedModel = "anthropic.claude-test"
	model.settings = map[string]string{"region": "us-west-2"}

	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if model.step != stepSelectModel || model.selectedModel != "anthropic.claude-test" || model.settings["region"] != "us-west-2" {
		t.Fatalf("review back state = step:%v model:%q settings:%v", model.step, model.selectedModel, model.settings)
	}
}

func TestOnboardingCLINativeTestFailureBackReturnsToReview(t *testing.T) {
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	model.providerIdx = onboardingProviderIndex(t, model, "codex_cli")
	model.step = stepTestConnection
	model.cliCommandPath = "/test/bin/codex"
	model.testError = "connection failed"

	model, _ = model.updateTestConnection(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if model.step != stepReview || model.cliCommandPath != "/test/bin/codex" {
		t.Fatalf("CLI test back state = step:%v path:%q", model.step, model.cliCommandPath)
	}
}

func TestOnboardingTestFailureBackKeepsSavedProvider(t *testing.T) {
	deps := testOnboardingDeps()
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "bedrock")
	model.step = stepTestConnection
	model.added = true
	model.testError = "connection failed"

	model, cmd := model.updateTestConnection(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if cmd != nil || model.step != stepReview || !model.added {
		t.Fatalf("back state = cmd:%v step:%v added:%v", cmd, model.step, model.added)
	}
}

func TestOnboardingCancelReportsPreviouslySavedProvider(t *testing.T) {
	saved := &pb.Provider{Alias: "bedrock", Type: "bedrock", BaseUrl: "https://example.invalid", IsDefault: true}
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	model.step = stepSelectProvider
	model.savedProvider = saved

	_, cmd := model.updateSelectProvider(tea.KeyPressMsg{Code: tea.KeyEsc})
	msg, ok := runOnboardingCmd(t, cmd).(OnboardingCancelledMsg)
	if !ok || msg.Provider != saved {
		t.Fatalf("cancel message = %#v", msg)
	}
}

func TestOnboardingProviderTestEscCancelsOperation(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	deps := testOnboardingDeps()
	deps.testProvider = func(ctx context.Context, _ string) (*pb.TestProviderResult, error) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		return nil, ctx.Err()
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "bedrock")
	model.selectedModel = "test-model"
	model, cmd := model.startTest()
	batch := cmd().(tea.BatchMsg)
	model, cmd = model.Update(batch[len(batch)-1]())
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	<-started
	model, _ = model.updateTestConnection(tea.KeyPressMsg{Code: tea.KeyEsc})
	<-cancelled
	model, _ = model.Update(<-result)
	if model.testing || model.providerOpCancel != nil || !strings.Contains(model.testError, "canceled") {
		t.Fatalf("cancelled test = testing:%v cancel:%v error:%q", model.testing, model.providerOpCancel, model.testError)
	}
}

func TestOnboardingAddSuccessRacingEscKeepsCommittedProviderAndSkipsTest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	testCalls := 0
	deps := testOnboardingDeps()
	deps.addProvider = func(_ context.Context, req *pb.AddProviderReq) (*pb.Provider, error) {
		close(started)
		<-release
		return &pb.Provider{Alias: req.Alias, Type: req.Type}, nil
	}
	deps.testProvider = func(context.Context, string) (*pb.TestProviderResult, error) {
		testCalls++
		return &pb.TestProviderResult{Success: true}, nil
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "bedrock")
	model.selectedModel = "test-model"
	model, cmd := model.startTest()
	batch := cmd().(tea.BatchMsg)
	result := make(chan tea.Msg, 1)
	go func() { result <- batch[len(batch)-1]() }()
	<-started
	model, _ = model.updateTestConnection(tea.KeyPressMsg{Code: tea.KeyEsc})
	view := model.View(theme.Dark(), 80, 24)
	if !strings.Contains(view, "finish after save") || strings.Contains(strings.ToLower(view), "remove") {
		t.Fatalf("cancel-after-save view:\n%s", view)
	}
	close(release)
	model, cmd = model.Update(<-result)
	if cmd == nil || !model.added || model.testing || testCalls != 0 {
		t.Fatalf("raced add state = cmd:%v added:%v testing:%v tests:%d", cmd, model.added, model.testing, testCalls)
	}
	done, ok := runOnboardingCmd(t, cmd).(OnboardingDoneMsg)
	if !ok || done.Provider.GetAlias() != "bedrock" {
		t.Fatalf("cancel-after-save message = %#v", done)
	}
}

func TestOnboardingProviderAddHasDeadline(t *testing.T) {
	deps := testOnboardingDeps()
	deps.addProvider = func(ctx context.Context, _ *pb.AddProviderReq) (*pb.Provider, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			return nil, errors.New("provider add context has no deadline")
		}
		if remaining := time.Until(deadline); remaining <= 0 || remaining > 31*time.Second {
			return nil, fmt.Errorf("provider add deadline = %s", remaining)
		}
		return &pb.Provider{Alias: "bedrock", Type: "bedrock"}, nil
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "bedrock")
	model.selectedModel = "test-model"
	_, cmd := model.startTest()
	msg := runOnboardingCmd(t, cmd).(providerAddedMsg)
	if msg.err != nil {
		t.Fatalf("provider add: %v", msg.err)
	}
}

func TestOnboardingAmbiguousAddReconcilesMatchingOperation(t *testing.T) {
	var operationID string
	deps := testOnboardingDeps()
	deps.addProvider = func(_ context.Context, req *pb.AddProviderReq) (*pb.Provider, error) {
		operationID = req.OperationId
		return nil, status.Error(codes.Unavailable, "response lost")
	}
	deps.listProviders = func(context.Context) ([]*pb.Provider, error) {
		return []*pb.Provider{{Alias: "bedrock", Type: "bedrock", Model: "test-model", IsDefault: true, OperationId: operationID}}, nil
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "bedrock")
	model.selectedModel = "test-model"

	model, cmd := model.startTest()
	model, cmd = model.Update(runOnboardingCmd(t, cmd))
	if cmd == nil {
		t.Fatal("ambiguous add did not start reconciliation")
	}
	model, cmd = model.Update(runOnboardingCmd(t, cmd))
	if model.savedProvider == nil || model.savedProvider.OperationId != operationID || cmd == nil {
		t.Fatalf("reconciled add = provider:%v operation:%q cmd:%v", model.savedProvider, operationID, cmd)
	}
}

func TestOnboardingAmbiguousAddRejectsPriorAliasOperation(t *testing.T) {
	deps := testOnboardingDeps()
	deps.addProvider = func(context.Context, *pb.AddProviderReq) (*pb.Provider, error) {
		return nil, status.Error(codes.DeadlineExceeded, "response lost")
	}
	deps.listProviders = func(context.Context) ([]*pb.Provider, error) {
		return []*pb.Provider{{Alias: "bedrock", Type: "bedrock", OperationId: "prior-operation"}}, nil
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "bedrock")
	model.selectedModel = "test-model"

	model, cmd := model.startTest()
	model, cmd = model.Update(runOnboardingCmd(t, cmd))
	model, cmd = model.Update(runOnboardingCmd(t, cmd))
	if cmd != nil || model.savedProvider != nil || !strings.Contains(model.testError, "response lost") {
		t.Fatalf("mismatched reconciliation = provider:%v error:%q cmd:%v", model.savedProvider, model.testError, cmd)
	}
}

func TestOnboardingReconciliationFailurePausesRequestedQuit(t *testing.T) {
	deps := testOnboardingDeps()
	deps.addProvider = func(context.Context, *pb.AddProviderReq) (*pb.Provider, error) {
		return nil, status.Error(codes.Unavailable, "response lost")
	}
	deps.listProviders = func(context.Context) ([]*pb.Provider, error) {
		return nil, errors.New("daemon unavailable")
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "bedrock")
	model.selectedModel = "test-model"
	model, cmd := model.startTest()
	if !model.RequestQuit() {
		t.Fatal("quit did not wait for in-flight add")
	}
	model, cmd = model.Update(runOnboardingCmd(t, cmd))
	model, cmd = model.Update(runOnboardingCmd(t, cmd))
	if cmd != nil || model.quitAfterAdd || !strings.Contains(model.testError, "save result unknown") {
		t.Fatalf("failed reconciliation = cmd:%v quit:%v error:%q", cmd, model.quitAfterAdd, model.testError)
	}
}

func TestOnboardingQuitWaitsForInFlightAddResult(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	deps := testOnboardingDeps()
	deps.addProvider = func(_ context.Context, req *pb.AddProviderReq) (*pb.Provider, error) {
		close(started)
		<-release
		return &pb.Provider{Alias: req.Alias, Type: req.Type, OperationId: req.OperationId}, nil
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "bedrock")
	model.selectedModel = "test-model"
	model, cmd := model.startTest()
	batch := cmd().(tea.BatchMsg)
	result := make(chan tea.Msg, 1)
	go func() { result <- batch[len(batch)-1]() }()
	<-started
	if !model.RequestQuit() {
		t.Fatal("quit did not wait for in-flight add")
	}
	close(release)
	model, cmd = model.Update(<-result)
	msg, ok := runOnboardingCmd(t, cmd).(OnboardingQuitMsg)
	if !ok || msg.Provider.GetOperationId() == "" {
		t.Fatalf("quit message = %#v", msg)
	}
}

func TestOnboardingAuthRejectsEmptySuccessResponses(t *testing.T) {
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	model.providerIdx = onboardingProviderIndex(t, model, "openai_chatgpt")
	model.step = stepBrowserAuth
	model.authing = true

	model, cmd := model.Update(browserAuthResultMsg{flowID: model.flowID})
	if cmd != nil || model.authToken != "" || model.step != stepBrowserAuth || model.authError == "" {
		t.Fatalf("empty auth result = step:%v token:%q error:%q cmd:%v", model.step, model.authToken, model.authError, cmd)
	}

	model.authing = true
	model, cmd = model.Update(deviceCodeMsg{flowID: model.flowID})
	if model.authing || model.step != stepBrowserAuth || model.authError == "" || cmd != nil {
		t.Fatalf("nil device result = step:%v authing:%v error:%q cmd:%v", model.step, model.authing, model.authError, cmd)
	}
}

func TestOnboardingRequiredBaseURLIsTrimmedBeforeDiscovery(t *testing.T) {
	var discoveredBaseURL string
	deps := testOnboardingDeps()
	deps.listModels = func(_ context.Context, _, _, baseURL string, _ map[string]string) ([]providerauth.ModelInfo, error) {
		discoveredBaseURL = baseURL
		return []providerauth.ModelInfo{{ID: "test-model", Name: "Test model"}}, nil
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "custom")
	model.step = stepEnterBaseURL
	model.baseURLInput.SetValue("  https://models.example/v1  ")

	model, cmd := model.updateEnterBaseURL(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, _ = model.Update(runOnboardingCmd(t, cmd))
	if discoveredBaseURL != "https://models.example/v1" || model.baseURLInput.Value() != discoveredBaseURL {
		t.Fatalf("normalized base URL = input:%q discovery:%q", model.baseURLInput.Value(), discoveredBaseURL)
	}

	model.step = stepEnterBaseURL
	model.baseURLInput.SetValue("   ")
	model, cmd = model.updateEnterBaseURL(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || model.step != stepEnterBaseURL {
		t.Fatalf("blank required base URL advanced = step:%v cmd:%v", model.step, cmd)
	}
}

func TestOnboardingSubmissionTrimsBaseURL(t *testing.T) {
	var request *pb.AddProviderReq
	deps := testOnboardingDeps()
	deps.addProvider = func(_ context.Context, req *pb.AddProviderReq) (*pb.Provider, error) {
		request = req
		return &pb.Provider{Alias: req.Alias, Type: req.Type}, nil
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "custom")
	model.selectedModel = "test-model"
	model.baseURLInput.SetValue("  https://models.example/v1  ")

	_, cmd := model.startTest()
	_ = runOnboardingCmd(t, cmd)
	if request == nil || request.BaseUrl != "https://models.example/v1" {
		t.Fatalf("submitted base URL = %#v", request)
	}
}

func TestOnboardingProviderTestHasDeadline(t *testing.T) {
	deps := testOnboardingDeps()
	deps.testProvider = func(ctx context.Context, _ string) (*pb.TestProviderResult, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			return nil, errors.New("provider test context has no deadline")
		}
		if remaining := time.Until(deadline); remaining <= 0 || remaining > 31*time.Second {
			return nil, fmt.Errorf("provider test deadline = %s", remaining)
		}
		return &pb.TestProviderResult{Success: true}, nil
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "bedrock")
	model.step = stepTestConnection
	model.added = true

	model, cmd := model.updateTestConnection(tea.KeyPressMsg{Code: 'r', Text: "r"})
	model, _ = model.Update(runOnboardingCmd(t, cmd))
	if model.testError != "" || model.testResult == nil || !model.testResult.Success {
		t.Fatalf("provider test = result:%v error:%q", model.testResult, model.testError)
	}
}

func TestOnboardingRejectsNilSuccessfulProviderResponses(t *testing.T) {
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	model.step = stepTestConnection
	model.testing = true
	model.adding = true
	model.providerOpContext = t.Context()

	model, cmd := model.Update(providerAddedMsg{flowID: model.flowID})
	if cmd != nil || model.testing || model.adding || !strings.Contains(model.testError, "no provider") {
		t.Fatalf("nil add response = cmd:%v testing:%v adding:%v error:%q", cmd, model.testing, model.adding, model.testError)
	}

	model.testing = true
	model, cmd = model.Update(providerTestedMsg{flowID: model.flowID})
	if cmd != nil || model.testing || !strings.Contains(model.testError, "no result") {
		t.Fatalf("nil test response = cmd:%v testing:%v error:%q", cmd, model.testing, model.testError)
	}
}

func TestOnboardingCancelStopsAllActiveWork(t *testing.T) {
	model := newOnboarding(nil, theme.Dark(), testOnboardingDeps())
	contexts := make([]context.Context, 0, 6)
	newCancel := func() context.CancelFunc {
		ctx, cancel := context.WithCancel(t.Context())
		contexts = append(contexts, ctx)
		return cancel
	}
	model.authCancel = newCancel()
	model.modelFetchCancel = newCancel()
	model.ollamaSetupCancel = newCancel()
	model.pullCancel = newCancel()
	model.cliCheckCancel = newCancel()
	model.providerOpCancel = newCancel()

	model.Cancel()

	for i, ctx := range contexts {
		select {
		case <-ctx.Done():
		default:
			t.Errorf("active context %d was not canceled", i)
		}
	}
}

func TestWaitForBackgroundProcessReapsChild(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestOnboardingBackgroundProcessHelper$")
	cmd.Env = append(os.Environ(), "RATCHET_ONBOARDING_PROCESS_HELPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	if err := <-waitForBackgroundProcess(cmd); err != nil {
		t.Fatalf("wait helper: %v", err)
	}
	if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
		t.Fatalf("helper process was not reaped: %#v", cmd.ProcessState)
	}
}

func TestOnboardingBackgroundProcessHelper(t *testing.T) {
	if os.Getenv("RATCHET_ONBOARDING_PROCESS_HELPER") != "1" {
		return
	}
}

func testOnboardingDeps() onboardingDeps {
	return onboardingDeps{
		listProviders: func(context.Context) ([]*pb.Provider, error) { return nil, nil },
		listModels: func(context.Context, string, string, string, map[string]string) ([]providerauth.ModelInfo, error) {
			return []providerauth.ModelInfo{{ID: "test-model", Name: "Test model"}}, nil
		},
		addProvider: func(_ context.Context, req *pb.AddProviderReq) (*pb.Provider, error) {
			return &pb.Provider{Alias: req.Alias, Type: req.Type, Model: req.Model}, nil
		},
		testProvider: func(context.Context, string) (*pb.TestProviderResult, error) {
			return &pb.TestProviderResult{Success: true}, nil
		},
		startGitHubDevice: func(context.Context) (*providerauth.DeviceCodeResult, error) {
			return &providerauth.DeviceCodeResult{DeviceCode: "github-device", UserCode: "GITHUB-CODE", VerificationURI: "https://github.com/login/device", ExpiresIn: 60, Interval: 1}, nil
		},
		pollGitHubDevice: func(context.Context, string, int) (string, error) { return "github-token", nil },
		startOpenAIDevice: func(context.Context) (*providerauth.DeviceCodeResult, error) {
			return &providerauth.DeviceCodeResult{DeviceCode: "openai-device", UserCode: "OPENAI-CODE", VerificationURI: "https://auth.openai.com/device", ExpiresIn: 60, Interval: 1}, nil
		},
		pollOpenAIDevice: func(context.Context, string, string, int) (string, error) { return "openai-token", nil },
		startAnthropic:   func(context.Context) (string, error) { return "anthropic-token", nil },
		startAnthropicMax: func(context.Context) (string, error) {
			return "anthropic-max-token", nil
		},
		lookPath:   func(command string) (string, error) { return "/test/bin/" + command, nil },
		checkCLI:   func(context.Context, string, string) error { return nil },
		workingDir: func() (string, error) { return "/test/workspace", nil },
		setupOllama: func(context.Context, string) (string, error) {
			return "ready", nil
		},
	}
}

func runOnboardingCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected command")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return msg
	}
	var result tea.Msg
	for _, child := range batch {
		if child != nil {
			result = runOnboardingCmd(t, child)
		}
	}
	if result == nil {
		t.Fatal("batch produced no messages")
	}
	return result
}

func onboardingProviderIndex(t *testing.T, model OnboardingModel, providerType string) int {
	t.Helper()
	index := slices.IndexFunc(model.providers, func(entry providerauth.SetupEntry) bool { return entry.Type == providerType })
	if index < 0 {
		t.Fatalf("provider %q not found", providerType)
	}
	return index
}
