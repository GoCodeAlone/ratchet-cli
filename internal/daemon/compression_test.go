package daemon

import (
	"context"
	"testing"

	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

func TestTokenTracker_ThresholdDetection(t *testing.T) {
	tt := NewTokenTracker()

	// Initially no tokens
	if tt.ShouldCompress("sess1", 0.9, 100000) {
		t.Error("empty tracker should not need compression")
	}

	// Add tokens below threshold
	tt.AddTokens("sess1", 40000, 40000) // 80000 total
	if tt.ShouldCompress("sess1", 0.9, 100000) {
		t.Error("80% should not trigger 90% threshold")
	}

	// Push over threshold
	tt.AddTokens("sess1", 5000, 5001) // 90001 total
	if !tt.ShouldCompress("sess1", 0.9, 100000) {
		t.Error("90001/100000 should trigger 90% threshold")
	}

	// Reset clears state
	tt.Reset("sess1")
	if tt.ShouldCompress("sess1", 0.9, 100000) {
		t.Error("after Reset, should not trigger compression")
	}
	if tt.Total("sess1") != 0 {
		t.Errorf("Total after Reset: got %d want 0", tt.Total("sess1"))
	}
}

func TestTokenTracker_MultipleSessionsIsolated(t *testing.T) {
	tt := NewTokenTracker()
	tt.AddTokens("sess1", 50000, 50000)
	tt.AddTokens("sess2", 100, 100)

	if !tt.ShouldCompress("sess1", 0.9, 100000) {
		t.Error("sess1 should need compression")
	}
	if tt.ShouldCompress("sess2", 0.9, 100000) {
		t.Error("sess2 should not need compression")
	}
}

func TestTokenTracker_ZeroLimitOrThreshold(t *testing.T) {
	tt := NewTokenTracker()
	tt.AddTokens("sess1", 999999, 999999)

	if tt.ShouldCompress("sess1", 0, 100000) {
		t.Error("zero threshold should never trigger")
	}
	if tt.ShouldCompress("sess1", 0.9, 0) {
		t.Error("zero model limit should never trigger")
	}
}

func TestCompression_SummarizeMessages(t *testing.T) {
	messages := []provider.Message{
		{Role: provider.RoleUser, Content: "How do I write a function in Go?"},
		{Role: provider.RoleAssistant, Content: "You can write a function using the func keyword..."},
		{Role: provider.RoleUser, Content: "What about error handling?"},
		{Role: provider.RoleAssistant, Content: "Go uses multiple return values for errors..."},
		{Role: provider.RoleUser, Content: "Thanks, can you show me an example?"},
		{Role: provider.RoleAssistant, Content: "Sure! Here is an example..."},
	}

	// Use nil provider — falls back to simple summary
	compressed, summary, err := Compress(context.Background(), messages, 2, nil)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}
	// Should have 1 system summary + 2 preserved
	if len(compressed) != 3 {
		t.Errorf("expected 3 messages (1 summary + 2 preserved), got %d", len(compressed))
	}
	if compressed[0].Role != provider.RoleSystem {
		t.Errorf("first message should be system, got %s", compressed[0].Role)
	}
	// Last 2 messages preserved
	if compressed[1].Content != messages[4].Content {
		t.Errorf("preserved[0] mismatch")
	}
	if compressed[2].Content != messages[5].Content {
		t.Errorf("preserved[1] mismatch")
	}
}

func TestCompression_PreservesRecent(t *testing.T) {
	messages := make([]provider.Message, 20)
	for i := range messages {
		messages[i] = provider.Message{Role: provider.RoleUser, Content: "message " + string(rune('a'+i))}
	}

	preserved := 5
	compressed, _, err := Compress(context.Background(), messages, preserved, nil)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	// 1 summary + 5 preserved = 6 messages
	if len(compressed) != preserved+1 {
		t.Errorf("expected %d messages, got %d", preserved+1, len(compressed))
	}
	// The last 5 original messages should be preserved
	for i := 0; i < preserved; i++ {
		expected := messages[len(messages)-preserved+i].Content
		got := compressed[i+1].Content
		if got != expected {
			t.Errorf("preserved[%d]: got %q want %q", i, got, expected)
		}
	}
}

func TestCompression_NoOpWhenFewMessages(t *testing.T) {
	messages := []provider.Message{
		{Role: provider.RoleUser, Content: "hello"},
		{Role: provider.RoleAssistant, Content: "world"},
	}
	// preserveCount >= len(messages) — nothing to compress
	compressed, summary, err := Compress(context.Background(), messages, 5, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary != "" {
		t.Errorf("expected empty summary, got %q", summary)
	}
	if len(compressed) != len(messages) {
		t.Errorf("expected %d messages unchanged, got %d", len(messages), len(compressed))
	}
}
