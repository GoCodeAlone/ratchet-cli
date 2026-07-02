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

func TestTrustClientWrappers(t *testing.T) {
	server := grpc.NewServer()
	pb.RegisterRatchetDaemonServer(server, &trustServer{})
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

	state, err := c.GetTrustState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Mode != "conservative" || len(state.Rules) != 1 || state.Rules[0].Pattern != "git *" {
		t.Fatalf("initial trust state = %+v", state)
	}

	state, err = c.SetTrustMode(ctx, "locked")
	if err != nil {
		t.Fatal(err)
	}
	if state.Mode != "locked" {
		t.Fatalf("set mode returned %q, want locked", state.Mode)
	}

	state, err = c.AddTrustRule(ctx, "go test ./...", "allow", "repo")
	if err != nil {
		t.Fatal(err)
	}
	last := state.Rules[len(state.Rules)-1]
	if last.Pattern != "go test ./..." || last.Action != "allow" || last.Scope != "repo" {
		t.Fatalf("added trust rule = %+v", last)
	}

	state, err = c.ResetTrust(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Mode != "conservative" || len(state.Rules) != 1 {
		t.Fatalf("reset trust state = %+v", state)
	}
}

type trustServer struct {
	pb.UnimplementedRatchetDaemonServer
}

func (trustServer) GetTrustState(context.Context, *pb.Empty) (*pb.TrustState, error) {
	return &pb.TrustState{
		Mode:  "conservative",
		Rules: []*pb.TrustRule{{Pattern: "git *", Action: "allow", Scope: "repo"}},
	}, nil
}

func (trustServer) SetTrustMode(_ context.Context, req *pb.SetTrustModeReq) (*pb.TrustState, error) {
	return &pb.TrustState{
		Mode:  req.Mode,
		Rules: []*pb.TrustRule{{Pattern: "git *", Action: "allow", Scope: "repo"}},
	}, nil
}

func (trustServer) AddTrustRule(_ context.Context, req *pb.AddTrustRuleReq) (*pb.TrustState, error) {
	return &pb.TrustState{
		Mode: "locked",
		Rules: []*pb.TrustRule{
			{Pattern: "git *", Action: "allow", Scope: "repo"},
			{Pattern: req.Pattern, Action: req.Action, Scope: req.Scope},
		},
	}, nil
}

func (trustServer) ResetTrust(context.Context, *pb.Empty) (*pb.TrustState, error) {
	return &pb.TrustState{
		Mode:  "conservative",
		Rules: []*pb.TrustRule{{Pattern: "git *", Action: "allow", Scope: "repo"}},
	}, nil
}
