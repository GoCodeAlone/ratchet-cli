package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	"github.com/GoCodeAlone/ratchet-cli/internal/mcp"
	"github.com/GoCodeAlone/ratchet-cli/internal/mesh"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func handleMCP(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet mcp <blackboard|daemon|config> [flags]")
		return
	}

	switch args[0] {
	case "blackboard":
		handleMCPBlackboard(args[1:])
	case "daemon":
		handleMCPDaemon(args[1:])
	case "config":
		handleMCPConfig(args[1:])
	default:
		fmt.Printf("unknown mcp command: %s\n", args[0])
	}
}

func handleMCPBlackboard(_ []string) {
	// For now, create a standalone Blackboard instance.
	// TODO: connect to daemon's shared Blackboard via Unix socket when
	// team-id flag is implemented.
	bb := mesh.NewBlackboard()

	srv := mcp.NewBBMCPServer(bb)
	if err := srv.Serve(bufio.NewReader(os.Stdin), os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "mcp server error: %v\n", err)
		os.Exit(1)
	}
}

func handleMCPDaemon(_ []string) {
	c, err := client.EnsureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	srv := mcp.NewDaemonMCPServer(mcpDaemonClient{client: c})
	if err := srv.Serve(bufio.NewReader(os.Stdin), os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "mcp server error: %v\n", err)
		os.Exit(1)
	}
}

type mcpDaemonClient struct {
	client *client.Client
}

func (c mcpDaemonClient) ListSessions() ([]*pb.Session, error) {
	resp, err := c.client.ListSessions(context.Background())
	if err != nil {
		return nil, err
	}
	return resp.Sessions, nil
}

func (c mcpDaemonClient) KillSession(id string) error {
	return c.client.KillSession(context.Background(), id)
}

func (c mcpDaemonClient) ListProjects() ([]*pb.ProjectStatus, error) {
	resp, err := c.client.ListProjects(context.Background())
	if err != nil {
		return nil, err
	}
	return resp.Projects, nil
}

func (c mcpDaemonClient) ReadBlackboard(section, key string) (*pb.BlackboardReadResp, error) {
	return c.client.BlackboardRead(context.Background(), section, key)
}

func (c mcpDaemonClient) WriteBlackboard(section, key, value string) (*pb.BlackboardEntry, error) {
	return c.client.BlackboardWrite(context.Background(), section, key, value, "mcp-client")
}

func (c mcpDaemonClient) ListBlackboard(section string) (*pb.BlackboardListResp, error) {
	return c.client.BlackboardList(context.Background(), section)
}

func (c mcpDaemonClient) ListTeams() ([]*pb.TeamStatus, error) {
	resp, err := c.client.ListTeams(context.Background(), "")
	if err != nil {
		return nil, err
	}
	return resp.Teams, nil
}

func (c mcpDaemonClient) GetTeamStatus(teamID string) (*pb.TeamStatus, error) {
	return c.client.GetTeamStatus(context.Background(), teamID)
}

func (c mcpDaemonClient) DirectMessage(teamID, toAgent, content string) error {
	return c.client.DirectMessage(context.Background(), teamID, toAgent, content)
}

func handleMCPConfig(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet mcp config <claude|copilot|generic|zed> [path] [blackboard|daemon]")
		return
	}
	format, path, target := parseMCPConfigArgs(args)
	entry, err := mcpConfigEntry(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if path == "" {
		path = defaultMCPConfigPath(format)
	}

	const serverName = "ratchet"
	switch format {
	case "claude":
		err = mcp.WriteMCPConfig(path, serverName, entry)
	case "copilot":
		err = mcp.WriteCopilotMCPConfig(path, serverName, entry)
	case "generic":
		err = mcp.WriteGenericMCPConfig(path, serverName, entry)
	case "zed":
		err = mcp.WriteZedMCPConfig(path, serverName, mcp.ZedMCPServerEntry{
			Command: entry.Command,
			Args:    entry.Args,
			Env:     map[string]string{},
		})
	default:
		fmt.Fprintf(os.Stderr, "unknown mcp config format: %s\n", format)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s MCP config: %s\n", format, path)
}

func parseMCPConfigArgs(args []string) (format, path, target string) {
	format = args[0]
	target = "daemon"
	if len(args) > 1 {
		if isMCPTarget(args[1]) {
			target = args[1]
			return format, "", target
		}
		path = args[1]
	}
	if len(args) > 2 {
		target = args[2]
	}
	return format, path, target
}

func mcpConfigEntry(target string) (mcp.MCPServerEntry, error) {
	switch target {
	case "blackboard":
		return mcp.MCPServerEntry{Command: "ratchet", Args: []string{"mcp", "blackboard"}}, nil
	case "daemon":
		return mcp.MCPServerEntry{Command: "ratchet", Args: []string{"mcp", "daemon"}}, nil
	default:
		return mcp.MCPServerEntry{}, fmt.Errorf("unknown mcp target: %s", target)
	}
}

func defaultMCPConfigPath(format string) string {
	switch format {
	case "claude":
		return filepath.Join(".claude", "mcp.json")
	case "copilot":
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, ".copilot", "mcp-config.json")
		}
		return filepath.Join(".copilot", "mcp-config.json")
	case "zed":
		return filepath.Join(".zed", "settings.json")
	default:
		return "mcp.json"
	}
}

func isMCPTarget(value string) bool {
	return value == "blackboard" || value == "daemon"
}
