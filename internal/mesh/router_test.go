package mesh

import (
	"testing"
	"time"
)

func TestRouter_Unicast(t *testing.T) {
	r := NewRouter()
	inbox, err := r.Register("node-a")
	if err != nil {
		t.Fatal(err)
	}

	msg := Message{
		ID:        "m1",
		From:      "node-b",
		To:        "node-a",
		Type:      "task",
		Content:   "hello",
		Timestamp: time.Now(),
	}
	if err := r.Send(msg); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-inbox:
		if got.Content != "hello" {
			t.Fatalf("got %q, want %q", got.Content, "hello")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestRouter_Broadcast(t *testing.T) {
	r := NewRouter()
	inboxA, _ := r.Register("a")
	inboxB, _ := r.Register("b")
	_, _ = r.Register("sender") // sender should not receive own broadcast

	msg := Message{
		ID:   "m2",
		From: "sender",
		To:   "*",
		Type: "feedback",
	}
	if err := r.Send(msg); err != nil {
		t.Fatal(err)
	}

	// Both a and b should receive it
	for _, inbox := range []<-chan Message{inboxA, inboxB} {
		select {
		case <-inbox:
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for broadcast")
		}
	}
}

func TestRouter_UnregisteredTarget(t *testing.T) {
	r := NewRouter()
	msg := Message{To: "ghost", From: "sender"}
	err := r.Send(msg)
	if err == nil {
		t.Fatal("expected error for unregistered target")
	}
}

func TestRouter_DuplicateRegister(t *testing.T) {
	r := NewRouter()
	_, err := r.Register("dup")
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Register("dup")
	if err == nil {
		t.Fatal("expected error for duplicate register")
	}
}

func TestRouter_Unregister(t *testing.T) {
	r := NewRouter()
	inbox, _ := r.Register("temp")
	r.Unregister("temp")

	// Channel should be closed
	_, ok := <-inbox
	if ok {
		t.Fatal("expected closed channel after unregister")
	}

	// Sending to unregistered node should fail
	err := r.Send(Message{To: "temp", From: "other"})
	if err == nil {
		t.Fatal("expected error sending to unregistered node")
	}
}
