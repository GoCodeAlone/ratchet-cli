package daemon

import (
	"context"
	"errors"
	"net"
	"os"
	"runtime"
	"sync"
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

func TestServiceCloseWaitsForActiveCronCallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := EnsureDataDir(); err != nil {
		t.Fatalf("EnsureDataDir: %v", err)
	}

	svc, err := NewService(t.Context())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	svc.cron.onTick = func(string, string) {
		startedOnce.Do(func() { close(started) })
		<-release
	}
	if _, err := svc.cron.Create(t.Context(), "session-1", time.Millisecond.String(), "tick"); err != nil {
		svc.Close()
		t.Fatalf("Create cron job: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		svc.Close()
		t.Fatal("cron callback did not start")
	}

	closeDone := make(chan struct{})
	go func() {
		svc.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
		close(release)
		t.Fatal("Service.Close returned before active cron callback completed")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("Service.Close did not return after active cron callback completed")
	}
}

func TestServiceCloseCancelsLifecycleBeforeJoiningCron(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := EnsureDataDir(); err != nil {
		t.Fatalf("EnsureDataDir: %v", err)
	}

	svc, err := NewService(t.Context())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	started := make(chan struct{})
	canceled := make(chan struct{})
	var startedOnce sync.Once
	var canceledOnce sync.Once
	svc.cron.onTick = func(string, string) {
		startedOnce.Do(func() { close(started) })
		<-svc.lifecycleCtx.Done()
		canceledOnce.Do(func() { close(canceled) })
	}
	if _, err := svc.cron.Create(t.Context(), "session-1", time.Millisecond.String(), "tick"); err != nil {
		svc.Close()
		t.Fatalf("Create cron job: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		svc.Close()
		t.Fatal("cron callback did not start")
	}

	closeDone := make(chan struct{})
	go func() {
		svc.Close()
		close(closeDone)
	}()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("Service.Close did not cancel the cron callback context")
	}
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("Service.Close did not join the canceled cron callback")
	}
}

func TestServiceCloseCancelsAndJoinsAdmittedLifecycleWork(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	svc := &Service{lifecycleCtx: ctx, lifecycleCancel: cancel}
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	if admitted := svc.startLifecycleWork(func(workCtx context.Context) {
		close(started)
		<-workCtx.Done()
		close(canceled)
		<-release
	}); !admitted {
		t.Fatal("lifecycle work was not admitted")
	}
	<-started

	closeDone := make(chan struct{})
	go func() {
		svc.Close()
		close(closeDone)
	}()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("Service.Close did not cancel admitted lifecycle work")
	}
	select {
	case <-closeDone:
		close(release)
		t.Fatal("Service.Close returned before admitted lifecycle work completed")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("Service.Close did not join admitted lifecycle work")
	}
	if svc.startLifecycleWork(func(context.Context) {}) {
		t.Fatal("Service admitted lifecycle work after close")
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
	var exited atomic.Bool
	go func() {
		errCh <- Start(ctx, false)
	}()
	t.Cleanup(func() {
		cancel()
		if !exited.Load() {
			select {
			case <-errCh:
				exited.Store(true)
			case <-time.After(3 * time.Second):
				t.Log("daemon Start did not exit before cleanup timeout")
			}
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
		exited.Store(true)
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
