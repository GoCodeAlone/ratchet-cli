package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverInstructionsEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	result := DiscoverInstructions(tmp, []string{"claude"}, "")
	if len(result) != 0 {
		t.Errorf("expected no instructions in empty dir, got %d", len(result))
	}
}

func TestDiscoverClaudeMd(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	content := "# Project Instructions\nDo the thing."
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte(content), 0644)

	result := DiscoverInstructions(tmp, []string{"claude"}, "")
	if len(result) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(result))
	}
	if result[0].Source != "claude" {
		t.Errorf("expected source 'claude', got %q", result[0].Source)
	}
	if result[0].Content != content {
		t.Errorf("content mismatch: got %q, want %q", result[0].Content, content)
	}
}

func TestDiscoverMultipleSources(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("claude instructions"), 0644)
	os.MkdirAll(filepath.Join(tmp, ".github"), 0755)
	os.WriteFile(filepath.Join(tmp, ".github/copilot-instructions.md"), []byte("copilot instructions"), 0644)

	result := DiscoverInstructions(tmp, []string{"claude", "copilot"}, "")
	if len(result) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(result))
	}
}

func TestSourceFiltering(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("claude"), 0644)
	os.WriteFile(filepath.Join(tmp, ".cursorrules"), []byte("cursor"), 0644)

	// Only enable claude
	result := DiscoverInstructions(tmp, []string{"claude"}, "")
	for _, r := range result {
		if r.Source == "cursor" {
			t.Error("expected cursor to be filtered out")
		}
	}
}

func TestProviderFilter(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	ratchetDir := filepath.Join(tmp, ".ratchet")
	os.MkdirAll(ratchetDir, 0700)

	// Global ratchet dir
	homeRatchet := filepath.Join(tmp, ".ratchet")
	os.WriteFile(filepath.Join(homeRatchet, "instructions.anthropic.md"), []byte("anthropic specific"), 0644)

	result := DiscoverInstructions(tmp, []string{}, "anthropic")
	found := false
	for _, r := range result {
		if r.Source == "ratchet" && r.Scope == "global" {
			found = true
		}
	}
	if !found {
		t.Error("expected anthropic-specific global instructions to be discovered")
	}
}

func TestDirectoryBasedInstructions(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	claudeDir := filepath.Join(tmp, ".claude")
	os.MkdirAll(claudeDir, 0755)
	os.WriteFile(filepath.Join(claudeDir, "project.md"), []byte("project context"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "coding.md"), []byte("coding standards"), 0644)

	result := DiscoverInstructions(tmp, []string{"claude"}, "")
	claudeFiles := 0
	for _, r := range result {
		if r.Source == "claude" {
			claudeFiles++
		}
	}
	if claudeFiles != 2 {
		t.Errorf("expected 2 claude directory files, got %d", claudeFiles)
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	instructions := []InstructionSource{
		{Path: "/a.md", Source: "claude", Content: "first"},
		{Path: "/b.md", Source: "copilot", Content: "second"},
	}

	prompt := BuildSystemPrompt(instructions)
	if prompt == "" {
		t.Error("expected non-empty system prompt")
	}
	if len(prompt) < 10 {
		t.Error("prompt seems too short")
	}
}

func TestBuildSystemPromptEmpty(t *testing.T) {
	prompt := BuildSystemPrompt(nil)
	if prompt != "" {
		t.Errorf("expected empty prompt for nil instructions, got %q", prompt)
	}
}
