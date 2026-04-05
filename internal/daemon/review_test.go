package daemon

import (
	"context"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"google.golang.org/grpc/metadata"
)

// captureStream collects ChatEvents sent to it.
type captureStream struct {
	ctx    context.Context
	events []*pb.ChatEvent
}

func (c *captureStream) Send(ev *pb.ChatEvent) error {
	c.events = append(c.events, ev)
	return nil
}
func (c *captureStream) Context() context.Context    { return c.ctx }
func (c *captureStream) SetHeader(metadata.MD) error { return nil }
func (c *captureStream) SendHeader(metadata.MD) error { return nil }
func (c *captureStream) SetTrailer(metadata.MD)       {}
func (c *captureStream) SendMsg(interface{}) error    { return nil }
func (c *captureStream) RecvMsg(interface{}) error    { return nil }

func TestReview_SentinelRouting(t *testing.T) {
	// Sending reviewSentinel routes to handleReview, not the regular chat path.
	// Use a real session manager so the session lookup doesn't panic.
	engine := newTestEngine(t)
	svc := &Service{
		engine:   engine,
		sessions: NewSessionManager(engine.DB),
	}

	stream := &captureStream{ctx: context.Background()}
	// handleReview will fail to find the session, sending an error event.
	_ = svc.handleChat(context.Background(), "nonexistent-sess", reviewSentinel+"diff content", stream)

	// Verify that we got some event (error from review path, not a panic).
	if len(stream.events) == 0 {
		t.Error("expected at least one event from handleReview")
	}
}

func TestReview_ExecutorCalled(t *testing.T) {
	engine := newTestEngine(t)

	// Create a session so handleReview can look up the provider.
	engine.DB.Exec(`INSERT INTO sessions (id, name, status, provider) VALUES ('sess-review', 'test', 'active', 'default')`)

	svc := &Service{
		engine:   engine,
		sessions: NewSessionManager(engine.DB),
	}

	stream := &captureStream{ctx: context.Background()}
	diff := "--- a/foo.go\n+++ b/foo.go\n@@ -1 +1 @@\n-old\n+new"
	err := svc.handleChat(context.Background(), "sess-review", reviewSentinel+diff, stream)
	if err != nil {
		t.Fatalf("handleChat: %v", err)
	}

	// Verify we got token events (mock provider returns text).
	var gotToken bool
	var gotComplete bool
	for _, ev := range stream.events {
		if tok, ok := ev.Event.(*pb.ChatEvent_Token); ok && tok.Token.Content != "" {
			gotToken = true
		}
		if _, ok := ev.Event.(*pb.ChatEvent_Complete); ok {
			gotComplete = true
		}
	}
	if !gotToken {
		t.Error("expected at least one token event from mock provider")
	}
	if !gotComplete {
		t.Error("expected a complete event")
	}

}
