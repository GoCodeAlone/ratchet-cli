package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// ThinkingPanel displays a collapsible panel for model reasoning traces.
// It auto-starts expanded and can be toggled with Ctrl+H.
type ThinkingPanel struct {
	content   string
	collapsed bool
	width     int
}

// NewThinkingPanel creates a new thinking panel.
func NewThinkingPanel(width int) ThinkingPanel {
	return ThinkingPanel{width: width}
}

// AppendContent adds text to the thinking panel.
func (p ThinkingPanel) AppendContent(text string) ThinkingPanel {
	p.content += text
	return p
}

// SetCollapsed sets the collapsed state.
func (p ThinkingPanel) SetCollapsed(collapsed bool) ThinkingPanel {
	p.collapsed = collapsed
	return p
}

// ToggleCollapsed flips the collapsed state.
func (p ThinkingPanel) ToggleCollapsed() ThinkingPanel {
	p.collapsed = !p.collapsed
	return p
}

// Reset clears the content and resets to expanded state.
func (p ThinkingPanel) Reset() ThinkingPanel {
	p.content = ""
	p.collapsed = false
	return p
}

// HasContent returns true if the panel has any content.
func (p ThinkingPanel) HasContent() bool {
	return p.content != ""
}

// SetWidth updates the panel width.
func (p ThinkingPanel) SetWidth(w int) ThinkingPanel {
	p.width = w
	return p
}

// View renders the panel. Returns empty string if no content.
func (p ThinkingPanel) View() string {
	if p.content == "" {
		return ""
	}

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Bold(true)

	contentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Italic(true)

	lines := strings.Split(strings.TrimRight(p.content, "\n"), "\n")
	lineCount := len(lines)

	if p.collapsed {
		header := headerStyle.Render(fmt.Sprintf("▶ Thinking (%d lines)", lineCount))
		return header
	}

	header := headerStyle.Render("▼ Thinking")
	body := contentStyle.Render(p.content)
	return header + "\n" + body
}
