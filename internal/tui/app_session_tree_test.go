package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/components"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/pages"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

func TestAppCtrlBOpensSessionTreeBrowser(t *testing.T) {
	app := readyChatApp(t, "root-session-12345678")

	model, _ := app.Update(ctrlKey('b'))
	app = model.(App)

	if app.page != pageSessionTree {
		t.Fatalf("page = %v, want pageSessionTree", app.page)
	}
	if !strings.Contains(app.View().Content, "Session Tree") {
		t.Fatalf("view missing session tree:\n%s", app.View().Content)
	}
}

func TestAppTreeSelectionSwitchesChatAndLoadsHistory(t *testing.T) {
	app := readyChatApp(t, "root-session-12345678")

	model, _ := app.Update(components.SessionTreeSelectedMsg{
		SessionID: "fork-session-12345678",
		Session: &pb.Session{
			Id:         "fork-session-12345678",
			WorkingDir: "/tmp/fork-workdir",
		},
	})
	app = model.(App)

	if app.page != pageChat {
		t.Fatalf("page = %v, want pageChat", app.page)
	}
	if app.sessionID != "fork-session-12345678" {
		t.Fatalf("sessionID = %q, want fork-session-12345678", app.sessionID)
	}
	if app.chat.SessionID() != "fork-session-12345678" {
		t.Fatalf("chat sessionID = %q, want fork-session-12345678", app.chat.SessionID())
	}

	model, _ = app.Update(sessionHistoryLoadedMsg{
		sessionID: "fork-session-12345678",
		messages: []*pb.HistoryMessage{
			{Role: "user", Content: "fork prompt"},
			{Role: "assistant", Content: "fork answer"},
		},
	})
	app = model.(App)

	view := app.View().Content
	for _, want := range []string{"fork prompt", "answer", "/tmp/fork-workdir"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q after history load:\n%s", want, view)
		}
	}
}

func TestAppBlocksSubmitWhileSessionHistoryLoads(t *testing.T) {
	app := readyChatApp(t, "root-session-12345678")
	model, _ := app.Update(components.SessionTreeSelectedMsg{
		SessionID: "fork-session-12345678",
		Session:   &pb.Session{Id: "fork-session-12345678"},
	})
	app = model.(App)
	app.loading = true

	model, _ = app.Update(components.SubmitMsg{Content: "send before history"})
	app = model.(App)

	if strings.Contains(app.View().Content, "send before history") {
		t.Fatalf("submit reached chat while history was still loading:\n%s", app.View().Content)
	}
}

func TestAppShowsSessionHistoryLoadErrors(t *testing.T) {
	app := readyChatApp(t, "root-session-12345678")
	model, _ := app.Update(components.SessionTreeSelectedMsg{
		SessionID: "fork-session-12345678",
		Session:   &pb.Session{Id: "fork-session-12345678"},
	})
	app = model.(App)

	model, _ = app.Update(sessionHistoryLoadedMsg{
		sessionID: "fork-session-12345678",
		err:       assertErr("history unavailable"),
	})
	app = model.(App)

	if !strings.Contains(app.View().Content, "Could not load session history: history unavailable") {
		t.Fatalf("view missing history load error:\n%s", app.View().Content)
	}
	if app.loading {
		t.Fatal("history load error did not clear loading state")
	}
}

func TestAppDropsStaleChatEventsAfterSessionSwitch(t *testing.T) {
	app := readyChatApp(t, "root-session-12345678")
	model, _ := app.Update(components.SessionTreeSelectedMsg{
		SessionID: "fork-session-12345678",
		Session:   &pb.Session{Id: "fork-session-12345678"},
	})
	app = model.(App)

	model, _ = app.Update(pages.ChatEventMsg{
		SessionID: "root-session-12345678",
		Event: &pb.ChatEvent{Event: &pb.ChatEvent_Token{
			Token: &pb.TokenDelta{Content: "stale root token"},
		}},
	})
	app = model.(App)

	if strings.Contains(app.View().Content, "stale root token") {
		t.Fatalf("stale root event reached switched chat:\n%s", app.View().Content)
	}
}

func TestAppEscClosesSessionTreeWithoutChangingSession(t *testing.T) {
	app := readyChatApp(t, "root-session-12345678")
	model, _ := app.Update(ctrlKey('b'))
	app = model.(App)

	model, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	app = model.(App)

	if app.page != pageChat {
		t.Fatalf("page = %v, want pageChat", app.page)
	}
	if app.sessionID != "root-session-12345678" {
		t.Fatalf("sessionID = %q, want root-session-12345678", app.sessionID)
	}
}

func TestAppOnboardingCancelReturnsExistingProviderToChat(t *testing.T) {
	app := readyChatApp(t, "root-session-12345678")
	app.page = pageOnboarding
	app.providers = []*pb.Provider{{Alias: "existing", Type: "anthropic"}}

	model, _ := app.Update(pages.OnboardingCancelledMsg{})
	app = model.(App)
	if app.page != pageChat {
		t.Fatalf("page = %v, want pageChat", app.page)
	}
}

func TestAppOnboardingCancelWithoutProviderQuits(t *testing.T) {
	app := readyChatApp(t, "root-session-12345678")
	app.page = pageOnboarding
	app.providers = nil

	_, cmd := app.Update(pages.OnboardingCancelledMsg{})
	if cmd == nil || cmd() != (tea.QuitMsg{}) {
		t.Fatalf("cancel command = %v", cmd)
	}
}

func TestAppOnboardingCancelRetainsProviderSavedByWizard(t *testing.T) {
	app := readyChatApp(t, "root-session-12345678")
	app.page = pageOnboarding
	app.providers = nil
	provider := &pb.Provider{Alias: "configured", Type: "anthropic", BaseUrl: "https://example.invalid", IsDefault: true}

	model, cmd := app.Update(pages.OnboardingCancelledMsg{Provider: provider})
	app = model.(App)
	if cmd == nil || app.page != pageChat || len(app.providers) != 1 || app.providers[0] != provider {
		t.Fatalf("saved cancel state = cmd:%v page:%v providers:%v", cmd, app.page, app.providers)
	}
}

func TestAppOnboardingDoneUpdatesProviderStateForLaterCancellation(t *testing.T) {
	app := readyChatApp(t, "root-session-12345678")
	app.page = pageOnboarding
	app.providers = []*pb.Provider{{Alias: "old", Type: "openai", IsDefault: true}}
	provider := &pb.Provider{Alias: "configured", Type: "anthropic", IsDefault: true}

	model, _ := app.Update(pages.OnboardingDoneMsg{Provider: provider})
	app = model.(App)
	if app.page != pageChat || len(app.providers) != 2 || app.providers[0].GetIsDefault() || app.providers[1].GetAlias() != "configured" {
		t.Fatalf("onboarding success state = page:%v providers:%v", app.page, app.providers)
	}

	app.page = pageOnboarding
	model, _ = app.Update(pages.OnboardingCancelledMsg{})
	app = model.(App)
	if app.page != pageChat {
		t.Fatalf("post-success cancel page = %v, want pageChat", app.page)
	}
}

func TestAppOnboardingCommittedProviderRouting(t *testing.T) {
	app := readyChatApp(t, "root-session-12345678")
	app.page = pageOnboarding
	app.providers = []*pb.Provider{{Alias: "old", Type: "openai", IsDefault: true}}
	provider := &pb.Provider{Alias: "configured", Type: "anthropic", Model: "claude-test", IsDefault: true}
	operationID := "f1f183c5-b277-4c27-bc3d-50cdd20f7ed1"

	model, _ := app.Update(pages.ProviderSaveResultMsg{
		OperationID: operationID,
		Unresolved:  true,
		Err:         assertErr("operation unresolved"),
	})
	app = model.(App)
	model, cmd := app.Update(ctrlKey('c'))
	app = model.(App)
	if cmd != nil {
		t.Fatalf("Ctrl+C returned quit while provider operation was unresolved: %v", cmd())
	}

	model, cmd = app.Update(pages.ProviderSaveResultMsg{Provider: provider, OperationID: operationID})
	app = model.(App)
	msg := runAppCmd(t, cmd)
	if _, ok := msg.(pages.OnboardingQuitMsg); !ok {
		t.Fatalf("resolved provider command = %T", msg)
	}
	model, cmd = app.Update(msg)
	app = model.(App)
	if cmd == nil || cmd() != (tea.QuitMsg{}) {
		t.Fatalf("committed provider quit command = %v", cmd)
	}
	if len(app.providers) != 2 || app.providers[0].GetIsDefault() || app.providers[1] != provider {
		t.Fatalf("committed provider routing = %v", app.providers)
	}
}

func runAppCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected app command")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return msg
	}
	for _, child := range batch {
		if child != nil {
			return runAppCmd(t, child)
		}
	}
	t.Fatal("app command batch produced no message")
	return nil
}

func TestAppRecordProviderRetainsDefaultForExistingPointer(t *testing.T) {
	configured := &pb.Provider{Alias: "configured", Type: "anthropic", IsDefault: true}
	app := readyChatApp(t, "root-session-12345678")
	app.providers = []*pb.Provider{nil, configured}

	app.recordProvider(configured)

	if !configured.GetIsDefault() || app.providers[1] != configured {
		t.Fatalf("recorded provider = default:%v providers:%v", configured.GetIsDefault(), app.providers)
	}
}

func TestAppEscClosesJobPanel(t *testing.T) {
	app := readyChatApp(t, "root-session-12345678")
	model, _ := app.Update(ctrlKey('j'))
	app = model.(App)
	if !app.showJobs {
		t.Fatal("ctrl+j did not open job panel")
	}

	model, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	app = model.(App)
	if app.showJobs {
		t.Fatal("esc did not close job panel")
	}
	if app.page != pageChat {
		t.Fatalf("page = %v, want pageChat", app.page)
	}
}

func TestAppEscClosesSidebar(t *testing.T) {
	app := readyChatApp(t, "root-session-12345678")
	model, _ := app.Update(ctrlKey('s'))
	app = model.(App)
	if !app.showSidebar {
		t.Fatal("ctrl+s did not open sidebar")
	}

	model, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	app = model.(App)
	if app.showSidebar {
		t.Fatal("esc did not close sidebar")
	}
	if app.page != pageChat {
		t.Fatalf("page = %v, want pageChat", app.page)
	}
}

func TestAppEscClosesTeamPanel(t *testing.T) {
	app := readyChatApp(t, "root-session-12345678")
	model, _ := app.Update(ctrlKey('t'))
	app = model.(App)
	if !app.showTeam {
		t.Fatal("ctrl+t did not open team panel")
	}

	model, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	app = model.(App)
	if app.showTeam {
		t.Fatal("esc did not close team panel")
	}
	if app.page != pageChat {
		t.Fatalf("page = %v, want pageChat", app.page)
	}
}

func TestAppSidebarKeepsShortcutHintsVisible(t *testing.T) {
	app := readyChatApp(t, "root-session-12345678")
	model, _ := app.Update(tea.WindowSizeMsg{Width: 72, Height: 24})
	app = model.(App)

	model, _ = app.Update(ctrlKey('s'))
	app = model.(App)

	if width, _ := app.chatLayoutSize(); width != 47 {
		t.Fatalf("chat width with sidebar = %d, want 47", width)
	}
	view := app.View().Content
	for _, want := range []string{"Sessions", "Ctrl+S close", "Ctrl+C quit", "Ctrl+B tree"} {
		if !strings.Contains(view, want) {
			t.Fatalf("sidebar view missing %q:\n%s", want, view)
		}
	}

	model, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	app = model.(App)
	if width, _ := app.chatLayoutSize(); width != 72 {
		t.Fatalf("chat width after closing sidebar = %d, want 72", width)
	}
}

func TestAppSidebarSelectionRebuildsChatAndUpdatesMarker(t *testing.T) {
	app := readyChatApp(t, "root-session-12345678")
	app.sidebar = components.NewSidebar([]*pb.Session{
		{Id: "root-session-12345678", Name: "root", Status: "active"},
		{Id: "fork-session-12345678", Name: "fork", Status: "active", WorkingDir: "/tmp/fork-workdir"},
	}, "root-session-12345678")
	app.showSidebar = true

	model, _ := app.Update(components.SessionSelectedMsg{
		SessionID: "fork-session-12345678",
		Session:   &pb.Session{Id: "fork-session-12345678", WorkingDir: "/tmp/fork-workdir"},
	})
	app = model.(App)

	if app.sessionID != "fork-session-12345678" {
		t.Fatalf("sessionID = %q, want fork-session-12345678", app.sessionID)
	}
	if app.chat.SessionID() != "fork-session-12345678" {
		t.Fatalf("chat sessionID = %q, want fork-session-12345678", app.chat.SessionID())
	}

	app.showSidebar = true
	view := app.View().Content
	if !strings.Contains(view, "● fork") {
		t.Fatalf("sidebar marker did not move to selected session:\n%s", view)
	}
	if !strings.Contains(view, "/tmp/fork-workdir") {
		t.Fatalf("selected session metadata was not applied:\n%s", view)
	}
}

func readyChatApp(t *testing.T, sessionID string) App {
	t.Helper()
	th := theme.Dark()
	app := NewApp(nil, &pb.Session{Id: sessionID, WorkingDir: "/tmp/root-workdir"}, th, true)
	model, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	app = model.(App)
	app.splashDone = true
	app.providersReady = true
	app.providers = []*pb.Provider{{Alias: "test", Type: "mock", Model: "mock-model", IsDefault: true}}
	model, _ = app.transitionFromSplash()
	app = model.(App)
	if app.page != pageChat {
		t.Fatalf("page = %v, want pageChat", app.page)
	}
	return app
}

func ctrlKey(ch rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: ch, Mod: tea.ModCtrl}
}

type assertErr string

func (e assertErr) Error() string {
	return string(e)
}
