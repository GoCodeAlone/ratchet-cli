package components

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

// InputResizedMsg is sent when the input height changes due to content.
type InputResizedMsg struct {
	Height int
}

// SubmitMsg is sent when the user submits input.
type SubmitMsg struct {
	Content string
}

type InputModel struct {
	textarea textarea.Model
	history  []string
	histIdx  int
	height   int
}

func NewInput(t theme.Theme) InputModel {
	ta := textarea.New()
	ta.Placeholder = "Message ratchet..."
	ta.ShowLineNumbers = false
	ta.SetHeight(1)

	styles := textarea.DefaultDarkStyles()
	styles.Focused.Text = lipgloss.NewStyle().Foreground(t.Foreground)
	ta.SetStyles(styles)

	// Focus the textarea immediately so it accepts input.
	// textarea.Focus() is a pointer-receiver method that sets focus=true
	// in-place. Calling it here preserves the state in the returned model.
	// Init() re-calls Focus() to obtain the cursor blink Cmd, but its
	// side-effect on focus is harmless since it's already true.
	ta.Focus()

	return InputModel{
		textarea: ta,
		histIdx:  -1,
		height:   1,
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

	newHeight := m.calcHeight()
	if newHeight != m.height {
		m.height = newHeight
		m.textarea.SetHeight(newHeight)
		return m, tea.Batch(cmd, func() tea.Msg { return InputResizedMsg{Height: newHeight} })
	}

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

func (m InputModel) calcHeight() int {
	val := m.textarea.Value()
	lines := strings.Count(val, "\n") + 1
	return min(lines, 6)
}

// Height returns the current content-driven height of the input.
func (m InputModel) Height() int {
	return m.height
}

// Value returns the current text content of the input.
func (m InputModel) Value() string {
	return m.textarea.Value()
}

// SetValue replaces the input text content.
func (m *InputModel) SetValue(s string) {
	m.textarea.SetValue(s)
}
