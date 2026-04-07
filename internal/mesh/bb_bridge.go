package mesh

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// BBBridge translates mesh messages into PTY prompts and parses responses
// back into BB writes and outgoing messages.
type BBBridge struct {
	agentName   string
	role        string
	teamMembers []string
	workDir     string // working directory passed to the PTY provider session
	bb          *Blackboard
	transcript  *TranscriptLogger
	sendToPTY   func(ctx context.Context, prompt string) (string, error)
}

// NewBBBridge creates a bridge for a PTY-backed agent.
// workDir is the working directory for the underlying PTY provider session;
// pass "" to use the provider's default.
func NewBBBridge(
	agentName, role string,
	teamMembers []string,
	workDir string,
	bb *Blackboard,
	transcript *TranscriptLogger,
	sendToPTY func(ctx context.Context, prompt string) (string, error),
) *BBBridge {
	return &BBBridge{
		agentName:   agentName,
		role:        role,
		teamMembers: teamMembers,
		workDir:     workDir,
		bb:          bb,
		transcript:  transcript,
		sendToPTY:   sendToPTY,
	}
}

// FormatPrompt builds a rich prompt from a mesh message, including BB state.
func (b *BBBridge) FormatPrompt(msg Message) string {
	var sb strings.Builder

	// Team context.
	sb.WriteString("[TEAM CONTEXT]\n")
	sb.WriteString(fmt.Sprintf("You are %q (%s role) in a multi-agent team.\n", b.agentName, b.role))
	sb.WriteString(fmt.Sprintf("The orchestrator is directing you. Other team members: %s\n\n",
		strings.Join(b.teamMembers, ", ")))

	// Inject relevant BB sections.
	for _, section := range b.bb.ListSections() {
		entries := b.bb.List(section)
		if len(entries) == 0 {
			continue
		}
		// Skip internal init entries.
		hasReal := false
		for k, e := range entries {
			if k != "_init" {
				if !hasReal {
					sb.WriteString(fmt.Sprintf("[BLACKBOARD — %s]\n", section))
					hasReal = true
				}
				v := fmt.Sprintf("%v", e.Value)
				if len(v) > 2000 {
					v = v[:2000] + "...(truncated)"
				}
				sb.WriteString(fmt.Sprintf("%s: %s\n", k, v))
			}
		}
		if hasReal {
			sb.WriteString("\n")
		}
	}

	// Task.
	sb.WriteString(fmt.Sprintf("[TASK FROM %s]\n", msg.From))
	sb.WriteString(msg.Content)
	sb.WriteString("\n\nWhen done, end your response with [RESULT: <one-line summary>].\n")

	return sb.String()
}

// ParseResponse extracts result markers and writes artifacts/status to BB.
func (b *BBBridge) ParseResponse(response string) string {
	// Write full response as artifact.
	artifactKey := fmt.Sprintf("%s/%s", b.agentName, uuid.NewString()[:8])
	b.bb.Write("artifacts", artifactKey, response, b.agentName)

	// Extract [RESULT: ...] marker.
	var resultSummary string
	if idx := strings.Index(response, "[RESULT:"); idx >= 0 {
		end := strings.Index(response[idx:], "]")
		if end > 0 {
			resultSummary = strings.TrimSpace(response[idx+8 : idx+end])
		}
		b.bb.Write("status", b.agentName, "done", b.agentName)
	}

	if resultSummary == "" {
		resultSummary = truncate(response, 200)
	}
	return resultSummary
}

// Run is the agent loop for PTY nodes. It processes inbox messages, sends
// prompts to the PTY, parses responses, and writes results to BB/outbox.
func (b *BBBridge) Run(ctx context.Context, _ string, _ *Blackboard, inbox <-chan Message, outbox chan<- Message) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-inbox:
			if !ok {
				return nil
			}
			if b.transcript != nil {
				b.transcript.LogMessage(msg)
			}

			prompt := b.FormatPrompt(msg)
			response, err := b.sendToPTY(ctx, prompt)
			if err != nil {
				return fmt.Errorf("PTY send for %s: %w", b.agentName, err)
			}

			resultSummary := b.ParseResponse(response)

			// Send result back to the sender.
			outMsg := Message{
				ID:        uuid.New().String(),
				From:      b.agentName,
				To:        msg.From,
				Type:      "result",
				Content:   resultSummary,
				Timestamp: time.Now(),
			}
			if b.transcript != nil {
				b.transcript.LogMessage(outMsg)
			}
			select {
			case outbox <- outMsg:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
