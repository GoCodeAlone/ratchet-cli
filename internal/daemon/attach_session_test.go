package daemon

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// fakeAttachStream implements pb.RatchetDaemon_AttachSessionServer for testing.
type fakeAttachStream struct {
	ctx    context.Context
	events []*pb.ChatEvent
}

func (f *fakeAttachStream) Send(e *pb.ChatEvent) error {
	f.events = append(f.events, e)
	return nil
}
func (f *fakeAttachStream) Context() context.Context           { return f.ctx }
func (f *fakeAttachStream) SetHeader(metadata.MD) error        { return nil }
func (f *fakeAttachStream) SendHeader(metadata.MD) error       { return nil }
func (f *fakeAttachStream) SetTrailer(metadata.MD)             {}
func (f *fakeAttachStream) SendMsg(any) error                  { return nil }
func (f *fakeAttachStream) RecvMsg(any) error                  { return nil }

func TestAttachSession_ReceivesPublishedEvents(t *testing.T) {
	b := NewSessionBroadcaster()
	svc := &Service{broadcaster: b}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream := &fakeAttachStream{ctx: ctx}

	done := make(chan error, 1)
	go func() {
		done <- svc.AttachSession(&pb.AttachReq{SessionId: "sess1"}, stream)
	}()

	// Give AttachSession time to subscribe.
	time.Sleep(20 * time.Millisecond)

	ev := &pb.ChatEvent{Event: &pb.ChatEvent_Token{Token: &pb.TokenDelta{Content: "hello"}}}
	b.Publish("sess1", ev)

	// Give the goroutine time to process.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("AttachSession returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("AttachSession did not return after context cancel")
	}

	if len(stream.events) == 0 {
		t.Fatal("expected at least one event delivered to attach stream")
	}
	if stream.events[0].GetToken().GetContent() != "hello" {
		t.Errorf("expected 'hello', got %q", stream.events[0].GetToken().GetContent())
	}
}

func TestDetachSession_ReturnsEmpty(t *testing.T) {
	svc := &Service{broadcaster: NewSessionBroadcaster()}
	resp, err := svc.DetachSession(context.Background(), &pb.DetachReq{SessionId: "sess1"})
	if err != nil {
		t.Fatalf("DetachSession: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}
