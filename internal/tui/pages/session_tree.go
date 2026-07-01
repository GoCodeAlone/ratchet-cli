package pages

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/components"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

type sessionTreeClient interface {
	GetSessionTree(context.Context, string) (*pb.SessionList, error)
	ListSessionMessages(context.Context, string) (*pb.SessionHistory, error)
}

type sessionTreeLoadedMsg struct {
	sessions []*pb.Session
	err      error
}

type sessionPreviewLoadedMsg struct {
	sessionID string
	messages  []*pb.HistoryMessage
	err       error
}

// SessionTreeBrowser loads and displays a daemon-backed session tree.
type SessionTreeBrowser struct {
	client sessionTreeClient
	rootID string
	theme  theme.Theme
	tree   components.SessionTreeModel
	err    string
}

// NewSessionTreeBrowser creates a browser for the lineage tree containing rootID.
func NewSessionTreeBrowser(c sessionTreeClient, rootID string, t theme.Theme) SessionTreeBrowser {
	return SessionTreeBrowser{
		client: c,
		rootID: rootID,
		theme:  t,
		tree:   components.NewSessionTree(),
	}
}

// Init loads the initial session tree.
func (m SessionTreeBrowser) Init() tea.Cmd {
	return m.loadTree()
}

// Update handles load results and navigation.
func (m SessionTreeBrowser) Update(msg tea.Msg) (SessionTreeBrowser, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionTreeLoadedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.tree = m.tree.SetSessions(msg.sessions)
		return m, m.loadPreview(m.tree.SelectedSessionID())

	case sessionPreviewLoadedMsg:
		if msg.sessionID != "" && msg.sessionID != m.tree.SelectedSessionID() {
			return m, nil
		}
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.tree = m.tree.SetPreview(msg.sessionID, msg.messages)
		return m, nil

	case tea.KeyPressMsg:
		if msg.String() == "r" {
			return m, m.loadTree()
		}
		before := m.tree.SelectedSessionID()
		var cmd tea.Cmd
		m.tree, cmd = m.tree.Update(msg)
		after := m.tree.SelectedSessionID()
		if cmd != nil {
			return m, cmd
		}
		if after != "" && after != before {
			return m, m.loadPreview(after)
		}
	}
	return m, nil
}

// View renders the browser.
func (m SessionTreeBrowser) View() string {
	view := m.tree.View(m.theme)
	if m.err == "" {
		return view
	}
	errLine := lipgloss.NewStyle().
		Foreground(m.theme.Error).
		Padding(0, 1).
		Render(fmt.Sprintf("error: %s", m.err))
	return errLine + "\n" + view
}

func (m SessionTreeBrowser) loadTree() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return sessionTreeLoadedMsg{err: fmt.Errorf("session tree client is nil")}
		}
		resp, err := m.client.GetSessionTree(context.Background(), m.rootID)
		if err != nil {
			return sessionTreeLoadedMsg{err: err}
		}
		if resp == nil {
			return sessionTreeLoadedMsg{}
		}
		return sessionTreeLoadedMsg{sessions: resp.Sessions}
	}
}

func (m SessionTreeBrowser) loadPreview(sessionID string) tea.Cmd {
	if sessionID == "" {
		return nil
	}
	return func() tea.Msg {
		if m.client == nil {
			return sessionPreviewLoadedMsg{sessionID: sessionID, err: fmt.Errorf("session tree client is nil")}
		}
		resp, err := m.client.ListSessionMessages(context.Background(), sessionID)
		if err != nil {
			return sessionPreviewLoadedMsg{sessionID: sessionID, err: err}
		}
		if resp == nil {
			return sessionPreviewLoadedMsg{sessionID: sessionID}
		}
		return sessionPreviewLoadedMsg{sessionID: sessionID, messages: resp.Messages}
	}
}
