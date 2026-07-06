package components

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

func TestStatusBarHintsFitNarrowWidth(t *testing.T) {
	for _, width := range []int{0, 4, 8, 12, 24} {
		if got := lipgloss.Width(statusBarHints(width)); got > max(width, 0) {
			t.Fatalf("status hints width = %d, want <= %d: %q", got, width, statusBarHints(width))
		}
	}

	bar := NewStatusBar()
	bar.Width = 24
	for _, line := range strings.Split(bar.View(theme.Dark()), "\n") {
		if got := lipgloss.Width(line); got > bar.Width {
			t.Fatalf("status bar line width = %d, want <= %d:\n%s", got, bar.Width, line)
		}
	}
}

func TestStatusBarHintsKeepQuitActionReadableWhenNarrow(t *testing.T) {
	for _, width := range []int{6, 8, 12} {
		hints := strings.ToLower(statusBarHints(width))
		if !strings.Contains(hints, "quit") {
			t.Fatalf("status hints for width %d = %q, want visible quit action", width, hints)
		}
	}
}
