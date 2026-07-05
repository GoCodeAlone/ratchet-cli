package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/routines"
)

func TestExecuteRoutinesAddListShowPauseResumeRemove(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var out bytes.Buffer
	if err := executeRoutines([]string{"add", "--schedule", "15m", "--prompt", "summarize status", "--cwd", "/tmp/project", "--provider", "openai"}, &out); err != nil {
		t.Fatalf("add: %v", err)
	}
	added := out.String()
	if !strings.Contains(added, "Added routine:") {
		t.Fatalf("add output = %q", added)
	}
	id := strings.TrimSpace(strings.TrimPrefix(added, "Added routine:"))

	out.Reset()
	if err := executeRoutines([]string{"list"}, &out); err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := out.String(); !strings.Contains(got, id) || !strings.Contains(got, "15m") || !strings.Contains(got, "active") {
		t.Fatalf("list output = %q", got)
	}

	out.Reset()
	if err := executeRoutines([]string{"show", id}, &out); err != nil {
		t.Fatalf("show: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "summarize status") || !strings.Contains(got, "/tmp/project") {
		t.Fatalf("show output = %q", got)
	}

	if err := executeRoutines([]string{"pause", id}, &out); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if err := executeRoutines([]string{"resume", id}, &out); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if err := executeRoutines([]string{"remove", id}, &out); err != nil {
		t.Fatalf("remove: %v", err)
	}
}

func TestExecuteRoutinesRunRecordsVisibleRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store, err := routines.Load(filepath.Join(home, ".ratchet", "routines", "routines.json"))
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	def, err := store.Add(routines.AddRequest{Schedule: "daily", Prompt: "make plan"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	var out bytes.Buffer
	if err := executeRoutines([]string{"run", def.ID}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "Recorded routine run:") || strings.Contains(strings.ToLower(got), "execut") {
		t.Fatalf("run output = %q", got)
	}

	data, err := os.ReadFile(filepath.Join(home, ".ratchet", "routines", "routines.json"))
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	if !bytes.Contains(data, []byte(`"status": "recorded"`)) {
		t.Fatalf("store did not persist visible run state: %s", data)
	}
}
