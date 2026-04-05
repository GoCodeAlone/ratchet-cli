package mesh

import "time"

// Message is the envelope exchanged between mesh nodes.
type Message struct {
	ID        string
	From      string            // sender node ID
	To        string            // recipient node ID, or "*" for broadcast
	Type      string            // "task", "result", "feedback", "request"
	Content   string
	Metadata  map[string]string
	Timestamp time.Time
}
