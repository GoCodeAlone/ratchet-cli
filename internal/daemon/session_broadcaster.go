package daemon

import (
	"sync"

	"github.com/google/uuid"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// SessionBroadcaster fans out ChatEvents to multiple subscribers per session.
type SessionBroadcaster struct {
	mu   sync.RWMutex
	subs map[string]map[string]chan *pb.ChatEvent // sessionID → subID → channel
}

func NewSessionBroadcaster() *SessionBroadcaster {
	return &SessionBroadcaster{
		subs: make(map[string]map[string]chan *pb.ChatEvent),
	}
}

// Subscribe returns a channel that receives events for the given sessionID plus a
// unique subscription ID that must be passed to Unsubscribe.
func (b *SessionBroadcaster) Subscribe(sessionID string) (<-chan *pb.ChatEvent, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subID := uuid.New().String()
	ch := make(chan *pb.ChatEvent, 64)
	if b.subs[sessionID] == nil {
		b.subs[sessionID] = make(map[string]chan *pb.ChatEvent)
	}
	b.subs[sessionID][subID] = ch
	return ch, subID
}

// Unsubscribe removes the subscription and closes the channel.
func (b *SessionBroadcaster) Unsubscribe(sessionID, subID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if subs, ok := b.subs[sessionID]; ok {
		if ch, ok := subs[subID]; ok {
			close(ch)
			delete(subs, subID)
		}
		if len(subs) == 0 {
			delete(b.subs, sessionID)
		}
	}
}

// Publish sends an event to all current subscribers for sessionID. Sends are
// non-blocking: slow subscribers are skipped without blocking the caller.
func (b *SessionBroadcaster) Publish(sessionID string, event *pb.ChatEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs[sessionID] {
		select {
		case ch <- event:
		default:
		}
	}
}
