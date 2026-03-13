package components

import (
	"strings"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

func TestFleetPanel_Empty(t *testing.T) {
	fp := NewFleetPanel().SetSize(80, 24)
	view := fp.View(theme.Dark())
	if !strings.Contains(view, "No fleet workers") {
		t.Errorf("expected 'No fleet workers' in empty view, got: %s", view)
	}
}

func TestFleetPanel_SetFleetStatus(t *testing.T) {
	fp := NewFleetPanel().SetSize(80, 24)
	fs := &pb.FleetStatus{
		FleetId:   "fleet-1",
		SessionId: "sess-1",
		Workers: []*pb.FleetWorker{
			{Id: "w1", Name: "worker-1", StepId: "step-a", Status: "running", Model: "gpt-4"},
			{Id: "w2", Name: "worker-2", StepId: "step-b", Status: "completed", Model: "claude"},
			{Id: "w3", Name: "worker-3", StepId: "step-c", Status: "failed", Error: "timeout"},
		},
		Status:    "running",
		Total:     3,
		Completed: 1,
	}
	fp = fp.SetFleetStatus(fs)
	view := fp.View(theme.Dark())

	for _, name := range []string{"worker-1", "worker-2", "worker-3"} {
		if !strings.Contains(view, name) {
			t.Errorf("expected worker name %q in view", name)
		}
	}
	if strings.Contains(view, "No fleet workers") {
		t.Error("should not show 'No fleet workers' when workers exist")
	}
}

func TestFleetPanel_StatusIcons(t *testing.T) {
	tests := []struct {
		status string
		icon   string
	}{
		{"running", "⠋"},
		{"completed", "✓"},
		{"failed", "✗"},
		{"pending", "·"},
	}
	for _, tt := range tests {
		got := statusIcon(tt.status)
		if got != tt.icon {
			t.Errorf("statusIcon(%q) = %q, want %q", tt.status, got, tt.icon)
		}
	}
}

func TestFleetPanel_Truncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate short: got %q", got)
	}
	if got := truncate("hello world long name", 10); len([]rune(got)) != 10 {
		t.Errorf("truncate long: rune len=%d, got %q", len([]rune(got)), got)
	}
}
