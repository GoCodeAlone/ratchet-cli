package main

import (
	"fmt"
)

func handleTeam(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet team <start|status>")
		return
	}
	switch args[0] {
	case "start":
		fmt.Println("team start: not yet implemented")
	case "status":
		fmt.Println("team status: not yet implemented")
	default:
		fmt.Printf("unknown team command: %s\n", args[0])
	}
}
