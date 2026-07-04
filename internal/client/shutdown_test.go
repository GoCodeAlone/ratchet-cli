package client

import (
	"context"
	"net"
	"sync/atomic"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestClientShutdownCallsDaemonRPC(t *testing.T) {
	server := grpc.NewServer()
	fake := &shutdownServer{}
	pb.RegisterRatchetDaemonServer(server, fake)
	listener := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})
	go func() {
		_ = server.Serve(listener)
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	c := &Client{conn: conn, daemon: pb.NewRatchetDaemonClient(conn)}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if fake.calls.Load() != 1 {
		t.Fatalf("Shutdown RPC calls = %d, want 1", fake.calls.Load())
	}
}

type shutdownServer struct {
	pb.UnimplementedRatchetDaemonServer
	calls atomic.Int64
}

func (s *shutdownServer) Shutdown(context.Context, *pb.Empty) (*pb.Empty, error) {
	s.calls.Add(1)
	return &pb.Empty{}, nil
}
