# TUI Improvements Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Improve the ratchet TUI with auto-expanding input, enhanced slash commands with autocompletion, richer status bar below the input, and fix the Copilot 403 token exchange bug.

**Architecture:** Five independent workstreams across two repos (ratchet-cli and workflow-plugin-agent). The TUI changes are in ratchet-cli; the Copilot fix is in workflow-plugin-agent. Status bar moves below input. Slash commands extended with `/model`, `/clear`, `/cost`, `/agents`, `/sessions`, `/exit`. Autocomplete renders a floating list above the input when `/` is typed.

**Tech Stack:** Go 1.26, Bubbletea v2, Bubbles v2 (textarea, viewport), Lipgloss v2, gRPC

**Repos:**
- `ratchet-cli`: `/Users/jon/workspace/ratchet-cli`
- `workflow-plugin-agent`: `/Users/jon/workspace/workflow-plugin-agent`

---

### Task 1: Auto-Expanding Input

**Files:**
- Modify: `internal/tui/components/input.go`
- Modify: `internal/tui/pages/chat.go`
- Modify: `internal/tui/tui_render_test.go`

**Step 1: Update InputModel to track dynamic height**

In `internal/tui/components/input.go`, change the textarea height from fixed 3 to start at 1. Add a method `Height()` that returns the current content height. Add a `ResizeMsg` that the chat page can listen for.

```go
// At top of file, add this message type:
type InputResizedMsg struct {
	Height int
}
```

In `NewInput()`, change `ta.SetHeight(3)` to `ta.SetHeight(1)`.

In `Update()`, after the textarea processes the message (line 94), calculate the new height based on line count and emit `InputResizedMsg` if it changed:

```go
// After m.textarea, cmd = m.textarea.Update(msg) on line 94:
newHeight := m.calcHeight()
if newHeight != m.height {
    m.height = newHeight
    m.textarea.SetHeight(newHeight)
    return m, tea.Batch(cmd, func() tea.Msg { return InputResizedMsg{Height: newHeight} })
}
```

Add a `height int` field to `InputModel` struct (initialize to 1 in `NewInput`).

Add the height calculation method:

```go
func (m InputModel) calcHeight() int {
    val := m.textarea.Value()
    lines := strings.Count(val, "\n") + 1
    if lines < 1 {
        lines = 1
    }
    if lines > 6 {
        lines = 6
    }
    return lines
}

func (m InputModel) Height() int {
    return m.height
}
```

Add `"strings"` to the imports.

**Step 2: Update chat relayout to use dynamic input height**

In `internal/tui/pages/chat.go`, change `relayout()` (line 159) to use dynamic input height:

```go
func (m *ChatModel) relayout() {
    inputHeight := m.input.Height() + 2  // +2 for border
    statusHeight := 2                     // two-line status bar
    vpHeight := m.height - inputHeight - statusHeight - 1  // -1 for newline between viewport and input
    if vpHeight < 1 {
        vpHeight = 1
    }
    m.viewport.SetHeight(vpHeight)
    m.viewport.SetWidth(m.width)
    m.statusBar.Width = m.width
    m.input.SetWidth(m.width)
    m.refreshViewport()
}
```

In `Update()`, add a case for `InputResizedMsg` to trigger relayout:

```go
case components.InputResizedMsg:
    m.relayout()
```

**Step 3: Update View() to render status bar below input**

In `internal/tui/pages/chat.go`, update `View()` (line 305):

```go
func (m ChatModel) View(t theme.Theme) string {
    var sb strings.Builder
    sb.WriteString(m.viewport.View())
    sb.WriteString("\n")
    sb.WriteString(m.input.View(t, m.width))
    sb.WriteString("\n")
    sb.WriteString(m.statusBar.View(t))
    return sb.String()
}
```

This is already the correct order (viewport → input → status). The layout is already correct since status bar renders last.

**Step 4: Update tests**

In `internal/tui/tui_render_test.go`, update `TestChatViewWithDimensions` to verify the input starts at 1 line height. Existing tests should still pass since `SetSize` triggers `relayout()`.

**Step 5: Run tests**

```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/tui/...
```

Expected: PASS

**Step 6: Commit**

```bash
git add internal/tui/components/input.go internal/tui/pages/chat.go internal/tui/tui_render_test.go
git commit -m "feat: auto-expanding input (1-6 lines)"
```

---

### Task 2: Enhanced Status Bar

**Files:**
- Modify: `internal/tui/components/statusbar.go`
- Modify: `internal/tui/pages/chat.go`
- Modify: `internal/tui/app.go`

**Step 1: Expand StatusBar struct and rendering**

Replace the entire `internal/tui/components/statusbar.go` with a two-line status bar:

```go
package components

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

type StatusBar struct {
	WorkingDir      string
	Provider        string
	Model           string
	SessionStart    time.Time
	InputTokens     int
	OutputTokens    int
	ActiveAgents    int
	BackgroundTasks int
	Width           int
}

func NewStatusBar() StatusBar {
	return StatusBar{
		SessionStart: time.Now(),
	}
}

func (s StatusBar) View(t theme.Theme) string {
	// Line 1: contextual info
	dir := shortenPath(s.WorkingDir)
	elapsed := formatElapsed(time.Since(s.SessionStart))

	segments := []string{" " + dir}
	if s.Model != "" {
		segments = append(segments, s.Model)
	}
	if s.ActiveAgents > 0 {
		segments = append(segments, fmt.Sprintf("agents: %d", s.ActiveAgents))
	}
	if s.BackgroundTasks > 0 {
		segments = append(segments, fmt.Sprintf("tasks: %d", s.BackgroundTasks))
	}
	segments = append(segments, "⏱ "+elapsed)
	if s.InputTokens > 0 || s.OutputTokens > 0 {
		segments = append(segments, fmt.Sprintf("↑%s ↓%s", formatTokens(s.InputTokens), formatTokens(s.OutputTokens)))
	}

	line1 := strings.Join(segments, "  ")

	// Line 2: keybind hints (right-aligned)
	hints := "Ctrl+S sidebar  Ctrl+T team  Ctrl+C quit "
	pad1 := s.Width - lipgloss.Width(line1)
	if pad1 < 0 {
		pad1 = 0
	}
	row1 := line1 + strings.Repeat(" ", pad1)

	pad2 := s.Width - lipgloss.Width(hints)
	if pad2 < 0 {
		pad2 = 0
	}
	row2 := strings.Repeat(" ", pad2) + hints

	return t.StatusBar.Width(s.Width).Render(row1 + "\n" + row2)
}

func shortenPath(p string) string {
	if p == "" {
		return "~"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

func formatElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func formatTokens(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
```

**Step 2: Wire working dir and model into StatusBar**

In `internal/tui/app.go`, in `transitionToChat()` (line 204), set the status bar's WorkingDir and Model after creating the chat:

```go
func (a App) transitionToChat() (tea.Model, tea.Cmd) {
    chat := pages.NewChat(a.client, a.sessionID, a.theme, a.dark)
    team := pages.NewTeam()
    chatHeight := a.height - 1
    if chatHeight < 1 {
        chatHeight = 1
    }
    chat.SetSize(a.width, chatHeight)
    // Set status bar context
    if a.session != nil {
        chat.SetWorkingDir(a.session.GetWorkingDir())
    }
    // Find default provider's model
    for _, p := range a.providers {
        if p.IsDefault {
            chat.SetProviderModel(p.Type, p.Model)
            break
        }
    }
    a.chat = chat
    a.team = team
    a.page = pageChat
    return a, a.chat.Init()
}
```

In `internal/tui/pages/chat.go`, add setter methods:

```go
func (m *ChatModel) SetWorkingDir(dir string) {
    m.statusBar.WorkingDir = dir
}

func (m *ChatModel) SetProviderModel(provider, model string) {
    m.statusBar.Provider = provider
    m.statusBar.Model = model
}
```

**Step 3: Update relayout for 2-line status bar**

Already done in Task 1 Step 2 (`statusHeight := 2`).

**Step 4: Run tests**

```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/tui/...
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/components/statusbar.go internal/tui/pages/chat.go internal/tui/app.go
git commit -m "feat: enhanced two-line status bar with project dir, model, tokens"
```

---

### Task 3: New Slash Commands

**Files:**
- Modify: `internal/tui/commands/commands.go`
- Modify: `internal/tui/pages/chat.go`

**Step 1: Add Cmd field to Result**

In `internal/tui/commands/commands.go`, extend the Result struct:

```go
type Result struct {
    Lines                []string
    NavigateToOnboarding bool
    Quit                 bool   // if true, chat returns tea.Quit
    ClearChat            bool   // if true, chat clears messages
}
```

**Step 2: Add new commands to Parse()**

In `commands.go`, add cases to the switch in `Parse()`:

```go
case "/model":
    return modelCmd(parts[1:], c)
case "/clear":
    return &Result{
        Lines:     []string{"Conversation cleared."},
        ClearChat: true,
    }
case "/cost":
    return &Result{Lines: []string{"Token usage is shown in the status bar below the input."}}
case "/agents":
    return agentsCmd(c)
case "/sessions":
    return sessionsCmd(c)
case "/exit":
    return &Result{
        Lines: []string{"Goodbye!"},
        Quit:  true,
    }
```

**Step 3: Implement modelCmd**

Add to `commands.go`:

```go
func modelCmd(args []string, c *client.Client) *Result {
    if c == nil {
        return &Result{Lines: []string{"Not connected to daemon"}}
    }
    resp, err := c.ListProviders(context.Background())
    if err != nil {
        return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
    }

    // No args: show current model
    if len(args) == 0 {
        lines := []string{"Current providers and models:", ""}
        for _, p := range resp.Providers {
            marker := "  "
            if p.IsDefault {
                marker = "→ "
            }
            lines = append(lines, fmt.Sprintf("%s%-12s %s", marker, p.Alias, p.Model))
        }
        lines = append(lines, "", "Use /model <alias> <model-name> to change a provider's model.")
        return &Result{Lines: lines}
    }

    // With args: could be /model <model-name> (for default provider)
    // For now, show usage
    if len(args) == 1 {
        return &Result{Lines: []string{
            fmt.Sprintf("To switch model, use: /model <alias> <model-name>"),
            "Use /model to see available providers and their current models.",
        }}
    }

    return &Result{Lines: []string{
        fmt.Sprintf("Model switching requires daemon support (not yet implemented)."),
        "For now, use /provider remove + /provider add to change models.",
    }}
}
```

**Step 4: Implement agentsCmd and sessionsCmd**

```go
func agentsCmd(c *client.Client) *Result {
    if c == nil {
        return &Result{Lines: []string{"Not connected to daemon"}}
    }
    resp, err := c.ListAgents(context.Background())
    if err != nil {
        return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
    }
    if len(resp.Agents) == 0 {
        return &Result{Lines: []string{"No active agents."}}
    }
    lines := []string{"Active agents:", ""}
    for _, a := range resp.Agents {
        lines = append(lines, fmt.Sprintf("  %-20s %-10s %s", a.Name, a.Status, a.Role))
    }
    return &Result{Lines: lines}
}

func sessionsCmd(c *client.Client) *Result {
    if c == nil {
        return &Result{Lines: []string{"Not connected to daemon"}}
    }
    resp, err := c.ListSessions(context.Background())
    if err != nil {
        return &Result{Lines: []string{fmt.Sprintf("Error: %v", err)}}
    }
    if len(resp.Sessions) == 0 {
        return &Result{Lines: []string{"No sessions."}}
    }
    lines := []string{"Sessions:", ""}
    for _, s := range resp.Sessions {
        lines = append(lines, fmt.Sprintf("  %-10s %-10s %s", s.Id[:8], s.Status, s.Name))
    }
    return &Result{Lines: lines}
}
```

**Step 5: Update helpCmd**

```go
func helpCmd() *Result {
    return &Result{Lines: []string{
        "Available commands:",
        "  /help                      Show this help",
        "  /model                     Show current model",
        "  /clear                     Clear conversation",
        "  /cost                      Show token usage",
        "  /agents                    List active agents",
        "  /sessions                  List sessions",
        "  /provider list             List configured providers",
        "  /provider add              Add a new provider (opens wizard)",
        "  /provider remove <alias>   Remove a provider",
        "  /provider default <alias>  Set default provider",
        "  /provider test <alias>     Test provider connection",
        "  /exit                      Quit ratchet",
    }}
}
```

**Step 6: Handle new Result fields in chat.go**

In `internal/tui/pages/chat.go`, in the `SubmitMsg` handler (around line 107), handle the new fields:

```go
case components.SubmitMsg:
    if result := commands.Parse(msg.Content, m.client); result != nil {
        m.messages = append(m.messages, components.Message{
            Role:    components.RoleUser,
            Content: msg.Content,
        })
        for _, line := range result.Lines {
            m.messages = append(m.messages, components.Message{
                Role:    components.RoleSystem,
                Content: line,
            })
        }
        if result.ClearChat {
            m.messages = nil
        }
        m.refreshViewport()
        if result.NavigateToOnboarding {
            return m, func() tea.Msg { return NavigateToOnboardingMsg{} }
        }
        if result.Quit {
            return m, tea.Quit
        }
        return m, nil
    }
    // ... rest of normal message handling
```

**Step 7: Run tests**

```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/tui/...
```

Expected: PASS

**Step 8: Commit**

```bash
git add internal/tui/commands/commands.go internal/tui/pages/chat.go
git commit -m "feat: add /model, /clear, /cost, /agents, /sessions, /exit commands"
```

---

### Task 4: Slash Command Autocomplete

**Files:**
- Create: `internal/tui/components/autocomplete.go`
- Modify: `internal/tui/pages/chat.go`
- Modify: `internal/tui/components/input.go`

**Step 1: Create autocomplete component**

Create `internal/tui/components/autocomplete.go`:

```go
package components

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

type CommandEntry struct {
	Name string
	Desc string
}

type AutocompleteModel struct {
	commands []CommandEntry
	matches  []CommandEntry
	filter   string
	cursor   int
	visible  bool
}

// AutocompleteSelectedMsg is sent when a command is selected from the dropdown.
type AutocompleteSelectedMsg struct {
	Command string
}

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
	}
	return AutocompleteModel{commands: commands}
}

func (m AutocompleteModel) Visible() bool { return m.visible }

func (m AutocompleteModel) SetFilter(input string) AutocompleteModel {
	input = strings.TrimSpace(input)
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

func (m AutocompleteModel) View(t theme.Theme, width int) string {
	if !m.visible || len(m.matches) == 0 {
		return ""
	}

	style := lipgloss.NewStyle().
		Background(t.Background).
		Foreground(t.Foreground).
		Width(min(width-4, 50))

	selectedStyle := style.
		Background(t.Primary).
		Foreground(lipgloss.Color("#FFFFFF"))

	var sb strings.Builder
	for i, cmd := range m.matches {
		line := " " + cmd.Name + "  " + cmd.Desc
		if i == m.cursor {
			sb.WriteString(selectedStyle.Render(line))
		} else {
			sb.WriteString(style.Render(line))
		}
		if i < len(m.matches)-1 {
			sb.WriteString("\n")
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Muted).
		Render(sb.String())
}
```

**Step 2: Add method to InputModel to get current value**

In `internal/tui/components/input.go`, add:

```go
func (m InputModel) Value() string {
	return m.textarea.Value()
}
```

**Step 3: Integrate autocomplete into chat.go**

In `internal/tui/pages/chat.go`:

1. Add `autocomplete components.AutocompleteModel` field to `ChatModel` struct.
2. In `NewChat()`, add: `autocomplete: components.NewAutocomplete(),`
3. In `Update()`, before forwarding keys to input, check if autocomplete is active:

```go
case tea.KeyPressMsg:
    // Cancel in-flight streaming with Escape
    if msg.String() == "esc" && m.cancelChat != nil {
        m.cancelChat()
        m.cancelChat = nil
    }

    // If autocomplete is visible, route navigation keys to it
    if m.autocomplete.Visible() {
        switch msg.String() {
        case "up", "down", "tab", "enter", "esc":
            var acCmd tea.Cmd
            m.autocomplete, acCmd = m.autocomplete.Update(msg)
            cmds = append(cmds, acCmd)
            return m, tea.Batch(cmds...)
        }
    }
```

4. Handle `AutocompleteSelectedMsg`:

```go
case components.AutocompleteSelectedMsg:
    // Replace input with selected command + space
    m.input.SetValue(msg.Command + " ")
    m.autocomplete = m.autocomplete.SetFilter("")
```

5. Add `SetValue` method to InputModel in `input.go`:

```go
func (m *InputModel) SetValue(s string) {
    m.textarea.SetValue(s)
}
```

6. After input update (where `m.input, inputCmd = m.input.Update(msg)` is called), update the autocomplete filter:

```go
m.autocomplete = m.autocomplete.SetFilter(m.input.Value())
```

7. In `View()`, render autocomplete between viewport and input:

```go
func (m ChatModel) View(t theme.Theme) string {
    var sb strings.Builder
    sb.WriteString(m.viewport.View())
    sb.WriteString("\n")
    ac := m.autocomplete.View(t, m.width)
    if ac != "" {
        sb.WriteString(ac)
        sb.WriteString("\n")
    }
    sb.WriteString(m.input.View(t, m.width))
    sb.WriteString("\n")
    sb.WriteString(m.statusBar.View(t))
    return sb.String()
}
```

**Step 4: Run tests**

```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/tui/...
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/components/autocomplete.go internal/tui/components/input.go internal/tui/pages/chat.go
git commit -m "feat: slash command autocomplete dropdown"
```

---

### Task 5: Fix Copilot Token Exchange (workflow-plugin-agent)

**Files:**
- Modify: `/Users/jon/workspace/workflow-plugin-agent/provider/copilot.go`
- Create: `/Users/jon/workspace/workflow-plugin-agent/provider/copilot_test.go` (test for token exchange)

**Step 1: Add token exchange types and state**

In `provider/copilot.go`, add the token exchange URL constant and types:

```go
const (
    copilotTokenExchangeURL = "https://api.github.com/copilot_internal/v2/token"
    copilotEditorVersion    = "ratchet/0.1.0"
)
```

Add fields to `CopilotProvider` for managing the exchanged token:

```go
type CopilotProvider struct {
    config      CopilotConfig
    mu          sync.Mutex
    bearerToken string
    expiresAt   time.Time
}
```

Add `"sync"` and `"time"` to imports.

**Step 2: Implement token exchange**

Add the exchange method:

```go
// copilotTokenResponse is the response from the Copilot token exchange endpoint.
type copilotTokenResponse struct {
    Token     string `json:"token"`
    ExpiresAt int64  `json:"expires_at"`
    RefreshIn int    `json:"refresh_in"`
}

// ensureBearerToken exchanges the GitHub OAuth token for a short-lived Copilot
// bearer token, caching and refreshing as needed.
func (p *CopilotProvider) ensureBearerToken(ctx context.Context) (string, error) {
    p.mu.Lock()
    defer p.mu.Unlock()

    // Return cached token if still valid (with 60s buffer)
    if p.bearerToken != "" && time.Now().Before(p.expiresAt.Add(-60*time.Second)) {
        return p.bearerToken, nil
    }

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, copilotTokenExchangeURL, nil)
    if err != nil {
        return "", fmt.Errorf("copilot: create token exchange request: %w", err)
    }
    req.Header.Set("Authorization", "Token "+p.config.Token)
    req.Header.Set("User-Agent", copilotEditorVersion)
    req.Header.Set("Accept", "application/json")

    resp, err := p.config.HTTPClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("copilot: token exchange request: %w", err)
    }
    defer func() { _ = resp.Body.Close() }()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("copilot: read token exchange response: %w", err)
    }

    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("copilot: token exchange failed (status %d): %s", resp.StatusCode, truncate(string(body), 200))
    }

    var tokenResp copilotTokenResponse
    if err := json.Unmarshal(body, &tokenResp); err != nil {
        return "", fmt.Errorf("copilot: parse token exchange response: %w", err)
    }

    p.bearerToken = tokenResp.Token
    p.expiresAt = time.Unix(tokenResp.ExpiresAt, 0)

    return p.bearerToken, nil
}
```

**Step 3: Update setHeaders to use exchanged token**

Replace `setHeaders` with a context-aware version:

```go
func (p *CopilotProvider) setHeaders(ctx context.Context, req *http.Request) error {
    token, err := p.ensureBearerToken(ctx)
    if err != nil {
        return err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Copilot-Integration-Id", "vscode-chat")
    req.Header.Set("Editor-Version", "vscode/1.100.0")
    req.Header.Set("Editor-Plugin-Version", copilotEditorVersion)
    return nil
}
```

Note: `Copilot-Integration-Id` changes from `"ratchet"` to `"vscode-chat"` — this is required by the API.

**Step 4: Update Chat() and Stream() to pass context to setHeaders**

In `Chat()` (line 135), change:
```go
// Old:
p.setHeaders(req)
// New:
if err := p.setHeaders(ctx, req); err != nil {
    return nil, err
}
```

In `Stream()` (line 176), same change:
```go
// Old:
p.setHeaders(req)
// New:
if err := p.setHeaders(ctx, req); err != nil {
    return nil, err
}
```

**Step 5: Update listCopilotModels to also do token exchange**

In `provider/models.go`, `listCopilotModels()` (line 172) currently sends the raw token. It needs to exchange too. The simplest approach: create a temporary `CopilotProvider` and use its `ensureBearerToken`:

```go
func listCopilotModels(ctx context.Context, apiKey, baseURL string) ([]ModelInfo, error) {
    if baseURL == "" {
        baseURL = defaultCopilotBaseURL
    }

    // Exchange OAuth token for Copilot bearer token
    p := NewCopilotProvider(CopilotConfig{Token: apiKey, BaseURL: baseURL})
    token, err := p.ensureBearerToken(ctx)
    if err != nil {
        return copilotFallbackModels(), nil
    }

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
    if err != nil {
        return copilotFallbackModels(), nil
    }
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Copilot-Integration-Id", "vscode-chat")
    req.Header.Set("Editor-Version", "vscode/1.100.0")
    req.Header.Set("Editor-Plugin-Version", copilotEditorVersion)

    // ... rest unchanged
```

**Step 6: Run tests**

```bash
cd /Users/jon/workspace/workflow-plugin-agent && go test ./provider/...
```

Expected: PASS

**Step 7: Tag and release workflow-plugin-agent**

```bash
cd /Users/jon/workspace/workflow-plugin-agent
git add provider/copilot.go provider/models.go
git commit -m "fix: add Copilot token exchange to fix 403 on API calls"
git tag v0.1.2
git push origin main --tags
```

**Step 8: Update ratchet-cli's go.mod to use new version**

```bash
cd /Users/jon/workspace/ratchet-cli
go get github.com/GoCodeAlone/workflow-plugin-agent@v0.1.2
go mod tidy
```

**Step 9: Commit dependency update**

```bash
git add go.mod go.sum
git commit -m "chore: bump workflow-plugin-agent to v0.1.2 (copilot token exchange fix)"
```

---

### Task 6: Remove Header Keybind Hints (now in status bar)

**Files:**
- Modify: `internal/tui/app.go`

**Step 1: Clean up redundant header hints**

In `internal/tui/app.go`, `renderHeader()` (line 261) currently shows keybind hints. Remove them since they're now in the status bar:

```go
func (a App) renderHeader() string {
    title := lipgloss.NewStyle().
        Foreground(a.theme.Primary).
        Bold(true).
        Render("ratchet")

    sessionInfo := lipgloss.NewStyle().
        Foreground(a.theme.Muted).
        Render(fmt.Sprintf("  session: %s", a.sessionID[:8]))

    return title + sessionInfo
}
```

**Step 2: Run all tests**

```bash
cd /Users/jon/workspace/ratchet-cli && go test ./...
```

Expected: PASS

**Step 3: Commit**

```bash
git add internal/tui/app.go
git commit -m "refactor: move keybind hints from header to status bar"
```

---

### Task 7: Final Integration Test and Release

**Step 1: Run full test suite**

```bash
cd /Users/jon/workspace/ratchet-cli && go test ./...
```

Expected: All PASS

**Step 2: Build and verify locally**

```bash
cd /Users/jon/workspace/ratchet-cli && go build -o ratchet ./cmd/ratchet
```

Expected: Clean build, no errors.

**Step 3: Tag and push release**

```bash
git tag v0.1.9
git push origin master --tags
```

**Step 4: Monitor release**

```bash
gh run list --repo GoCodeAlone/ratchet-cli --limit 1
gh run watch <run-id> --exit-status
```

Expected: Release succeeds with linux/darwin amd64/arm64 binaries.
