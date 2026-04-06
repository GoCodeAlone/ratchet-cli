package main

import (
	"context"
	"fmt"
	"os"
	"strings"

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

// isLocalPath returns true if src looks like a local filesystem path
// rather than a GitHub owner/repo reference.
func isLocalPath(src string) bool {
	if len(src) == 0 {
		return false
	}
	// Explicit relative/absolute paths
	if src[0] == '.' || src[0] == '/' {
		return true
	}
	// Home-relative paths
	if strings.HasPrefix(src, "~") {
		return true
	}
	// Windows absolute paths (e.g. C:\...)
	if len(src) >= 2 && src[1] == ':' {
		return true
	}
	// If it exists on disk, treat it as local
	if _, err := os.Stat(src); err == nil {
		return true
	}
	return false
}
