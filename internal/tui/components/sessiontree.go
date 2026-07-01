package components

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

// SessionTreeSelectedMsg is emitted when a user selects a session in the tree.
type SessionTreeSelectedMsg struct {
	SessionID string
	Session   *pb.Session
}

type sessionTreeRow struct {
	session *pb.Session
	depth   int
}

// SessionTreeModel displays session lineage with keyboard navigation.
type SessionTreeModel struct {
	sessions  []*pb.Session
	rows      []sessionTreeRow
	cursor    int
	width     int
	height    int
	collapsed map[string]bool
	previews  map[string][]*pb.HistoryMessage
}

// NewSessionTree returns an empty session tree model.
func NewSessionTree() SessionTreeModel {
	return SessionTreeModel{
		width:     80,
		height:    20,
		collapsed: make(map[string]bool),
		previews:  make(map[string][]*pb.HistoryMessage),
	}
}

// SetSize updates the dimensions used by View.
func (m SessionTreeModel) SetSize(width, height int) SessionTreeModel {
	m.width = max(width, 1)
	m.height = max(height, 1)
	return m
}

// SetSessions replaces the sessions backing the tree.
func (m SessionTreeModel) SetSessions(sessions []*pb.Session) SessionTreeModel {
	m.sessions = sessions
	m.rebuildRows()
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}
	return m
}

// SetPreview stores messages for a session preview pane.
func (m SessionTreeModel) SetPreview(sessionID string, messages []*pb.HistoryMessage) SessionTreeModel {
	if m.previews == nil {
		m.previews = make(map[string][]*pb.HistoryMessage)
	}
	m.previews[sessionID] = messages
	return m
}

// SelectedSessionID returns the currently highlighted session ID.
func (m SessionTreeModel) SelectedSessionID() string {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return ""
	}
	return m.rows[m.cursor].session.GetId()
}

// Update handles tree keyboard navigation.
func (m SessionTreeModel) Update(msg tea.Msg) (SessionTreeModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
	case "left", "h":
		m.setCollapsed(true)
	case "right", "l":
		m.setCollapsed(false)
	case "enter":
		if m.cursor >= 0 && m.cursor < len(m.rows) {
			s := m.rows[m.cursor].session
			return m, func() tea.Msg {
				return SessionTreeSelectedMsg{SessionID: s.GetId(), Session: s}
			}
		}
	}
	return m, nil
}

// View renders the tree and any loaded message previews.
func (m SessionTreeModel) View(t theme.Theme) string {
	title := lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true).
		Padding(0, 1).
		Render("Session Tree")
	divider := strings.Repeat("─", max(m.width, 1))
	lines := []string{title, divider}

	if len(m.rows) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Padding(0, 1).Render("No sessions"))
		return strings.Join(lines, "\n")
	}

	for i, row := range m.rows {
		s := row.session
		indicator := "  "
		if i == m.cursor {
			indicator = "› "
		}
		prefix := strings.Repeat("  ", row.depth)
		childMark := " "
		if m.hasChildren(s.GetId()) {
			if m.collapsed[s.GetId()] {
				childMark = "▸"
			} else {
				childMark = "▾"
			}
		}
		fork := sanitizeTreeText(s.GetForkedFromMessageId())
		if fork == "" {
			fork = "-"
		}
		line := fmt.Sprintf("%s%s%s %-8s %-10s fork:%-8s %s",
			indicator,
			prefix,
			childMark,
			shortID(s.GetId()),
			truncate(sanitizeTreeText(s.GetStatus()), 10),
			shortID(fork),
			truncate(sanitizeTreeText(s.GetBranchSummary()), max(32, m.width/3)),
		)
		style := lipgloss.NewStyle().Padding(0, 1).Width(max(m.width-2, 1))
		if i == m.cursor {
			style = style.Background(t.Secondary)
		}
		lines = append(lines, style.Render(line))
	}

	previewLines := m.previewLines()
	if len(previewLines) > 0 {
		lines = append(lines, divider)
		lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Padding(0, 1).Render("Preview"))
		lines = append(lines, previewLines...)
	}
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(t.Muted).Padding(0, 1).Render("↑↓ navigate  ←→ collapse/expand  Enter: switch  r: refresh  Esc: close"))
	return strings.Join(lines, "\n")
}

func (m *SessionTreeModel) rebuildRows() {
	children := make(map[string][]*pb.Session)
	seenChildren := make(map[string]bool)
	for _, s := range m.sessions {
		if s.GetParentId() == "" {
			continue
		}
		children[s.GetParentId()] = append(children[s.GetParentId()], s)
		seenChildren[s.GetId()] = true
	}

	var rows []sessionTreeRow
	var walk func(*pb.Session, int)
	walk = func(s *pb.Session, depth int) {
		rows = append(rows, sessionTreeRow{session: s, depth: depth})
		if m.collapsed[s.GetId()] {
			return
		}
		for _, child := range children[s.GetId()] {
			walk(child, depth+1)
		}
	}

	for _, s := range m.sessions {
		if s.GetParentId() == "" || !seenChildren[s.GetId()] && s.GetRootId() == s.GetId() {
			walk(s, 0)
		}
	}
	m.rows = rows
}

func (m *SessionTreeModel) setCollapsed(collapsed bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	id := m.rows[m.cursor].session.GetId()
	if !m.hasChildren(id) {
		return
	}
	m.collapsed[id] = collapsed
	m.rebuildRows()
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}
}

func (m SessionTreeModel) hasChildren(id string) bool {
	for _, s := range m.sessions {
		if s.GetParentId() == id {
			return true
		}
	}
	return false
}

func (m SessionTreeModel) previewLines() []string {
	var lines []string
	for sessionID, messages := range m.previews {
		for _, msg := range messages {
			text := sanitizeTreeText(msg.GetContent())
			if text == "" {
				continue
			}
			lines = append(lines, lipgloss.NewStyle().Padding(0, 1).Render(
				fmt.Sprintf("%s %-9s %s", shortID(sessionID), truncate(msg.GetRole(), 9), truncate(text, max(20, m.width-20))),
			))
		}
	}
	return lines
}

func sanitizeTreeText(text string) string {
	var b strings.Builder
	lastSpace := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}
	return strings.TrimSpace(b.String())
}

func shortID(id string) string {
	if id == "" {
		return "-"
	}
	return truncate(id, 8)
}
