package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui"
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

	// One-shot mode: ratchet -p "prompt"
	if os.Args[1] == "-p" {
		prompt := strings.Join(os.Args[2:], " ")
		handleOneShot(prompt)
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
	ctx := context.Background()

	c, err := client.EnsureDaemon()
	if err != nil {
		return fmt.Errorf("connect to daemon: %w", err)
	}
	defer c.Close()

	wd, _ := os.Getwd()
	session, err := c.CreateSession(ctx, &pb.CreateSessionReq{
		WorkingDir: wd,
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	return tui.Run(ctx, c, session)
}

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
