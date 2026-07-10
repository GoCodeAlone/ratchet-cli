package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	acpbridge "github.com/GoCodeAlone/ratchet-cli/internal/acp"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	providerauth "github.com/GoCodeAlone/ratchet-cli/internal/provider"
	wfprovider "github.com/GoCodeAlone/workflow-plugin-agent/provider"
	"github.com/GoCodeAlone/workflow/secrets"
)

type providerSetupGuide struct {
	Alias              string `json:"alias"`
	ProviderType       string `json:"provider_type"`
	InstallHint        string `json:"install_hint"`
	AuthHint           string `json:"auth_hint"`
	SetupCommand       string `json:"setup_command"`
	ModelBehavior      string `json:"model_behavior"`
	CredentialBoundary string `json:"credential_boundary"`
}

func providerSetupGuideRows() []providerSetupGuide {
	entries := providerauth.Catalog()
	rows := make([]providerSetupGuide, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, providerSetupGuideFromEntry(entry))
	}
	return rows
}

func providerSetupGuideFromEntry(entry providerauth.SetupEntry) providerSetupGuide {
	alias := entry.SetupAlias
	if alias == "" {
		alias = entry.Type
	}
	return providerSetupGuide{
		Alias: alias, ProviderType: entry.Type, InstallHint: entry.InstallHint,
		AuthHint: entry.AuthHint, SetupCommand: entry.SetupCommand,
		ModelBehavior: entry.ModelBehavior, CredentialBoundary: entry.CredentialBoundary,
	}
}

func handleProvider(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet provider <add|list|test|remove|default|setup|discover>")
		return
	}

	switch args[0] {
	case "discover":
		handleProviderDiscover()
		return
	case "setup":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet provider setup <list|guide|provider>")
			return
		}
		switch args[1] {
		case "list":
			if err := printProviderSetupGuideList(args[2:], os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		case "guide":
			if err := printProviderSetupGuide(args[2:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		default:
			entry, ok := providerauth.LookupSetup(args[1])
			if !ok {
				fmt.Printf("unknown provider to setup: %s\n", args[1])
				return
			}
			switch {
			case entry.Setup == providerauth.SetupOllama:
				handleOllamaSetup(args[2:])
			case entry.Auth == providerauth.AuthOpenAIChatGPT:
				handleOpenAIChatGPTSetup(args[2:])
			case entry.Setup == providerauth.SetupCLINative:
				handleCLIToolSetup(entry.Type, entry.DefaultAlias, entry.CLICommand, args[2:])
			default:
				fmt.Printf("Use: %s\n", entry.SetupCommand)
			}
		}
		return
	}

	var addEntry providerauth.SetupEntry
	if args[0] == "add" {
		requestedType := "anthropic"
		if len(args) > 1 {
			requestedType = args[1]
		}
		var ok bool
		addEntry, ok = providerauth.LookupSetup(requestedType)
		if !ok {
			fmt.Fprintf(os.Stderr, "error: unknown provider type %q\n", requestedType)
			return
		}
		if requiresDedicatedProviderSetup(addEntry) {
			fmt.Printf("Use: %s\n", addEntry.SetupCommand)
			return
		}
	}

	c, err := ensureProviderDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = c.Close() }()

	switch args[0] {
	case "add":
		entry := addEntry
		alias := entry.Type
		if len(args) > 2 && !strings.HasPrefix(args[2], "--") {
			alias = args[2]
		}
		model, modelSet := parseProviderModelFlag(args)
		scanner := bufio.NewScanner(os.Stdin)
		input, err := collectProviderSetupInput(context.Background(), entry, model, modelSet, scanner, os.Stdout, providerSetupDeps{
			promptSecret: providerauth.PromptSecret,
			promptBaseURL: func(defaultURL string) (string, error) {
				return promptProviderBaseURL(scanner, os.Stdout, defaultURL)
			},
			deviceFlow: providerauth.DeviceFlow,
			listModels: wfprovider.ListModelsWithSettings,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		settingsJSON, err := providerSettingsJSON(input.Settings)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		p, err := c.AddProvider(context.Background(), &pb.AddProviderReq{
			Alias:    alias,
			Type:     entry.Type,
			Model:    input.Model,
			ApiKey:   input.APIKey,
			BaseUrl:  input.BaseURL,
			Settings: settingsJSON,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Added provider: %s (%s)\n", p.Alias, p.Type)
	case "list":
		resp, err := c.ListProviders(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(resp.Providers) == 0 {
			fmt.Println("No providers configured.")
			return
		}
		fmt.Printf("%-20s %-12s %-30s %s\n", "ALIAS", "TYPE", "MODEL", "DEFAULT")
		for _, p := range resp.Providers {
			def := ""
			if p.IsDefault {
				def = "*"
			}
			fmt.Printf("%-20s %-12s %-30s %s\n", p.Alias, p.Type, p.Model, def)
		}
	case "test":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet provider test <alias>")
			return
		}
		resp, err := c.TestProvider(context.Background(), args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if resp.Success {
			fmt.Printf("OK (%dms): %s\n", resp.LatencyMs, resp.Message)
		} else {
			fmt.Printf("FAIL: %s\n", resp.Message)
		}
	case "remove":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet provider remove <alias>")
			return
		}
		if err := c.RemoveProvider(context.Background(), args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Removed provider: %s\n", args[1])
	case "default":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet provider default <alias>")
			return
		}
		if err := c.SetDefaultProvider(context.Background(), args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Set default provider: %s\n", args[1])
	default:
		fmt.Printf("unknown provider command: %s\n", args[0])
	}
}

func requiresDedicatedProviderSetup(entry providerauth.SetupEntry) bool {
	return entry.Auth == providerauth.AuthOpenAIChatGPT || entry.Setup == providerauth.SetupCLINative
}

func printProviderSetupGuideList(args []string, w io.Writer) error {
	guides := providerSetupGuideRows()
	if hasJSONFlag(args) {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(guides)
	}
	fmt.Fprintf(w, "%-18s %-16s %s\n", "ALIAS", "TYPE", "SETUP")
	for _, guide := range guides {
		fmt.Fprintf(w, "%-18s %-16s %s\n", guide.Alias, guide.ProviderType, guide.SetupCommand)
	}
	return nil
}

func printProviderSetupGuide(args []string, w io.Writer, errw io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintln(errw, "Usage: ratchet provider setup guide <provider> [--json]")
		return nil
	}
	entry, ok := providerauth.LookupSetup(args[0])
	if !ok {
		fmt.Fprintf(errw, "unknown provider setup guide: %s\n", args[0])
		return nil
	}
	guide := providerSetupGuideFromEntry(entry)
	if hasJSONFlag(args[1:]) {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(guide)
	}
	fmt.Fprintf(w, "%s (%s)\n", guide.Alias, guide.ProviderType)
	fmt.Fprintf(w, "Install: %s\n", guide.InstallHint)
	fmt.Fprintf(w, "Auth: %s\n", guide.AuthHint)
	fmt.Fprintf(w, "Setup: %s\n", guide.SetupCommand)
	fmt.Fprintf(w, "Model: %s\n", guide.ModelBehavior)
	fmt.Fprintf(w, "Credentials: %s\n", guide.CredentialBoundary)
	return nil
}

func hasJSONFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--json" {
			return true
		}
	}
	return false
}

type openAIChatGPTSetupOptions struct {
	model     string
	modelSet  bool
	fromCodex string
	noBrowser bool
}

func parseOpenAIChatGPTSetupArgs(args []string) openAIChatGPTSetupOptions {
	opts := openAIChatGPTSetupOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--model":
			if i+1 < len(args) {
				opts.model = strings.TrimSpace(args[i+1])
				opts.modelSet = opts.model != ""
				i++
			}
		case "--from-codex":
			opts.fromCodex = providerauth.DefaultCodexAuthPath()
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				opts.fromCodex = args[i+1]
				i++
			}
		case "--no-browser":
			opts.noBrowser = true
		}
	}
	return opts
}

func parseProviderModelFlag(args []string) (string, bool) {
	for i := 1; i < len(args); i++ {
		if args[i] == "--model" && i+1 < len(args) {
			model := strings.TrimSpace(args[i+1])
			return model, model != ""
		}
	}
	return "", false
}

func handleOpenAIChatGPTSetup(args []string) {
	opts := parseOpenAIChatGPTSetupArgs(args)
	fmt.Println("=== OpenAI ChatGPT Setup ===")
	fmt.Println("This uses ChatGPT account credentials for local CLI/IDE use.")
	fmt.Println("Device codes are phishing targets. Never share the code outside the OpenAI page.")

	var tokenBundle string
	var err error
	if opts.fromCodex != "" {
		tokenBundle, err = providerauth.LoadCodexAuth(opts.fromCodex)
		if err != nil {
			fmt.Fprintf(os.Stderr, "import Codex auth failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Imported Codex auth from %s\n", opts.fromCodex)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		deviceResp, err := providerauth.StartOpenAIChatGPTDeviceFlow(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "start device flow failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\nOpen %s and enter code: %s\n", deviceResp.VerificationURI, deviceResp.UserCode)
		if !opts.noBrowser {
			if err := providerauth.OpenBrowserURL(deviceResp.VerificationURI); err != nil {
				fmt.Fprintf(os.Stderr, "could not open browser: %v\n", err)
			}
		}
		fmt.Print("Waiting for authorization... ")
		result := <-providerauth.PollOpenAIChatGPTDeviceFlow(ctx, deviceResp.DeviceCode, deviceResp.UserCode, deviceResp.Interval)
		if result.Err != nil {
			fmt.Printf("FAIL\n")
			fmt.Fprintf(os.Stderr, "poll device flow failed: %v\n", result.Err)
			os.Exit(1)
		}
		fmt.Printf("OK\n")
		tokenBundle = result.Token
	}

	scanner := bufio.NewScanner(os.Stdin)
	if !opts.modelSet {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		model, selectErr := promptProviderModelSelection(ctx, "openai_chatgpt", tokenBundle, "", nil, scanner, os.Stdout, wfprovider.ListModelsWithSettings)
		cancel()
		if selectErr != nil {
			fmt.Fprintf(os.Stderr, "model selection failed: %v\n", selectErr)
			os.Exit(1)
		}
		opts.model = model
	}

	fmt.Println("Registering provider with ratchet...")
	c, err := ensureProviderDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = c.Close() }()

	isDefault := promptYesNo("Set as default provider?", scanner)
	p, err := c.AddProvider(context.Background(), openAIChatGPTAddProviderReq(opts.model, tokenBundle, isDefault))
	if err != nil {
		fmt.Fprintf(os.Stderr, "add provider failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Provider registered: %s (%s)\n", p.Alias, p.Type)
	if isDefault {
		fmt.Printf("✓ Default provider set to: %s\n", p.Alias)
	}

	fmt.Printf("\n=== Setup complete ===\nProvider: openai-chatgpt  Model: %s\n", opts.model)
	fmt.Println("Run 'ratchet provider test openai-chatgpt' to verify the connection.")
}

func openAIChatGPTAddProviderReq(model, tokenBundle string, isDefault bool) *pb.AddProviderReq {
	return &pb.AddProviderReq{
		Alias:     "openai-chatgpt",
		Type:      "openai_chatgpt",
		Model:     model,
		ApiKey:    tokenBundle,
		IsDefault: isDefault,
	}
}

func handleOllamaSetup(args []string) {
	model := "qwen3:8b"
	for i := 0; i < len(args); i++ {
		if args[i] == "--model" && i+1 < len(args) {
			model = args[i+1]
			i++
		}
	}

	// Single scanner shared across all stdin reads in this command.
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("=== Ollama Setup ===")

	// 1. Check if ollama binary exists.
	ollamaPath, err := exec.LookPath("ollama")
	if err != nil {
		fmt.Println("✗ ollama not found in PATH")
		if promptYesNo("Ollama not found. Install it?", scanner) {
			if err := installOllama(); err != nil {
				fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
				fmt.Println("Manual install: https://ollama.com/download")
				return
			}
			fmt.Println("✓ Ollama installed")
		} else {
			fmt.Println("Install Ollama at: https://ollama.com/download")
			fmt.Println("Then re-run: ratchet provider setup ollama")
			return
		}
	} else {
		fmt.Printf("✓ ollama found: %s\n", ollamaPath)
	}

	// 2. Check server health; start if needed.
	ctx := context.Background()
	ollamaClient := wfprovider.NewOllamaClient("")
	if err := ollamaClient.Health(ctx); err != nil {
		fmt.Println("  Ollama server not running — starting it...")
		if err := startOllamaServer(); err != nil {
			fmt.Fprintf(os.Stderr, "could not start ollama: %v\n", err)
			fmt.Println("Start manually: ollama serve")
			return
		}
		fmt.Println("✓ Ollama server started")
	} else {
		fmt.Println("✓ Ollama server running")
	}

	// 3. List installed models.
	models, err := ollamaClient.ListModels(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not list models: %v\n", err)
		fmt.Println("Cannot continue setup until the Ollama server is reachable.")
		fmt.Println("Please verify Ollama is running, then re-run: ratchet provider setup ollama")
		return
	}

	wantNew := true
	if len(models) > 0 {
		fmt.Println("\nInstalled models:")
		for i, m := range models {
			fmt.Printf("  %d. %s\n", i+1, m.Name)
		}
		fmt.Println()
		if !promptYesNo("Pull a new model?", scanner) {
			model = promptModelSelection(models, scanner)
			wantNew = false
		}
	}

	// 4. Pull model if needed.
	if wantNew {
		recommended := []wfprovider.ModelInfo{
			{ID: "qwen3:8b", Name: "qwen3:8b      (8GB, fast, good tool use)"},
			{ID: "llama3.1:8b", Name: "llama3.1:8b   (8GB, general purpose)"},
			{ID: "gemma3:4b", Name: "gemma3:4b     (4GB, lightweight)"},
		}
		fmt.Println("Recommended models:")
		for i, m := range recommended {
			fmt.Printf("  %d. %s\n", i+1, m.Name)
		}
		fmt.Printf("  %d. Custom (enter name)\n", len(recommended)+1)
		fmt.Print("\nSelect [1]: ")
		scanner.Scan()
		choice := strings.TrimSpace(scanner.Text())
		switch choice {
		case "", "1":
			model = recommended[0].ID
		case "2":
			model = recommended[1].ID
		case "3":
			model = recommended[2].ID
		default:
			fmt.Print("Model name: ")
			if !scanner.Scan() {
				fmt.Fprintln(os.Stderr, "\nNo input received; using default model.")
				model = recommended[0].ID
			} else {
				model = strings.TrimSpace(scanner.Text())
				if model == "" {
					model = recommended[0].ID
				}
			}
		}

		fmt.Printf("\nPulling %s...\n", model)
		if err := pullModelWithProgress(ctx, ollamaClient, model); err != nil {
			fmt.Fprintf(os.Stderr, "pull failed: %v\n", err)
			return
		}
		fmt.Printf("✓ %s ready\n", model)
	}

	// 5. Ensure daemon running and register provider.
	fmt.Println("\nRegistering provider with ratchet...")
	c, err := ensureProviderDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = c.Close() }()

	p, err := c.AddProvider(ctx, &pb.AddProviderReq{
		Alias:     "ollama",
		Type:      "ollama",
		Model:     model,
		BaseUrl:   "http://localhost:11434",
		IsDefault: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "add provider failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Provider registered: %s (%s)\n", p.Alias, p.Type)

	// 6. Test connection.
	fmt.Print("Testing connection... ")
	result, err := c.TestProvider(ctx, "ollama")
	if err != nil {
		fmt.Fprintf(os.Stderr, "test failed: %v\n", err)
		return
	}
	if result.Success {
		fmt.Printf("OK (%dms)\n", result.LatencyMs)
	} else {
		fmt.Printf("FAIL: %s\n", result.Message)
		return
	}

	fmt.Println("\n=== Setup complete ===")
	fmt.Printf("Provider: ollama  Model: %s\n", model)
	fmt.Println("Run 'ratchet' to start chatting.")
}

// promptYesNo prints question + " [Y/n] " and returns true for yes (default).
// Returns false on EOF or read error (safe for non-interactive/piped contexts).
func promptYesNo(question string, scanner *bufio.Scanner) bool {
	fmt.Printf("%s [Y/n] ", question)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "\nUnable to read response: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "\nNo input received; defaulting to no.")
		}
		return false
	}
	ans := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return ans == "" || ans == "y" || ans == "yes"
}

// ollamaInstallCommand returns the exec.Cmd for installing Ollama on the current platform.
// Returns an error for unsupported platforms.
func ollamaInstallCommand() (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("brew", "install", "ollama"), nil
	case "linux":
		// Download to temp file and execute explicitly (safer than curl|sh).
		script := `set -e; t=$(mktemp); curl -fsSL https://ollama.com/install.sh -o "$t"; sh "$t"; rm -f "$t"`
		return exec.Command("sh", "-c", script), nil
	default:
		return nil, fmt.Errorf("automatic Ollama installation is not supported on %s; please install manually from https://ollama.com/download", runtime.GOOS)
	}
}

// installOllama installs Ollama using the platform-appropriate method.
func installOllama() error {
	cmd, err := ollamaInstallCommand()
	if err != nil {
		return err
	}
	fmt.Printf("Running: %s\n", strings.Join(cmd.Args, " "))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// startOllamaServer starts ollama serve in the background and waits up to 15s for it to be healthy.
// If the server doesn't become healthy, the background process is killed to avoid orphans.
func startOllamaServer() error {
	cmd := exec.Command("ollama", "serve")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ollama serve: %w", err)
	}

	c := wfprovider.NewOllamaClient("")
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := c.Health(ctx)
		cancel()
		if err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	// Server didn't become healthy — kill the orphaned process.
	_ = cmd.Process.Kill()
	return fmt.Errorf("ollama server did not become ready within 15s")
}

// pullModelWithProgress pulls a model via Ollama and prints progress to stdout.
func pullModelWithProgress(ctx context.Context, c *wfprovider.OllamaClient, model string) error {
	lastPct := -1.0
	err := c.Pull(ctx, model, func(pct float64) {
		if pct-lastPct >= 5.0 || pct >= 100.0 {
			fmt.Printf("\r  %.0f%%", pct)
			lastPct = pct
		}
	})
	if lastPct >= 0.0 {
		fmt.Println() // newline after progress output
	}
	return err
}

// cliInstallInstructions maps provider alias to human-readable install instructions.
var cliInstallInstructions = map[string]string{
	"claude-code": "Install Claude Code: curl -fsSL https://claude.ai/install.sh | bash",
	"copilot-cli": "Install GitHub Copilot CLI: see https://githubnext.com/projects/copilot-cli/",
	"codex-cli":   "Install Codex CLI: npm install -g @openai/codex",
	"gemini-cli":  "Install Gemini CLI: npm install -g @google/gemini-cli",
	"cursor-cli":  "Install Cursor agent CLI: see https://cursor.com/",
}

// handleCLIToolSetup is the generic setup flow for PTY-backed CLI providers.
// providerType is the ratchet type (e.g. "claude_code"), binary is the executable
// name (e.g. "claude"), and alias is the human-facing setup name (e.g. "claude-code").
func handleCLIToolSetup(providerType, binary, alias string, _ []string) {
	fmt.Printf("=== %s Setup ===\n", alias)

	scanner := bufio.NewScanner(os.Stdin)

	// 1. Check binary exists.
	binPath, err := exec.LookPath(binary)
	if err != nil {
		fmt.Printf("✗ %s not found in PATH\n", binary)
		if inst, ok := cliInstallInstructions[alias]; ok {
			fmt.Println(inst)
		}
		fmt.Printf("After installing, re-run: ratchet provider setup %s\n", alias)
		return
	}
	fmt.Printf("✓ %s found: %s\n", binary, binPath)

	// 2. Health check: run <binary> -p "say ok" with 30s timeout.
	fmt.Print("Running health check... ")
	healthArgs := cliHealthCheckArgs(providerType)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	out, err := exec.CommandContext(ctx, binary, healthArgs...).Output()
	cancel()
	if err != nil {
		fmt.Printf("FAIL\n  %v\n", err)
		fmt.Printf("Ensure %s is authenticated and working, then re-run: ratchet provider setup %s\n", binary, alias)
		return
	}
	response := strings.TrimSpace(string(out))
	if len(response) > 60 {
		response = response[:60] + "..."
	}
	fmt.Printf("OK (%q)\n", response)

	// 3. Register provider with daemon.
	fmt.Println("Registering provider with ratchet...")
	c, err := ensureProviderDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = c.Close() }()

	workDir, _ := os.Getwd()
	p, err := c.AddProvider(context.Background(), &pb.AddProviderReq{
		Alias:   alias,
		Type:    providerType,
		BaseUrl: workDir,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "add provider failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Provider registered: %s (%s)\n", p.Alias, p.Type)

	// 4. Optionally set as default.
	if promptYesNo("Set as default provider?", scanner) {
		if err := c.SetDefaultProvider(context.Background(), alias); err != nil {
			fmt.Fprintf(os.Stderr, "set default failed: %v\n", err)
		} else {
			fmt.Printf("✓ Default provider set to: %s\n", alias)
		}
	}

	fmt.Printf("\n=== Setup complete ===\nRun 'ratchet' to start chatting via %s.\n", alias)
}

// cliHealthCheckArgs returns the health-check args for a given PTY provider type.
func cliHealthCheckArgs(providerType string) []string {
	switch providerType {
	case "codex_cli":
		return []string{"exec", "say ok"}
	default:
		return []string{"-p", "say ok"}
	}
}

// promptModelSelection prints a numbered list of models and returns the selected model ID.
// The caller must pass the shared scanner for the current command.
func promptModelSelection(models []wfprovider.ModelInfo, scanner *bufio.Scanner) string {
	fmt.Println("Select model:")
	for i, m := range models {
		fmt.Printf("  %d. %s\n", i+1, m.Name)
	}
	fmt.Print("Select [1]: ")
	scanner.Scan()
	choice := strings.TrimSpace(scanner.Text())
	if choice == "" {
		return models[0].ID
	}
	for i := range models {
		if choice == fmt.Sprintf("%d", i+1) {
			return models[i].ID
		}
	}
	return models[0].ID
}

type providerSetupInput struct {
	APIKey   string
	BaseURL  string
	Model    string
	Settings map[string]string
}

type providerSetupDeps struct {
	promptSecret  func(string) (string, error)
	promptBaseURL func(string) (string, error)
	deviceFlow    func(context.Context) (string, error)
	listModels    providerModelLister
}

func collectProviderSetupInput(ctx context.Context, entry providerauth.SetupEntry, model string, modelSet bool, scanner *bufio.Scanner, out io.Writer, deps providerSetupDeps) (providerSetupInput, error) {
	input := providerSetupInput{Model: strings.TrimSpace(model)}
	var err error
	switch entry.Auth {
	case providerauth.AuthNone:
	case providerauth.AuthAPIKey, providerauth.AuthAnthropic:
		if deps.promptSecret == nil {
			return providerSetupInput{}, fmt.Errorf("provider %s has no credential prompter", entry.Type)
		}
		input.APIKey, err = deps.promptSecret(entry.CredentialLabel)
	case providerauth.AuthGitHubDevice:
		if deps.deviceFlow == nil {
			return providerSetupInput{}, fmt.Errorf("provider %s has no device flow", entry.Type)
		}
		input.APIKey, err = deps.deviceFlow(ctx)
	case providerauth.AuthOpenAIChatGPT, providerauth.AuthCLINative:
		return providerSetupInput{}, fmt.Errorf("provider %s requires dedicated setup: %s", entry.Type, entry.SetupCommand)
	default:
		return providerSetupInput{}, fmt.Errorf("provider %s has unsupported auth strategy %q", entry.Type, entry.Auth)
	}
	if err != nil {
		return providerSetupInput{}, err
	}
	input.APIKey = strings.TrimSpace(input.APIKey)
	if entry.CredentialRequired && input.APIKey == "" {
		return providerSetupInput{}, fmt.Errorf("%s is required", entry.CredentialLabel)
	}

	input.Settings, err = promptProviderSettings(scanner, out, entry.Settings)
	if err != nil {
		return providerSetupInput{}, err
	}
	if entry.PromptBaseURL {
		if deps.promptBaseURL == nil {
			return providerSetupInput{}, fmt.Errorf("provider %s has no base URL prompter", entry.Type)
		}
		input.BaseURL, err = deps.promptBaseURL(entry.DefaultBaseURL)
		if err != nil {
			return providerSetupInput{}, err
		}
		input.BaseURL = strings.TrimSpace(input.BaseURL)
		if entry.BaseURLRequired && input.BaseURL == "" {
			return providerSetupInput{}, fmt.Errorf("%s requires a base URL", entry.Type)
		}
	}

	if modelSet || input.Model != "" || entry.Model == providerauth.ModelExternal {
		return input, nil
	}
	switch entry.Model {
	case providerauth.ModelManual:
		input.Model, err = promptManualProviderModel(scanner, out)
	case providerauth.ModelDynamic, providerauth.ModelOllama:
		if deps.listModels == nil {
			return providerSetupInput{}, fmt.Errorf("provider %s has no model lister", entry.Type)
		}
		listCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		input.Model, err = promptProviderModelSelection(listCtx, entry.Type, input.APIKey, input.BaseURL, input.Settings, scanner, out, deps.listModels)
	default:
		err = fmt.Errorf("provider %s has unsupported model strategy %q", entry.Type, entry.Model)
	}
	if err != nil {
		return providerSetupInput{}, err
	}
	return input, nil
}

func promptProviderSettings(scanner *bufio.Scanner, out io.Writer, fields []providerauth.SettingField) (map[string]string, error) {
	settings := make(map[string]string, len(fields))
	for _, field := range fields {
		value, err := promptProviderSetting(scanner, out, field)
		if err != nil {
			return nil, err
		}
		if value != "" {
			settings[field.Key] = value
		}
	}
	return settings, nil
}

func promptProviderBaseURL(scanner *bufio.Scanner, out io.Writer, defaultURL string) (string, error) {
	if defaultURL != "" {
		fmt.Fprintf(out, "Base URL [%s]: ", defaultURL)
	} else {
		fmt.Fprint(out, "Base URL: ")
	}
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	baseURL := strings.TrimSpace(scanner.Text())
	if baseURL == "" {
		baseURL = defaultURL
	}
	return baseURL, nil
}

func promptProviderSetting(scanner *bufio.Scanner, out io.Writer, field providerauth.SettingField) (string, error) {
	if len(field.Choices) > 0 {
		fmt.Fprintf(out, "%s:\n", field.Label)
		for i, choice := range field.Choices {
			fmt.Fprintf(out, "  %d. %s\n", i+1, choice)
		}
		fmt.Fprintf(out, "Select [%s]: ", field.Default)
	} else if field.Default != "" {
		fmt.Fprintf(out, "%s [%s]: ", field.Label, field.Default)
	} else {
		fmt.Fprintf(out, "%s: ", field.Label)
	}
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	value := strings.TrimSpace(scanner.Text())
	if value == "" {
		value = field.Default
	}
	if len(field.Choices) > 0 && value != "" {
		var index int
		if _, err := fmt.Sscanf(value, "%d", &index); err == nil && index >= 1 && index <= len(field.Choices) {
			value = field.Choices[index-1]
		} else {
			matched := false
			for _, choice := range field.Choices {
				if strings.EqualFold(value, choice) {
					value = choice
					matched = true
					break
				}
			}
			if !matched {
				return "", fmt.Errorf("invalid %s selection %q", field.Label, value)
			}
		}
	}
	if field.Required && value == "" {
		return "", fmt.Errorf("%s is required", field.Label)
	}
	return value, nil
}

func promptManualProviderModel(scanner *bufio.Scanner, out io.Writer) (string, error) {
	fmt.Fprint(out, "Model ID: ")
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	model := strings.TrimSpace(scanner.Text())
	if model == "" {
		return "", fmt.Errorf("model ID is required")
	}
	return model, nil
}

type providerModelLister func(context.Context, string, string, string, map[string]string) ([]wfprovider.ModelInfo, error)

func providerSettingsJSON(settings map[string]string) (string, error) {
	if len(settings) == 0 {
		return "", nil
	}
	clean := make(map[string]string, len(settings))
	for k, v := range settings {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		clean[k] = v
	}
	if len(clean) == 0 {
		return "", nil
	}
	data, err := json.Marshal(clean)
	if err != nil {
		return "", fmt.Errorf("marshal provider settings: %w", err)
	}
	return string(data), nil
}

func promptProviderModelSelection(ctx context.Context, providerType, apiKey, baseURL string, settings map[string]string, scanner *bufio.Scanner, out io.Writer, list providerModelLister) (string, error) {
	models, err := list(ctx, providerType, apiKey, baseURL, settings)
	if err != nil || len(models) == 0 {
		if err != nil {
			redactor := secrets.NewRedactor()
			redactor.AddValue("provider credential", apiKey)
			fmt.Fprintf(out, "could not list models for %s: %s\n", providerType, redactor.Redact(err.Error()))
		} else {
			fmt.Fprintf(out, "could not list models for %s: no models returned\n", providerType)
		}
		fmt.Fprint(out, "Model ID: ")
		if !scanner.Scan() {
			if scanErr := scanner.Err(); scanErr != nil {
				return "", scanErr
			}
			return "", io.EOF
		}
		model := strings.TrimSpace(scanner.Text())
		if model == "" {
			return "", fmt.Errorf("model ID is required")
		}
		return model, nil
	}

	fmt.Fprintln(out, "Available models:")
	for i, m := range models {
		name := m.Name
		if name == "" {
			name = m.ID
		}
		fmt.Fprintf(out, "  %d. %s\n", i+1, name)
	}
	customChoice := len(models) + 1
	fmt.Fprintf(out, "  %d. Custom (enter model ID)\n", customChoice)
	fmt.Fprint(out, "Select [1]: ")
	if !scanner.Scan() {
		if scanErr := scanner.Err(); scanErr != nil {
			return "", scanErr
		}
		return models[0].ID, nil
	}
	choice := strings.TrimSpace(scanner.Text())
	if choice == "" {
		return models[0].ID, nil
	}
	var idx int
	if _, scanErr := fmt.Sscanf(choice, "%d", &idx); scanErr != nil || idx < 1 || idx > customChoice {
		return "", fmt.Errorf("invalid model selection %q", choice)
	}
	if idx <= len(models) {
		return models[idx-1].ID, nil
	}

	fmt.Fprint(out, "Model ID: ")
	if !scanner.Scan() {
		if scanErr := scanner.Err(); scanErr != nil {
			return "", scanErr
		}
		return "", io.EOF
	}
	model := strings.TrimSpace(scanner.Text())
	if model == "" {
		return "", fmt.Errorf("model ID is required")
	}
	return model, nil
}

func handleProviderDiscover() {
	fmt.Println("Fetching ACP agent registry...")
	registry := acpbridge.NewRegistry(24 * time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	agents, err := registry.Agents(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(agents) == 0 {
		fmt.Println("No ACP agents found in registry.")
		return
	}

	fmt.Printf("Found %d ACP agent(s):\n\n", len(agents))
	for _, a := range agents {
		desc := a.Description
		if len(desc) > 80 {
			desc = desc[:77] + "..."
		}
		fmt.Printf("  %-20s %s\n", a.ID, desc)
		if a.Homepage != "" {
			fmt.Printf("  %-20s %s\n", "", a.Homepage)
		}
	}
}
