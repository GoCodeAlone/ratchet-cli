package components

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

type StatusBar struct {
	Provider     string
	Model        string
	SessionStart time.Time
	TokenCount   int
	ActiveAgents int
	Width        int
}

func NewStatusBar() StatusBar {
	return StatusBar{
		SessionStart: time.Now(),
	}
}

func (s StatusBar) View(t theme.Theme) string {
	elapsed := time.Since(s.SessionStart).Round(time.Second).String()

	left := fmt.Sprintf(" %s/%s  %s", s.Provider, s.Model, elapsed)
	if s.ActiveAgents > 0 {
		left += fmt.Sprintf("  agents: %d", s.ActiveAgents)
	}
	if s.TokenCount > 0 {
		left += fmt.Sprintf("  tokens: %d", s.TokenCount)
	}

	hints := "Ctrl+D: detach  Ctrl+S: sidebar  Ctrl+C: quit"
	right := hints + " "

	// Pad to fill width
	padding := s.Width - lipgloss.Width(left) - lipgloss.Width(right)
	if padding < 0 {
		padding = 0
	}

	bar := left + strings.Repeat(" ", padding) + right
	return t.StatusBar.Width(s.Width).Render(bar)
}
