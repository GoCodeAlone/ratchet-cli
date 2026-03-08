package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
)

// Result holds the output of a parsed slash command.
type Result struct {
	Lines                []string
	NavigateToOnboarding bool
}

// Parse checks if input is a slash command and executes it.
// Returns nil if input is not a command.
func Parse(input string, c *client.Client) *Result {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return nil
	}

	parts := strings.Fields(input)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/help":
		return helpCmd()
	case "/provider":
		if len(parts) < 2 {
			return &Result{Lines: []string{
				"Usage: /provider <list|add|remove|default|test> [alias]",
			}}
		}
		return providerCmd(parts[1:], c)
	default:
		return &Result{Lines: []string{
			fmt.Sprintf("Unknown command: %s — type /help for available commands", cmd),
		}}
	}
}

func helpCmd() *Result {
	return &Result{Lines: []string{
		"Available commands:",
		"  /help                      Show this help",
		"  /provider list             List configured providers",
		"  /provider add              Add a new provider (opens wizard)",
		"  /provider remove <alias>   Remove a provider",
		"  /provider default <alias>  Set default provider",
		"  /provider test <alias>     Test provider connection",
	}}
}

func providerCmd(args []string, c *client.Client) *Result {
	sub := strings.ToLower(args[0])
	switch sub {
	case "list":
		return providerList(c)
	case "add":
		return &Result{
			Lines:                []string{"Opening provider setup wizard..."},
			NavigateToOnboarding: true,
		}
	case "remove":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /provider remove <alias>"}}
		}
		return providerRemove(args[1], c)
	case "default":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /provider default <alias>"}}
		}
		return providerDefault(args[1], c)
	case "test":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /provider test <alias>"}}
		}
		return providerTest(args[1], c)
	default:
		return &Result{Lines: []string{
			fmt.Sprintf("Unknown provider command: %s", sub),
			"Available: list, add, remove, default, test",
		}}
	}
}

func providerList(c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	resp, err := c.ListProviders(context.Background())
	if err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
	}
	if len(resp.Providers) == 0 {
		return &Result{Lines: []string{"No providers configured. Use /provider add to set one up."}}
	}
	lines := []string{"Configured providers:", ""}
	for _, p := range resp.Providers {
		def := ""
		if p.IsDefault {
			def = " (default)"
		}
		lines = append(lines, fmt.Sprintf("  %-12s %-10s model=%s%s", p.Alias, p.Type, p.Model, def))
	}
	return &Result{Lines: lines}
}

func providerRemove(alias string, c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	if err := c.RemoveProvider(context.Background(), alias); err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error removing %s: %v", alias, err)}}
	}
	return &Result{Lines: []string{fmt.Sprintf("Provider %q removed.", alias)}}
}

func providerDefault(alias string, c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	if err := c.SetDefaultProvider(context.Background(), alias); err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error setting default: %v", err)}}
	}
	return &Result{Lines: []string{fmt.Sprintf("Provider %q set as default.", alias)}}
}

func providerTest(alias string, c *client.Client) *Result {
	if c == nil {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	result, err := c.TestProvider(context.Background(), alias)
	if err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error testing %s: %v", alias, err)}}
	}
	if result.Success {
		return &Result{Lines: []string{
			fmt.Sprintf("Provider %q: OK (%dms)", alias, result.LatencyMs),
		}}
	}
	return &Result{Lines: []string{
		fmt.Sprintf("Provider %q: FAILED — %s", alias, result.Message),
	}}
}
