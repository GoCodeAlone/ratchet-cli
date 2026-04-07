package mesh

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// mockPTYSender returns a canned response for testing.
func mockPTYSender(name string) func(context.Context, string) (string, error) {
	return func(_ context.Context, prompt string) (string, error) {
		switch name {
		case "claude_code":
			return "package main\n\nfunc validate(email string) bool { return true }\n\n[RESULT: implemented email validator]", nil
		case "copilot":
			return "Code looks good, no issues found.\n\n[RESULT: approved with no changes]", nil
		default:
			return "[RESULT: done]", nil
		}
	}
}

func TestOrchestration_EndToEnd(t *testing.T) {
	bb := NewBlackboard()
	// Seed BB sections like SpawnTeam does.
	for _, section := range []string{"plan", "code", "reviews", "status", "artifacts"} {
		bb.Write(section, "_init", "empty", "mesh")
	}

	router := NewRouter()
	var transcript bytes.Buffer
	logger := NewTranscriptLogger(&transcript, bb, router, "test-orchestration")
	defer logger.Stop()
	logger.LogStart("Build email validator")

	// Simulate orchestrator writing plan.
	bb.Write("plan", "design", "Go function using regex to validate emails", "orchestrator")

	// Create BB Bridges for both PTY agents.
	claudeBridge := NewBBBridge(
		"claude_code", "implementation",
		[]string{"orchestrator", "copilot"},
		"", // workDir: use provider default in tests
		bb, logger,
		mockPTYSender("claude_code"),
	)
	copilotBridge := NewBBBridge(
		"copilot", "review",
		[]string{"orchestrator", "claude_code"},
		"", // workDir: use provider default in tests
		bb, logger,
		mockPTYSender("copilot"),
	)

	// Wire up inboxes/outboxes.
	claudeInbox := make(chan Message, 10)
	claudeOutbox := make(chan Message, 10)
	copilotInbox := make(chan Message, 10)
	copilotOutbox := make(chan Message, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start bridges in goroutines.
	errCh := make(chan error, 2)
	go func() { errCh <- claudeBridge.Run(ctx, "", bb, claudeInbox, claudeOutbox) }()
	go func() { errCh <- copilotBridge.Run(ctx, "", bb, copilotInbox, copilotOutbox) }()

	// Simulate orchestrator sending task to claude_code.
	claudeInbox <- Message{
		From:    "orchestrator",
		To:      "claude_code",
		Type:    "task",
		Content: "Implement the email validator based on the design in BB plan/design.",
	}

	// Wait for claude_code's response.
	select {
	case msg := <-claudeOutbox:
		if !strings.Contains(msg.Content, "implemented email validator") {
			t.Fatalf("unexpected claude_code result: %s", msg.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for claude_code response")
	}

	// Verify artifact written.
	artifacts := bb.List("artifacts")
	foundClaudeArtifact := false
	for k := range artifacts {
		if strings.HasPrefix(k, "claude_code/") {
			foundClaudeArtifact = true
		}
	}
	if !foundClaudeArtifact {
		t.Fatal("claude_code artifact not found in BB")
	}

	// Simulate orchestrator sending review task to copilot.
	copilotInbox <- Message{
		From:    "orchestrator",
		To:      "copilot",
		Type:    "task",
		Content: "Review the code in BB artifacts for correctness.",
	}

	select {
	case msg := <-copilotOutbox:
		if !strings.Contains(msg.Content, "approved") {
			t.Fatalf("unexpected copilot result: %s", msg.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for copilot response")
	}

	// Verify status entries.
	if e, ok := bb.Read("status", "claude_code"); !ok || e.Value != "done" {
		t.Fatal("claude_code status not set to done")
	}
	if e, ok := bb.Read("status", "copilot"); !ok || e.Value != "done" {
		t.Fatal("copilot status not set to done")
	}

	// Close inboxes to end bridges.
	close(claudeInbox)
	close(copilotInbox)

	for range 2 {
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("bridge error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for bridges to exit")
		}
	}

	logger.LogComplete(3, logger.Writes())

	// Verify transcript has key events.
	out := transcript.String()
	if !strings.Contains(out, "TEAM test-orchestration STARTED") {
		t.Error("transcript missing STARTED")
	}
	if !strings.Contains(out, "BB WRITE plan/design") {
		t.Error("transcript missing BB WRITE for plan")
	}
	if !strings.Contains(out, "MSG orchestrator → claude_code") {
		t.Error("transcript missing MSG to claude_code")
	}
	if !strings.Contains(out, "TEAM test-orchestration COMPLETED") {
		t.Error("transcript missing COMPLETED")
	}
}
