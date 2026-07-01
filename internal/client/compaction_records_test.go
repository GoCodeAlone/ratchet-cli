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

func TestCompactionRecordsClientWrapper(t *testing.T) {
	server := grpc.NewServer()
	pb.RegisterRatchetDaemonServer(server, &compactionServer{})
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
	got, err := c.ListSessionCompactions(t.Context(), "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Records) != 1 || got.Records[0].Id != "comp-1" {
		t.Fatalf("records = %+v, want comp-1", got.Records)
	}
}

type compactionServer struct {
	pb.UnimplementedRatchetDaemonServer
}

func (compactionServer) ListSessionCompactions(context.Context, *pb.SessionCompactionsReq) (*pb.SessionCompactionList, error) {
	return &pb.SessionCompactionList{Records: []*pb.CompactionRecord{{Id: "comp-1", Reason: "manual", Summary: "summary"}}}, nil
}
