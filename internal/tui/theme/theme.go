package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

type Theme struct {
	// Colors
	Primary    color.Color
	Secondary  color.Color
	Accent     color.Color
	Error      color.Color
	Success    color.Color
	Warning    color.Color
	Muted      color.Color
	Background color.Color
	Foreground color.Color

	// Styles
	UserMessage      lipgloss.Style
	AssistantMessage lipgloss.Style
	ToolCallCard     lipgloss.Style
	ToolCallSuccess  lipgloss.Style
	ToolCallPending  lipgloss.Style
	PermissionCard   lipgloss.Style
	StatusBar        lipgloss.Style
	InputArea        lipgloss.Style
	SessionHeader    lipgloss.Style
	SidebarItem      lipgloss.Style
	SidebarActive    lipgloss.Style
	AgentBadge       lipgloss.Style
	ErrorText        lipgloss.Style
}

func Dark() Theme {
	t := Theme{
		Primary:    lipgloss.Color("#7C3AED"),
		Secondary:  lipgloss.Color("#6B7280"),
		Accent:     lipgloss.Color("#10B981"),
		Error:      lipgloss.Color("#EF4444"),
		Success:    lipgloss.Color("#10B981"),
		Warning:    lipgloss.Color("#F59E0B"),
		Muted:      lipgloss.Color("#6B7280"),
		Background: lipgloss.Color("#1F2937"),
		Foreground: lipgloss.Color("#F9FAFB"),
	}

	t.UserMessage = lipgloss.NewStyle().
		Foreground(t.Foreground).
		Bold(true).
		PaddingLeft(1)

	t.AssistantMessage = lipgloss.NewStyle().
		Foreground(t.Foreground).
		PaddingLeft(1)

	t.ToolCallCard = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Secondary).
		Padding(0, 1).
		MarginLeft(2)

	t.ToolCallSuccess = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Success).
		Padding(0, 1).
		MarginLeft(2)

	t.ToolCallPending = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Warning).
		Padding(0, 1).
		MarginLeft(2)

	t.PermissionCard = lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(t.Warning).
		Padding(1, 2).
		MarginLeft(2)

	t.StatusBar = lipgloss.NewStyle().
		Background(t.Background).
		Foreground(t.Muted).
		Padding(0, 1)

	t.InputArea = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Primary).
		Padding(0, 1)

	t.SessionHeader = lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true)

	t.SidebarItem = lipgloss.NewStyle().
		Foreground(t.Muted).
		PaddingLeft(1)

	t.SidebarActive = lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true).
		PaddingLeft(1)

	t.AgentBadge = lipgloss.NewStyle().
		Background(t.Primary).
		Foreground(lipgloss.Color("#FFFFFF")).
		Padding(0, 1).
		Bold(true)

	t.ErrorText = lipgloss.NewStyle().
		Foreground(t.Error)

	return t
}

func Light() Theme {
	t := Dark()
	t.Background = lipgloss.Color("#FFFFFF")
	t.Foreground = lipgloss.Color("#111827")
	t.Muted = lipgloss.Color("#9CA3AF")

	// Rebuild styles with light colors
	t.UserMessage = lipgloss.NewStyle().
		Foreground(t.Foreground).
		Bold(true).
		PaddingLeft(1)

	t.AssistantMessage = lipgloss.NewStyle().
		Foreground(t.Foreground).
		PaddingLeft(1)

	t.StatusBar = lipgloss.NewStyle().
		Background(t.Background).
		Foreground(t.Muted).
		Padding(0, 1)

	t.SidebarItem = lipgloss.NewStyle().
		Foreground(t.Muted).
		PaddingLeft(1)

	return t
}
