//go:build tui_smoke && windows

package daemon

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// StartTUISmokeDaemonLoopback starts the Windows smoke daemon on loopback.
func StartTUISmokeDaemonLoopback(ctx context.Context, tempRoot string) (*pb.Session, string, func(), error) {
	svc, err := newTUISmokeService(ctx, tempRoot)
	if err != nil {
		return nil, "", func() {}, err
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		svc.close()
		return nil, "", func() {}, fmt.Errorf("listen smoke loopback: %w", err)
	}
	target := lis.Addr().String()
	server := grpc.NewServer()
	pb.RegisterRatchetDaemonServer(server, svc)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.Serve(lis)
	}()

	session, err := setupTUISmokeSession(ctx, target, tempRoot)
	if err != nil {
		server.GracefulStop()
		<-done
		_ = lis.Close()
		svc.close()
		return nil, "", func() {}, err
	}

	cleanup := func() {
		server.GracefulStop()
		<-done
		_ = lis.Close()
		svc.close()
	}
	return session, target, cleanup, nil
}

func setupTUISmokeSession(ctx context.Context, target, tempRoot string) (*pb.Session, error) {
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("connect smoke setup client: %w", err)
	}
	defer conn.Close()
	setupClient := pb.NewRatchetDaemonClient(conn)
	if _, err := setupClient.AddProvider(ctx, &pb.AddProviderReq{
		Alias:     "e2e-mock",
		Type:      "mock",
		IsDefault: true,
	}); err != nil {
		return nil, fmt.Errorf("add smoke provider: %w", err)
	}
	session, err := setupClient.CreateSession(ctx, &pb.CreateSessionReq{
		WorkingDir: tempRoot,
		Provider:   "e2e-mock",
	})
	if err != nil {
		return nil, fmt.Errorf("create smoke session: %w", err)
	}
	return session, nil
}
