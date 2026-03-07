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
// The channel is carried so handleChatEvent can schedule the next read.
type ChatEventMsg struct {
	Event *pb.ChatEvent
	ch    <-chan *pb.ChatEvent
}

// chatStreamDoneMsg signals the event channel was closed.
type chatStreamDoneMsg struct{}

type ChatModel struct {
	client    *client.Client
	sessionID string
	theme     theme.Theme
	dark      bool

	viewport   viewport.Model
	input      components.InputModel
	statusBar  components.StatusBar
	toolCalls  components.ToolCallListModel
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
		toolCalls: components.NewToolCallList(),
		ctx:       context.Background(),
	}
}

// SetSize sets the chat model's dimensions and recalculates layout.
func (m *ChatModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.relayout()
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

	case tea.KeyPressMsg:
		// Cancel in-flight streaming with Escape
		if msg.String() == "esc" && m.cancelChat != nil {
			m.cancelChat()
			m.cancelChat = nil
		}

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
		cmds = append(cmds, m.handleChatEvent(msg)...)

	case chatStreamDoneMsg:
		// Stream channel closed without a Complete event
		if m.streaming != "" {
			m.messages = append(m.messages, components.Message{
				Role:    components.RoleAssistant,
				Content: m.streaming,
			})
			m.streaming = ""
			m.refreshViewport()
		}
		m.cancelChat = nil
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
	// Render any active tool calls
	toolView := m.toolCalls.View(m.theme, m.width)
	if toolView != "" {
		sb.WriteString(toolView)
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
		m.cancelChat = cancel

		ch, err := m.client.SendMessage(ctx, m.sessionID, content)
		if err != nil {
			return ChatEventMsg{Event: &pb.ChatEvent{
				Event: &pb.ChatEvent_Error{
					Error: &pb.ErrorEvent{Message: err.Error()},
				},
			}}
		}

		// Read first event and carry channel for subsequent reads
		event, ok := <-ch
		if !ok {
			return chatStreamDoneMsg{}
		}
		return ChatEventMsg{Event: event, ch: ch}
	}
}

// nextEvent returns a Cmd that reads the next event from the channel.
func nextEvent(ch <-chan *pb.ChatEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return chatStreamDoneMsg{}
		}
		return ChatEventMsg{Event: event, ch: ch}
	}
}

func (m *ChatModel) handleChatEvent(msg ChatEventMsg) []tea.Cmd {
	event := msg.Event
	if event == nil {
		return nil
	}

	var cmds []tea.Cmd

	switch e := event.Event.(type) {
	case *pb.ChatEvent_Token:
		m.streaming += e.Token.Content
		m.refreshViewport()

	case *pb.ChatEvent_ToolStart:
		m.toolCalls.AddCard(e.ToolStart.CallId, e.ToolStart.ToolName, e.ToolStart.ArgumentsJson)
		m.refreshViewport()

	case *pb.ChatEvent_ToolResult:
		m.toolCalls.UpdateResult(e.ToolResult.CallId, e.ToolResult.ResultJson, e.ToolResult.Success)
		m.refreshViewport()

	case *pb.ChatEvent_Permission:
		// Show permission prompt inline
		m.messages = append(m.messages, components.Message{
			Role:    components.RoleTool,
			Content: "Permission required: " + e.Permission.ToolName + " — " + e.Permission.Description,
		})
		m.refreshViewport()

	case *pb.ChatEvent_AgentSpawned:
		m.messages = append(m.messages, components.Message{
			Role:    components.RoleTool,
			Content: "Agent spawned: " + e.AgentSpawned.AgentName + " (" + e.AgentSpawned.Role + ")",
		})
		m.refreshViewport()

	case *pb.ChatEvent_AgentMessage:
		m.messages = append(m.messages, components.Message{
			Role:    components.RoleTool,
			Content: "[" + e.AgentMessage.FromAgent + "] " + e.AgentMessage.Content,
		})
		m.refreshViewport()

	case *pb.ChatEvent_Complete:
		if m.streaming != "" {
			m.messages = append(m.messages, components.Message{
				Role:    components.RoleAssistant,
				Content: m.streaming,
			})
			m.streaming = ""
			m.refreshViewport()
		}
		m.cancelChat = nil
		return cmds // don't schedule next read — stream is done

	case *pb.ChatEvent_Error:
		m.messages = append(m.messages, components.Message{
			Role:    components.RoleTool,
			Content: "error: " + e.Error.Message,
		})
		m.streaming = ""
		m.refreshViewport()
		m.cancelChat = nil
		return cmds // don't schedule next read — stream is done
	}

	// Schedule read of next event from the channel
	if msg.ch != nil {
		cmds = append(cmds, nextEvent(msg.ch))
	}
	return cmds
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
