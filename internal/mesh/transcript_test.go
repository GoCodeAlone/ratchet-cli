package mesh

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestTranscriptLogger_BBWrite(t *testing.T) {
	var buf bytes.Buffer
	bb := NewBlackboard()
	router := NewRouter()
	logger := NewTranscriptLogger(&buf, bb, router, "test-team")
	defer logger.Stop()

	bb.Write("plan", "design", "build a URL shortener", "orchestrator")

	// Give watcher time to fire.
	time.Sleep(50 * time.Millisecond)

	out := buf.String()
	if !strings.Contains(out, "BB WRITE plan/design by orchestrator") {
		t.Fatalf("expected BB WRITE log entry, got:\n%s", out)
	}
}

func TestTranscriptLogger_Message(t *testing.T) {
	var buf bytes.Buffer
	bb := NewBlackboard()
	router := NewRouter()
	logger := NewTranscriptLogger(&buf, bb, router, "test-team")
	defer logger.Stop()

	logger.LogMessage(Message{
		From:    "orchestrator",
		To:      "claude_code",
		Type:    "task",
		Content: "Implement the design",
	})

	out := buf.String()
	if !strings.Contains(out, "MSG orchestrator → claude_code") {
		t.Fatalf("expected MSG log entry, got:\n%s", out)
	}
}

func TestTranscriptLogger_TeamLifecycle(t *testing.T) {
	var buf bytes.Buffer
	bb := NewBlackboard()
	router := NewRouter()
	logger := NewTranscriptLogger(&buf, bb, router, "test-team")

	logger.LogStart("Build email validator")
	bb.Write("plan", "design", "regex approach", "orchestrator")
	time.Sleep(50 * time.Millisecond)
	logger.LogComplete(3, 1)

	out := buf.String()
	if !strings.Contains(out, "TEAM test-team STARTED") {
		t.Fatalf("expected STARTED, got:\n%s", out)
	}
	if !strings.Contains(out, "TEAM test-team COMPLETED") {
		t.Fatalf("expected COMPLETED, got:\n%s", out)
	}
}
