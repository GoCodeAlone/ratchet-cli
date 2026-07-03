//go:build tui_smoke && !windows

package client

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// ConnectSmokeUnix creates a client for the test-only TUI smoke daemon socket.
func ConnectSmokeUnix(_ context.Context, tempRoot, socketPath string) (*Client, error) {
	if strings.Contains(socketPath, "://") {
		return nil, fmt.Errorf("smoke client requires unix socket path, got %q", socketPath)
	}

	root, err := filepath.Abs(tempRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve temp root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve temp root: %w", err)
	}

	socketAbs, err := filepath.Abs(socketPath)
	if err != nil {
		return nil, fmt.Errorf("resolve socket path: %w", err)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(socketAbs))
	if err != nil {
		return nil, fmt.Errorf("resolve socket parent: %w", err)
	}
	socketAbs = filepath.Join(parent, filepath.Base(socketAbs))

	rel, err := filepath.Rel(root, socketAbs)
	if err != nil {
		return nil, fmt.Errorf("compare socket path to temp root: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("smoke socket must be inside temp root %s: %s", root, socketAbs)
	}

	info, err := os.Lstat(socketAbs)
	if err != nil {
		return nil, fmt.Errorf("stat smoke socket: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("smoke socket final component must not be a symlink: %s", socketAbs)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return nil, fmt.Errorf("smoke path is not a unix socket: %s", socketAbs)
	}
	if info.Mode().Perm() != 0600 {
		return nil, fmt.Errorf("smoke socket permissions must be 0600, got %04o", info.Mode().Perm())
	}

	conn, err := grpc.NewClient(
		"unix://"+socketAbs,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("connect smoke daemon at %s: %w", socketAbs, err)
	}
	return &Client{
		conn:   conn,
		daemon: pb.NewRatchetDaemonClient(conn),
	}, nil
}
