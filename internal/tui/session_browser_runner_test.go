package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/tui/components"
)

func TestSessionBrowserProgramSelectionQuitsAndStoresID(t *testing.T) {
	model := sessionBrowserProgram{}

	updated, cmd := model.Update(components.SessionTreeSelectedMsg{SessionID: "fork-1"})
	if cmd == nil {
		t.Fatal("selection did not request quit")
	}

	final, ok := updated.(sessionBrowserProgram)
	if !ok {
		t.Fatalf("updated model = %T, want sessionBrowserProgram", updated)
	}
	if final.selectedSessionID != "fork-1" {
		t.Fatalf("selected session = %q, want fork-1", final.selectedSessionID)
	}

	if got := cmd(); got != (tea.QuitMsg{}) {
		t.Fatalf("selection cmd = %T, want tea.QuitMsg", got)
	}
}
