package commands

import (
	"testing"
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
