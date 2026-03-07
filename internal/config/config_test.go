package config

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Theme != "dark" {
		t.Errorf("expected theme 'dark', got %q", cfg.Theme)
	}
	if !cfg.Daemon.AutoStart {
		t.Error("expected daemon auto_start to be true")
	}
	if len(cfg.InstructionCompat) == 0 {
		t.Error("expected non-empty instruction_compat")
	}
}

func TestConfigSaveLoad(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := DefaultConfig()
	cfg.DefaultProvider = "anthropic"
	cfg.DefaultModel = "claude-opus-4-6"
	cfg.Theme = "light"

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.DefaultProvider != "anthropic" {
		t.Errorf("got DefaultProvider %q, want %q", loaded.DefaultProvider, "anthropic")
	}
	if loaded.DefaultModel != "claude-opus-4-6" {
		t.Errorf("got DefaultModel %q, want %q", loaded.DefaultModel, "claude-opus-4-6")
	}
	if loaded.Theme != "light" {
		t.Errorf("got Theme %q, want %q", loaded.Theme, "light")
	}
}

func TestLoadMissingFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	// Should return defaults
	if cfg.Theme != "dark" {
		t.Errorf("expected default theme 'dark', got %q", cfg.Theme)
	}
}
