package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	"github.com/GoCodeAlone/ratchet-cli/internal/mesh"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func handleTeam(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet team <start|status|list> [args...]")
		return
	}

	switch args[0] {
	case "start":
		handleTeamStart(args[1:])
	case "status":
		handleTeamStatus(args[1:])
	case "list":
		handleTeamList()
	default:
		fmt.Printf("unknown team command: %s\n", args[0])
	}
}

func handleTeamStart(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: ratchet team start [<name|yaml-path>] --task \"description\"")
		return
	}

	// Parse args: first positional is either a team name/path or the task description.
	// If --task flag is present, the first positional is the team config identifier.
	var teamConfigName string
	var task string

	for i := 0; i < len(args); i++ {
		if args[i] == "--task" && i+1 < len(args) {
			task = args[i+1]
			i++
		} else if teamConfigName == "" {
			teamConfigName = args[i]
		}
	}

	// If no --task flag, treat the first positional as the task (backward compatible).
	if task == "" && teamConfigName != "" {
		// Check if teamConfigName matches a builtin or file path.
		if !isTeamConfig(teamConfigName) {
			task = teamConfigName
			teamConfigName = ""
		}
	}

	if task == "" {
		fmt.Println("Usage: ratchet team start [<name|yaml-path>] --task \"description\"")
		return
	}

	c, err := client.EnsureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	// If a team config is specified, display it and proceed with normal start.
	if teamConfigName != "" {
		tc, err := resolveTeamConfig(teamConfigName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Using team config: %s (%d agents)\n", tc.Name, len(tc.Agents))
		for _, a := range tc.Agents {
			fmt.Printf("  • %s (%s) — %s/%s\n", a.Name, a.Role, a.Provider, a.Model)
		}
		fmt.Println()
	}

	stream, err := c.StartTeam(context.Background(), &pb.StartTeamReq{
		Task:           task,
		TeamConfigName: teamConfigName,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	for event := range stream {
		switch e := event.Event.(type) {
		case *pb.TeamEvent_AgentSpawned:
			if e.AgentSpawned.AgentName == "__team__" {
				fmt.Printf("Team ID: %s\n", e.AgentSpawned.AgentId)
				fmt.Printf("(Use 'ratchet team status %s' to check status)\n\n", e.AgentSpawned.AgentId)
			} else {
				fmt.Printf("[spawned] %s (%s)\n", e.AgentSpawned.AgentName, e.AgentSpawned.Role)
			}
		case *pb.TeamEvent_Token:
			fmt.Print(e.Token.Content)
		case *pb.TeamEvent_AgentMessage:
			fmt.Printf("[%s → %s] %s\n", e.AgentMessage.FromAgent, e.AgentMessage.ToAgent, e.AgentMessage.Content)
		case *pb.TeamEvent_Complete:
			fmt.Printf("\nTeam complete: %s\n", e.Complete.Summary)
		case *pb.TeamEvent_Error:
			fmt.Fprintf(os.Stderr, "error: %s\n", e.Error.Message)
		}
	}
}

// isTeamConfig returns true if the name matches a builtin config or is a YAML file path.
func isTeamConfig(name string) bool {
	builtins, err := mesh.BuiltinTeamConfigs()
	if err == nil {
		if _, ok := builtins[name]; ok {
			return true
		}
	}
	if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
		if _, err := os.Stat(name); err == nil {
			return true
		}
	}
	return false
}

// resolveTeamConfig loads a team config by builtin name or file path.
func resolveTeamConfig(name string) (*mesh.TeamConfig, error) {
	builtins, err := mesh.BuiltinTeamConfigs()
	if err == nil {
		if tc, ok := builtins[name]; ok {
			return tc, nil
		}
	}
	return mesh.LoadTeamConfig(name)
}

func handleTeamStatus(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: ratchet team status <team-id>")
		return
	}
	teamID := args[0]

	c, err := client.EnsureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	resp, err := c.GetTeamStatus(context.Background(), teamID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Team: %s  Status: %s  Task: %s\n", resp.TeamId, resp.Status, resp.Task)
	if len(resp.Agents) > 0 {
		fmt.Printf("%-20s %-10s %s\n", "NAME", "STATUS", "MODEL")
		for _, a := range resp.Agents {
			fmt.Printf("%-20s %-10s %s\n", a.Name, a.Status, a.Model)
		}
	}
}

func handleTeamList() {
	// List builtin team configs (no daemon connection needed).
	fmt.Println("Built-in team configs:")
	builtins, err := mesh.BuiltinTeamConfigs()
	if err == nil {
		names := make([]string, 0, len(builtins))
		for name := range builtins {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			tc := builtins[name]
			fmt.Printf("  %-16s %d agents  timeout: %s\n", name, len(tc.Agents), tc.Timeout)
		}
	}
	fmt.Println()

	// Active team listing requires a dedicated ListTeams RPC which is not yet
	// implemented. Direct GetTeamStatus("") calls always return NotFound.
	fmt.Println("Active team listing is not available via this command.")
	fmt.Println("Use `ratchet team status <team-id>` for a known team ID.")
}
