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
	var queueDrain policyMatrixRow
	for _, row := range payload.Rows {
		if row.Layer == "" || row.Owner == "" || row.Status == "" || row.Rule == "" {
			t.Fatalf("incomplete row: %#v", row)
		}
		statuses[row.Status] = true
		layers[row.Layer] = true
		if row.Layer == "ACP client queue/drain" {
			queueDrain = row
		}
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
	if queueDrain.Status != "explicit-operator" || !strings.Contains(queueDrain.Rule, "acknowledgement") || !strings.Contains(queueDrain.Rule, "trusted profile") {
		t.Fatalf("ACP client queue/drain row = %#v, want explicit acknowledgement and trusted profile", queueDrain)
	}
	if layers["Background drain"] || !layers["Arbitrary ACP scheduling"] {
		t.Fatalf("policy layers retain deferred background drain or omit arbitrary scheduling: %#v", layers)
	}
}

func TestRunPolicyMatrixStatusFilterText(t *testing.T) {
	var stdout bytes.Buffer
	if err := runPolicy([]string{"matrix", "--status", "deferred"}, &stdout); err != nil {
		t.Fatalf("runPolicy matrix --status deferred: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"Arbitrary ACP scheduling", "Extension SDK", "deferred"} {
		if !strings.Contains(out, want) {
			t.Fatalf("filtered policy matrix missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Static config trust rules") || strings.Contains(out, "Managed hooks") || strings.Contains(out, "supported") {
		t.Fatalf("filtered policy matrix included non-deferred rows:\n%s", out)
	}
}

func TestRunPolicyMatrixReportsManagedHooksSupported(t *testing.T) {
	var stdout bytes.Buffer
	if err := runPolicy([]string{"matrix", "--json"}, &stdout); err != nil {
		t.Fatalf("runPolicy matrix --json: %v", err)
	}
	var payload struct {
		Rows []policyMatrixRow `json:"rows"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode policy matrix json %q: %v", stdout.String(), err)
	}
	for _, row := range payload.Rows {
		if row.Layer != "Managed hooks" {
			continue
		}
		for _, required := range []string{"Fixed-path", "additive", "managed-only", "immutable", "audit", "remote distribution remains deferred"} {
			if !strings.Contains(row.Rule, required) {
				t.Fatalf("managed hooks row = %#v, missing %q", row, required)
			}
		}
		if row.Status != "supported" {
			t.Fatalf("managed hooks row = %#v, want supported enforcement and audit", row)
		}
		return
	}
	t.Fatal("policy matrix omits Managed hooks row")
}

func TestRunPolicyMatrixStatusFilterJSON(t *testing.T) {
	var stdout bytes.Buffer
	if err := runPolicy([]string{"matrix", "--status", "partial", "--json"}, &stdout); err != nil {
		t.Fatalf("runPolicy matrix --status partial --json: %v", err)
	}
	var payload struct {
		Source string            `json:"source"`
		Status string            `json:"status,omitempty"`
		Rows   []policyMatrixRow `json:"rows"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode filtered matrix json %q: %v", stdout.String(), err)
	}
	if payload.Status != "partial" || len(payload.Rows) == 0 {
		t.Fatalf("payload = %#v", payload)
	}
	for _, row := range payload.Rows {
		if row.Status != "partial" {
			t.Fatalf("filtered row has status %q: %#v", row.Status, row)
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
	if err := runPolicy([]string{"matrix", "--status", "unknown"}, &stdout); err == nil {
		t.Fatal("runPolicy accepted unknown status")
	}
}

func TestRunPolicyMatrixHelp(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		var stdout bytes.Buffer
		if err := runPolicy([]string{"matrix", flag}, &stdout); err != nil {
			t.Fatalf("runPolicy matrix %s: %v", flag, err)
		}
		out := stdout.String()
		for _, want := range []string{"Usage: ratchet policy matrix [--status status] [--json]", "--status status", "partial", "explicit-operator"} {
			if !strings.Contains(out, want) {
				t.Fatalf("help for %s missing %q:\n%s", flag, want, out)
			}
		}
	}
}
