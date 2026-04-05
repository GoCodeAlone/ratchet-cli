package mesh

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// minimalMeshServer implements just the MeshStream RPC for testing.
type minimalMeshServer struct {
	pb.UnimplementedRatchetDaemonServer
	received chan *pb.MeshEvent
}

func (s *minimalMeshServer) MeshStream(stream pb.RatchetDaemon_MeshStreamServer) error {
	for {
		ev, err := stream.Recv()
		if err != nil {
			if status.Code(err) == codes.Canceled || status.Code(err) == codes.DeadlineExceeded {
				return nil
			}
			return err
		}
		select {
		case s.received <- ev:
		default:
		}
	}
}

func startMinimalMeshServer(t *testing.T) (addr string, srv *minimalMeshServer) {
	t.Helper()
	srv = &minimalMeshServer{received: make(chan *pb.MeshEvent, 32)}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	grpcSrv := grpc.NewServer()
	pb.RegisterRatchetDaemonServer(grpcSrv, srv)
	go func() { _ = grpcSrv.Serve(lis) }()
	t.Cleanup(grpcSrv.GracefulStop)
	return lis.Addr().String(), srv
}

func TestRemoteNode_DialsAndReceivesAgentMessage(t *testing.T) {
	addr, srv := startMinimalMeshServer(t)

	bb := NewBlackboard()
	inbox := make(chan Message, 8)
	outbox := make(chan Message, 8)

	node := NewRemoteNode("test-node", addr, NodeInfo{Name: "test", Role: "worker"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run the node in a goroutine.
	runErr := make(chan error, 1)
	go func() {
		runErr <- node.Run(ctx, "test task", bb, inbox, outbox)
	}()

	// Send a message via the inbox to test forwarding.
	inbox <- Message{From: "local", To: "remote", Content: "hello remote", Type: "task"}

	// Give time for the message to be forwarded.
	select {
	case ev := <-srv.received:
		if ev.GetAgentMessage() == nil {
			t.Fatalf("expected AgentMessage event, got %T", ev.Event)
		}
		if ev.GetAgentMessage().Content != "hello remote" {
			t.Errorf("expected 'hello remote', got %q", ev.GetAgentMessage().Content)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for message to reach server")
	}

	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Logf("RemoteNode.Run returned: %v (expected on context cancel)", err)
		}
	case <-time.After(time.Second):
		t.Error("RemoteNode.Run did not return after context cancel")
	}
}

func TestRemoteNode_DialFailureReturnsError(t *testing.T) {
	node := NewRemoteNode("bad-node", "127.0.0.1:1", NodeInfo{})
	bb := NewBlackboard()
	inbox := make(chan Message)
	outbox := make(chan Message, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := node.Run(ctx, "task", bb, inbox, outbox)
	if err == nil {
		t.Error("expected error when connecting to invalid address")
	}
}

func TestRemoteNode_ConnectToRealServer(t *testing.T) {
	// Verify that connecting to a real (but minimal) server works.
	addr, _ := startMinimalMeshServer(t)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	client := pb.NewRatchetDaemonClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = client.MeshStream(ctx)
	if err != nil {
		t.Fatalf("MeshStream: %v", err)
	}
}
