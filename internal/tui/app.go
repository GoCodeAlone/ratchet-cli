package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/pages"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

// App is the root Bubbletea v2 model.
type App struct {
	client      *client.Client
	sessionID   string
	chat        pages.ChatModel
	theme       theme.Theme
	dark        bool
	width       int
	height      int
	showSidebar bool
	ready       bool
}

// NewApp creates the root TUI application model.
func NewApp(c *client.Client, session *pb.Session, t theme.Theme, dark bool) App {
	chat := pages.NewChat(c, session.GetId(), t, dark)
	return App{
		client:    c,
		sessionID: session.GetId(),
		chat:      chat,
		theme:     t,
		dark:      dark,
	}
}

func (a App) Init() tea.Cmd {
	return tea.Batch(
		a.chat.Init(),
	)
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return a, tea.Quit
		case "ctrl+d":
			// Detach: quit TUI, leave session running
			return a, tea.Quit
		case "ctrl+s":
			a.showSidebar = !a.showSidebar
		}
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.ready = true
	}

	var chatCmd tea.Cmd
	a.chat, chatCmd = a.chat.Update(msg)
	cmds = append(cmds, chatCmd)

	return a, tea.Batch(cmds...)
}

func (a App) View() tea.View {
	if !a.ready {
		v := tea.NewView("Connecting to ratchet daemon...")
		return v
	}

	header := a.renderHeader()
	body := a.chat.View(a.theme)

	content := header + "\n" + body

	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

func (a App) renderHeader() string {
	title := lipgloss.NewStyle().
		Foreground(a.theme.Primary).
		Bold(true).
		Render("ratchet")

	sessionInfo := lipgloss.NewStyle().
		Foreground(a.theme.Muted).
		Render(fmt.Sprintf("  session: %s", a.sessionID[:8]))

	return title + sessionInfo
}

// Run launches the TUI for a given session.
func Run(ctx context.Context, c *client.Client, session *pb.Session) error {
	t := theme.Dark()
	app := NewApp(c, session, t, true)

	p := tea.NewProgram(app, tea.WithContext(ctx))
	_, err := p.Run()
	return err
}
