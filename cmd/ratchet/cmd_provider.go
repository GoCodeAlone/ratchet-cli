package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	providerauth "github.com/GoCodeAlone/ratchet-cli/internal/provider"
)

func handleProvider(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet provider <add|list|test|remove|default|setup>")
		return
	}

	switch args[0] {
	case "setup":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet provider setup <ollama>")
			return
		}
		switch args[1] {
		case "ollama":
			handleOllamaSetup(args[2:])
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
		var apiKey, baseURL string
		switch providerType {
		case "ollama":
			// No API key needed for Ollama
			baseURL, err = providerauth.PromptBaseURL("http://localhost:11434")
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
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

	fmt.Println("=== Ollama Setup ===")

	// 1. Check if ollama binary exists.
	ollamaPath, err := exec.LookPath("ollama")
	if err != nil {
		fmt.Println("✗ ollama binary not found in PATH")
		fmt.Println()
		fmt.Println("Install Ollama:")
		fmt.Println("  curl -fsSL https://ollama.com/install.sh | sh")
		fmt.Println()
		fmt.Println("Then re-run: ratchet provider setup ollama")
		return
	}
	fmt.Printf("✓ ollama found: %s\n", ollamaPath)

	// 2. Check if ollama server is running.
	fmt.Print("  Checking if Ollama server is running... ")
	httpClient := &http.Client{Timeout: 3 * time.Second}
	resp, err := httpClient.Get("http://localhost:11434/api/tags")
	if err != nil {
		fmt.Println("not running")
		fmt.Println()
		fmt.Println("Start the Ollama server:")
		fmt.Println("  ollama serve")
		fmt.Println()
		fmt.Println("Then re-run: ratchet provider setup ollama")
		return
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Printf("not running (HTTP %d)\n", resp.StatusCode)
		fmt.Println()
		fmt.Println("Start the Ollama server:")
		fmt.Println("  ollama serve")
		fmt.Println()
		fmt.Println("Then re-run: ratchet provider setup ollama")
		return
	}
	fmt.Println("running ✓")

	// 3. Suggest model to pull.
	fmt.Printf("\nRecommended model: %s\n", model)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Pull the model:   ollama pull %s\n", model)
	fmt.Printf("  2. Add to ratchet:   ratchet provider add ollama local-qwen\n")
	fmt.Printf("  3. Test connection:  ratchet provider test local-qwen\n")
}
