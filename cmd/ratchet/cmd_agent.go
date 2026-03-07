package main

import (
	"context"
	"fmt"
	"os"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
)

func handleAgent(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet agent <list>")
		return
	}

	c, err := client.EnsureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	switch args[0] {
	case "list":
		resp, err := c.ListAgents(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(resp.Agents) == 0 {
			fmt.Println("No agents configured.")
			return
		}
		fmt.Printf("%-20s %-10s %s\n", "NAME", "STATUS", "MODEL")
		for _, a := range resp.Agents {
			fmt.Printf("%-20s %-10s %s\n", a.Name, a.Status, a.Model)
		}
	default:
		fmt.Printf("unknown agent command: %s\n", args[0])
	}
}
