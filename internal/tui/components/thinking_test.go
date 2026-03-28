package components

import (
	"strings"
	"testing"
)

func TestThinkingPanel_AppendContent(t *testing.T) {
	p := NewThinkingPanel(80)
	p = p.AppendContent("First chunk. ")
	p = p.AppendContent("Second chunk.")

	if p.content != "First chunk. Second chunk." {
		t.Errorf("content: want %q, got %q", "First chunk. Second chunk.", p.content)
	}
}

func TestThinkingPanel_CollapsedView(t *testing.T) {
	p := NewThinkingPanel(80)
	p = p.AppendContent("line1\nline2\nline3")
	p = p.SetCollapsed(true)

	view := p.View()
	if !strings.Contains(view, "3 lines") {
		t.Errorf("collapsed view should show line count, got: %q", view)
	}
	if !strings.Contains(view, "▶") {
		t.Errorf("collapsed view should show ▶, got: %q", view)
	}
}

func TestThinkingPanel_ExpandedView(t *testing.T) {
	p := NewThinkingPanel(80)
	p = p.AppendContent("reasoning here")
	p = p.SetCollapsed(false)

	view := p.View()
	if !strings.Contains(view, "reasoning here") {
		t.Errorf("expanded view should show content, got: %q", view)
	}
	if !strings.Contains(view, "▼") {
		t.Errorf("expanded view should show ▼, got: %q", view)
	}
}

func TestThinkingPanel_Reset(t *testing.T) {
	p := NewThinkingPanel(80)
	p = p.AppendContent("old content")
	p = p.Reset()

	if p.content != "" {
		t.Errorf("after reset, content should be empty, got %q", p.content)
	}
	if p.collapsed {
		t.Error("after reset, should not be collapsed")
	}
}

func TestThinkingPanel_EmptyView(t *testing.T) {
	p := NewThinkingPanel(80)
	view := p.View()
	if view != "" {
		t.Errorf("empty panel should render nothing, got %q", view)
	}
}
