package mesh

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestBBBridge_FormatPrompt(t *testing.T) {
	bb := NewBlackboard()
	bb.Write("plan", "design", "Build a URL shortener with Go", "orchestrator")

	bridge := &BBBridge{
		agentName:   "claude_code",
		role:        "implementation",
		teamMembers: []string{"orchestrator", "copilot"},
		bb:          bb,
	}

	msg := Message{
		From:    "orchestrator",
		To:      "claude_code",
		Type:    "task",
		Content: "Implement the URL shortener based on the design.",
	}

	prompt := bridge.FormatPrompt(msg)

	if !strings.Contains(prompt, "[TEAM CONTEXT]") {
		t.Fatal("missing TEAM CONTEXT header")
	}
	if !strings.Contains(prompt, "claude_code") {
		t.Fatal("missing agent name")
	}
	if !strings.Contains(prompt, "[BLACKBOARD — plan]") {
		t.Fatal("missing blackboard section")
	}
	if !strings.Contains(prompt, "Build a URL shortener") {
		t.Fatal("missing BB content")
	}
	if !strings.Contains(prompt, "[TASK FROM orchestrator]") {
		t.Fatal("missing task header")
	}
	if !strings.Contains(prompt, "[RESULT:") {
		t.Fatal("missing RESULT instruction")
	}
}

func TestBBBridge_ParseResponse(t *testing.T) {
	bb := NewBlackboard()
	var buf bytes.Buffer
	router := NewRouter()
	logger := NewTranscriptLogger(&buf, bb, router, "test")
	defer logger.Stop()

	bridge := &BBBridge{
		agentName:  "claude_code",
		role:       "implementation",
		bb:         bb,
		transcript: logger,
	}

	response := "Here is the implementation...\n[RESULT: implemented URL shortener with tests]"
	bridge.ParseResponse(response)

	// Check artifact was written to BB.
	entries := bb.List("artifacts")
	if entries == nil {
		t.Fatal("expected artifacts section")
	}
	found := false
	for k := range entries {
		if strings.HasPrefix(k, "claude_code/") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected artifact key starting with claude_code/")
	}

	// Check status was written.
	e, ok := bb.Read("status", "claude_code")
	if !ok {
		t.Fatal("expected status entry")
	}
	if e.Value != "done" {
		t.Fatalf("expected status=done, got %v", e.Value)
	}
}

func TestBBBridge_RunLoop(t *testing.T) {
	bb := NewBlackboard()
	bb.Write("plan", "design", "test design", "orchestrator")

	var buf bytes.Buffer
	router := NewRouter()
	logger := NewTranscriptLogger(&buf, bb, router, "test")
	defer logger.Stop()

	inbox := make(chan Message, 1)
	outbox := make(chan Message, 10)

	// Mock PTY sender: just echoes back a result.
	mockSend := func(_ context.Context, prompt string) (string, error) {
		return "Done.\n[RESULT: completed the task]", nil
	}

	bridge := &BBBridge{
		agentName:   "claude_code",
		role:        "implementation",
		teamMembers: []string{"orchestrator"},
		bb:          bb,
		transcript:  logger,
		sendToPTY:   mockSend,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Send a task message.
	inbox <- Message{
		From:    "orchestrator",
		To:      "claude_code",
		Type:    "task",
		Content: "Do the thing",
	}
	close(inbox)

	err := bridge.Run(ctx, "initial task", bb, inbox, outbox)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify outbox got a result message back to orchestrator.
	select {
	case msg := <-outbox:
		if msg.To != "orchestrator" {
			t.Fatalf("expected message to orchestrator, got to=%s", msg.To)
		}
		if !strings.Contains(msg.Content, "completed the task") {
			t.Fatalf("expected result content, got: %s", msg.Content)
		}
	default:
		t.Fatal("expected outbox message")
	}
}
