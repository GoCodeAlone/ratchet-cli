package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/GoCodeAlone/ratchet-cli/internal/mcp"
	"github.com/GoCodeAlone/ratchet-cli/internal/mesh"
)

func handleMCP(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet mcp <blackboard> [flags]")
		return
	}

	switch args[0] {
	case "blackboard":
		handleMCPBlackboard(args[1:])
	default:
		fmt.Printf("unknown mcp command: %s\n", args[0])
	}
}

func handleMCPBlackboard(_ []string) {
	// For now, create a standalone Blackboard instance.
	// TODO: connect to daemon's shared Blackboard via Unix socket when
	// team-id flag is implemented.
	bb := mesh.NewBlackboard()

	srv := mcp.NewBBMCPServer(bb)
	if err := srv.Serve(bufio.NewReader(os.Stdin), os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "mcp server error: %v\n", err)
		os.Exit(1)
	}
}
