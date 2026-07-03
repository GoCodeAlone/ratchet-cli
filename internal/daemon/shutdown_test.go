package daemon

import (
	"context"
	"errors"
	"net"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestShutdown_CallsShutdownFn(t *testing.T) {
	svc := &Service{broadcaster: NewSessionBroadcaster()}

	var called atomic.Bool
	svc.SetShutdownFunc(func() { called.Store(true) })

	_, err := svc.Shutdown(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// shutdownFn is called in a goroutine after ~100ms.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if called.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("shutdownFn was not called within 500ms")
}

func TestShutdown_NoShutdownFn(t *testing.T) {
	svc := &Service{broadcaster: NewSessionBroadcaster()}
	// Must not panic when shutdownFn is nil.
	if _, err := svc.Shutdown(context.Background(), &pb.Empty{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartShutdownRPCStopsServerAndRemovesFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("daemon Start uses a Unix socket; Windows package build coverage is handled separately")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- Start(ctx, false)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-errCh:
		case <-time.After(3 * time.Second):
			t.Log("daemon Start did not exit before cleanup timeout")
		}
		CleanupSocket()
		CleanupPID()
	})

	waitForPath(t, SocketPath(), 5*time.Second)
	if info, err := os.Stat(SocketPath()); err != nil {
		t.Fatalf("stat socket: %v", err)
	} else {
		if info.Mode()&os.ModeSocket == 0 {
			t.Fatalf("daemon path mode = %v, want socket", info.Mode())
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("daemon socket permissions = %v, want 0600", info.Mode().Perm())
		}
	}
	waitForPath(t, PIDPath(), 5*time.Second)

	conn, err := grpc.NewClient(
		"unix://"+SocketPath(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("connect daemon: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	rpc := pb.NewRatchetDaemonClient(conn)
	callCtx, callCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer callCancel()
	if _, err := rpc.Shutdown(callCtx, &pb.Empty{}); err != nil {
		t.Fatalf("Shutdown RPC: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) && !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Start returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon Start did not exit after Shutdown RPC")
	}
	waitForMissing(t, SocketPath(), 2*time.Second)
	waitForMissing(t, PIDPath(), 2*time.Second)
}

func waitForPath(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func waitForMissing(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to be removed", path)
}
