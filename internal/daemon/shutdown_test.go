package daemon

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
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
