package acpclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompareRunStoreWritesBundleAndAgentEvents(t *testing.T) {
	root := t.TempDir()
	store := NewCompareRunStore(root)
	at := time.Date(2026, 7, 3, 9, 35, 0, 0, time.UTC)
	events := []EventLogLine{{
		Seq:       1,
		At:        at,
		Direction: EventDirectionOutbound,
		Message:   json.RawMessage(`{"jsonrpc":"2.0","id":"prompt-1","method":"session/prompt","params":{"sessionId":"s"}}`),
	}}

	bundle, err := store.Save(CompareRun{
		RunID:        "fixed-run",
		PromptDigest: "sha256:abc123",
		StartedAt:    at,
		FinishedAt:   at.Add(time.Second),
		Status:       "completed",
		Rows: []CompareRow{{
			Agent:      "agent/one",
			Status:     "ok",
			WallMS:     10,
			StopReason: "end_turn",
			Final:      "done",
			Events:     events,
		}, {
			Agent:      "agent-two",
			Status:     "ok",
			WallMS:     12,
			StopReason: "end_turn",
			Final:      "done",
		}},
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if bundle.RunID != "fixed-run" || bundle.RunDir != filepath.Join(root, "fixed-run") {
		t.Fatalf("bundle = %#v", bundle)
	}
	comparePath := filepath.Join(bundle.RunDir, "compare.json")
	b, err := os.ReadFile(comparePath)
	if err != nil {
		t.Fatalf("read compare.json: %v", err)
	}
	var payload struct {
		RunID        string       `json:"run_id"`
		RunDir       string       `json:"run_dir"`
		Status       string       `json:"status"`
		PromptDigest string       `json:"prompt_digest"`
		Rows         []CompareRow `json:"rows"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("compare.json: %v\n%s", err, b)
	}
	if payload.RunID != "fixed-run" || payload.Status != "completed" || payload.PromptDigest != "sha256:abc123" || len(payload.Rows) != 2 {
		t.Fatalf("payload = %#v", payload)
	}
	eventPath := filepath.Join(bundle.RunDir, "agents", bundle.AgentDirs["agent/one"], "events.ndjson")
	eventBytes, err := os.ReadFile(eventPath)
	if err != nil {
		t.Fatalf("read events.ndjson: %v", err)
	}
	if !strings.Contains(string(eventBytes), `"session/prompt"`) {
		t.Fatalf("events.ndjson = %s", eventBytes)
	}
	if strings.Contains(bundle.AgentDirs["agent/one"], "/") || strings.Contains(bundle.AgentDirs["agent/one"], `\`) {
		t.Fatalf("agent dir was not escaped: %q", bundle.AgentDirs["agent/one"])
	}
	emptyEventPath := filepath.Join(bundle.RunDir, "agents", bundle.AgentDirs["agent-two"], "events.ndjson")
	emptyBytes, err := os.ReadFile(emptyEventPath)
	if err != nil {
		t.Fatalf("read empty events.ndjson: %v", err)
	}
	if len(emptyBytes) != 0 {
		t.Fatalf("empty events.ndjson = %s, want empty file", emptyBytes)
	}
}

func TestCompareRunIDIncludesSubsecondPrecision(t *testing.T) {
	at := time.Date(2026, 7, 3, 9, 45, 0, 0, time.UTC)
	first := newCompareRunID(at)
	second := newCompareRunID(at.Add(time.Nanosecond))
	if first == second {
		t.Fatalf("run ids collided within one second: %q", first)
	}
}
