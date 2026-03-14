package daemon

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

// TokenTracker tracks input/output token usage per session.
type TokenTracker struct {
	mu     sync.RWMutex
	totals map[string]*sessionTokens
}

type sessionTokens struct {
	input  int
	output int
}

func NewTokenTracker() *TokenTracker {
	return &TokenTracker{
		totals: make(map[string]*sessionTokens),
	}
}

// AddTokens updates the running token count for a session.
func (t *TokenTracker) AddTokens(sessionID string, input, output int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.totals[sessionID]
	if !ok {
		s = &sessionTokens{}
		t.totals[sessionID] = s
	}
	s.input += input
	s.output += output
}

// Total returns the combined input+output token count for a session.
func (t *TokenTracker) Total(sessionID string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if s, ok := t.totals[sessionID]; ok {
		return s.input + s.output
	}
	return 0
}

// Reset clears the token count for a session (after compression).
func (t *TokenTracker) Reset(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.totals, sessionID)
}

// ShouldCompress returns true when the session token total exceeds threshold
// fraction of modelLimit.
func (t *TokenTracker) ShouldCompress(sessionID string, threshold float64, modelLimit int) bool {
	if modelLimit <= 0 || threshold <= 0 {
		return false
	}
	total := t.Total(sessionID)
	return float64(total) >= threshold*float64(modelLimit)
}

// Compress summarizes older messages using a fast provider call and returns
// the compressed history (summary message + preserved recent messages) plus
// the summary text.
//
// preserveCount controls how many of the most recent messages are kept verbatim.
// If no provider is given, a simple concatenation summary is used.
func Compress(ctx context.Context, messages []provider.Message, preserveCount int, prov provider.Provider) ([]provider.Message, string, error) {
	if preserveCount < 0 {
		preserveCount = 0
	}
	if len(messages) <= preserveCount {
		return messages, "", nil
	}

	splitAt := len(messages) - preserveCount
	toSummarize := messages[:splitAt]
	toKeep := messages[splitAt:]

	summary, err := summarize(ctx, toSummarize, prov)
	if err != nil {
		return messages, "", fmt.Errorf("summarize: %w", err)
	}

	compressed := []provider.Message{
		{
			Role:    provider.RoleSystem,
			Content: "[Conversation summary]\n" + summary,
		},
	}
	compressed = append(compressed, toKeep...)
	return compressed, summary, nil
}

// summarize produces a text summary of a message slice.
// Uses the provider if available, otherwise falls back to a simple join.
func summarize(ctx context.Context, messages []provider.Message, prov provider.Provider) (string, error) {
	if prov == nil || len(messages) == 0 {
		return buildFallbackSummary(messages), nil
	}

	var sb strings.Builder
	sb.WriteString("Summarize this conversation history concisely in 2-3 sentences. Focus on key decisions, context, and outcomes. Do not include greetings or pleasantries.\n\nConversation:\n")
	for _, m := range messages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, m.Content))
	}

	req := []provider.Message{
		{Role: provider.RoleUser, Content: sb.String()},
	}

	ch, err := prov.Stream(ctx, req, nil)
	if err != nil {
		return buildFallbackSummary(messages), nil
	}

	var result strings.Builder
	for event := range ch {
		if event.Type == "text" {
			result.WriteString(event.Text)
		}
		if event.Type == "error" {
			break
		}
	}
	if result.Len() == 0 {
		return buildFallbackSummary(messages), nil
	}
	return result.String(), nil
}

// buildFallbackSummary produces a simple text summary without a provider call.
func buildFallbackSummary(messages []provider.Message) string {
	if len(messages) == 0 {
		return "(no prior context)"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Compressed %d messages. Topics covered: ", len(messages)))
	seen := make(map[string]bool)
	var topics []string
	for _, m := range messages {
		if m.Role == provider.RoleUser && len(m.Content) > 0 {
			// Use first ~50 runes of each user message as a topic hint
			snippet := m.Content
			if runes := []rune(snippet); len(runes) > 50 {
				snippet = string(runes[:50]) + "..."
			}
			if !seen[snippet] {
				seen[snippet] = true
				topics = append(topics, snippet)
			}
		}
	}
	if len(topics) > 3 {
		topics = topics[:3]
	}
	if len(topics) > 0 {
		sb.WriteString(strings.Join(topics, "; "))
	} else {
		sb.WriteString("(assistant responses only)")
	}
	return sb.String()
}
