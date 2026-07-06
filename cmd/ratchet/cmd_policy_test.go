package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunPolicyMatrixText(t *testing.T) {
	var stdout bytes.Buffer
	if err := runPolicy([]string{"matrix"}, &stdout); err != nil {
		t.Fatalf("runPolicy matrix: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"Ratchet policy matrix",
		"Static config trust rules",
		"Runtime trust rules",
		"Persistent trust grants",
		"ACP client queue/drain",
		"supported",
		"partial",
		"explicit-operator",
		"deferred",
		"docs/policy-matrix.md",
	} {
		if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
			t.Fatalf("policy matrix text missing %q:\n%s", want, out)
		}
	}
}

func TestRunPolicyMatrixJSON(t *testing.T) {
	var stdout bytes.Buffer
	if err := runPolicy([]string{"matrix", "--json"}, &stdout); err != nil {
		t.Fatalf("runPolicy matrix --json: %v", err)
	}
	var payload struct {
		Source string            `json:"source"`
		Rows   []policyMatrixRow `json:"rows"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode policy matrix json %q: %v", stdout.String(), err)
	}
	if payload.Source != "docs/policy-matrix.md" {
		t.Fatalf("source = %q, want docs/policy-matrix.md", payload.Source)
	}
	if len(payload.Rows) < 8 {
		t.Fatalf("rows = %d, want at least 8", len(payload.Rows))
	}
	statuses := map[string]bool{}
	layers := map[string]bool{}
	for _, row := range payload.Rows {
		if row.Layer == "" || row.Owner == "" || row.Status == "" || row.Rule == "" {
			t.Fatalf("incomplete row: %#v", row)
		}
		statuses[row.Status] = true
		layers[row.Layer] = true
	}
	for _, want := range []string{"supported", "partial", "explicit-operator", "deferred"} {
		if !statuses[want] {
			t.Fatalf("json statuses missing %q: %#v", want, statuses)
		}
	}
	for _, want := range []string{"Static config trust rules", "Persistent trust grants", "Release artifact gates"} {
		if !layers[want] {
			t.Fatalf("json layers missing %q", want)
		}
	}
}

func TestRunPolicyMatrixRejectsUnknownArgs(t *testing.T) {
	var stdout bytes.Buffer
	if err := runPolicy([]string{"matrix", "--format", "yaml"}, &stdout); err == nil {
		t.Fatal("runPolicy accepted unknown flag")
	}
	if err := runPolicy([]string{"unknown"}, &stdout); err == nil {
		t.Fatal("runPolicy accepted unknown subcommand")
	}
}
