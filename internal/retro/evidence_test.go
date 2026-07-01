package retro

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoCodeAlone/workflow/secrets"
)

func TestEvidenceStoreAppendLoadAndRedact(t *testing.T) {
	redactor := secrets.NewRedactor()
	redactor.AddValue("api-key", "sk-test-secret")
	path := filepath.Join(t.TempDir(), "evidence.jsonl")
	store := NewEvidenceStore(path, redactor)

	if err := store.Append(Event{
		SessionID: "s1",
		Kind:      EventError,
		Message:   "provider leaked sk-test-secret",
		Project:   "ratchet-cli",
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := os.WriteFile(path, append([]byte("{bad json}\n"), mustRead(t, path)...), 0600); err != nil {
		t.Fatalf("prepend malformed line: %v", err)
	}

	events, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	got := events[0]
	if got.SessionID != "s1" || got.Kind != EventError || got.Project != "ratchet-cli" {
		t.Fatalf("event = %#v", got)
	}
	if strings.Contains(got.Message, "sk-test-secret") {
		t.Fatalf("message leaked secret: %q", got.Message)
	}
	if !strings.Contains(got.Message, "[REDACTED:api-key]") {
		t.Fatalf("message missing redaction marker: %q", got.Message)
	}
	if got.Timestamp.IsZero() {
		t.Fatal("expected timestamp")
	}
}

func TestEvidenceStoreLoadMissingFile(t *testing.T) {
	store := NewEvidenceStore(filepath.Join(t.TempDir(), "missing.jsonl"), nil)
	events, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %#v, want empty", events)
	}
}

func TestEvidenceStoreLoadSkipsOversizedMalformedLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.jsonl")
	longMalformed := strings.Repeat("x", 70*1024) + "\n"
	valid := `{"kind":"error","message":"kept"}` + "\n"
	if err := os.WriteFile(path, []byte(longMalformed+valid), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	events, err := NewEvidenceStore(path, nil).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(events) != 1 || events[0].Message != "kept" {
		t.Fatalf("events = %#v", events)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return data
}
