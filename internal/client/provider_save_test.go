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

func TestProviderSaveClient(t *testing.T) {
	serverImpl := &providerSaveClientServer{}
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
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	client := &Client{conn: conn, daemon: pb.NewRatchetDaemonClient(conn)}
	request := &pb.CommitProviderSaveReq{
		OperationId: "2af44cfe-b52e-4555-9ac5-9717348a04c5",
		Provider: &pb.AddProviderReq{
			Alias:     "work",
			Type:      "openai",
			Model:     "gpt-test",
			ApiKey:    "transient-secret",
			IsDefault: true,
		},
	}

	committed, err := client.CommitProviderSave(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if committed.GetOperationId() != request.GetOperationId() {
		t.Fatalf("operation ID = %q, want %q", committed.GetOperationId(), request.GetOperationId())
	}
	if serverImpl.commitRequest != request.GetOperationId() {
		t.Fatalf("server commit ID = %q, want %q", serverImpl.commitRequest, request.GetOperationId())
	}

	queried, err := client.GetProviderOperation(t.Context(), request.GetOperationId())
	if err != nil {
		t.Fatal(err)
	}
	if queried.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED {
		t.Fatalf("queried state = %s, want committed", queried.GetState())
	}
	if serverImpl.getRequest != request.GetOperationId() {
		t.Fatalf("server query ID = %q, want %q", serverImpl.getRequest, request.GetOperationId())
	}
}

type providerSaveClientServer struct {
	pb.UnimplementedRatchetDaemonServer
	commitRequest string
	getRequest    string
}

func (s *providerSaveClientServer) CommitProviderSave(_ context.Context, req *pb.CommitProviderSaveReq) (*pb.ProviderOperation, error) {
	s.commitRequest = req.GetOperationId()
	return &pb.ProviderOperation{
		OperationId: req.GetOperationId(),
		Alias:       req.GetProvider().GetAlias(),
		State:       pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED,
		Result: &pb.ProviderSaveResult{
			Alias:     req.GetProvider().GetAlias(),
			Type:      req.GetProvider().GetType(),
			Model:     req.GetProvider().GetModel(),
			IsDefault: req.GetProvider().GetIsDefault(),
		},
	}, nil
}

func (s *providerSaveClientServer) GetProviderOperation(_ context.Context, req *pb.GetProviderOperationReq) (*pb.ProviderOperation, error) {
	s.getRequest = req.GetOperationId()
	return &pb.ProviderOperation{
		OperationId: req.GetOperationId(),
		State:       pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED,
	}, nil
}
