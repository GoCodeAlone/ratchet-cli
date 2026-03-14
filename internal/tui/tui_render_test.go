package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/components"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/pages"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

func TestSplashView(t *testing.T) {
	th := theme.Dark()
	splash := pages.NewSplash()

	// Simulate a few ticks
	for i := 0; i < 10; i++ {
		splash, _ = splash.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	}

	view := splash.View(th, 100, 30)
	if view == "" {
		t.Fatal("splash view is empty")
	}
	fmt.Println("=== SPLASH VIEW ===")
	fmt.Println(view)
}

func TestChatViewWithDimensions(t *testing.T) {
	th := theme.Dark()

	// Create chat without a real client — just test rendering
	chat := pages.NewChat(nil, "test-session-id", th, true)
	chat.SetSize(100, 28) // 100 wide, 28 tall (30 - 2 for header)

	view := chat.View(th)
	if view == "" {
		t.Fatal("chat view is empty")
	}
	fmt.Println("=== CHAT VIEW (with SetSize 100x28) ===")
	fmt.Println(view)
	fmt.Printf("View length: %d chars\n", len(view))
}

func TestChatViewWithoutDimensions(t *testing.T) {
	th := theme.Dark()

	// Create chat without setting dimensions — simulates the bug
	chat := pages.NewChat(nil, "test-session-id", th, true)
	// Don't call SetSize — this is what was happening before

	view := chat.View(th)
	fmt.Println("=== CHAT VIEW (NO SetSize — the bug) ===")
	fmt.Println(view)
	fmt.Printf("View length: %d chars\n", len(view))
}

// charKey creates a KeyPressMsg for a printable character.
func charKey(ch rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: ch, Text: string(ch)}
}

func TestInputAcceptsKeystrokes(t *testing.T) {
	th := theme.Dark()
	input := components.NewInput(th)

	if !input.Focused() {
		t.Fatal("textarea must be focused after NewInput")
	}

	// Type "hello" into the textarea
	for _, ch := range "hello" {
		input, _ = input.Update(charKey(ch))
	}

	view := input.View(th, 80)
	if !strings.Contains(view, "hello") {
		t.Fatalf("expected textarea to contain 'hello', got view:\n%s", view)
	}
}

func TestInputSubmit(t *testing.T) {
	th := theme.Dark()
	input := components.NewInput(th)

	// Type "test message"
	for _, ch := range "test message" {
		input, _ = input.Update(charKey(ch))
	}

	// Press Enter to submit
	var cmd tea.Cmd
	_, cmd = input.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected SubmitMsg cmd after Enter")
	}

	msg := cmd()
	submit, ok := msg.(components.SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", msg)
	}
	if submit.Content != "test message" {
		t.Fatalf("expected content 'test message', got %q", submit.Content)
	}
}

func TestChatInputDoesNotScrollViewport(t *testing.T) {
	th := theme.Dark()
	chat := pages.NewChat(nil, "test-session-id", th, true)
	chat.SetSize(100, 28)

	// Type characters that were previously captured by viewport keybindings
	// (f=PageDown, b=PageUp, u=HalfPageUp, d=HalfPageDown, j=Down, k=Up)
	for _, ch := range "fbudjkhl" {
		chat, _ = chat.Update(charKey(ch))
	}

	view := chat.View(th)
	if !strings.Contains(view, "fbudjkhl") {
		t.Fatalf("expected textarea to contain 'fbudjkhl' (viewport should not intercept), got:\n%s", view)
	}
}

func TestAppTransitionToChat(t *testing.T) {
	th := theme.Dark()
	session := &pb.Session{Id: "test-session-12345678"}

	app := NewApp(nil, session, th, true)

	// Simulate initial window size
	model, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	app = model.(App)

	// Simulate splash done + providers exist
	app.splashDone = true
	app.providersReady = true
	app.providers = []*pb.Provider{{Alias: "test", Type: "mock", Model: "mock-model"}}

	// Trigger transition
	model, _ = app.transitionFromSplash()
	app = model.(App)

	if app.page != pageChat {
		t.Fatalf("expected pageChat, got %d", app.page)
	}

	view := app.View()
	content := view.Content
	if content == "" {
		t.Fatal("app view is empty after transitioning to chat")
	}
	fmt.Println("=== APP VIEW (after transition to chat) ===")
	fmt.Println(content)
}
