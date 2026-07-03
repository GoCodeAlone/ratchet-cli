//go:build tui_smoke && !windows

package client

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectSmokeUnixRejectsUnsafeSocketPaths(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	socketPath := filepath.Join(root, "ratchet.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	if err := os.Chmod(socketPath, 0600); err != nil {
		t.Fatalf("chmod socket: %v", err)
	}

	c, err := ConnectSmokeUnix(ctx, root, socketPath)
	if err != nil {
		t.Fatalf("ConnectSmokeUnix valid socket: %v", err)
	}
	_ = c.Close()

	outside := filepath.Join(t.TempDir(), "outside.sock")
	outsideLn, err := net.Listen("unix", outside)
	if err != nil {
		t.Fatalf("listen outside socket: %v", err)
	}
	t.Cleanup(func() { _ = outsideLn.Close() })
	if err := os.Chmod(outside, 0600); err != nil {
		t.Fatalf("chmod outside socket: %v", err)
	}

	cases := []struct {
		name       string
		tempRoot   string
		socketPath string
		want       string
	}{
		{name: "outside temp root", tempRoot: root, socketPath: outside, want: "inside temp root"},
		{name: "tcp address", tempRoot: root, socketPath: "tcp://127.0.0.1:1", want: "unix socket path"},
		{name: "unresolved parent", tempRoot: root, socketPath: filepath.Join(root, "missing", "ratchet.sock"), want: "resolve socket parent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := ConnectSmokeUnix(ctx, tc.tempRoot, tc.socketPath)
			if c != nil {
				_ = c.Close()
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
	}
}

func TestConnectSmokeUnixRejectsSymlinkAndBadModes(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	regular := filepath.Join(root, "regular.sock")
	if err := os.WriteFile(regular, []byte("not a socket"), 0600); err != nil {
		t.Fatalf("write regular file: %v", err)
	}
	assertSmokeConnectError(t, ctx, root, regular, "unix socket")

	openSocket := filepath.Join(root, "open.sock")
	ln, err := net.Listen("unix", openSocket)
	if err != nil {
		t.Fatalf("listen open socket: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	if err := os.Chmod(openSocket, 0666); err != nil {
		t.Fatalf("chmod open socket: %v", err)
	}
	assertSmokeConnectError(t, ctx, root, openSocket, "0600")

	target := filepath.Join(root, "target.sock")
	targetLn, err := net.Listen("unix", target)
	if err != nil {
		t.Fatalf("listen target socket: %v", err)
	}
	t.Cleanup(func() { _ = targetLn.Close() })
	if err := os.Chmod(target, 0600); err != nil {
		t.Fatalf("chmod target socket: %v", err)
	}
	link := filepath.Join(root, "link.sock")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink socket: %v", err)
	}
	assertSmokeConnectError(t, ctx, root, link, "symlink")
}

func assertSmokeConnectError(t *testing.T, ctx context.Context, root, socketPath, want string) {
	t.Helper()
	c, err := ConnectSmokeUnix(ctx, root, socketPath)
	if c != nil {
		_ = c.Close()
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("ConnectSmokeUnix(%q): expected %q error, got %v", socketPath, want, err)
	}
}
