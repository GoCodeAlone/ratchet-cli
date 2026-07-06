package commands

import (
	"strings"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestParseNonCommand(t *testing.T) {
	cases := []string{
		"hello world",
		"",
		"   ",
		"not a /command",
	}
	for _, input := range cases {
		result := Parse(input, nil)
		if result != nil {
			t.Errorf("Parse(%q) = non-nil, expected nil for non-command input", input)
		}
	}
}

func TestParseHelp(t *testing.T) {
	result := Parse("/help", nil)
	if result == nil {
		t.Fatal("expected result for /help")
	}
	if len(result.Lines) == 0 {
		t.Error("expected help output lines")
	}
}

func TestParseHelpEndsWithRecoveryCues(t *testing.T) {
	result := Parse("/help", nil)
	if result == nil {
		t.Fatal("expected result for /help")
	}
	tail := strings.Join(result.Lines[max(0, len(result.Lines)-8):], "\n")
	for _, want := range []string{"Ctrl+C", "Esc", "Ctrl+S", "/model", "/provider add"} {
		if !strings.Contains(tail, want) {
			t.Fatalf("help tail missing %q:\n%s", want, tail)
		}
	}
}

func TestParseClear(t *testing.T) {
	result := Parse("/clear", nil)
	if result == nil {
		t.Fatal("expected result for /clear")
	}
	if !result.ClearChat {
		t.Error("expected ClearChat=true for /clear")
	}
}

func TestParseExit(t *testing.T) {
	result := Parse("/exit", nil)
	if result == nil {
		t.Fatal("expected result for /exit")
	}
	if !result.Quit {
		t.Error("expected Quit=true for /exit")
	}
}

func TestParseCost(t *testing.T) {
	result := Parse("/cost", nil)
	if result == nil {
		t.Fatal("expected result for /cost")
	}
	if len(result.Lines) == 0 {
		t.Error("expected cost output")
	}
}

func TestParseUnknownCommand(t *testing.T) {
	result := Parse("/unknown", nil)
	if result == nil {
		t.Fatal("expected result for unknown command")
	}
	if len(result.Lines) == 0 {
		t.Error("expected error message for unknown command")
	}
}

func TestParseCaseInsensitive(t *testing.T) {
	result := Parse("/HELP", nil)
	if result == nil {
		t.Fatal("expected result for /HELP (case-insensitive)")
	}
}

func TestParseWithLeadingWhitespace(t *testing.T) {
	result := Parse("  /help  ", nil)
	if result == nil {
		t.Fatal("expected result for /help with whitespace")
	}
}

func TestParseModelNoClient(t *testing.T) {
	result := Parse("/model", nil)
	if result == nil {
		t.Fatal("expected result for /model")
	}
	if len(result.Lines) == 0 {
		t.Error("expected output for /model without client")
	}
	joined := strings.Join(result.Lines, "\n")
	for _, want := range []string{"/model add", "/provider add", "/model <alias> <model-name>", "ratchet provider add", "ratchet provider list", "ratchet provider test <alias>"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("/model without daemon missing %q:\n%s", want, joined)
		}
	}
}

func TestParseModelAddNavigatesToProviderSetup(t *testing.T) {
	result := Parse("/model add", nil)
	if result == nil {
		t.Fatal("expected result for /model add")
	}
	if !result.NavigateToOnboarding {
		t.Fatal("expected /model add to navigate to provider setup")
	}
	joined := strings.Join(result.Lines, "\n")
	if !strings.Contains(joined, "provider setup") {
		t.Fatalf("/model add output should explain provider setup:\n%s", joined)
	}
}

func TestParseModelOneArgShowsProviderAndModelActions(t *testing.T) {
	result := Parse("/model anthropic", nil)
	if result == nil {
		t.Fatal("expected result for /model <alias>")
	}
	joined := strings.Join(result.Lines, "\n")
	for _, want := range []string{"/model add", "/model <alias> <model-name>", "/provider add", "/provider default"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("/model one-arg help missing %q:\n%s", want, joined)
		}
	}
}

func TestModelProviderLineShowsTypeModelAndDefault(t *testing.T) {
	line := modelProviderLine(&pb.Provider{
		Alias:     "work",
		Type:      "anthropic",
		Model:     "claude-sonnet",
		IsDefault: true,
	})

	for _, want := range []string{"> work", "type=anthropic", "model=claude-sonnet", "default"} {
		if !strings.Contains(line, want) {
			t.Fatalf("provider line missing %q: %s", want, line)
		}
	}
}

func TestParseAgentsNoClient(t *testing.T) {
	result := Parse("/agents", nil)
	if result == nil {
		t.Fatal("expected result for /agents")
	}
	// Should report "Not connected to daemon"
	found := false
	for _, line := range result.Lines {
		if line == "Not connected to daemon" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Not connected to daemon', got %v", result.Lines)
	}
}

func TestParseSessionsNoClient(t *testing.T) {
	result := Parse("/sessions", nil)
	if result == nil {
		t.Fatal("expected result for /sessions")
	}
	joined := strings.Join(result.Lines, "\n")
	for _, want := range []string{"Not connected to daemon", "Ctrl+S", "Enter switch", "d kill", "Ctrl+B", "ratchet sessions browse"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("/sessions help missing %q:\n%s", want, joined)
		}
	}
}

func TestParseTreeRequestsSessionTreeNavigation(t *testing.T) {
	result := Parse("/tree", nil, "root-session-12345678")
	if result == nil {
		t.Fatal("expected result for /tree")
	}
	if !result.OpenSessionTree {
		t.Fatal("expected /tree to request session tree navigation")
	}
	for _, line := range result.Lines {
		if line != "" {
			t.Fatalf("expected /tree to navigate without printing table output, got %v", result.Lines)
		}
	}
}

func TestParseProviderNoSubcommand(t *testing.T) {
	result := Parse("/provider", nil)
	if result == nil {
		t.Fatal("expected result for /provider")
	}
	if len(result.Lines) == 0 {
		t.Error("expected usage message")
	}
}

func TestParseProviderAddNavigates(t *testing.T) {
	result := Parse("/provider add", nil)
	if result == nil {
		t.Fatal("expected result for /provider add")
	}
	if !result.NavigateToOnboarding {
		t.Error("expected NavigateToOnboarding=true for /provider add")
	}
}

func TestParseProviderRemoveNoAlias(t *testing.T) {
	result := Parse("/provider remove", nil)
	if result == nil {
		t.Fatal("expected result")
	}
	if len(result.Lines) == 0 {
		t.Error("expected usage message")
	}
}

func TestParseCompact(t *testing.T) {
	result := Parse("/compact", nil)
	if result == nil {
		t.Fatal("expected result for /compact")
	}
	if len(result.Lines) == 0 {
		t.Error("expected output for /compact")
	}
}

func TestReviewCommand_Parse(t *testing.T) {
	result := Parse("/review", nil)
	if result == nil {
		t.Fatal("expected result for /review")
	}
	if len(result.Lines) == 0 {
		t.Error("expected output for /review")
	}
}

// TestParseAfterAutocompleteSelection tests the exact flow of:
// 1. Autocomplete selects "/model" → input becomes "/model "
// 2. User presses Enter → SubmitMsg with content "/model "
// 3. Parse("/model ") should work as /model command
func TestParseAfterAutocompleteSelection(t *testing.T) {
	result := Parse("/model ", nil)
	if result == nil {
		t.Fatal("expected result for '/model ' (trailing space from autocomplete)")
	}
	// Should not be an unknown command
	for _, line := range result.Lines {
		if line == "Unknown command: /model" {
			t.Error("'/model ' was treated as unknown command")
		}
	}
}
