package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	acpbridge "github.com/GoCodeAlone/ratchet-cli/internal/acp"
	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
	acpsdk "github.com/coder/acp-go-sdk"
)

func runACP(_ []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	svc, err := daemon.NewService(ctx)
	if err != nil {
		return fmt.Errorf("start ratchet service: %w", err)
	}
	defer svc.Shutdown(context.Background(), nil) //nolint:errcheck

	agent := acpbridge.NewRatchetAgent(svc)

	// Stderr for logging so it doesn't interfere with the JSON-RPC protocol on stdout.
	log.SetOutput(os.Stderr)

	conn := acpsdk.NewAgentSideConnection(agent, os.Stdout, os.Stdin)
	agent.SetConnection(conn)

	log.Println("ratchet: ACP agent ready on stdio")

	// Block until the peer disconnects or we receive a signal.
	select {
	case <-conn.Done():
		log.Println("ratchet: ACP peer disconnected")
	case <-ctx.Done():
		log.Println("ratchet: shutting down on signal")
	}
	return nil
}
