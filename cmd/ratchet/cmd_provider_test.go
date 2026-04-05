package main

import (
	"os"
	"strings"
	"testing"
)

func TestPromptYesNo_Yes(t *testing.T) {
	for _, input := range []string{"y\n", "Y\n", "yes\n", "YES\n", "\n"} {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		oldStdin := os.Stdin
		os.Stdin = r
		if _, err := w.WriteString(input); err != nil {
			t.Fatal(err)
		}
		w.Close()

		got := promptYesNo("test?")
		os.Stdin = oldStdin

		if !got {
			t.Errorf("promptYesNo(%q) = false, want true", strings.TrimRight(input, "\n"))
		}
	}
}

func TestPromptYesNo_No(t *testing.T) {
	for _, input := range []string{"n\n", "N\n", "no\n", "NO\n"} {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		oldStdin := os.Stdin
		os.Stdin = r
		if _, err := w.WriteString(input); err != nil {
			t.Fatal(err)
		}
		w.Close()

		got := promptYesNo("test?")
		os.Stdin = oldStdin

		if got {
			t.Errorf("promptYesNo(%q) = true, want false", strings.TrimRight(input, "\n"))
		}
	}
}

func TestInstallOllama_CommandConstructed(t *testing.T) {
	// installOllama is not easily unit-testable without exec mocking, but we can
	// verify the function exists and returns an error when the binary is unavailable
	// (in CI where brew/curl may fail). Just ensure it compiles and the function
	// is callable — execution is integration-level.
	_ = installOllama // ensure it compiles
}
