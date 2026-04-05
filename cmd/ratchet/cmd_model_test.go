package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestHandleModel_NoArgs(t *testing.T) {
	out := captureStdout(t, func() {
		handleModel([]string{})
	})
	if !strings.Contains(out, "Usage: ratchet model") {
		t.Errorf("expected usage message, got: %s", out)
	}
	if !strings.Contains(out, "list") || !strings.Contains(out, "pull") {
		t.Errorf("expected list and pull in usage, got: %s", out)
	}
}

func TestHandleModel_UnknownSubcommand(t *testing.T) {
	out := captureStdout(t, func() {
		handleModel([]string{"unknown"})
	})
	if !strings.Contains(out, "unknown model command: unknown") {
		t.Errorf("expected unknown command message, got: %s", out)
	}
}

func TestHandleModelPull_NoArgs(t *testing.T) {
	err := handleModelPull([]string{})
	if err == nil {
		t.Fatal("expected error for missing model name")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Errorf("expected usage error, got: %v", err)
	}
}

func TestHandleModelPull_HuggingFace_MissingFile(t *testing.T) {
	err := handleModelPull([]string{"--from", "huggingface", "org/repo"})
	if err == nil {
		t.Fatal("expected error for missing file argument")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Errorf("expected usage error, got: %v", err)
	}
}

func TestHandleModelList_NoServer(t *testing.T) {
	// ListModels will fail because Ollama is not running in CI.
	err := handleModelList()
	if err == nil {
		t.Skip("Ollama appears to be running; skipping no-server test")
	}
	if !strings.Contains(err.Error(), "listing models") {
		t.Errorf("expected listing error, got: %v", err)
	}
}

// captureStdout redirects os.Stdout to a buffer and returns the captured output.
// Uses defers to safely restore os.Stdout even if fn panics.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = old
		r.Close()
	}()

	fn()

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}
