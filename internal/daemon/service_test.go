package daemon

import (
	"context"
	"net"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestServiceHealth(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	EnsureDataDir()

	sock := filepath.Join(tmp, "test.sock")
	lis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	srv := grpc.NewServer()
	svc, _ := NewService(context.Background())
	pb.RegisterRatchetDaemonServer(srv, svc)
	go srv.Serve(lis)
	defer srv.Stop()

	conn, err := grpc.NewClient(
		"unix://"+sock,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	client := pb.NewRatchetDaemonClient(conn)
	resp, err := client.Health(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Healthy {
		t.Error("expected healthy")
	}
	if resp.ActiveSessions != 0 {
		t.Errorf("expected 0 sessions, got %d", resp.ActiveSessions)
	}
}
