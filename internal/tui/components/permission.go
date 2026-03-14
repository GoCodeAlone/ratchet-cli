package components

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

// PermissionResponse is sent when the user resolves a permission request.
type PermissionResponse struct {
	RequestID string
	Allowed   bool
	Scope     string // "once", "session", "always"
}

type permissionOption struct {
	label   string
	allowed bool
	scope   string
}

var permissionOptions = []permissionOption{
	{label: "Allow once", allowed: true, scope: "once"},
	{label: "Allow for session", allowed: true, scope: "session"},
	{label: "Always allow", allowed: true, scope: "always"},
	{label: "Deny", allowed: false, scope: "once"},
}

// PermissionPrompt displays an inline permission request, blocking until resolved.
type PermissionPrompt struct {
	RequestID string
	ToolName  string
	ArgsJSON  string
	Desc      string
	selected  int
	resolved  bool
}

func NewPermissionPrompt(requestID, toolName, argsJSON, desc string) PermissionPrompt {
	return PermissionPrompt{
		RequestID: requestID,
		ToolName:  toolName,
		ArgsJSON:  argsJSON,
		Desc:      desc,
	}
}

func (p PermissionPrompt) Resolved() bool {
	return p.resolved
}

func (p PermissionPrompt) Update(msg tea.Msg) (PermissionPrompt, tea.Cmd) {
	if p.resolved {
		return p, nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "j", "down":
			if p.selected < len(permissionOptions)-1 {
				p.selected++
			}
		case "k", "up":
			if p.selected > 0 {
				p.selected--
			}
		case "1":
			p.selected = 0
		case "2":
			p.selected = 1
		case "3":
			p.selected = 2
		case "4":
			p.selected = 3
		case "enter", " ":
			opt := permissionOptions[p.selected]
			p.resolved = true
			return p, func() tea.Msg {
				return PermissionResponse{
					RequestID: p.RequestID,
					Allowed:   opt.allowed,
					Scope:     opt.scope,
				}
			}
		}
	}
	return p, nil
}

func (p PermissionPrompt) View(t theme.Theme, width int) string {
	var sb strings.Builder

	title := lipgloss.NewStyle().Foreground(t.Warning).Bold(true).Render("⚠ Permission Required")
	sb.WriteString(title + "\n\n")

	fmt.Fprintf(&sb, "Tool: %s\n", p.ToolName)
	if p.Desc != "" {
		fmt.Fprintf(&sb, "Description: %s\n", p.Desc)
	}
	if p.ArgsJSON != "" {
		args := p.ArgsJSON
		if len(args) > 200 {
			args = args[:197] + "..."
		}
		fmt.Fprintf(&sb, "Arguments: %s\n", args)
	}
	sb.WriteString("\n")

	for i, opt := range permissionOptions {
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(t.Muted)
		if i == p.selected {
			cursor = "▶ "
			if opt.allowed {
				style = lipgloss.NewStyle().Foreground(t.Success).Bold(true)
			} else {
				style = lipgloss.NewStyle().Foreground(t.Error).Bold(true)
			}
		}
		label := fmt.Sprintf("%s%d. %s", cursor, i+1, opt.label)
		sb.WriteString(style.Render(label) + "\n")
	}

	sb.WriteString("\n")
	hint := lipgloss.NewStyle().Foreground(t.Muted).Render("↑/↓ or 1-4: select  Enter: confirm")
	sb.WriteString(hint)

	return t.PermissionCard.Width(width - 4).Render(strings.TrimRight(sb.String(), "\n"))
}
