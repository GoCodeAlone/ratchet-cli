package client

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestBackgroundDrainClientWrappers(t *testing.T) {
	serverImpl := &backgroundDrainClientServer{}
	server := grpc.NewServer()
	pb.RegisterRatchetDaemonServer(server, serverImpl)
	listener := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})
	go func() { _ = server.Serve(listener) }()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := &Client{conn: conn, daemon: pb.NewRatchetDaemonClient(conn)}

	started, err := client.StartACPBackgroundDrain(t.Context(), "session-1", "codex", true)
	if err != nil || started.GetState() != "running" {
		t.Fatalf("start = %#v, %v", started, err)
	}
	stopped, err := client.StopACPBackgroundDrain(t.Context(), "session-1")
	if err != nil || stopped.GetState() != "disabled" {
		t.Fatalf("stop = %#v, %v", stopped, err)
	}
	got, err := client.GetACPBackgroundDrain(t.Context(), "session-1")
	if err != nil || got.GetSessionId() != "session-1" {
		t.Fatalf("get = %#v, %v", got, err)
	}
	listed, err := client.ListACPBackgroundDrains(t.Context())
	if err != nil || len(listed.GetDrains()) != 1 {
		t.Fatalf("list = %#v, %v", listed, err)
	}
	if serverImpl.sessionID != "session-1" || serverImpl.profile != "codex" || !serverImpl.acknowledged {
		t.Fatalf("server args = %q/%q/%t", serverImpl.sessionID, serverImpl.profile, serverImpl.acknowledged)
	}
}

type backgroundDrainClientServer struct {
	pb.UnimplementedRatchetDaemonServer
	sessionID    string
	profile      string
	acknowledged bool
}

func (s *backgroundDrainClientServer) StartACPBackgroundDrain(_ context.Context, req *pb.StartACPBackgroundDrainReq) (*pb.ACPBackgroundDrain, error) {
	s.sessionID = req.GetSessionId()
	s.profile = req.GetProfile()
	s.acknowledged = req.GetAcknowledged()
	return &pb.ACPBackgroundDrain{SessionId: req.GetSessionId(), Profile: req.GetProfile(), State: "running"}, nil
}

func (s *backgroundDrainClientServer) StopACPBackgroundDrain(_ context.Context, req *pb.ACPBackgroundDrainReq) (*pb.ACPBackgroundDrain, error) {
	return &pb.ACPBackgroundDrain{SessionId: req.GetSessionId(), State: "disabled"}, nil
}

func (s *backgroundDrainClientServer) GetACPBackgroundDrain(_ context.Context, req *pb.ACPBackgroundDrainReq) (*pb.ACPBackgroundDrain, error) {
	return &pb.ACPBackgroundDrain{SessionId: req.GetSessionId(), State: "running"}, nil
}

func (s *backgroundDrainClientServer) ListACPBackgroundDrains(context.Context, *pb.Empty) (*pb.ACPBackgroundDrainList, error) {
	return &pb.ACPBackgroundDrainList{Drains: []*pb.ACPBackgroundDrain{{SessionId: "session-1", State: "running"}}}, nil
}
