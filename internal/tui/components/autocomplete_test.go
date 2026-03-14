package components

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestAutocompleteSetFilter(t *testing.T) {
	ac := NewAutocomplete()

	t.Run("slash_prefix_shows_all_commands", func(t *testing.T) {
		ac = ac.SetFilter("/")
		if !ac.Visible() {
			t.Error("expected autocomplete to be visible when typing /")
		}
		if len(ac.matches) == 0 {
			t.Error("expected matches for /")
		}
	})

	t.Run("partial_prefix_filters", func(t *testing.T) {
		ac = ac.SetFilter("/mo")
		if !ac.Visible() {
			t.Error("expected autocomplete to be visible for /mo")
		}
		if len(ac.matches) != 1 {
			t.Fatalf("expected 1 match for /mo, got %d", len(ac.matches))
		}
		if ac.matches[0].Name != "/model" {
			t.Errorf("expected /model, got %s", ac.matches[0].Name)
		}
	})

	t.Run("no_match_hides", func(t *testing.T) {
		ac = ac.SetFilter("/zzz")
		if ac.Visible() {
			t.Error("expected autocomplete to be hidden for /zzz")
		}
	})

	t.Run("non_slash_input_hides", func(t *testing.T) {
		ac = ac.SetFilter("hello")
		if ac.Visible() {
			t.Error("expected autocomplete to be hidden for non-slash input")
		}
	})

	t.Run("empty_input_hides", func(t *testing.T) {
		ac = ac.SetFilter("")
		if ac.Visible() {
			t.Error("expected autocomplete to be hidden for empty input")
		}
	})

	// This is the exact bug that was caught by the user: after selecting a
	// command via Tab/Enter, the input is set to "/model " (with trailing
	// space). If SetFilter strips whitespace, it would re-match "/model" and
	// keep the dropdown visible.
	t.Run("trailing_space_hides_dropdown", func(t *testing.T) {
		ac = ac.SetFilter("/model ")
		if ac.Visible() {
			t.Error("expected autocomplete to be hidden when input has trailing space (post-selection)")
		}
	})

	t.Run("command_with_args_hides", func(t *testing.T) {
		ac = ac.SetFilter("/provider list")
		if ac.Visible() {
			t.Error("expected autocomplete to be hidden when input contains a space (args)")
		}
	})

	t.Run("case_insensitive_match", func(t *testing.T) {
		ac = ac.SetFilter("/HE")
		if !ac.Visible() {
			t.Error("expected autocomplete to be visible for /HE (case-insensitive)")
		}
		if len(ac.matches) != 1 || ac.matches[0].Name != "/help" {
			t.Errorf("expected /help match, got %v", ac.matches)
		}
	})
}

func TestAutocompleteNavigation(t *testing.T) {
	ac := NewAutocomplete()
	ac = ac.SetFilter("/")

	initialCount := len(ac.matches)
	if initialCount < 2 {
		t.Fatalf("need at least 2 matches for navigation test, got %d", initialCount)
	}

	t.Run("initial_cursor_at_zero", func(t *testing.T) {
		if ac.cursor != 0 {
			t.Errorf("expected cursor at 0, got %d", ac.cursor)
		}
	})

	t.Run("down_moves_cursor", func(t *testing.T) {
		ac, _ = ac.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		if ac.cursor != 1 {
			t.Errorf("expected cursor at 1, got %d", ac.cursor)
		}
	})

	t.Run("up_moves_cursor_back", func(t *testing.T) {
		ac, _ = ac.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // cursor=2
		ac, _ = ac.Update(tea.KeyPressMsg{Code: tea.KeyUp})   // cursor=1
		if ac.cursor != 1 {
			t.Errorf("expected cursor at 1, got %d", ac.cursor)
		}
	})

	t.Run("up_at_zero_stays", func(t *testing.T) {
		ac.cursor = 0
		ac, _ = ac.Update(tea.KeyPressMsg{Code: tea.KeyUp})
		if ac.cursor != 0 {
			t.Errorf("expected cursor to stay at 0, got %d", ac.cursor)
		}
	})

	t.Run("down_at_end_stays", func(t *testing.T) {
		ac.cursor = len(ac.matches) - 1
		ac, _ = ac.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		if ac.cursor != len(ac.matches)-1 {
			t.Errorf("expected cursor to stay at end, got %d", ac.cursor)
		}
	})
}

func TestAutocompleteSelection(t *testing.T) {
	t.Run("tab_selects_and_hides", func(t *testing.T) {
		ac := NewAutocomplete()
		ac = ac.SetFilter("/he")
		if !ac.Visible() {
			t.Fatal("expected visible for /he")
		}

		var cmd tea.Cmd
		ac, cmd = ac.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if ac.Visible() {
			t.Error("expected autocomplete to hide after tab selection")
		}
		if cmd == nil {
			t.Fatal("expected a command from tab selection")
		}

		msg := cmd()
		selected, ok := msg.(AutocompleteSelectedMsg)
		if !ok {
			t.Fatalf("expected AutocompleteSelectedMsg, got %T", msg)
		}
		if selected.Command != "/help" {
			t.Errorf("expected /help, got %s", selected.Command)
		}
	})

	t.Run("enter_selects_and_hides", func(t *testing.T) {
		ac := NewAutocomplete()
		ac = ac.SetFilter("/mo")
		if !ac.Visible() {
			t.Fatal("expected visible for /mo")
		}

		var cmd tea.Cmd
		ac, cmd = ac.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if ac.Visible() {
			t.Error("expected autocomplete to hide after enter selection")
		}
		if cmd == nil {
			t.Fatal("expected a command from enter selection")
		}

		msg := cmd()
		selected, ok := msg.(AutocompleteSelectedMsg)
		if !ok {
			t.Fatalf("expected AutocompleteSelectedMsg, got %T", msg)
		}
		if selected.Command != "/model" {
			t.Errorf("expected /model, got %s", selected.Command)
		}
	})

	t.Run("escape_hides_without_selecting", func(t *testing.T) {
		ac := NewAutocomplete()
		ac = ac.SetFilter("/he")

		var cmd tea.Cmd
		ac, cmd = ac.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if ac.Visible() {
			t.Error("expected autocomplete to hide after escape")
		}
		if cmd != nil {
			t.Error("expected no command from escape")
		}
	})
}

func TestAutocompleteFullSelectionFlow(t *testing.T) {
	// Simulate the exact flow that caused the user-reported bug:
	// 1. User types "/model"
	// 2. Autocomplete shows "/model" as a match
	// 3. User presses Enter to select
	// 4. AutocompleteSelectedMsg fires with Command="/model"
	// 5. Input is set to "/model " (with trailing space)
	// 6. SetFilter("/model ") must hide the dropdown

	ac := NewAutocomplete()

	// Step 1-2: Type "/model"
	ac = ac.SetFilter("/model")
	if !ac.Visible() {
		t.Fatal("expected autocomplete visible for /model")
	}
	if len(ac.matches) != 1 || ac.matches[0].Name != "/model" {
		t.Fatalf("expected exactly /model match, got %v", ac.matches)
	}

	// Step 3: Press Enter
	ac, cmd := ac.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from Enter")
	}
	msg := cmd()
	selected, ok := msg.(AutocompleteSelectedMsg)
	if !ok {
		t.Fatalf("expected AutocompleteSelectedMsg, got %T", msg)
	}
	if selected.Command != "/model" {
		t.Errorf("expected /model, got %s", selected.Command)
	}

	// Step 5-6: Input set to "/model " → SetFilter must hide
	ac = ac.SetFilter(selected.Command + " ")
	if ac.Visible() {
		t.Error("BUG: autocomplete still visible after selection with trailing space — this is the exact bug that was reported by the user")
	}
}

func TestAutocompleteCursorResetOnFilterChange(t *testing.T) {
	ac := NewAutocomplete()
	ac = ac.SetFilter("/")

	// Move cursor down
	ac, _ = ac.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	ac, _ = ac.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	prevCursor := ac.cursor

	// Filter to fewer matches — cursor should clamp
	ac = ac.SetFilter("/ex")
	if ac.cursor >= len(ac.matches) && len(ac.matches) > 0 {
		t.Errorf("cursor %d out of range for %d matches", ac.cursor, len(ac.matches))
	}
	_ = prevCursor
}

func TestAutocompleteNotVisibleWhenNoUpdate(t *testing.T) {
	ac := NewAutocomplete()

	// Without calling SetFilter, should not be visible
	if ac.Visible() {
		t.Error("expected autocomplete to be hidden by default")
	}

	// Update when not visible should be a no-op
	_, cmd := ac.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Error("expected no command when autocomplete is not visible")
	}
}
