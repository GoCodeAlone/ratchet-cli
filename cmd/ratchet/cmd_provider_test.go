package main

import (
	"bufio"
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	wfprovider "github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

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

func TestPromptProviderModelSelectionDefaultsToFirstEnumeratedModel(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("\n"))
	model, err := promptProviderModelSelection(
		context.Background(),
		"openai",
		"api-key",
		"",
		scanner,
		&strings.Builder{},
		func(context.Context, string, string, string) ([]wfprovider.ModelInfo, error) {
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
		scanner,
		&strings.Builder{},
		func(context.Context, string, string, string) ([]wfprovider.ModelInfo, error) {
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
		scanner,
		&out,
		func(context.Context, string, string, string) ([]wfprovider.ModelInfo, error) {
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
