package pages

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/viewport"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/components"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

// ChatEventMsg wraps an incoming proto ChatEvent for the TUI.
type ChatEventMsg struct {
	Event *pb.ChatEvent
}

type ChatModel struct {
	client    *client.Client
	sessionID string
	theme     theme.Theme
	dark      bool

	viewport   viewport.Model
	input      components.InputModel
	statusBar  components.StatusBar
	messages   []components.Message
	streaming  string // current streaming response
	width      int
	height     int
	ctx        context.Context
	cancelChat context.CancelFunc
}

func NewChat(c *client.Client, sessionID string, t theme.Theme, dark bool) ChatModel {
	vp := viewport.New(viewport.WithHeight(20))
	input := components.NewInput(t)
	statusBar := components.NewStatusBar()

	return ChatModel{
		client:    c,
		sessionID: sessionID,
		theme:     t,
		dark:      dark,
		viewport:  vp,
		input:     input,
		statusBar: statusBar,
		ctx:       context.Background(),
	}
}

func (m ChatModel) Init() tea.Cmd {
	return m.input.Init()
}

func (m ChatModel) Update(msg tea.Msg) (ChatModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.relayout()

	case components.SubmitMsg:
		// Add user message and send to daemon
		m.messages = append(m.messages, components.Message{
			Role:    components.RoleUser,
			Content: msg.Content,
		})
		m.streaming = ""
		m.refreshViewport()
		cmds = append(cmds, m.sendMessage(msg.Content))

	case ChatEventMsg:
		cmds = append(cmds, m.handleChatEvent(msg.Event))
	}

	var inputCmd, vpCmd tea.Cmd
	m.input, inputCmd = m.input.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, inputCmd, vpCmd)

	return m, tea.Batch(cmds...)
}

func (m *ChatModel) relayout() {
	inputHeight := 5
	statusHeight := 1
	vpHeight := m.height - inputHeight - statusHeight - 2
	if vpHeight < 1 {
		vpHeight = 1
	}
	m.viewport.SetHeight(vpHeight)
	m.viewport.SetWidth(m.width)
	m.statusBar.Width = m.width
	m.input.SetWidth(m.width)
	m.refreshViewport()
}

func (m *ChatModel) refreshViewport() {
	var sb strings.Builder
	for _, msg := range m.messages {
		sb.WriteString(msg.Render(m.theme, m.width, m.dark))
		sb.WriteString("\n")
	}
	if m.streaming != "" {
		assistantMsg := components.Message{
			Role:    components.RoleAssistant,
			Content: m.streaming,
		}
		sb.WriteString(assistantMsg.Render(m.theme, m.width, m.dark))
	}
	m.viewport.SetContent(sb.String())
	m.viewport.GotoBottom()
}

func (m ChatModel) sendMessage(content string) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return nil
		}
		ctx, cancel := context.WithCancel(m.ctx)
		_ = cancel // stored for potential cancellation

		ch, err := m.client.SendMessage(ctx, m.sessionID, content)
		if err != nil {
			return ChatEventMsg{Event: &pb.ChatEvent{
				Event: &pb.ChatEvent_Error{
					Error: &pb.ErrorEvent{Message: err.Error()},
				},
			}}
		}

		// Return first event; subsequent events come via streaming
		event, ok := <-ch
		if !ok {
			return nil
		}
		return ChatEventMsg{Event: event}
	}
}

func (m ChatModel) handleChatEvent(event *pb.ChatEvent) tea.Cmd {
	if event == nil {
		return nil
	}
	switch e := event.Event.(type) {
	case *pb.ChatEvent_Token:
		m.streaming += e.Token.Content
		m.refreshViewport()
		return nil
	case *pb.ChatEvent_Complete:
		if m.streaming != "" {
			m.messages = append(m.messages, components.Message{
				Role:    components.RoleAssistant,
				Content: m.streaming,
			})
			m.streaming = ""
			m.refreshViewport()
		}
		return nil
	case *pb.ChatEvent_Error:
		m.messages = append(m.messages, components.Message{
			Role:    components.RoleTool,
			Content: "error: " + e.Error.Message,
		})
		m.streaming = ""
		m.refreshViewport()
		return nil
	}
	return nil
}

func (m ChatModel) View(t theme.Theme) string {
	var sb strings.Builder
	sb.WriteString(m.viewport.View())
	sb.WriteString("\n")
	sb.WriteString(m.input.View(t, m.width))
	sb.WriteString("\n")
	sb.WriteString(m.statusBar.View(t))
	return sb.String()
}
