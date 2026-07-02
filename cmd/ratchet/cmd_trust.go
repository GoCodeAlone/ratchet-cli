package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

type trustCLIClient interface {
	Close() error
	GetTrustState(context.Context) (*pb.TrustState, error)
	AddTrustRule(context.Context, string, string, string) (*pb.TrustState, error)
	ResetTrust(context.Context) (*pb.TrustState, error)
	AddTrustGrant(context.Context, string, string, string) (*pb.TrustState, error)
	RevokeTrustGrant(context.Context, string, string) (*pb.TrustState, error)
}

var ensureTrustClient = func() (trustCLIClient, error) {
	return client.EnsureDaemon()
}

func handleTrust(args []string) {
	if len(args) == 0 {
		printTrustUsage()
		return
	}

	switch args[0] {
	case "allow", "deny":
		handleRuntimeTrustRule(args)
	case "list", "grants", "reset", "persist", "revoke":
		handlePersistentTrustCommand(args)
	default:
		fmt.Printf("unknown trust command: %s\n", args[0])
		printTrustUsage()
	}
}

func handleRuntimeTrustRule(args []string) {
	pattern, scope, ok := parseTrustPatternScope(args[1:])
	if !ok || pattern == "" {
		fmt.Printf("Usage: ratchet trust %s \"pattern\" [--scope scope]\n", args[0])
		return
	}
	c, err := ensureTrustClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()
	if _, err := c.AddTrustRule(context.Background(), pattern, args[0], scope); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Added runtime %s rule: %s (scope: %s)\n", args[0], pattern, scope)
}

func handlePersistentTrustCommand(args []string) {
	c, err := ensureTrustClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	switch args[0] {
	case "list":
		state, err := c.GetTrustState(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		printTrustState(state)
	case "grants":
		state, err := c.GetTrustState(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		printTrustGrants(state.GetGrants())
	case "reset":
		state, err := c.ResetTrust(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Trust rules reset to config defaults. Mode: %s\n", state.GetMode())
	case "persist":
		if len(args) < 3 || (args[1] != "allow" && args[1] != "deny") {
			fmt.Println("Usage: ratchet trust persist <allow|deny> \"pattern\" [--scope scope]")
			return
		}
		pattern, scope, ok := parseTrustPatternScope(args[2:])
		if !ok || pattern == "" {
			fmt.Println("Usage: ratchet trust persist <allow|deny> \"pattern\" [--scope scope]")
			return
		}
		if _, err := c.AddTrustGrant(context.Background(), pattern, args[1], scope); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Persisted %s grant: %s (scope: %s)\n", args[1], pattern, scope)
	case "revoke":
		pattern, scope, ok := parseTrustPatternScope(args[1:])
		if !ok || pattern == "" {
			fmt.Println("Usage: ratchet trust revoke \"pattern\" [--scope scope]")
			return
		}
		if _, err := c.RevokeTrustGrant(context.Background(), pattern, scope); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Revoked persistent trust grant: %s (scope: %s)\n", pattern, scope)
	}
}

func parseTrustPatternScope(args []string) (pattern, scope string, ok bool) {
	scope = "global"
	parts := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--scope" {
			if i+1 >= len(args) {
				return "", "", false
			}
			scope = args[i+1]
			i++
			continue
		}
		parts = append(parts, args[i])
	}
	pattern = strings.Trim(strings.Join(parts, " "), "\"")
	return pattern, scope, true
}

func printTrustState(state *pb.TrustState) {
	if state == nil {
		fmt.Println("Mode: unknown")
		fmt.Println("No runtime rules configured.")
		fmt.Println("No persistent grants configured.")
		return
	}
	fmt.Printf("Mode: %s\n", state.GetMode())
	printRuntimeTrustRules(state.GetRules())
	printTrustGrants(state.GetGrants())
}

func printRuntimeTrustRules(rules []*pb.TrustRule) {
	if len(rules) == 0 {
		fmt.Println("No runtime rules configured.")
		return
	}
	fmt.Println("Runtime rules:")
	fmt.Printf("%-7s %-12s %s\n", "ACTION", "SCOPE", "PATTERN")
	for _, rule := range rules {
		fmt.Printf("%-7s %-12s %s\n", rule.GetAction(), rule.GetScope(), rule.GetPattern())
	}
}

func printTrustGrants(grants []*pb.TrustGrant) {
	if len(grants) == 0 {
		fmt.Println("No persistent grants configured.")
		return
	}
	fmt.Println("Persistent grants:")
	fmt.Printf("%-4s %-7s %-12s %-10s %-25s %s\n", "ID", "ACTION", "SCOPE", "GRANTED_BY", "CREATED_AT", "PATTERN")
	for _, grant := range grants {
		fmt.Printf("%-4d %-7s %-12s %-10s %-25s %s\n",
			grant.GetId(),
			grant.GetAction(),
			grant.GetScope(),
			grant.GetGrantedBy(),
			formatTimestamp(grant.GetCreatedAt()),
			grant.GetPattern(),
		)
	}
}

func printTrustUsage() {
	fmt.Println(`Usage: ratchet trust <command>

Commands:
  list                                      Show runtime rules and persistent grants
  grants                                    Show persistent grants
  allow "pattern" [--scope scope]           Add a runtime allow rule
  deny "pattern" [--scope scope]            Add a runtime deny rule
  persist <allow|deny> "pattern" [--scope scope]
                                            Add a persistent grant
  revoke "pattern" [--scope scope]          Revoke a persistent grant
  reset                                     Reset runtime rules to config defaults`)
}
