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
	engine.DB.Exec(`INSERT INTO sessions (id, name, status, provider, working_dir, model) VALUES ('sess-review', 'test', 'active', 'default', '', '')`)

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

	// executor.Execute calls provider.Chat() and returns a single result.
	// Verify we got exactly one non-empty token event (the executor result) and a complete event.
	var tokenContents []string
	var gotComplete bool
	for _, ev := range stream.events {
		if tok, ok := ev.Event.(*pb.ChatEvent_Token); ok && tok.Token.Content != "" {
			tokenContents = append(tokenContents, tok.Token.Content)
		}
		if _, ok := ev.Event.(*pb.ChatEvent_Complete); ok {
			gotComplete = true
		}
	}
	if len(tokenContents) != 1 {
		t.Errorf("expected exactly 1 token event from executor.Execute, got %d", len(tokenContents))
	}
	if !gotComplete {
		t.Error("expected a complete event")
	}

	// Verify the result was saved to session history (handleReview saves via saveMessage).
	rows, err := engine.DB.Query(
		`SELECT role, content FROM messages WHERE session_id = 'sess-review' AND role = 'assistant'`,
	)
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	defer rows.Close()
	var savedContent string
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			t.Fatalf("scan: %v", err)
		}
		savedContent = content
	}
	if savedContent == "" {
		t.Error("expected executor result to be saved to session history")
	}
	if len(tokenContents) == 1 && tokenContents[0] != savedContent {
		t.Errorf("token content %q != saved history content %q", tokenContents[0], savedContent)
	}
}
