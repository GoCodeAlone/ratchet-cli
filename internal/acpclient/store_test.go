package acpclient

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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
	if len(canceled.PromptQueue) != 1 || canceled.PromptQueue[0].Status != QueuePromptStatusCanceled {
		t.Fatalf("migrated queued prompt after legacy cancel = %#v, want canceled", canceled.PromptQueue)
	}
	if next, ok, err := store.NextQueuedPrompt("sess-queued"); err != nil || ok {
		t.Fatalf("NextQueuedPrompt after legacy cancel = %#v, %v, %v; want none", next, ok, err)
	}

	owner := OwnerLock{SessionID: "sess-active", PID: 12345, CommandFingerprint: "fp", StartedAt: now}
	lease, err := store.AcquireOwnerLease(owner)
	if err != nil {
		t.Fatalf("AcquireOwnerLease: %v", err)
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
	if err := lease.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := store.Owner("sess-active"); err == nil {
		t.Fatal("Owner after ClearOwner succeeded, want error")
	}
}

func TestSessionStoreOwnerLeaseDoesNotOverwriteExistingOwner(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 20, 0, 0, 0, time.UTC)
	original := OwnerLock{SessionID: "sess-lock", PID: 111, CommandFingerprint: "first", StartedAt: now}
	lease, err := store.AcquireOwnerLease(original)
	if err != nil {
		t.Fatalf("AcquireOwnerLease original: %v", err)
	}
	defer func() { _ = lease.Release() }()

	other, err := store.AcquireOwnerLease(OwnerLock{
		SessionID:          "sess-lock",
		PID:                222,
		CommandFingerprint: "second",
		StartedAt:          now.Add(time.Second),
	})
	if other != nil {
		_ = other.Release()
	}
	if !errors.Is(err, ErrOwnerLeaseBusy) {
		t.Fatalf("AcquireOwnerLease error = %v, want ErrOwnerLeaseBusy", err)
	}
	got, err := store.Owner("sess-lock")
	if err != nil {
		t.Fatalf("Owner: %v", err)
	}
	if got.PID != original.PID || got.CommandFingerprint != original.CommandFingerprint {
		t.Fatalf("owner overwritten = %#v, want %#v", got, original)
	}
}

func TestSessionStoreMigratesLegacyPendingPromptToQueue(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	created := time.Date(2026, 7, 1, 20, 45, 0, 0, time.UTC)
	updated := created.Add(5 * time.Minute)
	payload := storeFile{Sessions: []SessionRecord{{
		ID:                 "legacy-queued",
		Agent:              "custom",
		CommandFingerprint: "fp",
		Cwd:                "/tmp/project",
		Status:             SessionStatusQueued,
		CreatedAt:          created,
		UpdatedAt:          updated,
		Turns: []TurnSummary{{
			Prompt:     "old prompt",
			Response:   "old response",
			StopReason: "end_turn",
			CreatedAt:  created,
		}},
		PendingPrompt: &PendingPrompt{
			ID:        "pending-legacy",
			Prompt:    "queued prompt",
			Status:    PendingPromptStatusPending,
			CreatedAt: created.Add(time.Minute),
		},
	}}}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(store.Path()), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(store.Path(), b, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := store.Get("legacy-queued")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ACPSessionID != "" {
		t.Fatalf("ACPSessionID = %q, want empty for legacy queued record", got.ACPSessionID)
	}
	if !got.CreatedAt.Equal(created) || !got.UpdatedAt.Equal(updated) {
		t.Fatalf("timestamps = %s/%s, want %s/%s", got.CreatedAt, got.UpdatedAt, created, updated)
	}
	if len(got.Turns) != 1 || got.Turns[0].Prompt != "old prompt" {
		t.Fatalf("Turns = %#v, want preserved legacy turns", got.Turns)
	}
	if len(got.PromptQueue) != 1 {
		t.Fatalf("PromptQueue len = %d, want 1: %#v", len(got.PromptQueue), got.PromptQueue)
	}
	queued := got.PromptQueue[0]
	if queued.ID != "pending-legacy" || queued.Prompt != "queued prompt" || queued.Status != QueuePromptStatusPending {
		t.Fatalf("queued prompt = %#v", queued)
	}
	if !queued.CreatedAt.Equal(created.Add(time.Minute)) {
		t.Fatalf("queued CreatedAt = %s", queued.CreatedAt)
	}

	if err := store.Upsert(got); err != nil {
		t.Fatalf("Upsert migrated record: %v", err)
	}
	reloaded, err := store.Get("legacy-queued")
	if err != nil {
		t.Fatalf("Get reloaded: %v", err)
	}
	if len(reloaded.PromptQueue) != 1 {
		t.Fatalf("PromptQueue after upsert len = %d, want idempotent migration", len(reloaded.PromptQueue))
	}
}

func TestSessionStoreQueueOperationsPreserveFIFOAndTurns(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	created := time.Date(2026, 7, 1, 21, 0, 0, 0, time.UTC)
	second := created.Add(time.Minute)

	first, err := store.AppendQueuedPrompt(SessionRecord{
		ID:                 "fifo-session",
		ACPSessionID:       "acp-session",
		Agent:              "fixture",
		CommandFingerprint: "fp",
		Cwd:                "/tmp/project",
	}, QueuedPrompt{ID: "q-1", Prompt: "first", CreatedAt: created})
	if err != nil {
		t.Fatalf("AppendQueuedPrompt first: %v", err)
	}
	if first.Status != SessionStatusQueued || len(first.PromptQueue) != 1 {
		t.Fatalf("first append record = %#v", first)
	}
	if first.PromptQueue[0].Status != QueuePromptStatusPending {
		t.Fatalf("first status = %q", first.PromptQueue[0].Status)
	}

	if _, err := store.AppendQueuedPrompt(SessionRecord{ID: "fifo-session"}, QueuedPrompt{ID: "q-2", Prompt: "second", CreatedAt: second}); err != nil {
		t.Fatalf("AppendQueuedPrompt second: %v", err)
	}
	next, ok, err := store.NextQueuedPrompt("fifo-session")
	if err != nil {
		t.Fatalf("NextQueuedPrompt: %v", err)
	}
	if !ok || next.ID != "q-1" {
		t.Fatalf("NextQueuedPrompt = %#v, %v; want q-1", next, ok)
	}

	if err := store.MarkQueueRunning("fifo-session", "q-1", created.Add(2*time.Minute)); err != nil {
		t.Fatalf("MarkQueueRunning: %v", err)
	}
	if err := store.MarkQueueCompleted("fifo-session", "q-1", "first response", "end_turn", created.Add(3*time.Minute)); err != nil {
		t.Fatalf("MarkQueueCompleted: %v", err)
	}
	afterFirst, err := store.Get("fifo-session")
	if err != nil {
		t.Fatalf("Get after first: %v", err)
	}
	if afterFirst.ACPSessionID != "acp-session" || afterFirst.CreatedAt.IsZero() {
		t.Fatalf("metadata not preserved: %#v", afterFirst)
	}
	if afterFirst.PromptQueue[0].Status != QueuePromptStatusCompleted || afterFirst.PromptQueue[0].Response != "first response" {
		t.Fatalf("completed prompt = %#v", afterFirst.PromptQueue[0])
	}
	if len(afterFirst.Turns) != 1 || afterFirst.Turns[0].Prompt != "first" || afterFirst.Turns[0].Response != "first response" {
		t.Fatalf("turns = %#v", afterFirst.Turns)
	}

	next, ok, err = store.NextQueuedPrompt("fifo-session")
	if err != nil {
		t.Fatalf("NextQueuedPrompt after first: %v", err)
	}
	if !ok || next.ID != "q-2" {
		t.Fatalf("NextQueuedPrompt after first = %#v, %v; want q-2", next, ok)
	}
	if err := store.MarkQueueFailed("fifo-session", "q-2", "boom", second.Add(4*time.Minute)); err != nil {
		t.Fatalf("MarkQueueFailed: %v", err)
	}
	failed, err := store.Get("fifo-session")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if failed.PromptQueue[1].Status != QueuePromptStatusFailed || failed.PromptQueue[1].Error != "boom" {
		t.Fatalf("failed prompt = %#v", failed.PromptQueue[1])
	}
}

func TestSessionStoreQueueOperationsReportMissingQueueItem(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 21, 5, 0, 0, time.UTC)
	if _, err := store.AppendQueuedPrompt(SessionRecord{ID: "missing-queue-item"}, QueuedPrompt{
		ID:        "existing",
		Prompt:    "hello",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("AppendQueuedPrompt: %v", err)
	}

	err := store.MarkQueueRunning("missing-queue-item", "absent", now.Add(time.Minute))
	if !errors.Is(err, ErrQueuePromptNotFound) {
		t.Fatalf("MarkQueueRunning error = %v, want ErrQueuePromptNotFound", err)
	}
	if errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("MarkQueueRunning error wraps ErrSessionNotFound for an existing session: %v", err)
	}
}

func TestSessionStoreCancelPendingQueueOnlyCancelsPendingItems(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 21, 10, 0, 0, time.UTC)
	started := now.Add(time.Minute)
	rec := SessionRecord{
		ID:        "cancel-queue",
		Status:    SessionStatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
		PromptQueue: []QueuedPrompt{
			{ID: "running", Prompt: "active", Status: QueuePromptStatusRunning, CreatedAt: now, StartedAt: &started},
			{ID: "pending-1", Prompt: "one", Status: QueuePromptStatusPending, CreatedAt: now.Add(2 * time.Minute)},
			{ID: "pending-2", Prompt: "two", Status: QueuePromptStatusPending, CreatedAt: now.Add(3 * time.Minute)},
			{ID: "done", Prompt: "done", Status: QueuePromptStatusCompleted, CreatedAt: now.Add(4 * time.Minute)},
		},
	}
	if err := store.Upsert(rec); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	count, err := store.CancelPendingQueue("cancel-queue", now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("CancelPendingQueue: %v", err)
	}
	if count != 2 {
		t.Fatalf("canceled count = %d, want 2", count)
	}
	got, err := store.Get("cancel-queue")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PromptQueue[0].Status != QueuePromptStatusRunning {
		t.Fatalf("running item status = %q", got.PromptQueue[0].Status)
	}
	if got.PromptQueue[1].Status != QueuePromptStatusCanceled || got.PromptQueue[1].CanceledAt == nil {
		t.Fatalf("pending-1 = %#v", got.PromptQueue[1])
	}
	if got.PromptQueue[2].Status != QueuePromptStatusCanceled || got.PromptQueue[2].CanceledAt == nil {
		t.Fatalf("pending-2 = %#v", got.PromptQueue[2])
	}
	if got.PromptQueue[3].Status != QueuePromptStatusCompleted {
		t.Fatalf("completed item status = %q", got.PromptQueue[3].Status)
	}
}

func TestSessionStoreRecoverStaleQueueRequeuesRunningItemsWithoutOwner(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 21, 20, 0, 0, time.UTC)
	started := now.Add(time.Minute)
	if err := store.Upsert(SessionRecord{
		ID:        "recover-missing-owner",
		Status:    SessionStatusRunning,
		CreatedAt: now,
		UpdatedAt: started,
		PromptQueue: []QueuedPrompt{{
			ID:        "q-running",
			Prompt:    "active",
			Status:    QueuePromptStatusRunning,
			CreatedAt: now,
			StartedAt: &started,
		}},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	count, err := store.RecoverStaleQueue("recover-missing-owner", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("RecoverStaleQueue: %v", err)
	}
	if count != 1 {
		t.Fatalf("recovered count = %d, want 1", count)
	}
	got, err := store.Get("recover-missing-owner")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != SessionStatusQueued {
		t.Fatalf("session status = %q, want queued", got.Status)
	}
	if got.PromptQueue[0].Status != QueuePromptStatusPending || got.PromptQueue[0].StartedAt != nil {
		t.Fatalf("recovered prompt = %#v", got.PromptQueue[0])
	}
}

func TestSessionStoreRecoverStaleQueueKeepsRunningItemsWithReadableOwner(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 21, 25, 0, 0, time.UTC)
	started := now.Add(time.Minute)
	if err := store.Upsert(SessionRecord{
		ID:        "recover-readable-owner",
		Status:    SessionStatusRunning,
		CreatedAt: now,
		UpdatedAt: started,
		PromptQueue: []QueuedPrompt{{
			ID:        "q-running",
			Prompt:    "active",
			Status:    QueuePromptStatusRunning,
			CreatedAt: now,
			StartedAt: &started,
		}},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	lease, err := store.AcquireOwnerLease(OwnerLock{SessionID: "recover-readable-owner", PID: 123, StartedAt: started})
	if err != nil {
		t.Fatalf("AcquireOwnerLease: %v", err)
	}
	defer func() { _ = lease.Release() }()

	count, err := store.RecoverStaleQueue("recover-readable-owner", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("RecoverStaleQueue: %v", err)
	}
	if count != 0 {
		t.Fatalf("recovered count = %d, want 0", count)
	}
	got, err := store.Get("recover-readable-owner")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PromptQueue[0].Status != QueuePromptStatusRunning || got.PromptQueue[0].StartedAt == nil {
		t.Fatalf("running prompt changed = %#v", got.PromptQueue[0])
	}
}

func TestSessionStoreRecoverStaleQueueIgnoresInvalidStaleProjection(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 21, 27, 0, 0, time.UTC)
	started := now.Add(time.Minute)
	if err := store.Upsert(SessionRecord{
		ID:        "recover-invalid-owner",
		Status:    SessionStatusRunning,
		CreatedAt: now,
		UpdatedAt: started,
		PromptQueue: []QueuedPrompt{{
			ID:        "q-running",
			Prompt:    "active",
			Status:    QueuePromptStatusRunning,
			CreatedAt: now,
			StartedAt: &started,
		}},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	ownerPath := store.ownerPath("recover-invalid-owner")
	if err := os.MkdirAll(filepath.Dir(ownerPath), 0o755); err != nil {
		t.Fatalf("mkdir owner: %v", err)
	}
	if err := os.WriteFile(ownerPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write owner: %v", err)
	}

	if count, err := store.RecoverStaleQueue("recover-invalid-owner", now.Add(2*time.Minute)); err != nil || count != 1 {
		t.Fatalf("RecoverStaleQueue with invalid stale owner = %d, %v; want 1, nil", count, err)
	}
	got, err := store.Get("recover-invalid-owner")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PromptQueue[0].Status != QueuePromptStatusPending {
		t.Fatalf("invalid stale-owner recovery prompt = %#v, want pending", got.PromptQueue[0])
	}
}

func TestSessionStoreRecoverStaleQueueIgnoresCorruptStaleProjection(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 21, 30, 0, 0, time.UTC)
	started := now.Add(time.Minute)
	if err := store.Upsert(SessionRecord{
		ID:        "recover-corrupt-owner",
		Status:    SessionStatusRunning,
		CreatedAt: now,
		UpdatedAt: started,
		PromptQueue: []QueuedPrompt{{
			ID:        "q-running",
			Prompt:    "active",
			Status:    QueuePromptStatusRunning,
			CreatedAt: now,
			StartedAt: &started,
		}},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	ownerPath := store.ownerPath("recover-corrupt-owner")
	if err := os.MkdirAll(filepath.Dir(ownerPath), 0o755); err != nil {
		t.Fatalf("mkdir owner: %v", err)
	}
	if err := os.WriteFile(ownerPath, []byte("{"), 0o600); err != nil {
		t.Fatalf("write owner: %v", err)
	}

	if count, err := store.RecoverStaleQueue("recover-corrupt-owner", now.Add(2*time.Minute)); err != nil || count != 1 {
		t.Fatalf("RecoverStaleQueue with corrupt stale owner = %d, %v; want 1, nil", count, err)
	}
	got, err := store.Get("recover-corrupt-owner")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PromptQueue[0].Status != QueuePromptStatusPending {
		t.Fatalf("corrupt stale-owner recovery prompt = %#v, want pending", got.PromptQueue[0])
	}
}

func TestSummarizeStoreTextTruncatesAtRuneBoundary(t *testing.T) {
	value := strings.Repeat("界", 201)

	got := summarizeStoreText(value)
	if !utf8.ValidString(got) {
		t.Fatalf("summary is not valid UTF-8: %q", got)
	}
	if utf8.RuneCountInString(got) != 200 {
		t.Fatalf("summary rune count = %d, want 200", utf8.RuneCountInString(got))
	}
}
