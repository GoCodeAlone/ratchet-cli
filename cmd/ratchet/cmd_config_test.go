package main

import (
	"strings"
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
)

func TestHandleConfigShowIncludesRetroSettings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.Retro.Enabled = true
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	out := captureStdout(t, func() {
		handleConfig([]string{"show"})
	})
	for _, want := range []string{
		"RetroEnabled:   true",
		"RetroLocal:     false",
		"RetroUpstream:  true",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("config show missing %q:\n%s", want, out)
		}
	}
}
