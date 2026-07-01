package acpclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultStorePathUsesXDGStateHome(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	store, err := NewDefaultStore()
	if err != nil {
		t.Fatalf("NewDefaultStore: %v", err)
	}

	want := filepath.Join(stateHome, "ratchet", "acp-client", "sessions.json")
	if store.Path() != want {
		t.Fatalf("Path = %q, want %q", store.Path(), want)
	}
}

func TestSessionStoreLoadsMissingFileAndPersistsRecords(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))

	records, err := store.List()
	if err != nil {
		t.Fatalf("List missing store: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("records len = %d, want 0", len(records))
	}

	now := time.Date(2026, 7, 1, 19, 10, 0, 0, time.UTC)
	rec := SessionRecord{
		ID:                 "sess-1",
		Agent:              "fixture",
		CommandFingerprint: "fp",
		Cwd:                "/tmp/project",
		Status:             SessionStatusCompleted,
		CreatedAt:          now,
		UpdatedAt:          now.Add(time.Second),
		LastStopReason:     "end_turn",
		Summary:            "fixture response",
		Turns: []TurnSummary{{
			Prompt:     "hello",
			Response:   "fixture response",
			StopReason: "end_turn",
			CreatedAt:  now,
		}},
	}
	if err := store.Upsert(rec); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	reopened := NewStore(store.Path())
	got, err := reopened.Get("sess-1")
	if err != nil {
		t.Fatalf("Get persisted record: %v", err)
	}
	if got.Agent != rec.Agent || got.CommandFingerprint != rec.CommandFingerprint || got.Summary != rec.Summary {
		t.Fatalf("record = %#v, want persisted metadata", got)
	}
	if len(got.Turns) != 1 || got.Turns[0].Response != "fixture response" {
		t.Fatalf("Turns = %#v", got.Turns)
	}
}

func TestSessionStoreUpsertPreservesCreatedAtWhenUpdateOmitsIt(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	created := time.Date(2026, 7, 1, 19, 10, 0, 0, time.UTC)
	updated := created.Add(time.Hour)

	if err := store.Upsert(SessionRecord{
		ID:        "sess-preserve",
		Status:    SessionStatusRunning,
		CreatedAt: created,
		UpdatedAt: created,
	}); err != nil {
		t.Fatalf("initial Upsert: %v", err)
	}
	if err := store.Upsert(SessionRecord{
		ID:        "sess-preserve",
		Status:    SessionStatusCompleted,
		UpdatedAt: updated,
	}); err != nil {
		t.Fatalf("update Upsert: %v", err)
	}

	got, err := store.Get("sess-preserve")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.CreatedAt.Equal(created) {
		t.Fatalf("CreatedAt = %s, want %s", got.CreatedAt, created)
	}
	if !got.UpdatedAt.Equal(updated) {
		t.Fatalf("UpdatedAt = %s, want %s", got.UpdatedAt, updated)
	}
}

func TestSessionStoreToleratesMissingAndNewFields(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	payload := map[string]any{
		"sessions": []map[string]any{{
			"id":      "legacy",
			"status":  SessionStatusCompleted,
			"unknown": "ignored",
		}},
		"futureField": true,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(store.Path()), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(store.Path(), b, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := store.Get("legacy")
	if err != nil {
		t.Fatalf("Get legacy: %v", err)
	}
	if got.ID != "legacy" || got.Status != SessionStatusCompleted {
		t.Fatalf("record = %#v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps were not defaulted: %#v", got)
	}
}

func TestSessionStorePendingPromptAndOwnerLifecycle(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 19, 20, 0, 0, time.UTC)

	rec := SessionRecord{
		ID:                 "sess-queued",
		Agent:              "custom",
		CommandFingerprint: "fp",
		Cwd:                "/tmp/project",
		Status:             SessionStatusQueued,
		CreatedAt:          now,
		UpdatedAt:          now,
		PendingPrompt: &PendingPrompt{
			ID:        "pending-1",
			Prompt:    "queued prompt",
			Status:    PendingPromptStatusPending,
			CreatedAt: now,
		},
	}
	if err := store.Upsert(rec); err != nil {
		t.Fatalf("Upsert queued: %v", err)
	}
	if err := store.MarkPendingCanceled("sess-queued", now.Add(time.Second)); err != nil {
		t.Fatalf("MarkPendingCanceled: %v", err)
	}
	canceled, err := store.Get("sess-queued")
	if err != nil {
		t.Fatalf("Get canceled: %v", err)
	}
	if canceled.Status != SessionStatusCanceled || canceled.PendingPrompt.Status != PendingPromptStatusCanceled {
		t.Fatalf("canceled record = %#v", canceled)
	}

	owner := OwnerLock{SessionID: "sess-active", PID: 12345, CommandFingerprint: "fp", StartedAt: now}
	if err := store.WriteOwner(owner); err != nil {
		t.Fatalf("WriteOwner: %v", err)
	}
	gotOwner, err := store.Owner("sess-active")
	if err != nil {
		t.Fatalf("Owner: %v", err)
	}
	if gotOwner.PID != owner.PID || gotOwner.CommandFingerprint != owner.CommandFingerprint {
		t.Fatalf("Owner = %#v, want %#v", gotOwner, owner)
	}
	if err := store.RequestCancel("sess-active", now.Add(2*time.Second)); err != nil {
		t.Fatalf("RequestCancel: %v", err)
	}
	cancelReq, err := store.CancelRequest("sess-active")
	if err != nil {
		t.Fatalf("CancelRequest: %v", err)
	}
	if cancelReq.SessionID != "sess-active" || cancelReq.RequestedAt.IsZero() {
		t.Fatalf("CancelRequest = %#v", cancelReq)
	}
	if err := store.ClearOwner("sess-active"); err != nil {
		t.Fatalf("ClearOwner: %v", err)
	}
	if _, err := store.Owner("sess-active"); err == nil {
		t.Fatal("Owner after ClearOwner succeeded, want error")
	}
}
