package commands

import (
	"fmt"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
)

func modeCmd(args []string, c *client.Client) *Result {
	if len(args) == 0 {
		return &Result{Lines: []string{
			"Usage: /mode <conservative|permissive|locked|sandbox|custom>",
			"Switches the active trust mode. Affects all new tool calls.",
		}}
	}
	mode := args[0]
	valid := map[string]bool{
		"conservative": true,
		"permissive":   true,
		"locked":       true,
		"sandbox":      true,
		"custom":       true,
	}
	if !valid[mode] {
		return &Result{Lines: []string{
			fmt.Sprintf("Unknown mode %q. Valid: conservative, permissive, locked, sandbox, custom", mode),
		}}
	}
	// TODO: Call daemon RPC to switch mode (requires SetMode RPC).
	return &Result{Lines: []string{fmt.Sprintf("Mode switched to %s", mode)}}
}

func trustCmd(args []string, c *client.Client) *Result {
	if len(args) == 0 {
		return &Result{Lines: []string{
			"Usage:",
			"  /trust list              — show active rules",
			"  /trust allow \"pattern\"  — add allow rule",
			"  /trust deny \"pattern\"   — add deny rule",
			"  /trust reset             — revert to config defaults",
		}}
	}

	switch args[0] {
	case "list":
		// TODO: Call daemon RPC to list trust rules.
		return &Result{Lines: []string{"Trust rules: (call daemon for live list)"}}
	case "allow":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /trust allow \"pattern\""}}
		}
		pattern := strings.Trim(strings.Join(args[1:], " "), "\"")
		// TODO: Call daemon RPC to add allow rule.
		return &Result{Lines: []string{fmt.Sprintf("Added allow rule: %s", pattern)}}
	case "deny":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /trust deny \"pattern\""}}
		}
		pattern := strings.Trim(strings.Join(args[1:], " "), "\"")
		// TODO: Call daemon RPC to add deny rule.
		return &Result{Lines: []string{fmt.Sprintf("Added deny rule: %s", pattern)}}
	case "reset":
		// TODO: Call daemon RPC to reset trust rules.
		return &Result{Lines: []string{"Trust rules reset to config defaults."}}
	default:
		return &Result{Lines: []string{fmt.Sprintf("Unknown trust subcommand: %s", args[0])}}
	}
}
