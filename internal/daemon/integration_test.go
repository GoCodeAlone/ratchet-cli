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

// startTestServer starts an in-process daemon gRPC server on a temp Unix socket.
// Returns the client connection. Caller must close conn and stop server.
func startTestServer(t *testing.T) (pb.RatchetDaemonClient, *grpc.ClientConn) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	EnsureDataDir()

	sock := filepath.Join(tmp, "integration.sock")
	lis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}

	srv := grpc.NewServer()
	svc, err := NewService(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pb.RegisterRatchetDaemonServer(srv, svc)
	go srv.Serve(lis)
	t.Cleanup(func() {
		srv.Stop()
		lis.Close()
	})

	conn, err := grpc.NewClient(
		"unix://"+sock,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	return pb.NewRatchetDaemonClient(conn), conn
}

func TestIntegrationHealth(t *testing.T) {
	client, _ := startTestServer(t)

	resp, err := client.Health(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Healthy {
		t.Error("expected healthy=true")
	}
}

func TestIntegrationSessionLifecycle(t *testing.T) {
	client, _ := startTestServer(t)
	ctx := context.Background()

	// Create session
	session, err := client.CreateSession(ctx, &pb.CreateSessionReq{
		WorkingDir: "/tmp",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if session.Id == "" {
		t.Error("expected non-empty session ID")
	}
	if session.WorkingDir != "/tmp" {
		t.Errorf("expected working_dir=/tmp, got %s", session.WorkingDir)
	}
	if session.Status != "active" {
		t.Errorf("expected status=active, got %s", session.Status)
	}

	// List sessions
	list, err := client.ListSessions(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(list.Sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(list.Sessions))
	}
	if list.Sessions[0].Id != session.Id {
		t.Errorf("expected session ID %s, got %s", session.Id, list.Sessions[0].Id)
	}

	// Kill session
	_, err = client.KillSession(ctx, &pb.KillReq{SessionId: session.Id})
	if err != nil {
		t.Fatalf("KillSession: %v", err)
	}

	// Verify session is killed
	list2, err := client.ListSessions(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListSessions after kill: %v", err)
	}
	for _, s := range list2.Sessions {
		if s.Id == session.Id && s.Status == "active" {
			t.Error("expected session to be killed")
		}
	}
}

func TestIntegrationMultipleSessions(t *testing.T) {
	client, _ := startTestServer(t)
	ctx := context.Background()

	dirs := []string{"/tmp/a", "/tmp/b", "/tmp/c"}
	for _, dir := range dirs {
		_, err := client.CreateSession(ctx, &pb.CreateSessionReq{WorkingDir: dir})
		if err != nil {
			t.Fatalf("CreateSession(%s): %v", dir, err)
		}
	}

	list, err := client.ListSessions(ctx, &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Sessions) != len(dirs) {
		t.Errorf("expected %d sessions, got %d", len(dirs), len(list.Sessions))
	}
}

func TestIntegrationProviderCRUD(t *testing.T) {
	client, _ := startTestServer(t)
	ctx := context.Background()

	// Add provider
	p, err := client.AddProvider(ctx, &pb.AddProviderReq{
		Alias:  "test-provider",
		Type:   "anthropic",
		ApiKey: "test-key",
	})
	if err != nil {
		t.Fatalf("AddProvider: %v", err)
	}
	if p.Alias != "test-provider" {
		t.Errorf("expected alias 'test-provider', got %s", p.Alias)
	}

	// List providers
	list, err := client.ListProviders(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(list.Providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(list.Providers))
	}
	if list.Providers[0].Alias != "test-provider" {
		t.Errorf("expected alias 'test-provider', got %s", list.Providers[0].Alias)
	}

	// Set as default
	_, err = client.SetDefaultProvider(ctx, &pb.SetDefaultProviderReq{Alias: "test-provider"})
	if err != nil {
		t.Fatalf("SetDefaultProvider: %v", err)
	}

	// Verify default
	list2, err := client.ListProviders(ctx, &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if !list2.Providers[0].IsDefault {
		t.Error("expected provider to be default")
	}

	// Remove provider
	_, err = client.RemoveProvider(ctx, &pb.RemoveProviderReq{Alias: "test-provider"})
	if err != nil {
		t.Fatalf("RemoveProvider: %v", err)
	}

	// Verify removed
	list3, err := client.ListProviders(ctx, &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list3.Providers) != 0 {
		t.Errorf("expected 0 providers after remove, got %d", len(list3.Providers))
	}
}

func TestIntegrationPermissionGate(t *testing.T) {
	client, _ := startTestServer(t)
	ctx := context.Background()

	// Responding to a non-existent permission request should return NotFound
	_, err := client.RespondToPermission(ctx, &pb.PermissionResponse{
		RequestId: "nonexistent",
		Allowed:   true,
	})
	if err == nil {
		t.Error("expected error for non-existent permission request")
	}
}
