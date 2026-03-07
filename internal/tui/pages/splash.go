package pages

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
	"github.com/GoCodeAlone/ratchet-cli/internal/version"
)

// SplashDoneMsg signals the splash screen animation is complete.
type SplashDoneMsg struct{}

type splashTickMsg time.Time

var logoLines = []string{
	`  ██████╗  █████╗ ████████╗ ██████╗██╗  ██╗███████╗████████╗`,
	`  ██╔══██╗██╔══██╗╚══██╔══╝██╔════╝██║  ██║██╔════╝╚══██╔══╝`,
	`  ██████╔╝███████║   ██║   ██║     ███████║█████╗     ██║   `,
	`  ██╔══██╗██╔══██║   ██║   ██║     ██╔══██║██╔══╝     ██║   `,
	`  ██║  ██║██║  ██║   ██║   ╚██████╗██║  ██║███████╗   ██║   `,
	`  ╚═╝  ╚═╝╚═╝  ╚═╝   ╚═╝    ╚═════╝╚═╝  ╚═╝╚══════╝   ╚═╝   `,
}

const (
	splashTickInterval = 80 * time.Millisecond
	// Each logo line is revealed per tick, then version, then hint
	splashAutoTimeout = 30 // ticks before auto-dismiss (~2.4s)
)

// SplashModel renders the branded splash screen.
type SplashModel struct {
	frame  int
	width  int
	height int
	done   bool
}

func NewSplash() SplashModel {
	return SplashModel{}
}

func (m SplashModel) Init() tea.Cmd {
	return tea.Tick(splashTickInterval, func(t time.Time) tea.Msg {
		return splashTickMsg(t)
	})
}

func (m SplashModel) Update(msg tea.Msg) (SplashModel, tea.Cmd) {
	if m.done {
		return m, nil
	}

	switch msg.(type) {
	case splashTickMsg:
		m.frame++
		if m.frame >= splashAutoTimeout {
			m.done = true
			return m, func() tea.Msg { return SplashDoneMsg{} }
		}
		return m, tea.Tick(splashTickInterval, func(t time.Time) tea.Msg {
			return splashTickMsg(t)
		})
	case tea.KeyPressMsg:
		m.done = true
		return m, func() tea.Msg { return SplashDoneMsg{} }
	case tea.WindowSizeMsg:
		ws := msg.(tea.WindowSizeMsg)
		m.width = ws.Width
		m.height = ws.Height
	}

	return m, nil
}

func (m SplashModel) View(t theme.Theme, width, height int) string {
	w := width
	h := height
	if m.width > 0 {
		w = m.width
	}
	if m.height > 0 {
		h = m.height
	}

	logoStyle := lipgloss.NewStyle().Foreground(t.Primary).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(t.Muted)

	var content string

	// Narrow terminal: compact text logo
	if w < 78 {
		title := lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true).
			Render("ratchet")
		content = title + "\n\n"
		if m.frame >= 2 {
			content += mutedStyle.Render(version.String()) + "\n"
		}
		if m.frame >= 4 {
			content += "\n" + mutedStyle.Render("press any key to continue")
		}
	} else {
		// Full ASCII art logo, revealed line by line
		for i, line := range logoLines {
			if m.frame > i {
				content += logoStyle.Render(line) + "\n"
			}
		}

		// Version after logo is fully revealed
		if m.frame > len(logoLines)+1 {
			content += "\n" + mutedStyle.Render("  "+version.String()) + "\n"
		}

		// Hint after a short delay
		if m.frame > len(logoLines)+4 {
			content += "\n" + mutedStyle.Render("  press any key to continue")
		}
	}

	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, content)
}
