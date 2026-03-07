package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/components"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/pages"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

// App is the root Bubbletea v2 model.
type App struct {
	client      *client.Client
	sessionID   string
	chat        pages.ChatModel
	team        pages.TeamModel
	sidebar     components.SidebarModel
	theme       theme.Theme
	dark        bool
	width       int
	height      int
	showSidebar bool
	showTeam    bool
	ready       bool
}

// NewApp creates the root TUI application model.
func NewApp(c *client.Client, session *pb.Session, t theme.Theme, dark bool) App {
	chat := pages.NewChat(c, session.GetId(), t, dark)
	team := pages.NewTeam()
	sidebar := components.NewSidebar([]*pb.Session{session}, session.GetId())
	return App{
		client:    c,
		sessionID: session.GetId(),
		chat:      chat,
		team:      team,
		sidebar:   sidebar,
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
			if a.showSidebar {
				a.showTeam = false
			}
		case "ctrl+t":
			a.showTeam = !a.showTeam
			if a.showTeam {
				a.showSidebar = false
			}
		}
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.ready = true
	case components.SessionSelectedMsg:
		a.sessionID = msg.SessionID
		a.showSidebar = false
	case components.SessionKillMsg:
		go func() {
			a.client.KillSession(context.Background(), msg.SessionID)
		}()
	}

	// Route key events to active panel
	if a.showSidebar {
		var sidebarCmd tea.Cmd
		a.sidebar, sidebarCmd = a.sidebar.Update(msg)
		cmds = append(cmds, sidebarCmd)
	} else if a.showTeam {
		var teamCmd tea.Cmd
		a.team, teamCmd = a.team.Update(msg)
		cmds = append(cmds, teamCmd)
	} else {
		var chatCmd tea.Cmd
		a.chat, chatCmd = a.chat.Update(msg)
		cmds = append(cmds, chatCmd)
	}

	return a, tea.Batch(cmds...)
}

func (a App) View() tea.View {
	if !a.ready {
		v := tea.NewView("Connecting to ratchet daemon...")
		return v
	}

	header := a.renderHeader()
	var body string

	switch {
	case a.showSidebar:
		sidebarWidth := 30
		if a.width > 0 && sidebarWidth > a.width/3 {
			sidebarWidth = a.width / 3
		}
		sidebarView := a.sidebar.SetSize(sidebarWidth, a.height-3).View(a.theme)
		chatView := a.chat.View(a.theme)
		body = joinColumns(sidebarView, chatView, sidebarWidth, a.width)
	case a.showTeam:
		teamView := a.team.SetSize(a.width, a.height-3).View(a.theme)
		body = teamView
	default:
		body = a.chat.View(a.theme)
	}

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

	hints := lipgloss.NewStyle().
		Foreground(a.theme.Muted).
		Render("  Ctrl+S: sidebar  Ctrl+T: team  Ctrl+D: detach  Ctrl+C: quit")

	return title + sessionInfo + hints
}

// joinColumns renders two column strings side by side.
func joinColumns(left, right string, leftWidth, totalWidth int) string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")

	maxLines := len(leftLines)
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}

	var sb strings.Builder
	for i := 0; i < maxLines; i++ {
		l := ""
		r := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		// Pad left column to fixed width
		padded := lipgloss.NewStyle().Width(leftWidth).Render(l)
		sb.WriteString(padded + "│" + r + "\n")
	}
	return sb.String()
}

// Run launches the TUI for a given session.
func Run(ctx context.Context, c *client.Client, session *pb.Session) error {
	t := theme.Dark()
	app := NewApp(c, session, t, true)

	p := tea.NewProgram(app, tea.WithContext(ctx))
	_, err := p.Run()
	return err
}
