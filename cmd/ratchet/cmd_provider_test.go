package main

import (
	"bufio"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestPromptYesNo_Yes(t *testing.T) {
	for _, input := range []string{"y\n", "Y\n", "yes\n", "YES\n", "\n"} {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.WriteString(input); err != nil {
			t.Fatal(err)
		}
		w.Close()

		scanner := bufio.NewScanner(r)
		got := promptYesNo("test?", scanner)
		r.Close()

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
		if _, err := w.WriteString(input); err != nil {
			t.Fatal(err)
		}
		w.Close()

		scanner := bufio.NewScanner(r)
		got := promptYesNo("test?", scanner)
		r.Close()

		if got {
			t.Errorf("promptYesNo(%q) = true, want false", strings.TrimRight(input, "\n"))
		}
	}
}

func TestPromptYesNo_EOF(t *testing.T) {
	// When stdin is closed (EOF), promptYesNo should default to false.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close() // immediate EOF

	scanner := bufio.NewScanner(r)
	got := promptYesNo("test?", scanner)
	r.Close()

	if got {
		t.Error("promptYesNo on EOF should return false, got true")
	}
}

func TestOllamaInstallCommand_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only test")
	}
	cmd, err := ollamaInstallCommand()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Args[0] != "brew" {
		t.Errorf("expected brew command on darwin, got: %v", cmd.Args)
	}
}

func TestOllamaInstallCommand_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only test")
	}
	cmd, err := ollamaInstallCommand()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Args[0] != "sh" {
		t.Errorf("expected sh command on linux, got: %v", cmd.Args)
	}
	// Verify it downloads to temp file instead of piping to sh
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "mktemp") {
		t.Errorf("expected mktemp-based download, got: %s", joined)
	}
}

func TestOllamaInstallCommand_UnsupportedPlatform(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		t.Skip("only runs on unsupported platforms")
	}
	_, err := ollamaInstallCommand()
	if err == nil {
		t.Error("expected error on unsupported platform")
	}
	if !strings.Contains(err.Error(), "not supported on") {
		t.Errorf("expected 'not supported' error, got: %v", err)
	}
}
