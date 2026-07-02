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
	AddTrustGrant(context.Context, string, string, string) (*pb.TrustState, error)
	RevokeTrustGrant(context.Context, string, string) (*pb.TrustState, error)
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
			"  /trust grants            — show persistent grants",
			"  /trust allow \"pattern\" [--scope scope]  — add allow rule",
			"  /trust deny \"pattern\" [--scope scope]   — add deny rule",
			"  /trust persist allow \"pattern\" [--scope scope]  — add persistent allow grant",
			"  /trust persist deny \"pattern\" [--scope scope]   — add persistent deny grant",
			"  /trust revoke \"pattern\" [--scope scope] — revoke persistent grant",
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
	case "grants":
		state, err := c.GetTrustState(context.Background())
		if err != nil {
			return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
		}
		return formatTrustGrants(state.GetGrants())
	case "allow":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /trust allow \"pattern\" [--scope scope]"}}
		}
		pattern, scope, ok := parseTrustRuleArgs(args[1:])
		if !ok || pattern == "" {
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
		pattern, scope, ok := parseTrustRuleArgs(args[1:])
		if !ok || pattern == "" {
			return &Result{Lines: []string{"Usage: /trust deny \"pattern\" [--scope scope]"}}
		}
		if _, err := c.AddTrustRule(context.Background(), pattern, "deny", scope); err != nil {
			return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
		}
		return &Result{Lines: []string{fmt.Sprintf("Added deny rule: %s", pattern)}}
	case "persist":
		if len(args) < 3 || (args[1] != "allow" && args[1] != "deny") {
			return &Result{Lines: []string{"Usage: /trust persist <allow|deny> \"pattern\" [--scope scope]"}}
		}
		pattern, scope, ok := parseTrustRuleArgs(args[2:])
		if !ok || pattern == "" {
			return &Result{Lines: []string{"Usage: /trust persist <allow|deny> \"pattern\" [--scope scope]"}}
		}
		if _, err := c.AddTrustGrant(context.Background(), pattern, args[1], scope); err != nil {
			return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
		}
		return &Result{Lines: []string{fmt.Sprintf("Persisted %s grant: %s", args[1], pattern)}}
	case "revoke":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /trust revoke \"pattern\" [--scope scope]"}}
		}
		pattern, scope, ok := parseTrustRuleArgs(args[1:])
		if !ok || pattern == "" {
			return &Result{Lines: []string{"Usage: /trust revoke \"pattern\" [--scope scope]"}}
		}
		if _, err := c.RevokeTrustGrant(context.Background(), pattern, scope); err != nil {
			return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
		}
		return &Result{Lines: []string{fmt.Sprintf("Revoked persistent trust grant: %s", pattern)}}
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
	} else {
		lines = append(lines, "Trust rules:")
		for _, rule := range state.Rules {
			lines = append(lines, fmt.Sprintf("  %-7s %-12s %s", rule.Action, rule.Scope, rule.Pattern))
		}
	}
	lines = append(lines, formatTrustGrantLines(state.GetGrants())...)
	return &Result{Lines: lines}
}

func formatTrustGrants(grants []*pb.TrustGrant) *Result {
	return &Result{Lines: formatTrustGrantLines(grants)}
}

func formatTrustGrantLines(grants []*pb.TrustGrant) []string {
	if len(grants) == 0 {
		return []string{"No persistent grants configured."}
	}
	lines := []string{"Persistent grants:"}
	for _, grant := range grants {
		lines = append(lines, fmt.Sprintf("  %-7s %-12s %-10s %s", grant.Action, grant.Scope, grant.GrantedBy, grant.Pattern))
	}
	return lines
}

func parseTrustRuleArgs(args []string) (pattern, scope string, ok bool) {
	scope = "global"
	parts := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--scope" && i+1 < len(args) {
			scope = args[i+1]
			i++
			continue
		}
		if args[i] == "--scope" {
			return "", "", false
		}
		parts = append(parts, args[i])
	}
	pattern = strings.Trim(strings.Join(parts, " "), "\"")
	return pattern, scope, true
}

func isNilTrustClient(c trustClient) bool {
	if c == nil {
		return true
	}
	v := reflect.ValueOf(c)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
