//go:build tui_smoke && windows

package main

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/http"
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
	if err := configureSmokeTrust(); err != nil {
		return err
	}

	tempRoot, removeRoot, err := smokeRoot()
	if err != nil {
		return fmt.Errorf("create temp root: %w", err)
	}
	defer removeRoot()

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

func smokeRoot() (string, func(), error) {
	if root := os.Getenv("RATCHET_TUI_SMOKE_ROOT"); root != "" {
		if err := os.MkdirAll(root, 0o700); err != nil {
			return "", func() {}, err
		}
		return root, func() {}, nil
	}
	root, err := os.MkdirTemp("", "ratchet-tui-smoke-*")
	if err != nil {
		return "", func() {}, err
	}
	return root, func() { _ = os.RemoveAll(root) }, nil
}

func configureSmokeTrust() error {
	certFile := os.Getenv("RATCHET_TUI_SMOKE_CA_FILE")
	if certFile == "" {
		return nil
	}
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return fmt.Errorf("read smoke CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certPEM) {
		return fmt.Errorf("load smoke CA: no certificates found")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig.RootCAs = roots
	http.DefaultTransport = transport
	return nil
}
