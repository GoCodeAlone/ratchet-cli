package pages

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/components"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

func TestSessionTreeBrowserLoadsTreeAndSelectedPreview(t *testing.T) {
	client := &fakeSessionTreeClient{
		tree: sampleSessionList(),
		history: map[string]*pb.SessionHistory{
			"root-1": {Messages: []*pb.HistoryMessage{{Id: "m1", Role: "user", Content: "root prompt"}}},
			"fork-1": {Messages: []*pb.HistoryMessage{{Id: "m2", Role: "assistant", Content: "fork reply"}}},
		},
	}
	model := NewSessionTreeBrowser(client, "root-1", theme.Dark())

	msg := runCmd(t, model.Init())
	var cmd tea.Cmd
	model, cmd = model.Update(msg)
	if client.treeID != "root-1" {
		t.Fatalf("tree loaded for %q, want root-1", client.treeID)
	}

	msg = runCmd(t, cmd)
	model, _ = model.Update(msg)
	if client.historyIDs[len(client.historyIDs)-1] != "root-1" {
		t.Fatalf("history loaded for %q, want root-1", client.historyIDs)
	}
	if !strings.Contains(model.View(), "root prompt") {
		t.Fatalf("view missing loaded preview:\n%s", model.View())
	}
}

func TestSessionTreeBrowserEmptyTreeDoesNotLoadPreview(t *testing.T) {
	client := &fakeSessionTreeClient{tree: &pb.SessionList{}}
	model := NewSessionTreeBrowser(client, "root-1", theme.Dark())

	var cmd tea.Cmd
	model, cmd = model.Update(runCmd(t, model.Init()))
	if cmd != nil {
		t.Fatalf("empty tree returned preview command")
	}
	if len(client.historyIDs) != 0 {
		t.Fatalf("history calls = %v, want none", client.historyIDs)
	}
	if !strings.Contains(model.View(), "No sessions") {
		t.Fatalf("view missing empty tree state:\n%s", model.View())
	}
}

func TestSessionTreeBrowserSelectionRefreshesPreviewAndSelects(t *testing.T) {
	client := &fakeSessionTreeClient{
		tree: sampleSessionList(),
		history: map[string]*pb.SessionHistory{
			"root-1": {Messages: []*pb.HistoryMessage{{Id: "m1", Role: "user", Content: "root prompt"}}},
			"fork-1": {Messages: []*pb.HistoryMessage{{Id: "m2", Role: "assistant", Content: "fork reply"}}},
		},
	}
	model := NewSessionTreeBrowser(client, "root-1", theme.Dark())
	var cmd tea.Cmd
	model, cmd = model.Update(runCmd(t, model.Init()))
	model, _ = model.Update(runCmd(t, cmd))

	model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model, _ = model.Update(runCmd(t, cmd))
	if client.historyIDs[len(client.historyIDs)-1] != "fork-1" {
		t.Fatalf("history loaded for %q, want fork-1", client.historyIDs)
	}
	if !strings.Contains(model.View(), "fork reply") {
		t.Fatalf("view missing fork preview:\n%s", model.View())
	}

	_, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	selected, ok := runCmd(t, cmd).(components.SessionTreeSelectedMsg)
	if !ok {
		t.Fatalf("enter msg type = %T, want SessionTreeSelectedMsg", selected)
	}
	if selected.SessionID != "fork-1" {
		t.Fatalf("selected session = %q, want fork-1", selected.SessionID)
	}
}

func TestSessionTreeBrowserIgnoresStalePreviewResults(t *testing.T) {
	client := &fakeSessionTreeClient{
		tree: sampleSessionList(),
		history: map[string]*pb.SessionHistory{
			"fork-1": {Messages: []*pb.HistoryMessage{{Id: "m2", Role: "assistant", Content: "fork reply"}}},
		},
	}
	model := NewSessionTreeBrowser(client, "root-1", theme.Dark())
	var cmd tea.Cmd
	model, cmd = model.Update(runCmd(t, model.Init()))
	model, _ = model.Update(runCmd(t, cmd))

	model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model, _ = model.Update(sessionPreviewLoadedMsg{
		sessionID: "root-1",
		err:       errors.New("stale root preview failed"),
	})
	if strings.Contains(model.View(), "stale root preview failed") {
		t.Fatalf("view rendered stale preview error:\n%s", model.View())
	}

	model, _ = model.Update(runCmd(t, cmd))
	if !strings.Contains(model.View(), "fork reply") {
		t.Fatalf("view missing current preview:\n%s", model.View())
	}
}

func TestSessionTreeBrowserRefreshAndErrors(t *testing.T) {
	client := &fakeSessionTreeClient{
		treeErr: errors.New("daemon unavailable"),
	}
	model := NewSessionTreeBrowser(client, "root-1", theme.Dark())

	model, _ = model.Update(runCmd(t, model.Init()))
	if !strings.Contains(model.View(), "daemon unavailable") {
		t.Fatalf("view missing tree error:\n%s", model.View())
	}

	client.treeErr = nil
	client.tree = sampleSessionList()
	model, cmd := model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	model, _ = model.Update(runCmd(t, cmd))
	if client.treeLoads != 2 {
		t.Fatalf("tree loads = %d, want 2", client.treeLoads)
	}
	if !strings.Contains(model.View(), "root summary") {
		t.Fatalf("view missing refreshed tree:\n%s", model.View())
	}
}

func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected command")
	}
	return cmd()
}

func sampleSessionList() *pb.SessionList {
	return &pb.SessionList{Sessions: []*pb.Session{
		{Id: "root-1", RootId: "root-1", Status: "active", BranchSummary: "root summary"},
		{Id: "fork-1", ParentId: "root-1", RootId: "root-1", Status: "active", BranchSummary: "fork summary"},
	}}
}

type fakeSessionTreeClient struct {
	tree       *pb.SessionList
	treeErr    error
	history    map[string]*pb.SessionHistory
	treeID     string
	treeLoads  int
	historyIDs []string
}

func (f *fakeSessionTreeClient) GetSessionTree(_ context.Context, sessionID string) (*pb.SessionList, error) {
	f.treeID = sessionID
	f.treeLoads++
	if f.treeErr != nil {
		return nil, f.treeErr
	}
	return f.tree, nil
}

func (f *fakeSessionTreeClient) ListSessionMessages(_ context.Context, sessionID string) (*pb.SessionHistory, error) {
	f.historyIDs = append(f.historyIDs, sessionID)
	if f.history == nil {
		return &pb.SessionHistory{}, nil
	}
	return f.history[sessionID], nil
}
