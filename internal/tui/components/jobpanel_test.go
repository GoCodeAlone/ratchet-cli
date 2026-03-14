package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

func TestJobPanel_Render(t *testing.T) {
	jp := NewJobPanel(nil).SetSize(100, 24)
	view := jp.View(theme.Dark())
	if !strings.Contains(view, "Active Jobs") {
		t.Error("expected 'Active Jobs' in view")
	}
	if !strings.Contains(view, "No active jobs") {
		t.Error("expected 'No active jobs' when list is empty")
	}
}

func TestJobPanel_Navigation(t *testing.T) {
	jp := NewJobPanel(nil).SetSize(100, 24)
	// Inject jobs directly via a refresh message.
	jp, _ = jp.Update(JobListRefreshedMsg{Jobs: []*pb.Job{
		{Id: "session:a", Type: "session", Name: "sess-a", Status: "running"},
		{Id: "cron:b", Type: "cron", Name: "cleanup", Status: "active"},
		{Id: "fleet_worker:c", Type: "fleet_worker", Name: "worker-1", Status: "running"},
	}})

	if jp.cursor != 0 {
		t.Errorf("expected cursor=0 initially, got %d", jp.cursor)
	}

	// Move down.
	jp, _ = jp.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if jp.cursor != 1 {
		t.Errorf("expected cursor=1 after down, got %d", jp.cursor)
	}

	// Move down again.
	jp, _ = jp.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if jp.cursor != 2 {
		t.Errorf("expected cursor=2 after second down, got %d", jp.cursor)
	}

	// Cannot go past last row.
	jp, _ = jp.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if jp.cursor != 2 {
		t.Errorf("expected cursor to stay at 2, got %d", jp.cursor)
	}

	// Move back up.
	jp, _ = jp.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if jp.cursor != 1 {
		t.Errorf("expected cursor=1 after up, got %d", jp.cursor)
	}
}

func TestJobPanel_PauseAction(t *testing.T) {
	jp := NewJobPanel(nil).SetSize(100, 24)
	jp, _ = jp.Update(JobListRefreshedMsg{Jobs: []*pb.Job{
		{Id: "cron:job1", Type: "cron", Name: "cleanup", Status: "active"},
	}})

	var gotMsg tea.Msg
	_, cmd := jp.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	if cmd != nil {
		gotMsg = cmd()
	}
	pm, ok := gotMsg.(JobPauseMsg)
	if !ok {
		t.Fatalf("expected JobPauseMsg, got %T", gotMsg)
	}
	if pm.JobID != "cron:job1" {
		t.Errorf("expected JobID=cron:job1, got %q", pm.JobID)
	}
}

func TestJobPanel_KillAction(t *testing.T) {
	jp := NewJobPanel(nil).SetSize(100, 24)
	jp, _ = jp.Update(JobListRefreshedMsg{Jobs: []*pb.Job{
		{Id: "session:sess1", Type: "session", Name: "my-session", Status: "running"},
	}})

	var gotMsg tea.Msg
	_, cmd := jp.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if cmd != nil {
		gotMsg = cmd()
	}
	km, ok := gotMsg.(JobKillMsg)
	if !ok {
		t.Fatalf("expected JobKillMsg, got %T", gotMsg)
	}
	if km.JobID != "session:sess1" {
		t.Errorf("expected JobID=session:sess1, got %q", km.JobID)
	}
}
