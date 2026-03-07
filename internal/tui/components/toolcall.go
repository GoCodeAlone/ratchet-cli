package components

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

type ToolCallStatus int

const (
	ToolCallPending ToolCallStatus = iota
	ToolCallRunning
	ToolCallSuccess
	ToolCallFailed
)

// ToolCallCard renders a collapsible tool call with status icon.
type ToolCallCard struct {
	CallID    string
	ToolName  string
	ArgsJSON  string
	ResultJSON string
	Status    ToolCallStatus
	Expanded  bool
	Summary   string // one-line summary set on completion
}

// ToggleMsg is sent when a tool call card is toggled.
type ToggleMsg struct{ CallID string }

func (tc *ToolCallCard) SetResult(resultJSON string, success bool) {
	tc.ResultJSON = resultJSON
	if success {
		tc.Status = ToolCallSuccess
	} else {
		tc.Status = ToolCallFailed
	}
}

func (tc ToolCallCard) statusIcon() string {
	switch tc.Status {
	case ToolCallRunning:
		return "⠋" // spinner-like
	case ToolCallSuccess:
		return "✓"
	case ToolCallFailed:
		return "✗"
	default:
		return "○"
	}
}

func (tc ToolCallCard) Render(t theme.Theme, width int) string {
	icon := tc.statusIcon()

	var style lipgloss.Style
	switch tc.Status {
	case ToolCallSuccess:
		style = t.ToolCallSuccess
	case ToolCallFailed:
		style = t.ToolCallCard // reuse with error color
	default:
		style = t.ToolCallPending
	}

	summary := tc.Summary
	if summary == "" {
		summary = tc.ToolName
	}
	header := fmt.Sprintf("%s %s: %s", icon, tc.ToolName, summary)

	if !tc.Expanded {
		return style.Width(width - 4).Render(header)
	}

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString("\n\nArguments:\n")
	sb.WriteString(tc.ArgsJSON)
	if tc.ResultJSON != "" {
		sb.WriteString("\n\nResult:\n")
		sb.WriteString(tc.ResultJSON)
	}
	return style.Width(width - 4).Render(sb.String())
}

// ToolCallListModel manages a list of tool call cards.
type ToolCallListModel struct {
	cards    map[string]*ToolCallCard
	order    []string // insertion order
	selected int
}

func NewToolCallList() ToolCallListModel {
	return ToolCallListModel{
		cards: make(map[string]*ToolCallCard),
	}
}

func (m *ToolCallListModel) AddCard(callID, toolName, argsJSON string) {
	card := &ToolCallCard{
		CallID:   callID,
		ToolName: toolName,
		ArgsJSON: argsJSON,
		Status:   ToolCallRunning,
	}
	m.cards[callID] = card
	m.order = append(m.order, callID)
}

func (m *ToolCallListModel) UpdateResult(callID, resultJSON string, success bool) {
	if card, ok := m.cards[callID]; ok {
		card.SetResult(resultJSON, success)
		// Generate summary from result
		if len(resultJSON) > 60 {
			card.Summary = resultJSON[:57] + "..."
		} else {
			card.Summary = resultJSON
		}
	}
}

func (m ToolCallListModel) Update(msg tea.Msg) (ToolCallListModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			if m.selected < len(m.order) {
				callID := m.order[m.selected]
				if card, ok := m.cards[callID]; ok {
					card.Expanded = !card.Expanded
				}
			}
		case "j", "down":
			if m.selected < len(m.order)-1 {
				m.selected++
			}
		case "k", "up":
			if m.selected > 0 {
				m.selected--
			}
		}
	}
	return m, nil
}

func (m ToolCallListModel) View(t theme.Theme, width int) string {
	var sb strings.Builder
	for i, callID := range m.order {
		card := m.cards[callID]
		rendered := card.Render(t, width)
		if i == m.selected {
			// Highlight selected
			rendered = lipgloss.NewStyle().BorderLeft(true).
				BorderForeground(t.Primary).
				PaddingLeft(1).
				Render(rendered)
		}
		sb.WriteString(rendered)
		sb.WriteString("\n")
	}
	return sb.String()
}
