package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	providerauth "github.com/GoCodeAlone/ratchet-cli/internal/provider"
	wfprovider "github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

func handleProvider(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet provider <add|list|test|remove|default|setup>")
		return
	}

	switch args[0] {
	case "setup":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet provider setup <ollama|claude-code|copilot-cli|codex-cli|gemini-cli|cursor-cli>")
			return
		}
		switch args[1] {
		case "ollama":
			handleOllamaSetup(args[2:])
		case "claude-code":
			handleCLIToolSetup("claude_code", "claude", "claude", args[2:])
		case "copilot-cli":
			handleCLIToolSetup("copilot_cli", "copilot", "copilot", args[2:])
		case "codex-cli":
			handleCLIToolSetup("codex_cli", "codex", "codex", args[2:])
		case "gemini-cli":
			handleCLIToolSetup("gemini_cli", "gemini", "gemini", args[2:])
		case "cursor-cli":
			handleCLIToolSetup("cursor_cli", "agent", "cursor-cli", args[2:])
		default:
			fmt.Printf("unknown provider to setup: %s\n", args[1])
		}
		return
	}

	c, err := client.EnsureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = c.Close() }()

	switch args[0] {
	case "add":
		providerType := "anthropic"
		if len(args) > 1 {
			providerType = args[1]
		}
		alias := providerType
		if len(args) > 2 {
			alias = args[2]
		}
		// Parse --model flag from remaining args.
		var model string
		for i := 1; i < len(args); i++ {
			if args[i] == "--model" && i+1 < len(args) {
				model = args[i+1]
			}
		}
		var apiKey, baseURL string
		switch providerType {
		case "ollama":
			// No API key needed for Ollama
			baseURL, err = providerauth.PromptBaseURL("http://localhost:11434")
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			// If no --model flag, try to list installed models and let user pick.
			if model == "" {
				ollamaClient := wfprovider.NewOllamaClient(baseURL)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				models, listErr := ollamaClient.ListModels(ctx)
				cancel()
				if listErr == nil && len(models) > 0 {
					fmt.Println("Available models:")
					for i, m := range models {
						fmt.Printf("  %d. %s\n", i+1, m.Name)
					}
					fmt.Print("Select [1]: ")
					scanner := bufio.NewScanner(os.Stdin)
					if scanner.Scan() {
						choice := strings.TrimSpace(scanner.Text())
						idx := 0
						if choice != "" {
							fmt.Sscanf(choice, "%d", &idx)
							idx-- // 1-indexed
						}
						if idx >= 0 && idx < len(models) {
							model = models[idx].ID
						}
					}
					if model == "" {
						model = models[0].ID
					}
				}
			}
		case "llama_cpp":
			// No API key needed for llama.cpp
			baseURL, err = providerauth.PromptBaseURL("http://localhost:8081/v1")
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		default:
			apiKey, err = providerauth.PromptAPIKey(providerType)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			if providerType == "custom" || providerType == "openai" {
				baseURL, err = providerauth.PromptBaseURL("")
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: %v\n", err)
					os.Exit(1)
				}
			}
		}
		p, err := c.AddProvider(context.Background(), &pb.AddProviderReq{
			Alias:   alias,
			Type:    providerType,
			Model:   model,
			ApiKey:  apiKey,
			BaseUrl: baseURL,
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
	c, err := client.EnsureDaemon()
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
	c, err := client.EnsureDaemon()
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
