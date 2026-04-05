package main

import (
	"context"
	"fmt"
	"os"

	"github.com/GoCodeAlone/ratchet-cli/internal/plugins"
)

func handlePlugin(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet plugin <list|install|remove>")
		return
	}
	switch args[0] {
	case "list":
		reg, err := plugins.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(reg.Plugins) == 0 {
			fmt.Println("No plugins installed.")
			return
		}
		fmt.Printf("%-20s %-10s %s\n", "NAME", "VERSION", "SOURCE")
		for name, entry := range reg.Plugins {
			fmt.Printf("%-20s %-10s %s\n", name, entry.Version, entry.Source)
		}
	case "install":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet plugin install <owner/repo or ./path>")
			return
		}
		src := args[1]
		var err error
		if isLocalPath(src) {
			err = plugins.InstallFromLocal(src)
		} else {
			err = plugins.InstallFromGitHub(context.Background(), src)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Installed plugin: %s\n", src)
	case "remove":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet plugin remove <name>")
			return
		}
		if err := plugins.Uninstall(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Removed plugin: %s\n", args[1])
	default:
		fmt.Printf("unknown plugin command: %s\n", args[0])
	}
}

// isLocalPath returns true if src looks like a local filesystem path.
func isLocalPath(src string) bool {
	return len(src) > 0 && (src[0] == '.' || src[0] == '/')
}
