package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type cliSurfaceSpec struct {
	Commands []struct {
		Command  string `json:"command"`
		Evidence string `json:"evidence"`
	} `json:"commands"`
}

func TestCLIHelpSlashSurfaceMatchesCommandSpec(t *testing.T) {
	help := capturePrintUsage(t)
	spec := loadCLISurfaceSpec(t)
	for _, row := range spec.Commands {
		if row.Evidence != "pty-proven" {
			continue
		}
		want := publicHelpSurface(row.Command)
		if !strings.Contains(help, want) {
			t.Fatalf("CLI help missing public surface %q for pty-proven slash command %q", want, row.Command)
		}
	}
	for _, required := range []string{
		"blackboard       Shared daemon blackboard",
		"/mode <mode>",
		"/trust allow \"pattern\" [--scope scope]",
		"/trust persist allow \"pattern\" [--scope scope]",
		"/trust revoke \"pattern\" [--scope scope]",
	} {
		if !strings.Contains(help, required) {
			t.Fatalf("CLI help missing focused slash command %q", required)
		}
	}
}

func publicHelpSurface(command string) string {
	switch {
	case strings.HasPrefix(command, "/mode "):
		return "/mode <mode>"
	case strings.HasPrefix(command, "/trust allow "):
		return "/trust allow \"pattern\" [--scope scope]"
	case strings.HasPrefix(command, "/trust deny "):
		return "/trust deny \"pattern\" [--scope scope]"
	case strings.HasPrefix(command, "/trust persist allow "):
		return "/trust persist allow \"pattern\" [--scope scope]"
	case strings.HasPrefix(command, "/trust persist deny "):
		return "/trust persist deny \"pattern\" [--scope scope]"
	case strings.HasPrefix(command, "/trust revoke "):
		return "/trust revoke \"pattern\" [--scope scope]"
	default:
		return command
	}
}

func capturePrintUsage(t *testing.T) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	wClosed := false
	defer func() {
		os.Stdout = old
		_ = r.Close()
		if !wClosed {
			_ = w.Close()
		}
	}()
	os.Stdout = w
	printUsage()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	wClosed = true
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func loadCLISurfaceSpec(t *testing.T) cliSurfaceSpec {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "internal", "tui", "commands", "testdata", "command_surface_spec.json"))
	if err != nil {
		t.Fatalf("read command surface spec: %v", err)
	}
	var spec cliSurfaceSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse command surface spec: %v", err)
	}
	return spec
}
