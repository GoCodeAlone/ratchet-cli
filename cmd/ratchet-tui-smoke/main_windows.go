//go:build tui_smoke && windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ratchet-tui-smoke: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tempRoot, err := os.MkdirTemp("", "ratchet-tui-smoke-*")
	if err != nil {
		return fmt.Errorf("create temp root: %w", err)
	}
	defer os.RemoveAll(tempRoot)

	session, target, cleanup, err := daemon.StartTUISmokeDaemonLoopback(ctx, tempRoot)
	if err != nil {
		return err
	}
	defer cleanup()

	c, err := client.ConnectSmokeLoopback(ctx, target)
	if err != nil {
		return err
	}
	defer c.Close()

	return tui.Run(ctx, c, session)
}
