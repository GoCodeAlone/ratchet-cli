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

	switch args[0] {
	case "list":
		handleModelList()
	case "pull":
		handleModelPull(args[1:])
	default:
		fmt.Printf("unknown model command: %s\n", args[0])
		fmt.Println("Usage: ratchet model <list|pull>")
	}
}

func handleModelList() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c := wfprovider.NewOllamaClient("")
	models, err := c.ListModels(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error listing models: %v\n", err)
		fmt.Fprintln(os.Stderr, "Is Ollama running? Try: ollama serve")
		os.Exit(1)
	}
	if len(models) == 0 {
		fmt.Println("No models installed.")
		fmt.Println("Pull one with: ratchet model pull qwen3:8b")
		return
	}
	fmt.Printf("%-40s %s\n", "NAME", "CONTEXT")
	for _, m := range models {
		ctx := ""
		if m.ContextWindow > 0 {
			ctx = fmt.Sprintf("%d", m.ContextWindow)
		}
		fmt.Printf("%-40s %s\n", m.Name, ctx)
	}
}

func handleModelPull(args []string) {
	// Check for --from huggingface flag
	if len(args) >= 3 && args[0] == "--from" && args[1] == "huggingface" {
		repo := args[2]
		if len(args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: ratchet model pull --from huggingface <repo> <file>")
			os.Exit(1)
		}
		file := args[3]
		handleHuggingFacePull(repo, file)
		return
	}

	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: ratchet model pull <name>")
		fmt.Fprintln(os.Stderr, "       ratchet model pull --from huggingface <repo> <file>")
		os.Exit(1)
	}

	name := args[0]
	ctx := context.Background()
	c := wfprovider.NewOllamaClient("")

	fmt.Printf("Pulling %s...\n", name)
	if err := pullModelWithProgress(ctx, c, name); err != nil {
		fmt.Fprintf(os.Stderr, "\npull failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n✓ %s ready\n", name)
}

func handleHuggingFacePull(repo, file string) {
	ctx := context.Background()
	fmt.Printf("Downloading %s/%s from HuggingFace...\n", repo, file)
	lastPct := -1.0
	path, err := wfprovider.DownloadHuggingFaceFile(ctx, repo, file, "", func(pct float64) {
		if pct-lastPct >= 5.0 || pct >= 100.0 {
			fmt.Printf("\r  %.0f%%", pct)
			lastPct = pct
		}
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "\ndownload failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n✓ Saved to: %s\n", path)
}
