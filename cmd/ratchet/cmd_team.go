package main

import (
	"context"
	"fmt"
	"os"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func handleTeam(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet team <start|status> [args...]")
		return
	}

	c, err := client.EnsureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	switch args[0] {
	case "start":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet team start \"task description\"")
			return
		}
		task := args[1]
		stream, err := c.StartTeam(context.Background(), &pb.StartTeamReq{Task: task})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		for event := range stream {
			switch e := event.Event.(type) {
			case *pb.TeamEvent_AgentSpawned:
				fmt.Printf("[spawned] %s (%s)\n", e.AgentSpawned.AgentName, e.AgentSpawned.Role)
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
	case "status":
		resp, err := c.GetTeamStatus(context.Background(), "")
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
	default:
		fmt.Printf("unknown team command: %s\n", args[0])
	}
}
