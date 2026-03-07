package components

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

// SubmitMsg is sent when the user submits input.
type SubmitMsg struct {
	Content string
}

type InputModel struct {
	textarea textarea.Model
	history  []string
	histIdx  int
}

func NewInput(t theme.Theme) InputModel {
	ta := textarea.New()
	ta.Placeholder = "Message ratchet..."
	ta.ShowLineNumbers = false
	ta.SetHeight(3)

	styles := textarea.DefaultDarkStyles()
	styles.Focused.Text = lipgloss.NewStyle().Foreground(t.Foreground)
	ta.SetStyles(styles)

	return InputModel{
		textarea: ta,
		histIdx:  -1,
	}
}

func (m InputModel) Init() tea.Cmd {
	return m.textarea.Focus()
}

func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			// Submit on bare Enter (no modifiers)
			content := m.textarea.Value()
			if content == "" {
				return m, nil
			}
			m.history = append(m.history, content)
			m.histIdx = -1
			m.textarea.SetValue("")
			return m, func() tea.Msg {
				return SubmitMsg{Content: content}
			}
		case "shift+enter":
			// Insert newline - fall through to textarea
		case "up":
			// History recall when textarea is empty
			if m.textarea.Value() == "" && len(m.history) > 0 {
				if m.histIdx == -1 {
					m.histIdx = len(m.history) - 1
				} else if m.histIdx > 0 {
					m.histIdx--
				}
				m.textarea.SetValue(m.history[m.histIdx])
				return m, nil
			}
		case "down":
			if m.histIdx >= 0 {
				m.histIdx++
				if m.histIdx >= len(m.history) {
					m.histIdx = -1
					m.textarea.SetValue("")
				} else {
					m.textarea.SetValue(m.history[m.histIdx])
				}
				return m, nil
			}
		}
	}

	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

func (m InputModel) View(t theme.Theme, width int) string {
	m.textarea.SetWidth(width - 2)
	return t.InputArea.Width(width).Render(m.textarea.View())
}

func (m InputModel) Focused() bool {
	return m.textarea.Focused()
}

func (m *InputModel) SetWidth(w int) {
	m.textarea.SetWidth(w - 2)
}
