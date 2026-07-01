package client

import (
	"context"
	"net"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestSessionLineageClientWrappers(t *testing.T) {
	server := grpc.NewServer()
	pb.RegisterRatchetDaemonServer(server, &lineageServer{})
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
	ctx := t.Context()

	history, err := c.ListSessionMessages(ctx, "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Messages) != 1 || history.Messages[0].Id != "msg-1" {
		t.Fatalf("history = %+v, want msg-1", history.Messages)
	}
	clone, err := c.CloneSession(ctx, "sess-1", "parallel")
	if err != nil {
		t.Fatal(err)
	}
	if clone.ParentId != "sess-1" {
		t.Fatalf("clone ParentId = %q, want sess-1", clone.ParentId)
	}
	fork, err := c.ForkSession(ctx, "sess-1", "msg-1", "branch")
	if err != nil {
		t.Fatal(err)
	}
	if fork.ForkedFromMessageId != "msg-1" {
		t.Fatalf("forked from = %q, want msg-1", fork.ForkedFromMessageId)
	}

	updated, err := c.UpdateSessionSummary(ctx, "fork-1", "new summary")
	if err != nil {
		t.Fatal(err)
	}
	if updated.BranchSummary != "new summary" {
		t.Fatalf("updated summary = %q, want new summary", updated.BranchSummary)
	}
	tree, err := c.GetSessionTree(ctx, "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Sessions) != 2 {
		t.Fatalf("tree length = %d, want 2", len(tree.Sessions))
	}
}

type lineageServer struct {
	pb.UnimplementedRatchetDaemonServer
}

func (lineageServer) ListSessionMessages(context.Context, *pb.SessionMessagesReq) (*pb.SessionHistory, error) {
	return &pb.SessionHistory{Messages: []*pb.HistoryMessage{{Id: "msg-1", Role: "user", Content: "hello"}}}, nil
}

func (lineageServer) CloneSession(context.Context, *pb.CloneSessionReq) (*pb.Session, error) {
	return &pb.Session{Id: "clone-1", ParentId: "sess-1", RootId: "sess-1"}, nil
}

func (lineageServer) ForkSession(context.Context, *pb.ForkSessionReq) (*pb.Session, error) {
	return &pb.Session{Id: "fork-1", ParentId: "sess-1", RootId: "sess-1", ForkedFromMessageId: "msg-1"}, nil
}

func (lineageServer) GetSessionTree(context.Context, *pb.SessionTreeReq) (*pb.SessionList, error) {
	return &pb.SessionList{Sessions: []*pb.Session{{Id: "sess-1", RootId: "sess-1"}, {Id: "fork-1", ParentId: "sess-1", RootId: "sess-1"}}}, nil
}

func (lineageServer) UpdateSessionSummary(_ context.Context, req *pb.UpdateSessionSummaryReq) (*pb.Session, error) {
	return &pb.Session{Id: req.SessionId, BranchSummary: req.Summary}, nil
}
