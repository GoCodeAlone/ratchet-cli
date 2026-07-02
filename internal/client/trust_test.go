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
	if len(state.Grants) != 1 || state.Grants[0].Pattern != "go test *" {
		t.Fatalf("initial trust grants = %+v", state.Grants)
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

	state, err = c.AddTrustGrant(ctx, "bash:go test *", "allow", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Grants) != 2 {
		t.Fatalf("grant count = %d, want 2: %+v", len(state.Grants), state.Grants)
	}
	grant := state.Grants[len(state.Grants)-1]
	if grant.Pattern != "bash:go test *" || grant.Action != "allow" || grant.Scope != "repo" || grant.GrantedBy != "operator" {
		t.Fatalf("added trust grant = %+v", grant)
	}

	state, err = c.RevokeTrustGrant(ctx, "bash:go test *", "repo")
	if err != nil {
		t.Fatal(err)
	}
	for _, grant := range state.Grants {
		if grant.Pattern == "bash:go test *" && grant.Scope == "repo" {
			t.Fatalf("revoked grant still present: %+v", state.Grants)
		}
	}
}

type trustServer struct {
	pb.UnimplementedRatchetDaemonServer
}

func (trustServer) GetTrustState(context.Context, *pb.Empty) (*pb.TrustState, error) {
	return &pb.TrustState{
		Mode:  "conservative",
		Rules: []*pb.TrustRule{{Pattern: "git *", Action: "allow", Scope: "repo"}},
		Grants: []*pb.TrustGrant{{
			Id:        1,
			Pattern:   "go test *",
			Action:    "allow",
			Scope:     "repo",
			GrantedBy: "operator",
		}},
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

func (trustServer) AddTrustGrant(_ context.Context, req *pb.AddTrustRuleReq) (*pb.TrustState, error) {
	return &pb.TrustState{
		Mode: "conservative",
		Grants: []*pb.TrustGrant{
			{Id: 1, Pattern: "go test *", Action: "allow", Scope: "repo", GrantedBy: "operator"},
			{Id: 2, Pattern: req.Pattern, Action: req.Action, Scope: req.Scope, GrantedBy: "operator"},
		},
	}, nil
}

func (trustServer) RevokeTrustGrant(_ context.Context, req *pb.RevokeTrustGrantReq) (*pb.TrustState, error) {
	return &pb.TrustState{
		Mode:   "conservative",
		Grants: []*pb.TrustGrant{{Id: 1, Pattern: "go test *", Action: "allow", Scope: "repo", GrantedBy: "operator"}},
	}, nil
}
