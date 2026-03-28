package tui

import (
	"context"
	"fmt"
	"log"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/components"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/pages"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

type appPage int

const (
	pageSplash appPage = iota
	pageOnboarding
	pageChat
)

// ProvidersCheckedMsg carries the result of the async provider list check.
type ProvidersCheckedMsg struct {
	Providers []*pb.Provider
}

// VersionNoticeMsg carries a version compatibility message for the TUI.
type VersionNoticeMsg struct {
	Compatible        bool
	ReloadRecommended bool
	Message           string
}

// RPCErrorMsg carries an error from an async RPC call.
type RPCErrorMsg struct {
	Op  string
	Err error
}

// App is the root Bubbletea v2 model.
type App struct {
	client      *client.Client
	sessionID   string
	session     *pb.Session
	chat        pages.ChatModel
	team        pages.TeamModel
	splash      pages.SplashModel
	onboarding  pages.OnboardingModel
	sidebar     components.SidebarModel
	jobPanel    components.JobPanel
	theme       theme.Theme
	dark        bool
	width       int
	height      int
	showSidebar bool
	showTeam    bool
	showJobs    bool
	ready       bool
	page        appPage

	// Coordination between splash animation and provider check.
	splashDone     bool
	providersReady bool
	providers      []*pb.Provider
	reconfigure    bool

	// Version notice shown in the header when daemon and CLI differ.
	versionNotice string
}

// NewApp creates the root TUI application model.
func NewApp(c *client.Client, session *pb.Session, t theme.Theme, dark bool, reconfigure ...bool) App {
	splash := pages.NewSplash()
	sidebar := components.NewSidebar([]*pb.Session{session}, session.GetId())
	reconf := len(reconfigure) > 0 && reconfigure[0]
	return App{
		client:      c,
		sessionID:   session.GetId(),
		session:     session,
		splash:      splash,
		sidebar:     sidebar,
		theme:       t,
		dark:        dark,
		page:        pageSplash,
		reconfigure: reconf,
	}
}

func (a App) Init() tea.Cmd {
	return tea.Batch(
		a.splash.Init(),
		a.checkProviders(),
		a.checkVersion(),
	)
}

func (a App) checkVersion() tea.Cmd {
	return func() tea.Msg {
		resp, err := a.client.EnsureCompatible()
		if err != nil {
			return nil
		}
		if resp.Compatible && !resp.ReloadRecommended {
			return nil
		}
		return VersionNoticeMsg{
			Compatible:        resp.Compatible,
			ReloadRecommended: resp.ReloadRecommended,
			Message:           resp.Message,
		}
	}
}

func (a App) checkProviders() tea.Cmd {
	return func() tea.Msg {
		resp, err := a.client.ListProviders(context.Background())
		if err != nil {
			return ProvidersCheckedMsg{Providers: nil}
		}
		return ProvidersCheckedMsg{Providers: resp.Providers}
	}
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.ready = true
		// Propagate size to chat if active (header takes 1 line)
		if a.page == pageChat {
			chatHeight := a.height - 1
			if chatHeight < 1 {
				chatHeight = 1
			}
			a.chat.SetSize(a.width, chatHeight)
		}

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
		// Chat-only shortcuts
		if a.page == pageChat {
			switch msg.String() {
			case "ctrl+d":
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
					a.showJobs = false
				}
			case "ctrl+j":
				a.showJobs = !a.showJobs
				if a.showJobs {
					a.showSidebar = false
					a.showTeam = false
				}
			}
		}

	case pages.SplashDoneMsg:
		a.splashDone = true
		if a.providersReady {
			return a.transitionFromSplash()
		}
		return a, nil

	case ProvidersCheckedMsg:
		a.providersReady = true
		a.providers = msg.Providers
		if a.splashDone {
			return a.transitionFromSplash()
		}
		return a, nil

	case pages.OnboardingDoneMsg:
		return a.transitionToChat()

	case pages.NavigateToOnboardingMsg:
		a.onboarding = pages.NewOnboarding(a.client, a.theme)
		a.page = pageOnboarding
		return a, a.onboarding.Init()

	case components.SessionSelectedMsg:
		a.sessionID = msg.SessionID
		a.showSidebar = false

	case components.SessionKillMsg:
		sessionID := msg.SessionID
		return a, func() tea.Msg {
			if err := a.client.KillSession(context.Background(), sessionID); err != nil {
				return RPCErrorMsg{Op: "KillSession", Err: err}
			}
			return nil
		}

	case pages.KillAgentMsg:
		agentID := msg.AgentID
		return a, func() tea.Msg {
			if err := a.client.KillAgent(context.Background(), agentID); err != nil {
				return RPCErrorMsg{Op: "KillAgent", Err: err}
			}
			return nil
		}

	case RPCErrorMsg:
		log.Printf("rpc %s error: %v", msg.Op, msg.Err)

	case VersionNoticeMsg:
		if !msg.Compatible {
			a.versionNotice = "daemon incompatible: " + msg.Message
		} else if msg.ReloadRecommended {
			a.versionNotice = "version mismatch: " + msg.Message
		}
	}

	// Route updates to active page
	switch a.page {
	case pageSplash:
		var splashCmd tea.Cmd
		a.splash, splashCmd = a.splash.Update(msg)
		cmds = append(cmds, splashCmd)

	case pageOnboarding:
		var obCmd tea.Cmd
		a.onboarding, obCmd = a.onboarding.Update(msg)
		cmds = append(cmds, obCmd)

	case pageChat:
		if a.showSidebar {
			var sidebarCmd tea.Cmd
			a.sidebar, sidebarCmd = a.sidebar.Update(msg)
			cmds = append(cmds, sidebarCmd)
		} else if a.showTeam {
			var teamCmd tea.Cmd
			a.team, teamCmd = a.team.Update(msg)
			cmds = append(cmds, teamCmd)
		} else if a.showJobs {
			var jpCmd tea.Cmd
			a.jobPanel, jpCmd = a.jobPanel.Update(msg)
			cmds = append(cmds, jpCmd)
			// Escape closes the job panel
			if kp, ok := msg.(tea.KeyPressMsg); ok && kp.String() == "esc" {
				a.showJobs = false
			}
		} else {
			var chatCmd tea.Cmd
			a.chat, chatCmd = a.chat.Update(msg)
			cmds = append(cmds, chatCmd)
		}
	}

	return a, tea.Batch(cmds...)
}

func (a App) transitionFromSplash() (tea.Model, tea.Cmd) {
	if a.reconfigure || len(a.providers) == 0 {
		a.onboarding = pages.NewOnboarding(a.client, a.theme)
		a.page = pageOnboarding
		return a, a.onboarding.Init()
	}
	return a.transitionToChat()
}

func (a App) transitionToChat() (tea.Model, tea.Cmd) {
	chat := pages.NewChat(a.client, a.sessionID, a.theme, a.dark)
	team := pages.NewTeam()
	// Pass known dimensions — chat won't get an initial WindowSizeMsg
	// since that was consumed during the splash screen.
	chatHeight := a.height - 1 // reserve 1 line for header
	if chatHeight < 1 {
		chatHeight = 1
	}
	chat.SetSize(a.width, chatHeight)
	// Set status bar context
	if a.session != nil {
		chat.SetWorkingDir(a.session.GetWorkingDir())
	}
	for _, p := range a.providers {
		if p.IsDefault {
			chat.SetProviderModel(p.Type, p.Model)
			break
		}
	}
	a.chat = chat
	a.team = team
	a.jobPanel = components.NewJobPanel(a.client)
	a.page = pageChat
	return a, tea.Batch(a.chat.Init(), a.jobPanel.Init())
}

func (a App) View() tea.View {
	if !a.ready {
		v := tea.NewView("Connecting to ratchet daemon...")
		return v
	}

	var content string

	switch a.page {
	case pageSplash:
		content = a.splash.View(a.theme, a.width, a.height)

	case pageOnboarding:
		content = a.onboarding.View(a.theme, a.width, a.height)

	case pageChat:
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
		case a.showJobs:
			body = a.jobPanel.SetSize(a.width, a.height-3).View(a.theme)
		default:
			body = a.chat.View(a.theme)
		}
		content = header + "\n" + body
	}

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

	header := title + sessionInfo

	if a.versionNotice != "" {
		notice := lipgloss.NewStyle().
			Foreground(a.theme.Warning).
			Render("  [" + a.versionNotice + "]")
		header += notice
	}

	return header
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
func Run(ctx context.Context, c *client.Client, session *pb.Session, reconfigure ...bool) error {
	t := theme.Dark()
	reconf := len(reconfigure) > 0 && reconfigure[0]
	app := NewApp(c, session, t, true, reconf)

	p := tea.NewProgram(app, tea.WithContext(ctx))
	_, err := p.Run()
	return err
}
