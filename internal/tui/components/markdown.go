package components

import (
	"sync"

	"github.com/charmbracelet/glamour"
)

// minRenderWidth prevents negative or zero wrap widths when the terminal
// size hasn't been received yet.
const minRenderWidth = 40

var (
	cachedRenderer *glamour.TermRenderer
	cachedWidth    int
	cachedDark     bool
	rendererMu     sync.Mutex
)

// getRenderer returns a cached glamour renderer for the given width and dark
// mode, creating a new one only when the parameters change.
func getRenderer(width int, dark bool) *glamour.TermRenderer {
	rendererMu.Lock()
	defer rendererMu.Unlock()
	if cachedRenderer != nil && cachedWidth == width && cachedDark == dark {
		return cachedRenderer
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
		return nil
	}
	cachedRenderer = r
	cachedWidth = width
	cachedDark = dark
	return cachedRenderer
}

// RenderMarkdown renders markdown content to styled terminal output.
// Falls back to raw content if rendering fails.
func RenderMarkdown(content string, width int, dark bool) string {
	if width < minRenderWidth {
		width = minRenderWidth
	}

	r := getRenderer(width, dark)
	if r == nil {
		return content
	}

	rendered, err := r.Render(content)
	if err != nil {
		return content
	}
	return rendered
}
