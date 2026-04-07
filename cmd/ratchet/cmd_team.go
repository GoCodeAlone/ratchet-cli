package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	"github.com/GoCodeAlone/ratchet-cli/internal/mesh"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"gopkg.in/yaml.v3"
)

func handleTeam(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet team <start|status|list|save|kill> [args...]")
		return
	}

	switch args[0] {
	case "start":
		handleTeamStart(args[1:])
	case "status":
		handleTeamStatus(args[1:])
	case "list":
		handleTeamList()
	case "save":
		handleTeamSave(args[1:])
	case "kill":
		handleTeamKill(args[1:])
	default:
		fmt.Printf("unknown team command: %s\n", args[0])
	}
}

func handleTeamStart(args []string) {
	var (
		agentFlags   []string
		teamName     string
		bbMode       string
		orchestrator string
		configName   string
		task         string
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--agent":
			if i+1 < len(args) {
				agentFlags = append(agentFlags, args[i+1])
				i++
			}
		case "--agents":
			if i+1 < len(args) {
				for _, a := range strings.Split(args[i+1], ",") {
					agentFlags = append(agentFlags, strings.TrimSpace(a))
				}
				i++
			}
		case "--name":
			if i+1 < len(args) {
				teamName = args[i+1]
				i++
			}
		case "--bb":
			if i+1 < len(args) {
				bbMode = args[i+1]
				i++
			}
		case "--orchestrator":
			if i+1 < len(args) {
				orchestrator = args[i+1]
				i++
			}
		case "--config":
			if i+1 < len(args) {
				configName = args[i+1]
				i++
			}
		case "--task":
			if i+1 < len(args) {
				task = args[i+1]
				i++
			}
		default:
			// Positional: could be config name or task.
			if configName == "" && task == "" && isTeamConfig(args[i]) {
				configName = args[i]
			} else if task == "" {
				task = args[i]
			}
		}
	}

	// Build team config from --agent flags if no --config.
	if configName == "" && len(agentFlags) > 0 {
		tc, err := mesh.BuildTeamConfigFromFlags(teamName, agentFlags, orchestrator, bbMode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Team: %s (%d agents, bb=%s)\n", tc.Name, len(tc.Agents), bbMode)
		for _, a := range tc.Agents {
			prov := a.Provider
			if prov == "" {
				prov = "(default)"
			}
			fmt.Printf("  • %s — %s", a.Name, prov)
			if a.Model != "" {
				fmt.Printf("/%s", a.Model)
			}
			if a.Role == "orchestrator" {
				fmt.Print(" [orchestrator]")
			}
			fmt.Println()
		}
		fmt.Println()

		// Serialize to a temp YAML and pass as config name.
		configName = writeTemporaryTeamConfig(tc)
	}

	if task == "" {
		fmt.Println("Usage: ratchet team start [--agent name:provider[:model]]... [--config name] \"task\"")
		return
	}

	c, err := client.EnsureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	if configName != "" {
		tc, err := resolveTeamConfig(configName)
		if err == nil {
			fmt.Printf("Using team config: %s (%d agents)\n", tc.Name, len(tc.Agents))
			for _, a := range tc.Agents {
				fmt.Printf("  • %s (%s) — %s/%s\n", a.Name, a.Role, a.Provider, a.Model)
			}
			fmt.Println()
		}
	}

	stream, err := c.StartTeam(context.Background(), &pb.StartTeamReq{
		Task:           task,
		TeamConfigName: configName,
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

func writeTemporaryTeamConfig(tc *mesh.TeamConfig) string {
	data, err := yaml.Marshal(tc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling team config: %v\n", err)
		os.Exit(1)
	}
	tmpFile, err := os.CreateTemp("", "ratchet-team-*.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating temp file: %v\n", err)
		os.Exit(1)
	}
	if _, err := tmpFile.Write(data); err != nil {
		fmt.Fprintf(os.Stderr, "error writing temp file: %v\n", err)
		os.Exit(1)
	}
	tmpFile.Close()
	return tmpFile.Name()
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

func handleTeamSave(args []string) {
	var (
		name       string
		agentFlags []string
		outputPath string
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--agent":
			if i+1 < len(args) {
				agentFlags = append(agentFlags, args[i+1])
				i++
			}
		case "--output":
			if i+1 < len(args) {
				outputPath = args[i+1]
				i++
			}
		default:
			if name == "" {
				name = args[i]
			}
		}
	}

	if name == "" || len(agentFlags) == 0 {
		fmt.Println("Usage: ratchet team save <name> --agent name:provider[:model] [--output path]")
		return
	}

	tc, err := mesh.BuildTeamConfigFromFlags(name, agentFlags, "", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if outputPath == "" {
		dir := filepath.Join(".", ".ratchet", "teams")
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "error creating dir: %v\n", err)
			os.Exit(1)
		}
		outputPath = filepath.Join(dir, name+".yaml")
	}

	data, err := yaml.Marshal(tc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Saved team config to %s\n", outputPath)
}

func handleTeamKill(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: ratchet team kill <team-id>")
		return
	}
	// TODO: Wire to KillTeam RPC once it exists.
	fmt.Printf("team kill %s: not yet implemented\n", args[0])
}
