package mesh

import (
	"fmt"
	"sync"
)

// Router delivers Messages between mesh nodes. Each registered node gets a
// buffered inbox channel. Messages addressed to "*" are broadcast to every
// registered node (except the sender).
type Router struct {
	mu      sync.RWMutex
	inboxes map[string]chan Message
}

// NewRouter returns an empty Router.
func NewRouter() *Router {
	return &Router{
		inboxes: make(map[string]chan Message),
	}
}

// inboxBuffer is the capacity of each node's inbox channel.
const inboxBuffer = 64

// Register creates an inbox for nodeID and returns the receive end.
// Calling Register twice for the same nodeID returns an error.
func (r *Router) Register(nodeID string) (<-chan Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.inboxes[nodeID]; exists {
		return nil, fmt.Errorf("node %q already registered", nodeID)
	}
	ch := make(chan Message, inboxBuffer)
	r.inboxes[nodeID] = ch
	return ch, nil
}

// Send delivers msg to its target. If msg.To is "*", the message is sent to
// all registered nodes except the sender. Returns an error if a unicast
// target is not registered.
func (r *Router) Send(msg Message) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if msg.To == "*" {
		for id, ch := range r.inboxes {
			if id == msg.From {
				continue
			}
			select {
			case ch <- msg:
			default:
				// inbox full — drop to avoid blocking
			}
		}
		return nil
	}

	ch, ok := r.inboxes[msg.To]
	if !ok {
		return fmt.Errorf("node %q not registered", msg.To)
	}
	select {
	case ch <- msg:
	default:
		// inbox full — drop to avoid blocking
	}
	return nil
}

// Unregister removes a node's inbox and closes the channel.
func (r *Router) Unregister(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ch, ok := r.inboxes[nodeID]; ok {
		close(ch)
		delete(r.inboxes, nodeID)
	}
}
