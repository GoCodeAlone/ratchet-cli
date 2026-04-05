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
	ID                 string
	Name               string
	Role               string
	Model              string
	Status             string
	CurrentTask        string
	Messages           []string
	BlackboardSections map[string]int // section name -> entry count
	expanded           bool
}

// TeamModel shows the multi-agent team view.
type TeamModel struct {
	agents     []AgentCard
	events     []*pb.TeamEvent
	messageLog []string
	cursor     int
	width      int
	height     int
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
		m.messageLog = append(m.messageLog,
			fmt.Sprintf("[spawned] %s (%s)", e.AgentSpawned.AgentName, e.AgentSpawned.Role))
	case *pb.TeamEvent_AgentMessage:
		m.messageLog = append(m.messageLog,
			fmt.Sprintf("[%s → %s] %s", e.AgentMessage.FromAgent, e.AgentMessage.ToAgent, e.AgentMessage.Content))
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
	case *pb.TeamEvent_Complete:
		m.messageLog = append(m.messageLog,
			fmt.Sprintf("[complete] %s", e.Complete.Summary))
	case *pb.TeamEvent_Error:
		m.messageLog = append(m.messageLog,
			fmt.Sprintf("[error] %s", e.Error.Message))
	}
	return m
}

// TeamStatusMsg carries a refreshed TeamStatus from the daemon.
type TeamStatusMsg struct {
	Status *pb.TeamStatus
	Err    error
}

// KillAgentMsg signals that the selected agent should be killed.
type KillAgentMsg struct {
	TeamID string
	AgentID string
}

func (m TeamModel) Update(msg tea.Msg) (TeamModel, tea.Cmd) {
	switch msg := msg.(type) {
	case TeamStatusMsg:
		if msg.Err == nil && msg.Status != nil {
			m.agents = nil
			for _, a := range msg.Status.Agents {
				m.agents = append(m.agents, AgentCard{
					ID:          a.Id,
					Name:        a.Name,
					Role:        a.Role,
					Model:       a.Model,
					Status:      a.Status,
					CurrentTask: a.CurrentTask,
				})
			}
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up":
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
		case "k":
			if m.cursor < len(m.agents) {
				idx := m.cursor
				agentID := m.agents[idx].ID
				if agentID == "" {
					// ID not yet populated from daemon status; skip.
					break
				}
				return m, func() tea.Msg {
					return KillAgentMsg{AgentID: agentID}
				}
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

		if agent.expanded && len(agent.BlackboardSections) > 0 {
			var bbParts []string
			for section, count := range agent.BlackboardSections {
				bbParts = append(bbParts, fmt.Sprintf("%s:%d", section, count))
			}
			lines = append(lines, lipgloss.NewStyle().
				Foreground(t.Muted).
				Padding(0, 4).
				Render("bb: "+strings.Join(bbParts, ", ")))
		}
	}

	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().
		Foreground(t.Muted).
		Render("  ↑↓ navigate  Enter: expand  k: kill agent"))

	// Message log panel — show the last 5 entries.
	if len(m.messageLog) > 0 {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true).
			Render("  Event Log"))
		start := len(m.messageLog) - 5
		if start < 0 {
			start = 0
		}
		for _, entry := range m.messageLog[start:] {
			lines = append(lines, lipgloss.NewStyle().
				Foreground(t.Foreground).
				Padding(0, 2).
				Render(entry))
		}
	}

	return strings.Join(lines, "\n")
}

// SetBlackboardInfo sets the blackboard section data for a given agent.
func (m TeamModel) SetBlackboardInfo(agentName string, sections map[string]int) TeamModel {
	for i, a := range m.agents {
		if a.Name == agentName {
			m.agents[i].BlackboardSections = sections
			break
		}
	}
	return m
}
