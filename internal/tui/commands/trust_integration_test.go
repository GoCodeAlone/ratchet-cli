package commands

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"google.golang.org/grpc"
)

func TestTrustCommandsAgainstDaemon(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	daemon.EnsureDataDir()
	sock := daemon.SocketPath()
	_ = os.Remove(sock)

	lis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = lis.Close()
		_ = os.Remove(sock)
	})

	svc, err := daemon.NewService(context.Background())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	server := grpc.NewServer()
	pb.RegisterRatchetDaemonServer(server, svc)
	t.Cleanup(server.Stop)
	go func() {
		_ = server.Serve(lis)
	}()

	c, err := client.Connect()
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	result := Parse("/mode locked", c)
	if !resultContains(result, "Mode switched to locked") {
		t.Fatalf("/mode result = %v", result.Lines)
	}

	result = Parse(`/trust allow "bash:go test *" --scope repo`, c)
	if !resultContains(result, "Added allow rule: bash:go test *") {
		t.Fatalf("/trust allow result = %v", result.Lines)
	}

	result = Parse("/trust list", c)
	joined := strings.Join(result.Lines, "\n")
	if !strings.Contains(joined, "Mode: locked") || !strings.Contains(joined, "bash:go test *") || !strings.Contains(joined, "repo") {
		t.Fatalf("/trust list result = %v", result.Lines)
	}

	result = Parse(`/trust persist allow "bash:go vet *" --scope repo`, c)
	if !resultContains(result, "Persisted allow grant: bash:go vet *") {
		t.Fatalf("/trust persist result = %v", result.Lines)
	}

	result = Parse("/trust grants", c)
	joined = strings.Join(result.Lines, "\n")
	if !strings.Contains(joined, "Persistent grants:") || !strings.Contains(joined, "bash:go vet *") || !strings.Contains(joined, "repo") {
		t.Fatalf("/trust grants result = %v", result.Lines)
	}

	result = Parse(`/trust revoke "bash:go vet *" --scope repo`, c)
	if !resultContains(result, "Revoked persistent trust grant: bash:go vet *") {
		t.Fatalf("/trust revoke result = %v", result.Lines)
	}

	result = Parse("/trust reset", c)
	if !resultContains(result, "Trust rules reset to config defaults") || !resultContains(result, "conservative") {
		t.Fatalf("/trust reset result = %v", result.Lines)
	}
}
