package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui"
	"github.com/GoCodeAlone/ratchet-cli/internal/version"
)

func main() {
	// Check for --reconfigure / -r flag before subcommand dispatch
	reconfigure := false
	var filteredArgs []string
	for _, arg := range os.Args[1:] {
		if arg == "--reconfigure" || arg == "-r" {
			reconfigure = true
		} else {
			filteredArgs = append(filteredArgs, arg)
		}
	}

	if len(filteredArgs) == 0 {
		// Default: launch interactive TUI
		if err := runInteractive(reconfigure); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// One-shot mode: ratchet -p "prompt"
	if filteredArgs[0] == "-p" {
		prompt := strings.Join(filteredArgs[1:], " ")
		handleOneShot(prompt)
		return
	}

	switch filteredArgs[0] {
	case "daemon":
		handleDaemon(filteredArgs[1:])
	case "sessions":
		handleSessions(filteredArgs[1:])
	case "provider":
		handleProvider(filteredArgs[1:])
	case "agent":
		handleAgent(filteredArgs[1:])
	case "team":
		handleTeam(filteredArgs[1:])
	case "plugin":
		handlePlugin(filteredArgs[1:])
	case "skill":
		handleSkill(filteredArgs[1:])
	case "config":
		handleConfig(filteredArgs[1:])
	case "chat":
		handleChat(filteredArgs) // pass "chat" + remaining args
	case "version":
		fmt.Println(version.String())
	case "help", "--help", "-h":
		printUsage()
	default:
		// Treat as implicit chat: ratchet "fix the bug"
		handleChat(filteredArgs)
	}
}

func runInteractive(reconfigure bool) error {
	ctx := context.Background()

	c, err := client.EnsureDaemon()
	if err != nil {
		return fmt.Errorf("connect to daemon: %w", err)
	}
	defer c.Close()

	// Version handshake — warn or reload if needed.
	if resp, err := c.EnsureCompatible(); err == nil {
		if !resp.Compatible {
			fmt.Fprintf(os.Stderr, "warning: %s\n", resp.Message)
		} else if resp.ReloadRecommended {
			fmt.Fprintf(os.Stderr, "daemon version mismatch (%s). Reloading daemon...\n", resp.Message)
			c.Close()
			if reloadErr := reloadAndReconnect(); reloadErr != nil {
				fmt.Fprintf(os.Stderr, "reload failed: %v — continuing with existing daemon\n", reloadErr)
			} else {
				// Reconnect after successful reload.
				c, err = client.EnsureDaemon()
				if err != nil {
					return fmt.Errorf("reconnect after reload: %w", err)
				}
			}
		}
	}

	wd, _ := os.Getwd()
	session, err := c.CreateSession(ctx, &pb.CreateSessionReq{
		WorkingDir: wd,
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	return tui.Run(ctx, c, session, reconfigure)
}

// reloadAndReconnect triggers a daemon reload via SIGUSR1 then waits for the
// new daemon to become ready.
func reloadAndReconnect() error {
	exe, _ := os.Executable()
	return daemon.ReloadDaemon(exe)
}

func printUsage() {
	fmt.Print(`Usage: ratchet [flags] [command] [args]

Flags:
  --reconfigure, -r  Re-run provider setup wizard

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

Slash commands (inside TUI):
  /help                      Show available commands
  /provider list             List configured providers
  /provider add              Add a new provider
  /provider remove <alias>   Remove a provider
  /provider default <alias>  Set default provider
  /provider test <alias>     Test provider connection

Run 'ratchet <command> --help' for details.
`)
}
