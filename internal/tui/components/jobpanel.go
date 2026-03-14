package components

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

const jobRefreshInterval = 2 * time.Second

// JobPauseMsg is sent when the user requests to pause the selected job.
type JobPauseMsg struct{ JobID string }

// JobKillMsg is sent when the user requests to kill the selected job.
type JobKillMsg struct{ JobID string }

// JobListRefreshedMsg carries a fresh job list from the daemon.
type JobListRefreshedMsg struct{ Jobs []*pb.Job }

// JobTickMsg triggers a periodic refresh.
type JobTickMsg struct{}

// JobPanel displays active jobs from all managers in a table.
type JobPanel struct {
	jobs   []*pb.Job
	cursor int
	width  int
	height int
	c      *client.Client
}

// NewJobPanel creates a JobPanel backed by the given daemon client.
func NewJobPanel(c *client.Client) JobPanel {
	return JobPanel{c: c}
}

// SetSize updates the panel dimensions.
func (jp JobPanel) SetSize(w, h int) JobPanel {
	jp.width = w
	jp.height = h
	return jp
}

// Init returns a command that immediately triggers the first refresh tick.
func (jp JobPanel) Init() tea.Cmd {
	return tea.Tick(0, func(time.Time) tea.Msg { return JobTickMsg{} })
}

// Update handles key events and refresh messages.
func (jp JobPanel) Update(msg tea.Msg) (JobPanel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up":
			if jp.cursor > 0 {
				jp.cursor--
			}
		case "down":
			if jp.cursor < len(jp.jobs)-1 {
				jp.cursor++
			}
		case "p":
			if jp.cursor < len(jp.jobs) {
				jobID := jp.jobs[jp.cursor].Id
				return jp, func() tea.Msg { return JobPauseMsg{JobID: jobID} }
			}
		case "k":
			if jp.cursor < len(jp.jobs) {
				jobID := jp.jobs[jp.cursor].Id
				return jp, func() tea.Msg { return JobKillMsg{JobID: jobID} }
			}
		}

	case JobTickMsg:
		return jp, tea.Batch(
			jp.fetchJobs(),
			tea.Tick(jobRefreshInterval, func(time.Time) tea.Msg { return JobTickMsg{} }),
		)

	case JobListRefreshedMsg:
		jp.jobs = msg.Jobs
		if jp.cursor >= len(jp.jobs) {
			jp.cursor = max(0, len(jp.jobs)-1)
		}

	case JobPauseMsg:
		if jp.c != nil {
			go jp.c.PauseJob(context.Background(), msg.JobID) //nolint:errcheck
		}

	case JobKillMsg:
		if jp.c != nil {
			go jp.c.KillJob(context.Background(), msg.JobID) //nolint:errcheck
		}
	}
	return jp, nil
}

func (jp JobPanel) fetchJobs() tea.Cmd {
	return func() tea.Msg {
		if jp.c == nil {
			return JobListRefreshedMsg{}
		}
		list, err := jp.c.ListJobs(context.Background())
		if err != nil {
			return JobListRefreshedMsg{}
		}
		return JobListRefreshedMsg{Jobs: list.Jobs}
	}
}

// View renders the job panel.
func (jp JobPanel) View(t theme.Theme) string {
	title := lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true).
		Padding(0, 1).
		Render("Active Jobs")

	divider := strings.Repeat("─", jp.width)

	header := lipgloss.NewStyle().
		Foreground(t.Muted).
		Padding(0, 1).
		Render(fmt.Sprintf("%-12s %-20s %-12s %-10s %s",
			"Type", "Name", "Status", "Elapsed", "Session"))

	lines := []string{title, divider, header, divider}

	for i, job := range jp.jobs {
		icon := statusIcon(job.Status)
		elapsed := job.Elapsed
		if elapsed == "" {
			elapsed = "-"
		}
		sessionID := job.SessionId
		if len(sessionID) > 8 {
			sessionID = sessionID[:8]
		}
		if sessionID == "" {
			sessionID = "-"
		}

		style := lipgloss.NewStyle().Padding(0, 1)
		if i == jp.cursor {
			style = style.Background(t.Secondary)
		}

		line := style.Width(jp.width - 2).Render(
			fmt.Sprintf("%-12s %-20s %s %-10s %-10s %s",
				truncate(job.Type, 12),
				truncate(job.Name, 20),
				icon,
				truncate(job.Status, 10),
				truncate(elapsed, 10),
				sessionID,
			),
		)
		lines = append(lines, line)
	}

	if len(jp.jobs) == 0 {
		lines = append(lines, lipgloss.NewStyle().
			Foreground(t.Muted).
			Padding(0, 1).
			Render("No active jobs"))
	}

	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().
		Foreground(t.Muted).
		Padding(0, 1).
		Render("↑↓ navigate  p: pause  k: kill  Esc: close"))

	return strings.Join(lines, "\n")
}
