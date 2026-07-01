package components

import (
	"strings"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

func TestSidebarSetCurrentUpdatesSelectedMarker(t *testing.T) {
	sidebar := NewSidebar([]*pb.Session{
		{Id: "root-session-12345678", Name: "root", Status: "active"},
		{Id: "fork-session-12345678", Name: "fork", Status: "active"},
	}, "root-session-12345678").SetSize(40, 10)

	sidebar = sidebar.SetCurrent("fork-session-12345678")
	view := sidebar.View(theme.Dark())

	if !strings.Contains(view, "● fork") {
		t.Fatalf("sidebar marker did not move to fork:\n%s", view)
	}
	if strings.Contains(view, "● root") {
		t.Fatalf("sidebar still marks root after selecting fork:\n%s", view)
	}
}
