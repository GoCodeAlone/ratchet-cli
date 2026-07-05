package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteWorkflowsInstallListShowRunStopResume(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	file := filepath.Join(home, "workflow.yaml")
	if err := os.WriteFile(file, []byte(`
name: daily-plan
description: prepare a daily plan
nodes:
  - id: start
    type: prompt
    prompt: make a plan
edges: []
`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := executeWorkflows([]string{"install", file}, &out); err != nil {
		t.Fatalf("install: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "Installed workflow: daily-plan") {
		t.Fatalf("install output = %q", got)
	}

	out.Reset()
	if err := executeWorkflows([]string{"list"}, &out); err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "daily-plan") || !strings.Contains(got, "prepare a daily plan") {
		t.Fatalf("list output = %q", got)
	}

	out.Reset()
	if err := executeWorkflows([]string{"show", "daily-plan"}, &out); err != nil {
		t.Fatalf("show: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "nodes: 1") || !strings.Contains(got, "edges: 0") {
		t.Fatalf("show output = %q", got)
	}

	out.Reset()
	if err := executeWorkflows([]string{"run", "daily-plan"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "Recorded workflow run:") || strings.Contains(strings.ToLower(got), "execut") {
		t.Fatalf("run output = %q", got)
	}
	runID := strings.TrimSpace(strings.TrimPrefix(out.String(), "Recorded workflow run:"))

	out.Reset()
	if err := executeWorkflows([]string{"stop", runID}, &out); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if !strings.Contains(out.String(), "Stopped workflow run:") {
		t.Fatalf("stop output = %q", out.String())
	}
	out.Reset()
	if err := executeWorkflows([]string{"resume", runID}, &out); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !strings.Contains(out.String(), "Resumed workflow run:") {
		t.Fatalf("resume output = %q", out.String())
	}
}

func TestExecuteWorkflowsRunInstallsPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	file := filepath.Join(home, "workflow.json")
	if err := os.WriteFile(file, []byte(`{"name":"path-run","nodes":[{"id":"start","type":"prompt","prompt":"plan"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := executeWorkflows([]string{"run", file}, &out); err != nil {
		t.Fatalf("run path: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "Recorded workflow run:") {
		t.Fatalf("run path output = %q", got)
	}
}
