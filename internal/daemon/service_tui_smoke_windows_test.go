//go:build tui_smoke && windows

package daemon

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestSmokeServiceLoopbackInitializesSafeJobProviders(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	session, target, cleanup, err := StartTUISmokeDaemonLoopback(ctx, root)
	if err != nil {
		t.Fatalf("StartTUISmokeDaemonLoopback: %v", err)
	}
	defer cleanup()
	if session == nil {
		t.Fatal("expected initial smoke session")
	}
	if !strings.HasPrefix(target, "127.0.0.1:") {
		t.Fatalf("target = %q, want 127.0.0.1 loopback", target)
	}
	if session.GetWorkingDir() != root {
		t.Fatalf("expected smoke session working dir %q, got %q", root, session.GetWorkingDir())
	}

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("connect smoke loopback: %v", err)
	}
	defer conn.Close()
	client := pb.NewRatchetDaemonClient(conn)
	list, err := client.ListJobs(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(list.GetJobs()) == 0 {
		t.Fatal("expected smoke jobs")
	}
}
