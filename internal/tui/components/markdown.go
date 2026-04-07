package components

import (
	"github.com/charmbracelet/glamour"
)

// minRenderWidth prevents negative or zero wrap widths when the terminal
// size hasn't been received yet.
const minRenderWidth = 40

// RenderMarkdown renders markdown content to styled terminal output.
// Falls back to raw content if rendering fails.
func RenderMarkdown(content string, width int, dark bool) string {
	if width < minRenderWidth {
		width = minRenderWidth
	}

	styleName := "dark"
	if !dark {
		styleName = "light"
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(styleName),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return content
	}

	rendered, err := r.Render(content)
	if err != nil {
		return content
	}
	return rendered
}
