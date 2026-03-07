package main

import (
	"fmt"
	"os"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
)

func handleConfig(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet config <show>")
		return
	}
	switch args[0] {
	case "show":
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("DefaultProvider: %s\n", cfg.DefaultProvider)
		fmt.Printf("DefaultModel:    %s\n", cfg.DefaultModel)
		fmt.Printf("Theme:           %s\n", cfg.Theme)
	default:
		fmt.Printf("unknown config command: %s\n", args[0])
	}
}
