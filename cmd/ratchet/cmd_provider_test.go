package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	providerauth "github.com/GoCodeAlone/ratchet-cli/internal/provider"
	wfprovider "github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

func TestProviderSetupListOutputsKnownGuides(t *testing.T) {
	out := captureStdout(t, func() {
		handleProvider([]string{"setup", "list"})
	})
	for _, want := range []string{"ALIAS", "openai-chatgpt", "codex-cli", "ratchet provider setup openai-chatgpt"} {
		if !strings.Contains(out, want) {
			t.Fatalf("setup list output missing %q:\n%s", want, out)
		}
	}
}

func TestProviderSetupListJSON(t *testing.T) {
	out := captureStdout(t, func() {
		handleProvider([]string{"setup", "list", "--json"})
	})
	var rows []providerSetupGuide
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("unmarshal setup list: %v\n%s", err, out)
	}
	if len(rows) != len(providerauth.Catalog()) {
		t.Fatalf("guide count = %d, want %d", len(rows), len(providerauth.Catalog()))
	}
	if rows[0].Alias == "" || rows[0].SetupCommand == "" || rows[0].CredentialBoundary == "" {
		t.Fatalf("first guide missing fields: %+v", rows[0])
	}
	for _, entry := range providerauth.Catalog() {
		if !slices.ContainsFunc(rows, func(row providerSetupGuide) bool { return row.ProviderType == entry.Type }) {
			t.Errorf("setup list missing catalog provider %q", entry.Type)
		}
	}
}

func TestProviderSetupGuideJSON(t *testing.T) {
	out := captureStdout(t, func() {
		handleProvider([]string{"setup", "guide", "openai-chatgpt", "--json"})
	})
	var guide providerSetupGuide
	if err := json.Unmarshal([]byte(out), &guide); err != nil {
		t.Fatalf("unmarshal setup guide: %v\n%s", err, out)
	}
	if guide.Alias != "openai-chatgpt" {
		t.Fatalf("alias = %q", guide.Alias)
	}
	if !strings.Contains(guide.AuthHint, "device") {
		t.Fatalf("auth hint = %q", guide.AuthHint)
	}
}

func TestProviderSetupListJSONPropagatesEncodeError(t *testing.T) {
	err := printProviderSetupGuideList([]string{"--json"}, errWriter{})
	if err == nil {
		t.Fatal("expected encode error")
	}
}

func TestProviderSetupGuideJSONPropagatesEncodeError(t *testing.T) {
	err := printProviderSetupGuide([]string{"openai-chatgpt", "--json"}, errWriter{}, io.Discard)
	if err == nil {
		t.Fatal("expected encode error")
	}
}

func TestProviderSetupGuideUnknownAlias(t *testing.T) {
	out := captureStderr(t, func() {
		handleProvider([]string{"setup", "guide", "missing"})
	})
	if !strings.Contains(out, "unknown provider setup guide") {
		t.Fatalf("stderr = %q", out)
	}
}

func TestProviderSetupGuideResolvesRuntimeAliasToCanonicalEntry(t *testing.T) {
	var out strings.Builder
	if err := printProviderSetupGuide([]string{"anthropic_bedrock", "--json"}, &out, io.Discard); err != nil {
		t.Fatalf("printProviderSetupGuide: %v", err)
	}
	var guide providerSetupGuide
	if err := json.Unmarshal([]byte(out.String()), &guide); err != nil {
		t.Fatalf("unmarshal guide: %v", err)
	}
	if guide.Alias != "bedrock" || guide.ProviderType != "bedrock" {
		t.Fatalf("guide = %+v", guide)
	}
}

func TestProviderSetupGuideResolvesEveryCatalogName(t *testing.T) {
	for _, entry := range providerauth.Catalog() {
		names := append([]string{entry.Type}, entry.Aliases...)
		for _, name := range names {
			t.Run(name, func(t *testing.T) {
				var out strings.Builder
				if err := printProviderSetupGuide([]string{name, "--json"}, &out, io.Discard); err != nil {
					t.Fatalf("printProviderSetupGuide: %v", err)
				}
				var guide providerSetupGuide
				if err := json.Unmarshal([]byte(out.String()), &guide); err != nil {
					t.Fatalf("unmarshal guide: %v", err)
				}
				if guide.ProviderType != entry.Type {
					t.Fatalf("guide.ProviderType = %q, want %q", guide.ProviderType, entry.Type)
				}
			})
		}
	}
}

func TestProviderDedicatedSetupDispatchComesFromCatalog(t *testing.T) {
	for _, providerType := range []string{"openai_chatgpt", "claude_code", "copilot_cli", "codex_cli", "gemini_cli", "cursor_cli"} {
		entry, ok := providerauth.LookupSetup(providerType)
		if !ok {
			t.Fatalf("LookupSetup(%q) not found", providerType)
		}
		if !requiresDedicatedProviderSetup(entry) {
			t.Errorf("requiresDedicatedProviderSetup(%q) = false", providerType)
		}
		if entry.SetupCommand == "" {
			t.Errorf("provider %q has no setup command", providerType)
		}
	}
	for _, providerType := range []string{"anthropic", "copilot", "bedrock", "ollama", "custom"} {
		entry, _ := providerauth.LookupSetup(providerType)
		if requiresDedicatedProviderSetup(entry) {
			t.Errorf("requiresDedicatedProviderSetup(%q) = true", providerType)
		}
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() {
		os.Stderr = old
		r.Close()
	}()

	fn()

	w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String()
}

func TestPromptYesNo_Yes(t *testing.T) {
	for _, input := range []string{"y\n", "Y\n", "yes\n", "YES\n", "\n"} {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.WriteString(input); err != nil {
			t.Fatal(err)
		}
		w.Close()

		scanner := bufio.NewScanner(r)
		got := promptYesNo("test?", scanner)
		r.Close()

		if !got {
			t.Errorf("promptYesNo(%q) = false, want true", strings.TrimRight(input, "\n"))
		}
	}
}

func TestPromptYesNo_No(t *testing.T) {
	for _, input := range []string{"n\n", "N\n", "no\n", "NO\n"} {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.WriteString(input); err != nil {
			t.Fatal(err)
		}
		w.Close()

		scanner := bufio.NewScanner(r)
		got := promptYesNo("test?", scanner)
		r.Close()

		if got {
			t.Errorf("promptYesNo(%q) = true, want false", strings.TrimRight(input, "\n"))
		}
	}
}

func TestPromptYesNo_EOF(t *testing.T) {
	// When stdin is closed (EOF), promptYesNo should default to false.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close() // immediate EOF

	scanner := bufio.NewScanner(r)
	got := promptYesNo("test?", scanner)
	r.Close()

	if got {
		t.Error("promptYesNo on EOF should return false, got true")
	}
}

func TestParseOpenAIChatGPTSetupArgs(t *testing.T) {
	got := parseOpenAIChatGPTSetupArgs([]string{"--model", "gpt-5-codex", "--from-codex", "/tmp/auth.json", "--no-browser"})
	if got.model != "gpt-5-codex" {
		t.Fatalf("model = %q", got.model)
	}
	if !got.modelSet {
		t.Fatal("modelSet = false")
	}
	if got.fromCodex != "/tmp/auth.json" {
		t.Fatalf("fromCodex = %q", got.fromCodex)
	}
	if !got.noBrowser {
		t.Fatal("noBrowser = false")
	}
}

func TestParseOpenAIChatGPTSetupArgsDefaults(t *testing.T) {
	got := parseOpenAIChatGPTSetupArgs([]string{"--from-codex"})
	if got.model != "" {
		t.Fatalf("model = %q", got.model)
	}
	if got.modelSet {
		t.Fatal("modelSet = true")
	}
	if got.fromCodex == "" || !strings.HasSuffix(got.fromCodex, ".codex/auth.json") {
		t.Fatalf("fromCodex = %q", got.fromCodex)
	}
}

func TestParseOpenAIChatGPTSetupArgsEmptyModelPrompts(t *testing.T) {
	got := parseOpenAIChatGPTSetupArgs([]string{"--model", "  "})
	if got.model != "" {
		t.Fatalf("model = %q", got.model)
	}
	if got.modelSet {
		t.Fatal("modelSet = true")
	}
}

func TestParseProviderModelFlagEmptyModelPrompts(t *testing.T) {
	model, modelSet := parseProviderModelFlag([]string{"add", "openai", "alias", "--model", "  "})
	if model != "" {
		t.Fatalf("model = %q", model)
	}
	if modelSet {
		t.Fatal("modelSet = true")
	}
}

func TestParseProviderModelFlagSet(t *testing.T) {
	model, modelSet := parseProviderModelFlag([]string{"add", "openai", "alias", "--model", "gpt-5.5"})
	if model != "gpt-5.5" {
		t.Fatalf("model = %q", model)
	}
	if !modelSet {
		t.Fatal("modelSet = false")
	}
}

func TestOpenAIChatGPTAddProviderReq(t *testing.T) {
	req := openAIChatGPTAddProviderReq("gpt-5-codex", `{"access_token":"token","refresh_token":"refresh"}`, true)
	if req.Alias != "openai-chatgpt" {
		t.Fatalf("Alias = %q", req.Alias)
	}
	if req.Type != "openai_chatgpt" {
		t.Fatalf("Type = %q", req.Type)
	}
	if req.Model != "gpt-5-codex" {
		t.Fatalf("Model = %q", req.Model)
	}
	if req.ApiKey == "" {
		t.Fatal("ApiKey token bundle is empty")
	}
	if !req.IsDefault {
		t.Fatal("IsDefault = false")
	}
}

func TestProviderSettingsJSON(t *testing.T) {
	settingsJSON, err := providerSettingsJSON(map[string]string{
		"region":        "us-west-2",
		"access_key_id": "AKIAEXAMPLE",
		"session_token": "",
	})
	if err != nil {
		t.Fatalf("providerSettingsJSON: %v", err)
	}
	var settings map[string]string
	if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
		t.Fatalf("settings JSON is invalid: %v", err)
	}
	if settings["region"] != "us-west-2" {
		t.Fatalf("region = %q", settings["region"])
	}
	if settings["access_key_id"] != "AKIAEXAMPLE" {
		t.Fatalf("access_key_id = %q", settings["access_key_id"])
	}
	if _, ok := settings["session_token"]; ok {
		t.Fatalf("empty session_token should be omitted: %#v", settings)
	}
}

func TestCollectProviderSetupInputUsesBedrockSettingsForDiscovery(t *testing.T) {
	entry, ok := providerauth.LookupSetup("anthropic_bedrock")
	if !ok {
		t.Fatal("bedrock setup entry not found")
	}
	scanner := bufio.NewScanner(strings.NewReader("AKIAEXAMPLE\nus-west-2\n\n"))
	var out strings.Builder
	input, err := collectProviderSetupInput(t.Context(), entry, "", false, scanner, &out, providerSetupDeps{
		promptSecret: func(label string) (string, error) {
			if label != "AWS secret access key" {
				t.Fatalf("secret label = %q", label)
			}
			return "SECRET-SENTINEL", nil
		},
		listModels: func(_ context.Context, providerType, apiKey, baseURL string, settings map[string]string) ([]wfprovider.ModelInfo, error) {
			if providerType != "bedrock" || apiKey != "SECRET-SENTINEL" || baseURL != "" {
				t.Fatalf("discovery args = %q %q %q", providerType, apiKey, baseURL)
			}
			if settings["access_key_id"] != "AKIAEXAMPLE" || settings["region"] != "us-west-2" {
				t.Fatalf("settings = %#v", settings)
			}
			return []wfprovider.ModelInfo{{ID: "anthropic.claude-test", Name: "Claude Test"}}, nil
		},
	})
	if err != nil {
		t.Fatalf("collectProviderSetupInput: %v", err)
	}
	if input.APIKey != "SECRET-SENTINEL" || input.Model != "anthropic.claude-test" {
		t.Fatalf("input = %+v", input)
	}
	if strings.Contains(out.String(), "SECRET-SENTINEL") {
		t.Fatalf("output leaked credential: %q", out.String())
	}
	settingsJSON, err := providerSettingsJSON(input.Settings)
	if err != nil {
		t.Fatalf("providerSettingsJSON: %v", err)
	}
	if strings.Contains(settingsJSON, "SECRET-SENTINEL") {
		t.Fatalf("settings leaked credential: %s", settingsJSON)
	}
}

func TestCollectProviderSetupInputCustomPassesCompatibilityAndFallsBackToManualModel(t *testing.T) {
	entry, ok := providerauth.LookupSetup("custom-endpoint")
	if !ok {
		t.Fatal("custom setup entry not found")
	}
	scanner := bufio.NewScanner(strings.NewReader("2\nhttps://models.example/v1\nmanual-model\n"))
	input, err := collectProviderSetupInput(t.Context(), entry, "", false, scanner, io.Discard, providerSetupDeps{
		promptSecret: func(string) (string, error) { return "", nil },
		promptBaseURL: func(defaultURL string) (string, error) {
			return promptProviderBaseURL(scanner, io.Discard, defaultURL)
		},
		listModels: func(_ context.Context, providerType, apiKey, baseURL string, settings map[string]string) ([]wfprovider.ModelInfo, error) {
			if providerType != "custom" || apiKey != "" || baseURL != "https://models.example/v1" {
				t.Fatalf("discovery args = %q %q %q", providerType, apiKey, baseURL)
			}
			if settings["api_compat"] != "anthropic" {
				t.Fatalf("settings = %#v", settings)
			}
			return nil, errors.New("listing unsupported")
		},
	})
	if err != nil {
		t.Fatalf("collectProviderSetupInput: %v", err)
	}
	if input.Model != "manual-model" || input.Settings["api_compat"] != "anthropic" {
		t.Fatalf("input = %+v", input)
	}
}

func TestCollectProviderSetupInputManualCloudModel(t *testing.T) {
	entry, ok := providerauth.LookupSetup("azure-openai")
	if !ok {
		t.Fatal("Azure OpenAI setup entry not found")
	}
	scanner := bufio.NewScanner(strings.NewReader("resource-a\ndeployment-a\n\nmodel-a\n"))
	input, err := collectProviderSetupInput(t.Context(), entry, "", false, scanner, io.Discard, providerSetupDeps{
		promptSecret: func(string) (string, error) { return "secret", nil },
		listModels: func(context.Context, string, string, string, map[string]string) ([]wfprovider.ModelInfo, error) {
			t.Fatal("manual provider must not list models")
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("collectProviderSetupInput: %v", err)
	}
	if input.Model != "model-a" || input.Settings["resource"] != "resource-a" ||
		input.Settings["deployment_name"] != "deployment-a" || input.Settings["api_version"] != "2024-10-21" {
		t.Fatalf("input = %+v", input)
	}
}

func TestCollectProviderSetupInputOtherManualCloudSettings(t *testing.T) {
	tests := []struct {
		provider string
		input    string
		want     map[string]string
	}{
		{provider: "anthropic_foundry", input: "foundry-resource\nfoundry-model\n", want: map[string]string{"resource": "foundry-resource"}},
		{provider: "anthropic_vertex", input: "gcp-project\n\nvertex-model\n", want: map[string]string{"project_id": "gcp-project", "region": "us-east5"}},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			entry, ok := providerauth.LookupSetup(tt.provider)
			if !ok {
				t.Fatalf("LookupSetup(%q) not found", tt.provider)
			}
			result, err := collectProviderSetupInput(t.Context(), entry, "", false, bufio.NewScanner(strings.NewReader(tt.input)), io.Discard, providerSetupDeps{
				promptSecret: func(string) (string, error) { return "secret", nil },
			})
			if err != nil {
				t.Fatalf("collectProviderSetupInput: %v", err)
			}
			for key, want := range tt.want {
				if got := result.Settings[key]; got != want {
					t.Errorf("settings[%q] = %q, want %q", key, got, want)
				}
			}
			if result.Model == "" {
				t.Error("manual model is empty")
			}
		})
	}
}

func TestCollectProviderSetupInputUsesGitHubDeviceFlow(t *testing.T) {
	entry, ok := providerauth.LookupSetup("copilot")
	if !ok {
		t.Fatal("Copilot setup entry not found")
	}
	scanner := bufio.NewScanner(strings.NewReader("\n"))
	input, err := collectProviderSetupInput(t.Context(), entry, "", false, scanner, io.Discard, providerSetupDeps{
		deviceFlow: func(context.Context) (string, error) { return "copilot-token", nil },
		listModels: func(_ context.Context, providerType, apiKey, _ string, _ map[string]string) ([]wfprovider.ModelInfo, error) {
			if providerType != "copilot" || apiKey != "copilot-token" {
				t.Fatalf("discovery args = %q %q", providerType, apiKey)
			}
			return []wfprovider.ModelInfo{{ID: "gpt-copilot"}}, nil
		},
	})
	if err != nil {
		t.Fatalf("collectProviderSetupInput: %v", err)
	}
	if input.APIKey != "copilot-token" || input.Model != "gpt-copilot" {
		t.Fatalf("input = %+v", input)
	}
}

func TestPromptProviderModelSelectionDefaultsToFirstEnumeratedModel(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("\n"))
	model, err := promptProviderModelSelection(
		context.Background(),
		"openai",
		"api-key",
		"",
		map[string]string{"region": "us-east-1"},
		scanner,
		&strings.Builder{},
		func(_ context.Context, providerType, apiKey, baseURL string, settings map[string]string) ([]wfprovider.ModelInfo, error) {
			if providerType != "openai" || apiKey != "api-key" || baseURL != "" {
				t.Fatalf("unexpected lister args: %q %q %q", providerType, apiKey, baseURL)
			}
			if settings["region"] != "us-east-1" {
				t.Fatalf("settings not passed to lister: %#v", settings)
			}
			return []wfprovider.ModelInfo{
				{ID: "gpt-5.5", Name: "GPT-5.5"},
				{ID: "gpt-5.4-mini", Name: "GPT-5.4-Mini"},
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("promptProviderModelSelection: %v", err)
	}
	if model != "gpt-5.5" {
		t.Fatalf("model = %q", model)
	}
}

func TestPromptProviderModelSelectionSupportsCustomModel(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("3\ncustom-model\n"))
	model, err := promptProviderModelSelection(
		context.Background(),
		"openai",
		"api-key",
		"",
		nil,
		scanner,
		&strings.Builder{},
		func(context.Context, string, string, string, map[string]string) ([]wfprovider.ModelInfo, error) {
			return []wfprovider.ModelInfo{
				{ID: "gpt-5.5", Name: "GPT-5.5"},
				{ID: "gpt-5.4-mini", Name: "GPT-5.4-Mini"},
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("promptProviderModelSelection: %v", err)
	}
	if model != "custom-model" {
		t.Fatalf("model = %q", model)
	}
}

func TestPromptProviderModelSelectionPromptsManualWhenEnumerationFails(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("manual-after-error\n"))
	var out strings.Builder
	model, err := promptProviderModelSelection(
		context.Background(),
		"anthropic_bedrock",
		"",
		"",
		nil,
		scanner,
		&out,
		func(context.Context, string, string, string, map[string]string) ([]wfprovider.ModelInfo, error) {
			return nil, errors.New("no dynamic catalog")
		},
	)
	if err != nil {
		t.Fatalf("promptProviderModelSelection: %v", err)
	}
	if model != "manual-after-error" {
		t.Fatalf("model = %q", model)
	}
	if !strings.Contains(out.String(), "could not list models") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestPromptProviderModelSelectionRedactsCredentialFromEnumerationError(t *testing.T) {
	const credential = "SECRET-PROVIDER-CREDENTIAL"
	scanner := bufio.NewScanner(strings.NewReader("manual-after-error\n"))
	var out strings.Builder
	model, err := promptProviderModelSelection(
		t.Context(), "custom", credential, "https://models.example/v1", nil,
		scanner, &out,
		func(context.Context, string, string, string, map[string]string) ([]wfprovider.ModelInfo, error) {
			return nil, errors.New("request rejected for " + credential)
		},
	)
	if err != nil {
		t.Fatalf("promptProviderModelSelection: %v", err)
	}
	if model != "manual-after-error" {
		t.Fatalf("model = %q", model)
	}
	if strings.Contains(out.String(), credential) {
		t.Fatalf("output leaked credential: %q", out.String())
	}
}

type fakeCompatibleDaemon struct {
	resp   *pb.VersionCheckResp
	closed bool
}

func (f *fakeCompatibleDaemon) EnsureCompatible() (*pb.VersionCheckResp, error) {
	return f.resp, nil
}

func (f *fakeCompatibleDaemon) Close() error {
	f.closed = true
	return nil
}

func TestEnsureCompatibleConnectedDaemonReloadsVersionMismatch(t *testing.T) {
	oldClient := &fakeCompatibleDaemon{resp: &pb.VersionCheckResp{Compatible: true, ReloadRecommended: true, Message: "version mismatch"}}
	newClient := &fakeCompatibleDaemon{resp: &pb.VersionCheckResp{Compatible: true}}
	connects := 0
	reloads := 0

	got, err := ensureCompatibleConnectedDaemon(
		func() (*fakeCompatibleDaemon, error) {
			connects++
			if connects == 1 {
				return oldClient, nil
			}
			return newClient, nil
		},
		func() error {
			reloads++
			return nil
		},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("ensureCompatibleConnectedDaemon: %v", err)
	}
	if got != newClient {
		t.Fatal("expected reconnected daemon client")
	}
	if connects != 2 || reloads != 1 || !oldClient.closed {
		t.Fatalf("connects=%d reloads=%d oldClosed=%v", connects, reloads, oldClient.closed)
	}
}

func TestEnsureCompatibleConnectedDaemonKeepsExistingDaemonWhenReloadFails(t *testing.T) {
	oldClient := &fakeCompatibleDaemon{resp: &pb.VersionCheckResp{Compatible: true, ReloadRecommended: true, Message: "version mismatch"}}
	connects := 0

	got, err := ensureCompatibleConnectedDaemon(
		func() (*fakeCompatibleDaemon, error) {
			connects++
			return oldClient, nil
		},
		func() error {
			return errors.New("reload denied")
		},
		&strings.Builder{},
	)
	if err != nil {
		t.Fatalf("ensureCompatibleConnectedDaemon: %v", err)
	}
	if got != oldClient {
		t.Fatal("expected existing daemon client")
	}
	if connects != 1 || oldClient.closed {
		t.Fatalf("connects=%d oldClosed=%v", connects, oldClient.closed)
	}
}

func TestOllamaInstallCommand_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only test")
	}
	cmd, err := ollamaInstallCommand()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Args[0] != "brew" {
		t.Errorf("expected brew command on darwin, got: %v", cmd.Args)
	}
}

func TestOllamaInstallCommand_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only test")
	}
	cmd, err := ollamaInstallCommand()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Args[0] != "sh" {
		t.Errorf("expected sh command on linux, got: %v", cmd.Args)
	}
	// Verify it downloads to temp file instead of piping to sh
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "mktemp") {
		t.Errorf("expected mktemp-based download, got: %s", joined)
	}
}

func TestOllamaInstallCommand_UnsupportedPlatform(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		t.Skip("only runs on unsupported platforms")
	}
	_, err := ollamaInstallCommand()
	if err == nil {
		t.Error("expected error on unsupported platform")
	}
	if !strings.Contains(err.Error(), "not supported on") {
		t.Errorf("expected 'not supported' error, got: %v", err)
	}
}
