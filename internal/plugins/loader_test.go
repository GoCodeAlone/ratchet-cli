package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAllEmpty(t *testing.T) {
	l := NewLoader(t.TempDir())
	plugins, err := l.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestLoadAllMissing(t *testing.T) {
	l := NewLoader(filepath.Join(t.TempDir(), "nonexistent"))
	plugins, err := l.LoadAll()
	if err != nil {
		t.Fatalf("expected no error for missing dir: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestLoadAllExecutable(t *testing.T) {
	dir := t.TempDir()

	// Create an executable file
	execPath := filepath.Join(dir, "my-plugin")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho hi"), 0755); err != nil {
		t.Fatal(err)
	}
	// Create a non-executable file (should be skipped)
	nonExecPath := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(nonExecPath, []byte("docs"), 0644); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(dir)
	plugins, err := l.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 {
		t.Errorf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Name != "my-plugin" {
		t.Errorf("expected name 'my-plugin', got %s", plugins[0].Name)
	}
	if plugins[0].Path != execPath {
		t.Errorf("expected path %s, got %s", execPath, plugins[0].Path)
	}
}
