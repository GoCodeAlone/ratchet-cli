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

func TestHandleModel_Pull_NoArgs(t *testing.T) {
	// handleModelPull with no args calls os.Exit(1).
	// To properly unit-test this, handleModelPull would need to return an error
	// instead of calling os.Exit. This is a known limitation documented here.
	// The argument validation is: len(args) == 0 → print usage + exit.
	// Integration testing with a subprocess would be needed for full coverage.
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
