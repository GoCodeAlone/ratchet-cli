package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestSessionTreeModelFlattensAndRendersHierarchy(t *testing.T) {
	model := NewSessionTree().SetSize(96, 16).SetSessions(sampleTreeSessions())
	model = model.SetPreview("fork-2", []*pb.HistoryMessage{
		{Id: "msg-3", Role: "assistant", Content: "branch\npreview\twith\x01 controls", Timestamp: timestamppb.Now()},
	})
	for range 3 {
		var cmd tea.Cmd
		model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		if cmd != nil {
			t.Fatalf("unexpected command while moving cursor: %T", cmd())
		}
	}

	if model.SelectedSessionID() != "fork-2" {
		t.Fatalf("selected session = %q, want fork-2", model.SelectedSessionID())
	}

	view := model.View(theme.Dark())
	for _, want := range []string{
		"Session Tree",
		"root-1",
		"fork-1",
		"clone-1",
		"fork-2",
		"root summary",
		"fork summary",
		"branch preview with controls",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestSessionTreeModelNavigationSelectionAndCollapse(t *testing.T) {
	model := NewSessionTree().SetSessions(sampleTreeSessions())

	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if model.SelectedSessionID() != "fork-1" {
		t.Fatalf("after down selected = %q, want fork-1", model.SelectedSessionID())
	}

	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if model.SelectedSessionID() != "clone-1" {
		t.Fatalf("after expanding fork-1 and down selected = %q, want clone-1", model.SelectedSessionID())
	}

	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if model.SelectedSessionID() != "fork-2" {
		t.Fatalf("after collapsing fork-1 selected = %q, want fork-2", model.SelectedSessionID())
	}

	var got tea.Msg
	_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		got = cmd()
	}
	selected, ok := got.(SessionTreeSelectedMsg)
	if !ok {
		t.Fatalf("enter msg = %T, want SessionTreeSelectedMsg", got)
	}
	if selected.SessionID != "fork-2" {
		t.Fatalf("selected msg session = %q, want fork-2", selected.SessionID)
	}
}

func TestSessionTreeModelBoundsAndSanitizesText(t *testing.T) {
	model := NewSessionTree().SetSize(64, 8).SetSessions([]*pb.Session{
		{
			Id:            "root-1",
			RootId:        "root-1",
			Status:        "active",
			BranchSummary: "summary\nwith\tcontrols\x01 and a very long suffix that should truncate",
		},
	})

	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if model.SelectedSessionID() != "root-1" {
		t.Fatalf("up at first changed selection to %q", model.SelectedSessionID())
	}
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if model.SelectedSessionID() != "root-1" {
		t.Fatalf("down at last changed selection to %q", model.SelectedSessionID())
	}

	view := model.View(theme.Dark())
	if strings.ContainsAny(view, "\x01\t") {
		t.Fatalf("view contains unsanitized control text:\n%q", view)
	}
	if !strings.Contains(view, "summary with controls") {
		t.Fatalf("view missing normalized summary:\n%s", view)
	}
	if strings.Contains(view, "very long suffix that should truncate") {
		t.Fatalf("view did not truncate long summary:\n%s", view)
	}
}

func sampleTreeSessions() []*pb.Session {
	return []*pb.Session{
		{Id: "root-1", RootId: "root-1", Status: "active", BranchSummary: "root summary"},
		{Id: "fork-1", ParentId: "root-1", RootId: "root-1", ForkedFromMessageId: "msg-1", Status: "active", BranchSummary: "fork summary"},
		{Id: "fork-2", ParentId: "root-1", RootId: "root-1", ForkedFromMessageId: "msg-2", Status: "completed", BranchSummary: "second fork"},
		{Id: "clone-1", ParentId: "fork-1", RootId: "root-1", Status: "active", BranchSummary: "clone summary"},
	}
}
