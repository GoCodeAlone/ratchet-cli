package components

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

// CommandEntry represents a slash command for autocomplete.
type CommandEntry struct {
	Name string
	Desc string
}

// AutocompleteModel manages the autocomplete dropdown state.
type AutocompleteModel struct {
	commands []CommandEntry
	matches  []CommandEntry
	filter   string
	cursor   int
	visible  bool
	height   int
}

// AutocompleteSelectedMsg is sent when a command is selected from the dropdown.
type AutocompleteSelectedMsg struct {
	Command string
}

// defaultMaxVisibleItems is the default maximum number of autocomplete items shown at once.
const defaultMaxVisibleItems = 10

// NewAutocomplete creates an autocomplete model with all known commands.
func NewAutocomplete() AutocompleteModel {
	commands := []CommandEntry{
		{Name: "/help", Desc: "Show this help"},
		{Name: "/model", Desc: "Show current model"},
		{Name: "/clear", Desc: "Clear conversation"},
		{Name: "/cost", Desc: "Show token usage"},
		{Name: "/agents", Desc: "List active agents"},
		{Name: "/sessions", Desc: "List sessions"},
		{Name: "/provider", Desc: "Provider management"},
		{Name: "/exit", Desc: "Quit ratchet"},
		{Name: "/plan", Desc: "Show plan mode info"},
		{Name: "/approve", Desc: "Approve a proposed plan"},
		{Name: "/reject", Desc: "Reject a proposed plan"},
		{Name: "/fleet", Desc: "Start fleet execution for a plan"},
		{Name: "/team", Desc: "Team management"},
		{Name: "/review", Desc: "Run code-reviewer on current git diff"},
		{Name: "/compact", Desc: "Compress conversation context"},
		{Name: "/loop", Desc: "Schedule a recurring command"},
		{Name: "/cron", Desc: "Schedule with cron expression"},
		{Name: "/mcp", Desc: "MCP tool management"},
		{Name: "/jobs", Desc: "Show job control panel"},
		{Name: "/login", Desc: "Re-authenticate provider"},
	}
	return AutocompleteModel{commands: commands}
}

// Visible returns whether the autocomplete dropdown is showing.
func (m AutocompleteModel) Visible() bool { return m.visible }

// SetHeight sets the available height so View() can cap the dropdown size.
func (m AutocompleteModel) SetHeight(h int) AutocompleteModel {
	m.height = h
	return m
}

// SetFilter updates the autocomplete based on current input text.
func (m AutocompleteModel) SetFilter(input string) AutocompleteModel {
	// Do NOT TrimSpace — a trailing space (e.g. after autocomplete selection)
	// must suppress the dropdown. Trimming would re-match the command prefix.
	if !strings.HasPrefix(input, "/") || strings.Contains(input, " ") {
		m.visible = false
		m.filter = ""
		m.matches = nil
		return m
	}

	m.filter = strings.ToLower(input)
	m.visible = true
	m.matches = nil
	for _, cmd := range m.commands {
		if strings.HasPrefix(strings.ToLower(cmd.Name), m.filter) {
			m.matches = append(m.matches, cmd)
		}
	}
	if len(m.matches) == 0 {
		m.visible = false
	}
	if m.cursor >= len(m.matches) {
		m.cursor = max(0, len(m.matches)-1)
	}
	return m
}

// Update handles keyboard input when the autocomplete is visible.
func (m AutocompleteModel) Update(msg tea.Msg) (AutocompleteModel, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch msg.String() {
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down":
			if m.cursor < len(m.matches)-1 {
				m.cursor++
			}
			return m, nil
		case "tab", "enter":
			if len(m.matches) > 0 {
				selected := m.matches[m.cursor].Name
				m.visible = false
				return m, func() tea.Msg {
					return AutocompleteSelectedMsg{Command: selected}
				}
			}
		case "esc":
			m.visible = false
			return m, nil
		}
	}
	return m, nil
}

// View renders the autocomplete dropdown, capped at maxVisibleItems rows.
func (m AutocompleteModel) View(t theme.Theme, width int) string {
	if !m.visible || len(m.matches) == 0 {
		return ""
	}

	maxWidth := min(width-4, 50)
	if maxWidth < 10 {
		maxWidth = 10
	}

	style := lipgloss.NewStyle().
		Background(t.Background).
		Foreground(t.Foreground).
		Width(maxWidth)

	selectedStyle := lipgloss.NewStyle().
		Background(t.Primary).
		Foreground(lipgloss.Color("#FFFFFF")).
		Width(maxWidth)

	mutedStyle := lipgloss.NewStyle().
		Foreground(t.Muted).
		Width(maxWidth)

	total := len(m.matches)

	// Cap visible items based on available height (leaving room for borders + input + status bar).
	maxVisible := defaultMaxVisibleItems
	if m.height > 0 {
		heightCap := m.height - 4
		if heightCap < 1 {
			heightCap = 1
		}
		if heightCap < maxVisible {
			maxVisible = heightCap
		}
	}

	// Compute a window of maxVisible around the cursor.
	start := m.cursor - maxVisible/2
	if start < 0 {
		start = 0
	}
	end := start + maxVisible
	if end > total {
		end = total
		start = end - maxVisible
		if start < 0 {
			start = 0
		}
	}

	var sb strings.Builder
	first := true

	if start > 0 {
		sb.WriteString(mutedStyle.Render(fmt.Sprintf(" ↑ %d more", start)))
		first = false
	}

	for i := start; i < end; i++ {
		if !first {
			sb.WriteString("\n")
		}
		first = false
		line := " " + m.matches[i].Name + "  " + m.matches[i].Desc
		if i == m.cursor {
			sb.WriteString(selectedStyle.Render(line))
		} else {
			sb.WriteString(style.Render(line))
		}
	}

	if end < total {
		sb.WriteString("\n")
		sb.WriteString(mutedStyle.Render(fmt.Sprintf(" ↓ %d more", total-end)))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Muted).
		Render(sb.String())
}
