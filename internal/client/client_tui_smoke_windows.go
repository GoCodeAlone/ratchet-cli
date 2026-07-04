//go:build tui_smoke && windows

package client

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// ConnectSmokeLoopback creates a client for the Windows test-only TUI smoke daemon.
func ConnectSmokeLoopback(_ context.Context, target string) (*Client, error) {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return nil, fmt.Errorf("parse smoke loopback target: %w", err)
	}
	if host != "127.0.0.1" {
		return nil, fmt.Errorf("smoke loopback target must use 127.0.0.1, got %q", host)
	}
	if parsed, err := strconv.Atoi(port); err != nil || parsed <= 0 || parsed > 65535 {
		return nil, fmt.Errorf("smoke loopback target has invalid port %q", port)
	}
	conn, err := grpc.NewClient(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("connect smoke daemon at %s: %w", target, err)
	}
	return &Client{
		conn:   conn,
		daemon: pb.NewRatchetDaemonClient(conn),
	}, nil
}
