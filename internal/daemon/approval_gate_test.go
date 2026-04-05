package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/GoCodeAlone/workflow-plugin-agent/executor"
)

func TestApprovalGate_RequestAndResolve(t *testing.T) {
	g := NewApprovalGate()

	done := make(chan *executor.ApprovalRecord, 1)
	go func() {
		rec, err := g.WaitForResolution(context.Background(), "req-1", 5*time.Second)
		if err != nil {
			t.Errorf("WaitForResolution: %v", err)
		}
		done <- rec
	}()

	time.Sleep(5 * time.Millisecond)
	if !g.Resolve("req-1", true, "looks good") {
		t.Fatal("Resolve returned false")
	}

	select {
	case rec := <-done:
		if rec.Status != executor.ApprovalApproved {
			t.Errorf("expected approved, got %v", rec.Status)
		}
		if rec.ReviewerComment != "looks good" {
			t.Errorf("unexpected comment: %q", rec.ReviewerComment)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for resolution")
	}
}

func TestApprovalGate_Timeout(t *testing.T) {
	g := NewApprovalGate()

	rec, err := g.WaitForResolution(context.Background(), "req-timeout", 20*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Status != executor.ApprovalTimeout {
		t.Errorf("expected timeout status, got %v", rec.Status)
	}
}

func TestApprovalGate_ContextCancel(t *testing.T) {
	g := NewApprovalGate()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := g.WaitForResolution(ctx, "req-cancel", 10*time.Second)
		done <- err
	}()

	time.Sleep(5 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error on context cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}

func TestApprovalGate_PendingCount(t *testing.T) {
	g := NewApprovalGate()
	if g.PendingCount() != 0 {
		t.Errorf("expected 0 pending, got %d", g.PendingCount())
	}

	_ = g.Request("r1")
	_ = g.Request("r2")
	if g.PendingCount() != 2 {
		t.Errorf("expected 2 pending, got %d", g.PendingCount())
	}

	g.Resolve("r1", true, "")
	if g.PendingCount() != 1 {
		t.Errorf("expected 1 pending after resolve, got %d", g.PendingCount())
	}
}

func TestApprovalGate_ResolveUnknown(t *testing.T) {
	g := NewApprovalGate()
	if g.Resolve("nonexistent", true, "") {
		t.Error("expected false for unknown request ID")
	}
}
