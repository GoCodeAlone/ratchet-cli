package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

func TestSidebarSetCurrentUpdatesSelectedMarker(t *testing.T) {
	sidebar := NewSidebar([]*pb.Session{
		{Id: "root-session-12345678", Name: "root", Status: "active"},
		{Id: "fork-session-12345678", Name: "fork", Status: "active"},
	}, "root-session-12345678").SetSize(40, 10)

	sidebar = sidebar.SetCurrent("fork-session-12345678")
	view := sidebar.View(theme.Dark())

	if !strings.Contains(view, "● fork") {
		t.Fatalf("sidebar marker did not move to fork:\n%s", view)
	}
	if strings.Contains(view, "● root") {
		t.Fatalf("sidebar still marks root after selecting fork:\n%s", view)
	}
}

func TestSidebarHelpExplainsSessionManagement(t *testing.T) {
	sidebar := NewSidebar([]*pb.Session{
		{Id: "root-session-12345678", Name: "root", Status: "active"},
	}, "root-session-12345678").SetSize(32, 8)

	view := sidebar.View(theme.Dark())

	if !strings.Contains(view, "Sessions (1)") {
		t.Fatalf("sidebar title should include session count:\n%s", view)
	}
	for _, want := range []string{"Enter switch", "d kill", "Esc close", "Ctrl+S close", "Ctrl+C quit", "Ctrl+B tree"} {
		if !strings.Contains(view, want) {
			t.Fatalf("sidebar help missing %q:\n%s", want, view)
		}
	}
	for i, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > 32 {
			t.Fatalf("sidebar line %d width = %d, want <= 32: %q", i, got, line)
		}
	}
}

func TestSidebarSelectionUsesNonColorCursorMarker(t *testing.T) {
	sidebar := NewSidebar([]*pb.Session{
		{Id: "root-session-12345678", Name: "root", Status: "active"},
		{Id: "fork-session-12345678", Name: "fork", Status: "active"},
	}, "root-session-12345678").SetSize(36, 10)

	model, _ := sidebar.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	view := model.View(theme.Dark())

	if !strings.Contains(view, ">  fork") {
		t.Fatalf("sidebar selected row missing cursor marker:\n%s", view)
	}
	if !strings.Contains(view, " ● root") {
		t.Fatalf("sidebar current row missing current-session marker:\n%s", view)
	}
}
