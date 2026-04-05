package daemon

import (
	"context"
	"sync"
	"time"

	"github.com/GoCodeAlone/workflow-plugin-agent/executor"
)

// ApprovalGate manages pending approval requests, allowing the TUI to resolve
// them asynchronously. It implements executor.Approver so it can be wired into
// executor.Execute() to gate tool calls that require user approval.
type ApprovalGate struct {
	mu      sync.Mutex
	pending map[string]chan ApprovalResponse
}

// NewApprovalGate returns an initialised ApprovalGate.
func NewApprovalGate() *ApprovalGate {
	return &ApprovalGate{
		pending: make(map[string]chan ApprovalResponse),
	}
}

// Request registers a pending approval and returns a channel that will receive
// exactly one ApprovalResponse when Resolve is called.
func (g *ApprovalGate) Request(requestID string) <-chan ApprovalResponse {
	ch := make(chan ApprovalResponse, 1)
	g.mu.Lock()
	g.pending[requestID] = ch
	g.mu.Unlock()
	return ch
}

// Resolve delivers a resolution to a pending approval request. Returns true if
// the request was found and resolved, false if it was unknown or already resolved.
func (g *ApprovalGate) Resolve(requestID string, approved bool, reason string) bool {
	g.mu.Lock()
	ch, ok := g.pending[requestID]
	if ok {
		delete(g.pending, requestID)
	}
	g.mu.Unlock()
	if !ok {
		return false
	}
	ch <- ApprovalResponse{Approved: approved, Reason: reason}
	return true
}

// WaitForResolution implements executor.Approver. It registers a request,
// then blocks until the TUI resolves it, the timeout elapses, or ctx is cancelled.
func (g *ApprovalGate) WaitForResolution(ctx context.Context, approvalID string, timeout time.Duration) (*executor.ApprovalRecord, error) {
	ch := g.Request(approvalID)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case resp := <-ch:
		status := executor.ApprovalApproved
		if !resp.Approved {
			status = executor.ApprovalRejected
		}
		return &executor.ApprovalRecord{
			ID:              approvalID,
			Status:          status,
			ReviewerComment: resp.Reason,
		}, nil
	case <-timer.C:
		// Clean up the pending entry.
		g.mu.Lock()
		delete(g.pending, approvalID)
		g.mu.Unlock()
		return &executor.ApprovalRecord{
			ID:     approvalID,
			Status: executor.ApprovalTimeout,
		}, nil
	case <-ctx.Done():
		// Clean up the pending entry.
		g.mu.Lock()
		delete(g.pending, approvalID)
		g.mu.Unlock()
		return &executor.ApprovalRecord{
			ID:     approvalID,
			Status: executor.ApprovalRejected,
		}, ctx.Err()
	}
}

// PendingCount returns the number of unresolved approval requests.
func (g *ApprovalGate) PendingCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.pending)
}
