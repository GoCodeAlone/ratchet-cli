package main

import (
	"fmt"
	"os"
)

func handleProject(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet project <start|list|pause|resume|kill> [args...]")
		return
	}

	switch args[0] {
	case "start":
		handleProjectStart(args[1:])
	case "list":
		handleProjectList()
	case "pause":
		handleProjectPauseResume(args[1:], "pause")
	case "resume":
		handleProjectPauseResume(args[1:], "resume")
	case "kill":
		handleProjectKill(args[1:])
	default:
		fmt.Printf("unknown project command: %s\n", args[0])
	}
}

func handleProjectStart(args []string) {
	// Phase 1 stub — will be wired to daemon RPC in Phase 4.
	fmt.Println("project start: not yet implemented (Phase 4)")
	_ = args
}

func handleProjectList() {
	fmt.Println("project list: not yet implemented (Phase 4)")
}

func handleProjectPauseResume(args []string, action string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ratchet project %s <project-id|name>\n", action)
		return
	}
	fmt.Printf("project %s %s: not yet implemented (Phase 4)\n", action, args[0])
}

func handleProjectKill(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: ratchet project kill <project-id|name>")
		return
	}
	fmt.Printf("project kill %s: not yet implemented (Phase 4)\n", args[0])
}
