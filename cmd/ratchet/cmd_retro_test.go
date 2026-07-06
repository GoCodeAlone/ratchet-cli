package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	"github.com/GoCodeAlone/ratchet-cli/internal/retro"
)

func TestHandleRetroAnalyzeTextRoutesFindings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.Retro.Enabled = true
	cfg.Retro.LocalChanges = true
	cfg.Retro.UpstreamInstructions = true
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	evidence := writeRetroEvidenceFixture(t, []retro.Event{
		{Timestamp: time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC), SessionID: "session-a", Kind: retro.EventTestFailure, Message: "go test ./...", Project: retro.ProjectRatchetCLI},
		{Timestamp: time.Date(2026, 7, 4, 10, 1, 0, 0, time.UTC), SessionID: "session-b", Kind: retro.EventPermissionDenied, Command: "bash:deploy", Project: retro.ProjectLocalConfig},
	})
	var stdout bytes.Buffer

	if err := runRetro(context.Background(), []string{"analyze", "--evidence", evidence, "--session", "session-a"}, &stdout); err != nil {
		t.Fatalf("runRetro analyze: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"Retro analysis for session-a",
		"Findings",
		"test failure",
		"go test ./...",
		"Local actions",
		"Record the failing command",
		"Upstream instructions",
		"ratchet-cli PR instruction:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("retro analyze output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "bash:deploy") {
		t.Fatalf("retro analyze did not filter by session:\n%s", out)
	}
}

func TestHandleRetroAnalyzeJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.Retro.Enabled = true
	cfg.Retro.UpstreamInstructions = true
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	evidence := writeRetroEvidenceFixture(t, []retro.Event{
		{Timestamp: time.Date(2026, 7, 4, 11, 0, 0, 0, time.UTC), SessionID: "json-session", Kind: retro.EventError, Message: "provider timeout", Project: retro.ProjectRatchetCLI},
	})
	var stdout bytes.Buffer

	if err := runRetro(context.Background(), []string{"analyze", "--evidence", evidence, "--session", "json-session", "--json"}, &stdout); err != nil {
		t.Fatalf("runRetro analyze json: %v", err)
	}

	var payload retroAnalyzeOutput
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode json %q: %v", stdout.String(), err)
	}
	if payload.SessionID != "json-session" || len(payload.Findings) != 1 {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Findings[0].Pattern != "runtime error" || !strings.Contains(payload.Findings[0].Evidence, "provider timeout") {
		t.Fatalf("finding = %#v", payload.Findings[0])
	}
	if len(payload.UpstreamInstructions) != 1 || !strings.Contains(payload.UpstreamInstructions[0], "ratchet-cli PR instruction:") {
		t.Fatalf("upstream instructions = %#v", payload.UpstreamInstructions)
	}

	var raw map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw json %q: %v", stdout.String(), err)
	}
	findings, ok := raw["findings"].([]any)
	if !ok || len(findings) != 1 {
		t.Fatalf("raw findings = %#v", raw["findings"])
	}
	finding, ok := findings[0].(map[string]any)
	if !ok {
		t.Fatalf("raw finding = %#v", findings[0])
	}
	for _, key := range []string{"pattern", "evidence", "project", "local_action", "upstream_action"} {
		if _, ok := finding[key]; !ok {
			t.Fatalf("raw finding missing snake_case key %q: %#v", key, finding)
		}
	}
	if _, ok := finding["Pattern"]; ok {
		t.Fatalf("raw finding uses exported Go field names: %#v", finding)
	}
}

func TestHandleRetroInstructionsWritesMarkdown(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.Retro.Enabled = true
	cfg.Retro.UpstreamInstructions = true
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	evidence := writeRetroEvidenceFixture(t, []retro.Event{
		{Timestamp: time.Date(2026, 7, 4, 11, 0, 0, 0, time.UTC), SessionID: "handoff-session", Kind: retro.EventTestFailure, Message: "go test ./cmd/ratchet", Project: retro.ProjectRatchetCLI},
	})
	outPath := filepath.Join(t.TempDir(), "instructions.md")
	var stdout bytes.Buffer

	if err := runRetro(context.Background(), []string{"instructions", "--evidence", evidence, "--session", "handoff-session", "--output", outPath}, &stdout); err != nil {
		t.Fatalf("runRetro instructions: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read instructions: %v", err)
	}
	out := string(data)
	for _, want := range []string{"# Ratchet Retro PR Instructions", "handoff-session", "go test ./cmd/ratchet", "ratchet-cli PR instruction:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("retro instructions missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(stdout.String(), "wrote retro instructions") {
		t.Fatalf("stdout missing write confirmation: %q", stdout.String())
	}
}

func TestRetroInstructionsMarkdownNormalizesMultilineItems(t *testing.T) {
	out := renderRetroInstructionsMarkdown(retroAnalyzeOutput{
		SessionID: "multi",
		Findings: []retroAnalyzeFinding{{
			Pattern:  "runtime error",
			Evidence: "first line\nsecond line",
		}},
		UpstreamInstructions: []string{"submit PR\nwith regression"},
		LocalActions:         []string{"rerun\nfocused test"},
	})

	for _, bad := range []string{"- runtime error: first line\nsecond line", "- submit PR\nwith regression", "- rerun\nfocused test"} {
		if strings.Contains(out, bad) {
			t.Fatalf("markdown contains multiline list item %q:\n%s", bad, out)
		}
	}
	for _, want := range []string{"- runtime error: first line second line", "- submit PR with regression", "- rerun focused test"} {
		if !strings.Contains(out, want) {
			t.Fatalf("markdown missing normalized item %q:\n%s", want, out)
		}
	}
}

func TestHandleRetroAnalyzeDisabledConfigSuppressesRoutes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := config.DefaultConfig().Save(); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	evidence := writeRetroEvidenceFixture(t, []retro.Event{
		{Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC), SessionID: "disabled", Kind: retro.EventCommandOutcome, Command: "ratchet publish", Outcome: "failed", Project: retro.ProjectRatchetCLI},
	})
	var stdout bytes.Buffer

	if err := runRetro(context.Background(), []string{"analyze", "--evidence", evidence, "--session", "disabled", "--json"}, &stdout); err != nil {
		t.Fatalf("runRetro analyze disabled: %v", err)
	}

	var payload retroAnalyzeOutput
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode json %q: %v", stdout.String(), err)
	}
	if len(payload.Findings) != 1 {
		t.Fatalf("findings = %#v", payload.Findings)
	}
	if len(payload.LocalActions) != 0 || len(payload.UpstreamInstructions) != 0 {
		t.Fatalf("routes should be suppressed when retro.enabled=false: %#v", payload)
	}
}

func TestHandleRetroAnalyzeMissingEvidence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var stdout bytes.Buffer

	err := runRetro(context.Background(), []string{"analyze", "--evidence", filepath.Join(t.TempDir(), "missing.jsonl")}, &stdout)
	if err == nil || !strings.Contains(err.Error(), "read retro evidence") {
		t.Fatalf("runRetro missing evidence error = %v", err)
	}
}

func TestHandleRetroUsageMentionsInstructions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var stdout bytes.Buffer

	err := runRetro(context.Background(), nil, &stdout)
	if err == nil || !strings.Contains(err.Error(), "<analyze|instructions>") {
		t.Fatalf("runRetro usage error = %v", err)
	}
}

func TestHandleRetroExitsNonzeroOnError(t *testing.T) {
	oldExit := exitProcess
	t.Cleanup(func() { exitProcess = oldExit })
	var gotCode int
	exitProcess = func(code int) {
		gotCode = code
		panic("exit")
	}

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("handleRetro did not exit")
			}
		}()
		handleRetro([]string{"analyze", "--evidence", filepath.Join(t.TempDir(), "missing.jsonl")})
	}()
	if gotCode != 1 {
		t.Fatalf("exit code = %d, want 1", gotCode)
	}
}

func writeRetroEvidenceFixture(t *testing.T, events []retro.Event) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evidence.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create evidence: %v", err)
	}
	enc := json.NewEncoder(f)
	for _, event := range events {
		if err := enc.Encode(event); err != nil {
			_ = f.Close()
			t.Fatalf("write evidence: %v", err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close evidence: %v", err)
	}
	return path
}
