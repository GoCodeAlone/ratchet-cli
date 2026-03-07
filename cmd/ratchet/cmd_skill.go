package main

import (
	"fmt"
	"os"

	"github.com/GoCodeAlone/ratchet-cli/internal/skills"
)

func handleSkill(args []string) {
	wd, _ := os.Getwd()
	if len(args) == 0 {
		fmt.Println("Usage: ratchet skill <list|show>")
		return
	}
	switch args[0] {
	case "list":
		discovered := skills.Discover(wd)
		if len(discovered) == 0 {
			fmt.Println("No skills found.")
			return
		}
		for _, s := range discovered {
			fmt.Printf("%-20s %s\n", s.Name, s.Path)
		}
	case "show":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet skill show <name>")
			return
		}
		discovered := skills.Discover(wd)
		for _, s := range discovered {
			if s.Name == args[1] {
				fmt.Println(s.Content)
				return
			}
		}
		fmt.Fprintf(os.Stderr, "skill not found: %s\n", args[1])
		os.Exit(1)
	default:
		fmt.Printf("unknown skill command: %s\n", args[0])
	}
}
