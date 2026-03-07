package main

import (
	"fmt"
	"os"

	"github.com/GoCodeAlone/ratchet-cli/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		// Default: launch interactive TUI
		if err := runInteractive(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	switch os.Args[1] {
	case "daemon":
		handleDaemon(os.Args[2:])
	case "sessions":
		handleSessions(os.Args[2:])
	case "provider":
		handleProvider(os.Args[2:])
	case "agent":
		handleAgent(os.Args[2:])
	case "team":
		handleTeam(os.Args[2:])
	case "plugin":
		handlePlugin(os.Args[2:])
	case "skill":
		handleSkill(os.Args[2:])
	case "config":
		handleConfig(os.Args[2:])
	case "chat":
		handleChat(os.Args[1:]) // pass "chat" + remaining args
	case "version":
		fmt.Println(version.String())
	case "help", "--help", "-h":
		printUsage()
	default:
		// Treat as implicit chat: ratchet "fix the bug"
		handleChat(os.Args[1:])
	}
}

func runInteractive() error {
	// TODO: auto-start daemon, connect, launch TUI
	fmt.Println("ratchet interactive mode (not yet implemented)")
	return nil
}

func handleDaemon(args []string)   { fmt.Println("daemon: not yet implemented") }
func handleSessions(args []string) { fmt.Println("sessions: not yet implemented") }
func handleProvider(args []string) { fmt.Println("provider: not yet implemented") }
func handleAgent(args []string)    { fmt.Println("agent: not yet implemented") }
func handleTeam(args []string)     { fmt.Println("team: not yet implemented") }
func handlePlugin(args []string)   { fmt.Println("plugin: not yet implemented") }
func handleSkill(args []string)    { fmt.Println("skill: not yet implemented") }
func handleConfig(args []string)   { fmt.Println("config: not yet implemented") }
func handleChat(args []string)     { fmt.Println("chat: not yet implemented") }

func printUsage() {
	fmt.Print(`Usage: ratchet [command] [args]

Commands:
  (default)        Launch interactive TUI
  chat "prompt"    Start session with initial prompt
  -p "prompt"      One-shot programmatic mode

  daemon           Manage background daemon
  sessions         Manage sessions
  provider         Manage AI providers
  agent            Manage agent definitions
  team             Multi-agent orchestration
  plugin           Manage plugins
  skill            Manage skills
  config           Configuration
  version          Print version

Run 'ratchet <command> --help' for details.
`)
}
