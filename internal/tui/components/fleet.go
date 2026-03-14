package components

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

// FleetWorkerKillMsg is sent when the user kills a fleet worker.
type FleetWorkerKillMsg struct {
	FleetID  string
	WorkerID string
}

// FleetStatusUpdatedMsg carries a new FleetStatus from the daemon.
type FleetStatusUpdatedMsg struct {
	Status *pb.FleetStatus
}

type fleetRow struct {
	worker  *pb.FleetWorker
	started time.Time
}

// FleetPanel displays the active fleet workers in a table.
type FleetPanel struct {
	fleetID string
	rows    []fleetRow
	cursor  int
	width   int
	height  int
}

// NewFleetPanel creates an empty FleetPanel.
func NewFleetPanel() FleetPanel {
	return FleetPanel{}
}

// SetSize updates the panel dimensions.
func (f FleetPanel) SetSize(w, h int) FleetPanel {
	f.width = w
	f.height = h
	return f
}

// SetFleetStatus replaces the current fleet data.
func (f FleetPanel) SetFleetStatus(fs *pb.FleetStatus) FleetPanel {
	if fs == nil {
		return f
	}
	f.fleetID = fs.FleetId
	f.rows = make([]fleetRow, len(fs.Workers))
	for i, w := range fs.Workers {
		r := fleetRow{worker: w}
		if w.Status == "running" {
			r.started = time.Now()
		}
		f.rows[i] = r
	}
	if f.cursor >= len(f.rows) {
		f.cursor = max(0, len(f.rows)-1)
	}
	return f
}

// Update handles key events for the fleet panel.
func (f FleetPanel) Update(msg tea.Msg) (FleetPanel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if f.cursor > 0 {
				f.cursor--
			}
		case "down", "j":
			if f.cursor < len(f.rows)-1 {
				f.cursor++
			}
		case "K": // shift-K to kill selected worker
			if f.cursor < len(f.rows) {
				w := f.rows[f.cursor].worker
				fleetID := f.fleetID
				workerID := w.Id
				return f, func() tea.Msg {
					return FleetWorkerKillMsg{FleetID: fleetID, WorkerID: workerID}
				}
			}
		}
	case FleetStatusUpdatedMsg:
		f = f.SetFleetStatus(msg.Status)
	}
	return f, nil
}

// View renders the fleet panel.
func (f FleetPanel) View(t theme.Theme) string {
	title := lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true).
		Padding(0, 1).
		Render("Fleet Workers")

	divider := strings.Repeat("─", f.width)

	header := lipgloss.NewStyle().
		Foreground(t.Muted).
		Padding(0, 1).
		Render(fmt.Sprintf("%-20s %-16s %-12s %-20s %s",
			"Worker", "Step", "Status", "Model", "Elapsed"))

	lines := []string{title, divider, header, divider}

	for i, row := range f.rows {
		w := row.worker
		elapsed := "-"
		if w.Status == "running" {
			if !row.started.IsZero() {
				elapsed = time.Since(row.started).Round(time.Second).String()
			} else {
				elapsed = "..."
			}
		}

		statusIcon := statusIcon(w.Status)
		style := lipgloss.NewStyle().Padding(0, 1)
		if i == f.cursor {
			style = style.Background(t.Secondary)
		}

		model := w.Model
		if model == "" {
			model = "-"
		}
		stepID := w.StepId
		if len(stepID) > 14 {
			stepID = stepID[:14]
		}

		line := style.Width(f.width - 2).Render(
			fmt.Sprintf("%-20s %-16s %s %-10s %-20s %s",
				truncate(w.Name, 20),
				truncate(stepID, 16),
				statusIcon,
				truncate(w.Status, 10),
				truncate(model, 20),
				elapsed,
			),
		)
		lines = append(lines, line)
	}

	if len(f.rows) == 0 {
		lines = append(lines, lipgloss.NewStyle().
			Foreground(t.Muted).
			Padding(0, 1).
			Render("No fleet workers"))
	}

	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().
		Foreground(t.Muted).
		Padding(0, 1).
		Render("↑↓ navigate  K: kill worker"))

	return strings.Join(lines, "\n")
}

func statusIcon(s string) string {
	switch s {
	case "running":
		return "⠋"
	case "completed":
		return "✓"
	case "failed":
		return "✗"
	default:
		return "·"
	}
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

