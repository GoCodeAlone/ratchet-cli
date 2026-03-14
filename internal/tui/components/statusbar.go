package components

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

type StatusBar struct {
	WorkingDir      string
	Provider        string
	Model           string
	SessionStart    time.Time
	InputTokens     int
	OutputTokens    int
	ActiveAgents    int
	BackgroundTasks int
	Width           int
}

func NewStatusBar() StatusBar {
	return StatusBar{
		SessionStart: time.Now(),
	}
}

func (s StatusBar) View(t theme.Theme) string {
	// Line 1: contextual info
	dir := shortenPath(s.WorkingDir)
	elapsed := formatElapsed(time.Since(s.SessionStart))

	segments := []string{" " + dir}
	if s.Provider != "" && s.Model != "" {
		segments = append(segments, s.Provider+"/"+s.Model)
	} else if s.Model != "" {
		segments = append(segments, s.Model)
	} else if s.Provider != "" {
		segments = append(segments, s.Provider)
	}
	if s.ActiveAgents > 0 {
		segments = append(segments, fmt.Sprintf("agents: %d", s.ActiveAgents))
	}
	if s.BackgroundTasks > 0 {
		segments = append(segments, fmt.Sprintf("tasks: %d", s.BackgroundTasks))
	}
	segments = append(segments, "T "+elapsed)
	if s.InputTokens > 0 || s.OutputTokens > 0 {
		segments = append(segments, fmt.Sprintf("^%s v%s", formatTokens(s.InputTokens), formatTokens(s.OutputTokens)))
	}

	line1 := strings.Join(segments, "  ")

	// Line 2: keybind hints (right-aligned)
	hints := "Ctrl+S sidebar  Ctrl+T team  Ctrl+C quit "
	pad1 := s.Width - lipgloss.Width(line1)
	if pad1 < 0 {
		pad1 = 0
	}
	row1 := line1 + strings.Repeat(" ", pad1)

	pad2 := s.Width - lipgloss.Width(hints)
	if pad2 < 0 {
		pad2 = 0
	}
	row2 := strings.Repeat(" ", pad2) + hints

	return t.StatusBar.Width(s.Width).Render(row1 + "\n" + row2)
}

func shortenPath(p string) string {
	if p == "" {
		return "~"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

func formatElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func formatTokens(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
