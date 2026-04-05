package main

import (
	"context"
	"fmt"
	"os"
	"time"

	wfprovider "github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

func handleModel(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet model <list|pull>")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  list                              List installed Ollama models")
		fmt.Println("  pull <name>                       Pull a model via Ollama")
		fmt.Println("  pull --from huggingface <repo> <file>  Download GGUF from HuggingFace")
		return
	}

	var err error
	switch args[0] {
	case "list":
		err = handleModelList()
	case "pull":
		err = handleModelPull(args[1:])
	default:
		fmt.Printf("unknown model command: %s\n", args[0])
		fmt.Println("Usage: ratchet model <list|pull>")
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func handleModelList() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c := wfprovider.NewOllamaClient("")
	models, err := c.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("listing models: %w\nIs Ollama running? Try: ollama serve", err)
	}
	if len(models) == 0 {
		fmt.Println("No models installed.")
		fmt.Println("Pull one with: ratchet model pull qwen3:8b")
		return nil
	}
	fmt.Printf("%-40s %s\n", "NAME", "CONTEXT")
	for _, m := range models {
		ctx := ""
		if m.ContextWindow > 0 {
			ctx = fmt.Sprintf("%d", m.ContextWindow)
		}
		fmt.Printf("%-40s %s\n", m.Name, ctx)
	}
	return nil
}

func handleModelPull(args []string) error {
	// Check for --from huggingface flag
	if len(args) >= 3 && args[0] == "--from" && args[1] == "huggingface" {
		repo := args[2]
		if len(args) < 4 {
			return fmt.Errorf("usage: ratchet model pull --from huggingface <repo> <file>")
		}
		file := args[3]
		return handleHuggingFacePull(repo, file)
	}

	if len(args) == 0 {
		return fmt.Errorf("usage: ratchet model pull <name>\n       ratchet model pull --from huggingface <repo> <file>")
	}

	name := args[0]
	ctx := context.Background()
	c := wfprovider.NewOllamaClient("")

	fmt.Printf("Pulling %s...\n", name)
	if err := pullModelWithProgress(ctx, c, name); err != nil {
		return fmt.Errorf("pull %s: %w", name, err)
	}
	fmt.Printf("✓ %s ready\n", name)
	return nil
}

func handleHuggingFacePull(repo, file string) error {
	ctx := context.Background()
	fmt.Printf("Downloading %s/%s from HuggingFace...\n", repo, file)
	lastPct := -1.0
	path, err := wfprovider.DownloadHuggingFaceFile(ctx, repo, file, "", func(pct float64) {
		if pct-lastPct >= 5.0 || pct >= 100.0 {
			fmt.Printf("\r  %.0f%%", pct)
			lastPct = pct
		}
	})
	if lastPct >= 0.0 {
		fmt.Println()
	}
	if err != nil {
		return fmt.Errorf("download %s/%s: %w", repo, file, err)
	}
	fmt.Printf("✓ Saved to: %s\n", path)
	return nil
}
