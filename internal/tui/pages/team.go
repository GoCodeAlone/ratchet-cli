package pages

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

// AgentCard represents an agent's current state in the team view.
type AgentCard struct {
	Name        string
	Role        string
	Model       string
	Status      string
	CurrentTask string
	Messages    []string
	expanded    bool
}

// TeamModel shows the multi-agent team view.
type TeamModel struct {
	agents  []AgentCard
	events  []*pb.TeamEvent
	cursor  int
	width   int
	height  int
}

func NewTeam() TeamModel {
	return TeamModel{}
}

func (m TeamModel) SetSize(w, h int) TeamModel {
	m.width = w
	m.height = h
	return m
}

// ApplyEvent updates the team model from a streaming team event.
func (m TeamModel) ApplyEvent(ev *pb.TeamEvent) TeamModel {
	m.events = append(m.events, ev)
	switch e := ev.Event.(type) {
	case *pb.TeamEvent_AgentSpawned:
		m.agents = append(m.agents, AgentCard{
			Name:   e.AgentSpawned.AgentName,
			Role:   e.AgentSpawned.Role,
			Status: "active",
		})
	case *pb.TeamEvent_AgentMessage:
		for i, a := range m.agents {
			if a.Name == e.AgentMessage.FromAgent {
				m.agents[i].Messages = append(m.agents[i].Messages, e.AgentMessage.Content)
				break
			}
		}
	case *pb.TeamEvent_Token:
		if len(m.agents) > 0 {
			last := len(m.agents) - 1
			if len(m.agents[last].Messages) == 0 {
				m.agents[last].Messages = append(m.agents[last].Messages, "")
			}
			lastMsg := len(m.agents[last].Messages) - 1
			m.agents[last].Messages[lastMsg] += e.Token.Content
		}
	}
	return m
}

func (m TeamModel) Update(msg tea.Msg) (TeamModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.agents)-1 {
				m.cursor++
			}
		case "enter":
			if m.cursor < len(m.agents) {
				m.agents[m.cursor].expanded = !m.agents[m.cursor].expanded
			}
		}
	}
	return m, nil
}

func (m TeamModel) View(t theme.Theme) string {
	title := lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true).
		Render("Team View")

	lines := []string{title, strings.Repeat("─", m.width)}

	if len(m.agents) == 0 {
		lines = append(lines, lipgloss.NewStyle().
			Foreground(t.Muted).
			Render("  No agents active"))
		return strings.Join(lines, "\n")
	}

	for i, agent := range m.agents {
		statusColor := t.Muted
		switch agent.Status {
		case "active", "running":
			statusColor = t.Success
		case "error", "failed":
			statusColor = t.Error
		case "idle":
			statusColor = t.Warning
		}

		selected := i == m.cursor

		header := fmt.Sprintf("  %s (%s)", agent.Name, agent.Role)
		if agent.CurrentTask != "" {
			header += fmt.Sprintf(" — %s", agent.CurrentTask)
		}

		headerStyle := lipgloss.NewStyle().Width(m.width - 4)
		if selected {
			headerStyle = headerStyle.Background(t.Secondary)
		}

		statusBadge := lipgloss.NewStyle().
			Foreground(statusColor).
			Render(fmt.Sprintf("[%s]", agent.Status))

		lines = append(lines, headerStyle.Render(header+" "+statusBadge))

		if agent.expanded && len(agent.Messages) > 0 {
			for _, msg := range agent.Messages {
				msgLines := strings.Split(msg, "\n")
				for _, ml := range msgLines {
					if ml != "" {
						lines = append(lines, lipgloss.NewStyle().
							Foreground(t.Foreground).
							Padding(0, 4).
							Render(ml))
					}
				}
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().
		Foreground(t.Muted).
		Render("  ↑↓ navigate  Enter: expand"))

	return strings.Join(lines, "\n")
}
