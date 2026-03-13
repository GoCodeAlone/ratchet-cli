package components

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

// PlanApproveMsg is dispatched when the user approves the plan.
type PlanApproveMsg struct {
	PlanID    string
	SkipSteps []string
}

// PlanRejectMsg is dispatched when the user rejects the plan.
type PlanRejectMsg struct {
	PlanID string
}

// PlanView renders a proposed plan as a numbered task list with status
// indicators and keyboard navigation.
//
//	Enter  → approve (with any toggled skip steps)
//	Esc    → reject
//	space  → toggle skip on the cursor step
//	↑/k, ↓/j → navigate steps
type PlanView struct {
	plan     *pb.Plan
	cursor   int
	skipped  map[string]bool // steps the user wants to skip
	width    int
	active   bool // whether the view is currently focused
}

func NewPlanView() PlanView {
	return PlanView{
		skipped: make(map[string]bool),
	}
}

// SetPlan replaces the displayed plan and resets cursor/skip state.
func (v PlanView) SetPlan(p *pb.Plan) PlanView {
	v.plan = p
	v.cursor = 0
	v.skipped = make(map[string]bool)
	v.active = true
	return v
}

// SetSize updates the rendering width.
func (v PlanView) SetSize(width int) PlanView {
	v.width = width
	return v
}

// Active reports whether the plan view is currently showing a plan.
func (v PlanView) Active() bool {
	return v.active && v.plan != nil
}

func (v PlanView) Update(msg tea.Msg) (PlanView, tea.Cmd) {
	if !v.active || v.plan == nil {
		return v, nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		steps := v.plan.Steps
		switch msg.String() {
		case "up", "k":
			if v.cursor > 0 {
				v.cursor--
			}
		case "down", "j":
			if v.cursor < len(steps)-1 {
				v.cursor++
			}
		case "space", " ":
			if v.cursor < len(steps) {
				id := steps[v.cursor].Id
				v.skipped[id] = !v.skipped[id]
			}
		case "enter":
			planID := v.plan.Id
			var skipList []string
			for id, skip := range v.skipped {
				if skip {
					skipList = append(skipList, id)
				}
			}
			v.active = false
			return v, func() tea.Msg {
				return PlanApproveMsg{PlanID: planID, SkipSteps: skipList}
			}
		case "esc":
			planID := v.plan.Id
			v.active = false
			return v, func() tea.Msg {
				return PlanRejectMsg{PlanID: planID}
			}
		}
	}
	return v, nil
}

func (v PlanView) View(t theme.Theme) string {
	if v.plan == nil {
		return ""
	}

	var sb strings.Builder

	title := lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true).
		Render(fmt.Sprintf("Plan: %s", v.plan.Goal))
	sb.WriteString(title)
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", v.width))
	sb.WriteString("\n")

	for i, step := range v.plan.Steps {
		icon := stepIcon(step, v.skipped[step.Id])

		style := lipgloss.NewStyle().Foreground(t.Foreground)
		switch {
		case v.skipped[step.Id]:
			style = style.Foreground(t.Muted).Strikethrough(true)
		case step.Status == "completed":
			style = style.Foreground(t.Success)
		case step.Status == "failed":
			style = style.Foreground(t.Error)
		case step.Status == "in_progress":
			style = style.Foreground(t.Warning)
		}

		cursor := "  "
		if i == v.cursor {
			cursor = "> "
			style = style.Background(t.Secondary)
		}

		line := fmt.Sprintf("%s%s %d. %s", cursor, icon, i+1, step.Description)
		if step.Error != "" {
			line += fmt.Sprintf(" (%s)", step.Error)
		}
		sb.WriteString(style.Width(v.width - 2).Render(line))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	hint := lipgloss.NewStyle().Foreground(t.Muted).Render(
		"↑↓ navigate  space: toggle skip  Enter: approve  Esc: reject",
	)
	sb.WriteString(hint)
	return sb.String()
}

func stepIcon(step *pb.PlanStep, skipped bool) string {
	if skipped {
		return "○"
	}
	switch step.Status {
	case "completed":
		return "✓"
	case "failed":
		return "✗"
	case "in_progress":
		return "⟳"
	default:
		return "○"
	}
}
