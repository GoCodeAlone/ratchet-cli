package components

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

// SessionSelectedMsg is sent when the user switches to a session.
type SessionSelectedMsg struct{ SessionID string }

// SessionKillMsg is sent when the user kills a session from the sidebar.
type SessionKillMsg struct{ SessionID string }

// SidebarModel displays a list of sessions and allows switching or killing.
type SidebarModel struct {
	sessions   []*pb.Session
	currentID  string
	cursor     int
	width      int
	height     int
}

func NewSidebar(sessions []*pb.Session, currentID string) SidebarModel {
	cursor := 0
	for i, s := range sessions {
		if s.Id == currentID {
			cursor = i
			break
		}
	}
	return SidebarModel{
		sessions:  sessions,
		currentID: currentID,
		cursor:    cursor,
	}
}

func (s SidebarModel) SetSize(w, h int) SidebarModel {
	s.width = w
	s.height = h
	return s
}

func (s SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if s.cursor > 0 {
				s.cursor--
			}
		case "down", "j":
			if s.cursor < len(s.sessions)-1 {
				s.cursor++
			}
		case "enter":
			if s.cursor < len(s.sessions) {
				return s, func() tea.Msg {
					return SessionSelectedMsg{SessionID: s.sessions[s.cursor].Id}
				}
			}
		case "d":
			if s.cursor < len(s.sessions) {
				id := s.sessions[s.cursor].Id
				return s, func() tea.Msg {
					return SessionKillMsg{SessionID: id}
				}
			}
		}
	}
	return s, nil
}

func (s SidebarModel) View(t theme.Theme) string {
	title := lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true).
		Padding(0, 1).
		Render("Sessions")

	lines := []string{title, strings.Repeat("─", s.width)}

	for i, sess := range s.sessions {
		id := sess.Id
		if len(id) > 8 {
			id = id[:8]
		}

		status := sess.Status
		name := sess.Name
		if name == "" {
			name = id
		}

		indicator := "  "
		style := lipgloss.NewStyle().Padding(0, 1)
		if sess.Id == s.currentID {
			indicator = "● "
			style = style.Foreground(t.Primary)
		} else {
			style = style.Foreground(t.Foreground)
		}
		if i == s.cursor {
			style = style.Background(t.Secondary)
		}

		line := style.Width(s.width - 2).Render(
			fmt.Sprintf("%s%s [%s]", indicator, name, status),
		)
		lines = append(lines, line)
	}

	if len(s.sessions) == 0 {
		lines = append(lines, lipgloss.NewStyle().
			Foreground(t.Muted).
			Padding(0, 1).
			Render("No sessions"))
	}

	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().
		Foreground(t.Muted).
		Padding(0, 1).
		Render("↑↓ navigate  Enter: switch  d: kill"))

	return strings.Join(lines, "\n")
}
