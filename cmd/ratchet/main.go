package main

import (
	"context"
	"fmt"
	"os"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	providerauth "github.com/GoCodeAlone/ratchet-cli/internal/provider"
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

func handleDaemon(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet daemon <start|stop|status>")
		return
	}
	switch args[0] {
	case "start":
		bg := false
		for _, a := range args[1:] {
			if a == "--background" || a == "-b" {
				bg = true
			}
		}
		if bg {
			if err := daemon.StartBackground(); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("daemon started in background")
		} else {
			if err := daemon.Start(context.Background()); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		}
	case "stop":
		if err := daemon.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("daemon stopped")
	case "status":
		s, err := daemon.Status()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(s)
	default:
		fmt.Printf("unknown daemon command: %s\n", args[0])
	}
}
func handleSessions(args []string) { fmt.Println("sessions: not yet implemented") }
func handleProvider(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet provider <add|list|test|remove|default>")
		return
	}
	c, err := client.EnsureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	switch args[0] {
	case "add":
		providerType := "anthropic"
		if len(args) > 1 {
			providerType = args[1]
		}
		alias := providerType
		if len(args) > 2 {
			alias = args[2]
		}
		apiKey, err := providerauth.PromptAPIKey(providerType)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		baseURL := ""
		if providerType == "ollama" || providerType == "custom" || providerType == "openai" {
			baseURL, _ = providerauth.PromptBaseURL("http://localhost:11434")
		}
		p, err := c.AddProvider(context.Background(), &pb.AddProviderReq{
			Alias:   alias,
			Type:    providerType,
			ApiKey:  apiKey,
			BaseUrl: baseURL,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Added provider: %s (%s)\n", p.Alias, p.Type)
	case "list":
		resp, err := c.ListProviders(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(resp.Providers) == 0 {
			fmt.Println("No providers configured.")
			return
		}
		fmt.Printf("%-20s %-12s %-30s %s\n", "ALIAS", "TYPE", "MODEL", "DEFAULT")
		for _, p := range resp.Providers {
			def := ""
			if p.IsDefault {
				def = "*"
			}
			fmt.Printf("%-20s %-12s %-30s %s\n", p.Alias, p.Type, p.Model, def)
		}
	case "test":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet provider test <alias>")
			return
		}
		resp, err := c.TestProvider(context.Background(), args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if resp.Success {
			fmt.Printf("OK (%dms): %s\n", resp.LatencyMs, resp.Message)
		} else {
			fmt.Printf("FAIL: %s\n", resp.Message)
		}
	case "remove":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet provider remove <alias>")
			return
		}
		if err := c.RemoveProvider(context.Background(), args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Removed provider: %s\n", args[1])
	case "default":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet provider default <alias>")
			return
		}
		if err := c.SetDefaultProvider(context.Background(), args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Set default provider: %s\n", args[1])
	default:
		fmt.Printf("unknown provider command: %s\n", args[0])
	}
}
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
