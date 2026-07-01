package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/components"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/pages"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

type SessionBrowserClient interface {
	GetSessionTree(context.Context, string) (*pb.SessionList, error)
	ListSessionMessages(context.Context, string) (*pb.SessionHistory, error)
}

func RunSessionBrowser(ctx context.Context, c SessionBrowserClient, rootID string) (string, error) {
	if rootID == "" {
		return "", fmt.Errorf("session id is required")
	}
	model := sessionBrowserProgram{
		browser: pages.NewSessionTreeBrowser(c, rootID, theme.Dark()).SetSize(100, 30),
	}
	finalModel, err := tea.NewProgram(model, tea.WithContext(ctx)).Run()
	if err != nil {
		return "", err
	}
	if final, ok := finalModel.(sessionBrowserProgram); ok {
		return final.selectedSessionID, nil
	}
	return "", nil
}

type sessionBrowserProgram struct {
	browser           pages.SessionTreeBrowser
	selectedSessionID string
}

func (m sessionBrowserProgram) Init() tea.Cmd {
	return m.browser.Init()
}

func (m sessionBrowserProgram) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.browser = m.browser.SetSize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			return m, tea.Quit
		}
	case components.SessionTreeSelectedMsg:
		m.selectedSessionID = msg.SessionID
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.browser, cmd = m.browser.Update(msg)
	return m, cmd
}

func (m sessionBrowserProgram) View() tea.View {
	view := tea.NewView(m.browser.View())
	view.AltScreen = true
	return view
}
