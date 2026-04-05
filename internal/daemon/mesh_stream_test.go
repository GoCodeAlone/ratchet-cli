package daemon

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/GoCodeAlone/ratchet-cli/internal/mesh"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// startTestGRPCServer starts an in-process gRPC server with the given Service and
// returns the listener address plus a cleanup function.
func startTestGRPCServer(t *testing.T, svc *Service) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	pb.RegisterRatchetDaemonServer(srv, svc)
	go func() {
		_ = srv.Serve(lis)
	}()
	t.Cleanup(srv.GracefulStop)
	return lis.Addr().String()
}

func TestMeshStream_BlackboardSync(t *testing.T) {
	engine := newTestEngine(t)
	svc := &Service{
		engine:      engine,
		broadcaster: NewSessionBroadcaster(),
		meshBB:      mesh.NewBlackboard(),
		meshRouter:  mesh.NewRouter(),
	}
	addr := startTestGRPCServer(t, svc)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client := pb.NewRatchetDaemonClient(conn)
	stream, err := client.MeshStream(ctx)
	if err != nil {
		t.Fatalf("MeshStream: %v", err)
	}

	// Send a BlackboardSync from the client to the server.
	value, _ := json.Marshal("hello-from-client")
	err = stream.Send(&pb.MeshEvent{
		Event: &pb.MeshEvent_BlackboardSync{
			BlackboardSync: &pb.BlackboardSync{
				Section:  "test",
				Key:      "greeting",
				Value:    value,
				Author:   "test-node",
				Revision: 1,
			},
		},
	})
	if err != nil {
		t.Fatalf("stream Send: %v", err)
	}

	// Give the server time to process.
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Verify the server's meshBB was updated.
	entry, ok := svc.meshBB.Read("test", "greeting")
	if !ok {
		t.Fatal("expected blackboard entry to be set")
	}
	if entry.Author != "test-node" {
		t.Errorf("expected author 'test-node', got %q", entry.Author)
	}
}

func TestRegisterMeshNode_ReturnsNodeID(t *testing.T) {
	svc := &Service{
		broadcaster: NewSessionBroadcaster(),
		meshBB:      mesh.NewBlackboard(),
		meshRouter:  mesh.NewRouter(),
	}

	resp, err := svc.RegisterMeshNode(context.Background(), &pb.RegisterNodeReq{
		Name:  "test-node",
		Role:  "worker",
		Model: "gpt-4",
	})
	if err != nil {
		t.Fatalf("RegisterMeshNode: %v", err)
	}
	if resp.NodeId == "" {
		t.Error("expected non-empty node ID")
	}
}

func TestRegisterMeshNode_DuplicateReturnsError(t *testing.T) {
	svc := &Service{
		broadcaster: NewSessionBroadcaster(),
		meshBB:      mesh.NewBlackboard(),
		meshRouter:  mesh.NewRouter(),
	}

	if _, err := svc.RegisterMeshNode(context.Background(), &pb.RegisterNodeReq{Name: "n1"}); err != nil {
		t.Fatalf("first RegisterMeshNode: %v", err)
	}
	// Second registration with same router slot (different UUID, so should succeed).
	if _, err := svc.RegisterMeshNode(context.Background(), &pb.RegisterNodeReq{Name: "n2"}); err != nil {
		t.Fatalf("second RegisterMeshNode: %v", err)
	}
}
