package main

import (
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
		names, err := plugins.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(names) == 0 {
			fmt.Println("No plugins installed.")
			return
		}
		for _, n := range names {
			fmt.Println(n)
		}
	case "install":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet plugin install <name>")
			return
		}
		if err := plugins.Install(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Installed plugin: %s\n", args[1])
	case "remove":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet plugin remove <name>")
			return
		}
		if err := plugins.Remove(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Removed plugin: %s\n", args[1])
	default:
		fmt.Printf("unknown plugin command: %s\n", args[0])
	}
}
