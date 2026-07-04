package pages

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/components"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

func TestChatCtrlHTogglesThinkingPanelWhenContentExists(t *testing.T) {
	model := NewChat(nil, "session-1", theme.Dark(), true)
	model.thinkingPanel = components.NewThinkingPanel(80).AppendContent("reasoning trace")

	model, _ = model.Update(tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl})
	if !model.thinkingPanel.Collapsed() {
		t.Fatal("ctrl+h did not collapse thinking panel with content")
	}

	model, _ = model.Update(tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl})
	if model.thinkingPanel.Collapsed() {
		t.Fatal("ctrl+h did not expand thinking panel with content")
	}
}
