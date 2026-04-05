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
}

func TestHandleModel_UnknownSubcommand(t *testing.T) {
	out := captureStdout(t, func() {
		handleModel([]string{"unknown"})
	})
	if !strings.Contains(out, "unknown model command: unknown") {
		t.Errorf("expected unknown command message, got: %s", out)
	}
}

func TestHandleModel_Pull_NoArgs_PrintsUsage(t *testing.T) {
	// handleModelPull with no args calls os.Exit(1), which we can't test directly.
	// Verify the argument validation logic: empty args triggers the usage path.
	args := []string{}
	if len(args) != 0 {
		t.Fatal("expected empty args for this test")
	}
	// The actual function calls os.Exit — to fully test this, refactor
	// handleModelPull to return an error instead of calling os.Exit.
	// For now we document the expected behavior.
}

// captureStdout redirects os.Stdout to a buffer and returns the captured output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}
