package main

import (
	"bufio"
	"os"
	"runtime"
	"strings"
	"testing"
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
	if got.fromCodex != "/tmp/auth.json" {
		t.Fatalf("fromCodex = %q", got.fromCodex)
	}
	if !got.noBrowser {
		t.Fatal("noBrowser = false")
	}
}

func TestParseOpenAIChatGPTSetupArgsDefaults(t *testing.T) {
	got := parseOpenAIChatGPTSetupArgs([]string{"--from-codex"})
	if got.model != "gpt-5-codex" {
		t.Fatalf("model = %q", got.model)
	}
	if got.fromCodex == "" || !strings.HasSuffix(got.fromCodex, ".codex/auth.json") {
		t.Fatalf("fromCodex = %q", got.fromCodex)
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
