package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui"
	"github.com/GoCodeAlone/ratchet-cli/internal/version"
)

func main() {
	// Check for --reconfigure / -r and --mode flags before subcommand dispatch.
	reconfigure := false
	var modeFlag string
	var filteredArgs []string
	args := os.Args[1:]
	for i, arg := range args {
		switch {
		case arg == "--reconfigure" || arg == "-r":
			reconfigure = true
		case arg == "--mode" && i+1 < len(args):
			modeFlag = args[i+1]
		case strings.HasPrefix(arg, "--mode="):
			modeFlag = strings.TrimPrefix(arg, "--mode=")
		default:
			filteredArgs = append(filteredArgs, arg)
		}
	}
	_ = modeFlag // consumed by daemon via config; reserved for future session wiring

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
	case "doctor":
		handleDoctor(filteredArgs[1:])
	case "sessions":
		handleSessions(filteredArgs[1:])
	case "provider":
		handleProvider(filteredArgs[1:])
	case "agent":
		handleAgent(filteredArgs[1:])
	case "team":
		handleTeam(filteredArgs[1:])
	case "trust":
		handleTrust(filteredArgs[1:])
	case "policy":
		handlePolicy(filteredArgs[1:])
	case "hooks":
		handleHooks(filteredArgs[1:])
	case "blackboard":
		handleBlackboard(filteredArgs[1:])
	case "retro":
		handleRetro(filteredArgs[1:])
	case "project":
		handleProject(filteredArgs[1:])
	case "plugin":
		handlePlugin(filteredArgs[1:])
	case "routines":
		handleRoutines(filteredArgs[1:])
	case "workflows":
		handleWorkflows(filteredArgs[1:])
	case "skill":
		handleSkill(filteredArgs[1:])
	case "model":
		handleModel(filteredArgs[1:])
	case "config":
		handleConfig(filteredArgs[1:])
	case "acp":
		if err := runACP(filteredArgs[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "acp error: %v\n", err)
			os.Exit(1)
		}
		return
	case "mcp":
		handleMCP(filteredArgs[1:])
	case "chat":
		handleChat(filteredArgs) // pass "chat" + remaining args
	case "version", "--version", "-v":
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

	c, err := ensureCompatibleConnectedDaemon(client.EnsureDaemon, reloadAndReconnect, os.Stderr)
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

	return tui.Run(ctx, c, session, reconfigure)
}

type compatibleDaemon interface {
	EnsureCompatible() (*pb.VersionCheckResp, error)
	Close() error
}

func ensureCompatibleConnectedDaemon[T compatibleDaemon](connect func() (T, error), reload func() error, stderr io.Writer) (T, error) {
	c, err := connect()
	if err != nil {
		var zero T
		return zero, err
	}
	if resp, err := c.EnsureCompatible(); err == nil {
		if !resp.Compatible {
			fmt.Fprintf(stderr, "warning: %s\n", resp.Message)
		} else if resp.ReloadRecommended {
			fmt.Fprintf(stderr, "daemon version mismatch (%s). Reloading daemon...\n", resp.Message)
			c.Close()
			if reloadErr := reload(); reloadErr != nil {
				var zero T
				return zero, fmt.Errorf("reload daemon: %w", reloadErr)
			}
			c, err = connect()
			if err != nil {
				var zero T
				return zero, fmt.Errorf("reconnect after reload: %w", err)
			}
		}
	}
	return c, nil
}

func ensureProviderDaemon() (*client.Client, error) {
	return ensureCompatibleConnectedDaemon(client.EnsureDaemon, reloadAndReconnect, os.Stderr)
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
  doctor           Print credential-free local diagnostics
  sessions         Manage sessions
  model            Manage local models (list, pull)
  provider         Manage AI providers
  agent            Manage agent definitions
  team             Multi-agent orchestration
  trust            Manage runtime trust rules and persistent grants
  policy           Show supported, partial, explicit-operator, and deferred policy surfaces
  hooks            Review and trust lifecycle hooks
  blackboard       Shared daemon blackboard
  retro            Analyze optional retro evidence
  plugin           Manage plugins, marketplaces, enable/disable, and reload
  routines         Manage visible scheduled prompt routines
  workflows        Manage visible declarative workflow run records
  skill            Manage skills
  config           Configuration
  acp              Run as ACP agent (stdio JSON-RPC)
  version          Print version

Slash commands (inside TUI):
  /help                      Show available commands
  /tree                      Open session branch tree
  /provider list             List configured providers
  /provider add              Add a new provider
  /provider remove <alias>   Remove a provider
  /provider default <alias>  Set default provider
  /provider test <alias>     Test provider connection
  /mode <mode>               Switch trust mode (conservative|permissive|locked|sandbox|custom)
  /trust list                Show active trust rules
  /trust grants              Show persistent grants
  /trust allow "pattern" [--scope scope]  Add allow rule
  /trust deny "pattern" [--scope scope]   Add deny rule
  /trust persist allow "pattern" [--scope scope]  Add persistent allow grant
  /trust persist deny "pattern" [--scope scope]   Add persistent deny grant
  /trust revoke "pattern" [--scope scope]  Revoke persistent grant
  /trust reset               Reset to config defaults
  /exit                      Quit ratchet

Run 'ratchet <command> --help' for details.
`)
}
