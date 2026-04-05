package daemon

import (
	"testing"
	"time"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func makeTokenEvent(content string) *pb.ChatEvent {
	return &pb.ChatEvent{Event: &pb.ChatEvent_Token{Token: &pb.TokenDelta{Content: content}}}
}

func TestSessionBroadcasterSubscribePublish(t *testing.T) {
	b := NewSessionBroadcaster()
	ch, subID := b.Subscribe("sess1")
	defer b.Unsubscribe("sess1", subID)

	ev := makeTokenEvent("hello")
	b.Publish("sess1", ev)

	select {
	case got := <-ch:
		if got.GetToken().GetContent() != "hello" {
			t.Fatalf("expected 'hello', got %q", got.GetToken().GetContent())
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSessionBroadcasterMultipleSubscribers(t *testing.T) {
	b := NewSessionBroadcaster()
	ch1, id1 := b.Subscribe("sess1")
	ch2, id2 := b.Subscribe("sess1")
	defer b.Unsubscribe("sess1", id1)
	defer b.Unsubscribe("sess1", id2)

	b.Publish("sess1", makeTokenEvent("hi"))

	for i, ch := range []<-chan *pb.ChatEvent{ch1, ch2} {
		select {
		case got := <-ch:
			if got.GetToken().GetContent() != "hi" {
				t.Fatalf("subscriber %d: expected 'hi', got %q", i, got.GetToken().GetContent())
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out", i)
		}
	}
}

func TestSessionBroadcasterUnsubscribe(t *testing.T) {
	b := NewSessionBroadcaster()
	ch, subID := b.Subscribe("sess1")
	b.Unsubscribe("sess1", subID)

	// Channel should be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for channel close")
	}
}

func TestSessionBroadcasterPublishNoSubscribers(t *testing.T) {
	b := NewSessionBroadcaster()
	// Should not panic.
	b.Publish("no-such-session", makeTokenEvent("x"))
}
