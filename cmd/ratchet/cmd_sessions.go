package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type sessionsClient interface {
	Close() error
	ListSessions(context.Context) (*pb.SessionList, error)
	KillSession(context.Context, string) error
	ListSessionMessages(context.Context, string) (*pb.SessionHistory, error)
	CloneSession(context.Context, string, string) (*pb.Session, error)
	ForkSession(context.Context, string, string, string) (*pb.Session, error)
	GetSessionTree(context.Context, string) (*pb.SessionList, error)
}

var ensureSessionsClient = func() (sessionsClient, error) {
	return client.EnsureDaemon()
}

func handleSessions(args []string) {
	if len(args) == 0 {
		printSessionsUsage()
		return
	}

	c, err := ensureSessionsClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	switch args[0] {
	case "list":
		resp, err := c.ListSessions(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(resp.Sessions) == 0 {
			fmt.Println("No sessions.")
			return
		}
		fmt.Printf("%-36s %-10s %-20s %s\n", "ID", "STATUS", "PROVIDER", "WORKING_DIR")
		for _, s := range resp.Sessions {
			fmt.Printf("%-36s %-10s %-20s %s\n", s.Id, s.Status, s.Provider, s.WorkingDir)
		}
	case "kill":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet sessions kill <id>")
			return
		}
		if err := c.KillSession(context.Background(), args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Killed session: %s\n", args[1])
	case "history":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet sessions history <id>")
			return
		}
		resp, err := c.ListSessionMessages(context.Background(), args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(resp.Messages) == 0 {
			fmt.Println("No messages.")
			return
		}
		fmt.Printf("%-36s %-10s %-25s %s\n", "MESSAGE_ID", "ROLE", "TIMESTAMP", "CONTENT")
		for _, msg := range resp.Messages {
			fmt.Printf("%-36s %-10s %-25s %s\n", msg.Id, msg.Role, formatTimestamp(msg.Timestamp), msg.Content)
		}
	case "clone":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet sessions clone <id>")
			return
		}
		session, err := c.CloneSession(context.Background(), args[1], "manual clone")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Cloned session: %s\n", session.Id)
		fmt.Printf("Parent: %s\nRoot: %s\n", session.ParentId, session.RootId)
	case "fork":
		sessionID, messageID, ok := parseForkArgs(args[1:])
		if !ok {
			fmt.Println("Usage: ratchet sessions fork <id> --at <message-id>")
			return
		}
		session, err := c.ForkSession(context.Background(), sessionID, messageID, "manual fork")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Forked session: %s\n", session.Id)
		fmt.Printf("Parent: %s\nRoot: %s\nForkedFrom: %s\n", session.ParentId, session.RootId, session.ForkedFromMessageId)
	case "tree":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet sessions tree <id>")
			return
		}
		resp, err := c.GetSessionTree(context.Background(), args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(resp.Sessions) == 0 {
			fmt.Println("No sessions.")
			return
		}
		fmt.Printf("%-36s %-10s %-36s %-36s %-36s\n", "SESSION_ID", "STATUS", "PARENT_ID", "ROOT_ID", "FORKED_FROM")
		for _, s := range resp.Sessions {
			fmt.Printf("%-36s %-10s %-36s %-36s %-36s\n", s.Id, s.Status, s.ParentId, s.RootId, s.ForkedFromMessageId)
		}
	default:
		fmt.Printf("unknown sessions command: %s\n", args[0])
	}
}

func printSessionsUsage() {
	fmt.Println("Usage: ratchet sessions <list|kill|history|clone|fork|tree>")
}

func parseForkArgs(args []string) (sessionID, messageID string, ok bool) {
	if len(args) != 3 || args[1] != "--at" {
		return "", "", false
	}
	return args[0], args[2], args[0] != "" && args[2] != ""
}

func formatTimestamp(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().Format(time.RFC3339)
}
