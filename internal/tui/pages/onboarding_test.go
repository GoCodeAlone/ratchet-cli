package pages

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	for _, forbidden := range []string{"var providerTypes", "type providerTypeInfo"} {
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

func TestOnboardingTestFailureWaitsForProviderRemovalBeforeReview(t *testing.T) {
	var removedAlias string
	deps := testOnboardingDeps()
	deps.removeProvider = func(_ context.Context, alias string) error {
		removedAlias = alias
		return nil
	}
	model := newOnboarding(nil, theme.Dark(), deps)
	model.providerIdx = onboardingProviderIndex(t, model, "bedrock")
	model.step = stepTestConnection
	model.added = true
	model.testError = "connection failed"

	model, cmd := model.updateTestConnection(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if cmd == nil || model.step != stepTestConnection || !model.removing {
		t.Fatalf("removal start = step:%v removing:%v cmd:%v", model.step, model.removing, cmd)
	}
	model, _ = model.Update(runOnboardingCmd(t, cmd))
	if removedAlias != "bedrock" || model.step != stepReview || model.removing || model.added {
		t.Fatalf("removal result = alias:%q step:%v removing:%v added:%v", removedAlias, model.step, model.removing, model.added)
	}
}

func testOnboardingDeps() onboardingDeps {
	return onboardingDeps{
		listModels: func(context.Context, string, string, string, map[string]string) ([]providerauth.ModelInfo, error) {
			return []providerauth.ModelInfo{{ID: "test-model", Name: "Test model"}}, nil
		},
		addProvider: func(_ context.Context, req *pb.AddProviderReq) (*pb.Provider, error) {
			return &pb.Provider{Alias: req.Alias, Type: req.Type, Model: req.Model}, nil
		},
		removeProvider: func(context.Context, string) error { return nil },
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
