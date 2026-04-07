package daemon

// E2E tests for chat roundtrip and session management (Task 3).
// All tests exercise real gRPC paths through E2EHarness.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// TestE2E_ChatRoundtrip verifies that a SendMessage RPC through the real gRPC
// stack returns at least one TokenDelta event with non-empty content followed
// by a SessionComplete event. Uses the default "e2e-mock" provider inserted by
// newE2EHarness.
func TestE2E_ChatRoundtrip(t *testing.T) {
	h := newE2EHarness(t)

	session := h.createSession(t, "e2e-mock")

	tokenContent, gotComplete := h.sendMessage(t, session.Id, "hello")

	if tokenContent == "" {
		t.Error("expected non-empty token content from mock provider")
	}
	if !gotComplete {
		t.Error("expected SessionComplete event")
	}
}

// TestE2E_SessionAttach verifies that a second stream attached via AttachSession
// receives the same events published during a SendMessage call on the same session.
func TestE2E_SessionAttach(t *testing.T) {
	h := newE2EHarness(t)

	session := h.createSession(t, "e2e-mock")

	// Open an attach stream before sending the message so it is subscribed to
	// the broadcaster when the events are published.
	attachCtx, attachCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer attachCancel()

	attachStream, err := h.Client.AttachSession(attachCtx, &pb.AttachReq{SessionId: session.Id})
	if err != nil {
		t.Fatalf("AttachSession: %v", err)
	}

	// Collect events from the attach stream concurrently.
	type result struct {
		events []*pb.ChatEvent
	}
	attachDone := make(chan result, 1)
	go func() {
		var evs []*pb.ChatEvent
		for {
			ev, err := attachStream.Recv()
			if err != nil {
				attachDone <- result{events: evs}
				return
			}
			evs = append(evs, ev)
		}
	}()

	// Give AttachSession time to subscribe to the SessionBroadcaster.
	time.Sleep(50 * time.Millisecond)

	// SendMessage publishes events to the broadcaster; the attach stream should
	// receive the same token and complete events.
	tokenContent, gotComplete := h.sendMessage(t, session.Id, "hello from attach test")
	if tokenContent == "" {
		t.Error("expected non-empty token content from SendMessage stream")
	}
	if !gotComplete {
		t.Error("expected SessionComplete event from SendMessage stream")
	}

	// Allow the broadcaster to deliver events to the attach subscriber.
	time.Sleep(100 * time.Millisecond)

	// Cancel the attach context to unblock the Recv goroutine.
	attachCancel()

	select {
	case res := <-attachDone:
		if len(res.events) == 0 {
			t.Error("expected attach stream to receive at least one broadcast event")
		}
		// Verify at least one Token event reached the attached stream.
		var gotToken bool
		for _, ev := range res.events {
			if tok := ev.GetToken(); tok != nil && tok.Content != "" {
				gotToken = true
				break
			}
		}
		if !gotToken {
			t.Error("expected at least one TokenDelta event on the attach stream")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for attach stream goroutine to finish")
	}
}

// TestE2E_Shutdown verifies that calling the Shutdown RPC fires the registered
// shutdown callback.
func TestE2E_Shutdown(t *testing.T) {
	h := newE2EHarness(t)

	var called atomic.Bool
	h.Svc.SetShutdownFunc(func() { called.Store(true) })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := h.Client.Shutdown(ctx, &pb.Empty{}); err != nil {
		t.Fatalf("Shutdown RPC: %v", err)
	}

	// shutdownFn is invoked asynchronously after ~100 ms inside the RPC handler.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if called.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("shutdownFn was not called within 500 ms of Shutdown RPC")
}
