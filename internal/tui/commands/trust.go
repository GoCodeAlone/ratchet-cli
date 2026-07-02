package commands

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

type trustClient interface {
	GetTrustState(context.Context) (*pb.TrustState, error)
	SetTrustMode(context.Context, string) (*pb.TrustState, error)
	AddTrustRule(context.Context, string, string, string) (*pb.TrustState, error)
	ResetTrust(context.Context) (*pb.TrustState, error)
}

func modeCmd(args []string, c trustClient) *Result {
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
	if isNilTrustClient(c) {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}
	state, err := c.SetTrustMode(context.Background(), mode)
	if err != nil {
		return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
	}
	return &Result{Lines: []string{fmt.Sprintf("Mode switched to %s", state.Mode)}}
}

func trustCmd(args []string, c trustClient) *Result {
	if len(args) == 0 {
		return &Result{Lines: []string{
			"Usage:",
			"  /trust list              — show active rules",
			"  /trust allow \"pattern\" [--scope scope]  — add allow rule",
			"  /trust deny \"pattern\" [--scope scope]   — add deny rule",
			"  /trust reset             — revert to config defaults",
		}}
	}
	if isNilTrustClient(c) {
		return &Result{Lines: []string{"Not connected to daemon"}}
	}

	switch args[0] {
	case "list":
		state, err := c.GetTrustState(context.Background())
		if err != nil {
			return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
		}
		return formatTrustState(state)
	case "allow":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /trust allow \"pattern\" [--scope scope]"}}
		}
		pattern, scope := parseTrustRuleArgs(args[1:])
		if pattern == "" {
			return &Result{Lines: []string{"Usage: /trust allow \"pattern\" [--scope scope]"}}
		}
		if _, err := c.AddTrustRule(context.Background(), pattern, "allow", scope); err != nil {
			return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
		}
		return &Result{Lines: []string{fmt.Sprintf("Added allow rule: %s", pattern)}}
	case "deny":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /trust deny \"pattern\" [--scope scope]"}}
		}
		pattern, scope := parseTrustRuleArgs(args[1:])
		if pattern == "" {
			return &Result{Lines: []string{"Usage: /trust deny \"pattern\" [--scope scope]"}}
		}
		if _, err := c.AddTrustRule(context.Background(), pattern, "deny", scope); err != nil {
			return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
		}
		return &Result{Lines: []string{fmt.Sprintf("Added deny rule: %s", pattern)}}
	case "reset":
		state, err := c.ResetTrust(context.Background())
		if err != nil {
			return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
		}
		return &Result{Lines: []string{
			fmt.Sprintf("Trust rules reset to config defaults. Mode: %s", state.Mode),
		}}
	default:
		return &Result{Lines: []string{fmt.Sprintf("Unknown trust subcommand: %s", args[0])}}
	}
}

func formatTrustState(state *pb.TrustState) *Result {
	if state == nil {
		return &Result{Lines: []string{"Mode: unknown", "No trust rules configured."}}
	}
	lines := []string{fmt.Sprintf("Mode: %s", state.Mode)}
	if len(state.Rules) == 0 {
		lines = append(lines, "No trust rules configured.")
		return &Result{Lines: lines}
	}
	lines = append(lines, "Trust rules:")
	for _, rule := range state.Rules {
		lines = append(lines, fmt.Sprintf("  %-7s %-12s %s", rule.Action, rule.Scope, rule.Pattern))
	}
	return &Result{Lines: lines}
}

func parseTrustRuleArgs(args []string) (pattern, scope string) {
	scope = "global"
	parts := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--scope" && i+1 < len(args) {
			scope = args[i+1]
			i++
			continue
		}
		parts = append(parts, args[i])
	}
	pattern = strings.Trim(strings.Join(parts, " "), "\"")
	return pattern, scope
}

func isNilTrustClient(c trustClient) bool {
	if c == nil {
		return true
	}
	v := reflect.ValueOf(c)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
