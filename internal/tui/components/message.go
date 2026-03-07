package components

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

type Message struct {
	Role    MessageRole
	Content string
	Sender  string // model name or "You"
}

// Render returns a styled string for the message.
func (m Message) Render(t theme.Theme, width int, dark bool) string {
	switch m.Role {
	case RoleUser:
		prefix := lipgloss.NewStyle().Foreground(t.Primary).Bold(true).Render("You: ")
		body := t.UserMessage.Width(width - 4).Render(m.Content)
		return prefix + "\n" + body + "\n"
	case RoleAssistant:
		sender := m.Sender
		if sender == "" {
			sender = "Assistant"
		}
		prefix := lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Render(sender + ": ")
		body := RenderMarkdown(m.Content, width-4, dark)
		return prefix + "\n" + body
	case RoleTool:
		return fmt.Sprintf("  [tool: %s]\n", m.Content)
	}
	return m.Content
}
