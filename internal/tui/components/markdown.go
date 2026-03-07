package components

import (
	"github.com/charmbracelet/glamour"
)

// RenderMarkdown renders markdown content to styled terminal output.
// Falls back to raw content if rendering fails.
func RenderMarkdown(content string, width int, dark bool) string {
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
