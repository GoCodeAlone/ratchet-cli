package main

import (
	"context"
	"fmt"
	"os"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
)

func handleSessions(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet sessions <list|kill>")
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
		resp, err := c.ListSessions(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(resp.Sessions) == 0 {
			fmt.Println("No sessions.")
			return
		}
		fmt.Printf("%-36s %-10s %-20s %s\n", "ID", "STATUS", "PROVIDER", "WORKING_DIR")
		for _, s := range resp.Sessions {
			fmt.Printf("%-36s %-10s %-20s %s\n", s.Id, s.Status, s.Provider, s.WorkingDir)
		}
	case "kill":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet sessions kill <id>")
			return
		}
		if err := c.KillSession(context.Background(), args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Killed session: %s\n", args[1])
	default:
		fmt.Printf("unknown sessions command: %s\n", args[0])
	}
}
