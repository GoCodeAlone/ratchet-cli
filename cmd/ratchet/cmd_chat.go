package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func handleChat(args []string) {
	// args[0] may be "chat" — skip it if present
	if len(args) > 0 && args[0] == "chat" {
		args = args[1:]
	}
	if len(args) == 0 {
		// Fall back to interactive TUI
		if err := runInteractive(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	prompt := strings.Join(args, " ")
	handleOneShot(prompt)
}

func handleOneShot(prompt string) {
	ctx := context.Background()
	c, err := client.EnsureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	wd, _ := os.Getwd()
	session, err := c.CreateSession(ctx, &pb.CreateSessionReq{
		WorkingDir:    wd,
		InitialPrompt: prompt,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating session: %v\n", err)
		os.Exit(1)
	}

	events, err := c.SendMessage(ctx, session.Id, prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error sending message: %v\n", err)
		os.Exit(1)
	}

	for event := range events {
		switch e := event.Event.(type) {
		case *pb.ChatEvent_Token:
			fmt.Print(e.Token.Content)
		case *pb.ChatEvent_Complete:
			fmt.Println()
		case *pb.ChatEvent_Error:
			fmt.Fprintf(os.Stderr, "\nerror: %s\n", e.Error.Message)
			os.Exit(1)
		}
	}
}
