package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	acpbridge "github.com/GoCodeAlone/ratchet-cli/internal/acp"
	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
	acpsdk "github.com/coder/acp-go-sdk"
)

func runACP(args []string) error {
	return runACPWithArgs(args)
}

func runACPWithArgs(args []string) error {
	if len(args) > 0 {
		switch {
		case args[0] == "client":
			return handleACPClient(args[1:])
		case args[0] == "config":
			return handleACPConfig(args[1:])
		case isHelpArg(args[0]):
			printACPUsage(os.Stdout)
			return nil
		default:
			return fmt.Errorf("unknown acp command: %s", args[0])
		}
	}

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

func printACPUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: ratchet acp [client] [command]

Commands:
  (default)   Run ratchet as an ACP agent over stdio JSON-RPC
  client      Spawn and control external ACP-compatible agents
  config      Write client configuration snippets

Run 'ratchet acp client --help' for client commands.
Run 'ratchet acp config zed [.zed/settings.json]' to add ratchet as a Zed custom ACP agent.
`)
}

func handleACPConfig(args []string) error {
	if len(args) == 0 || isHelpArg(args[0]) {
		fmt.Println("Usage: ratchet acp config zed [path]")
		return nil
	}

	format := args[0]
	path := ""
	if len(args) > 1 {
		path = args[1]
	}
	if path == "" {
		path = defaultACPConfigPath(format)
	}

	switch format {
	case "zed":
		err := acpbridge.WriteZedACPConfig(path, "ratchet", acpbridge.ZedACPAgentServer{
			Type:    "custom",
			Command: "ratchet",
			Args:    []string{"acp"},
			Env:     map[string]string{},
		})
		if err != nil {
			return fmt.Errorf("write zed ACP config: %w", err)
		}
	default:
		return fmt.Errorf("unknown acp config format: %s", format)
	}
	fmt.Printf("wrote %s ACP config: %s\n", format, path)
	return nil
}

func defaultACPConfigPath(format string) string {
	switch format {
	case "zed":
		return filepath.Join(".zed", "settings.json")
	default:
		return "acp.json"
	}
}
