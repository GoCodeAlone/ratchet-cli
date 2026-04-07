package daemon

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// HumanRequestEntry represents a pending question from an agent.
type HumanRequestEntry struct {
	ID        string
	TeamID    string
	FromAgent string
	Question  string
	Timestamp time.Time
	response  chan string
}

// HumanGate queues human requests and blocks agents until answered.
type HumanGate struct {
	mu      sync.Mutex
	pending map[string]*HumanRequestEntry // reqID → entry
}

// NewHumanGate returns an initialized HumanGate.
func NewHumanGate() *HumanGate {
	return &HumanGate{
		pending: make(map[string]*HumanRequestEntry),
	}
}

// Request enqueues a human request and returns its ID.
// The calling agent should then call Wait(ctx, id) to block until responded.
func (hg *HumanGate) Request(teamID, fromAgent, question string) string {
	reqID := "hr-" + uuid.NewString()[:8]
	entry := &HumanRequestEntry{
		ID:        reqID,
		TeamID:    teamID,
		FromAgent: fromAgent,
		Question:  question,
		Timestamp: time.Now(),
		response:  make(chan string, 1),
	}
	hg.mu.Lock()
	hg.pending[reqID] = entry
	hg.mu.Unlock()
	return reqID
}

// Wait blocks until the human responds or the context is cancelled.
// On success, the request is removed from the pending set.
func (hg *HumanGate) Wait(ctx context.Context, reqID string) (string, error) {
	hg.mu.Lock()
	entry, ok := hg.pending[reqID]
	hg.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("request %q not found", reqID)
	}

	select {
	case resp := <-entry.response:
		hg.mu.Lock()
		delete(hg.pending, reqID)
		hg.mu.Unlock()
		return resp, nil
	case <-ctx.Done():
		hg.mu.Lock()
		delete(hg.pending, reqID)
		hg.mu.Unlock()
		return "", ctx.Err()
	}
}

// Respond provides a human response to a pending request.
func (hg *HumanGate) Respond(reqID, content string) error {
	hg.mu.Lock()
	entry, ok := hg.pending[reqID]
	hg.mu.Unlock()
	if !ok {
		return fmt.Errorf("request %q not found or already responded", reqID)
	}
	entry.response <- content
	return nil
}

// Pending returns all pending human requests, optionally filtered by team.
func (hg *HumanGate) Pending(teamID string) []HumanRequestEntry {
	hg.mu.Lock()
	defer hg.mu.Unlock()
	var out []HumanRequestEntry
	for _, e := range hg.pending {
		if teamID == "" || e.TeamID == teamID {
			out = append(out, HumanRequestEntry{
				ID:        e.ID,
				TeamID:    e.TeamID,
				FromAgent: e.FromAgent,
				Question:  e.Question,
				Timestamp: e.Timestamp,
			})
		}
	}
	return out
}
