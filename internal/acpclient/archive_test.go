package acpclient

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestImportSessionConcurrentCollisionIsTransactional(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "sessions.json")
	now := time.Date(2026, 7, 13, 17, 30, 0, 0, time.UTC)
	archive := Archive{
		FormatVersion: archiveFormatVersion,
		ExportedAt:    now.Format(time.RFC3339Nano),
		ExportedBy:    "ratchet-cli",
		Session: ArchiveSession{
			RecordID:    "same-id",
			CWDRelative: ".",
			CreatedAt:   now.Format(time.RFC3339Nano),
			UpdatedAt:   now.Format(time.RFC3339Nano),
			State: SessionRecord{
				ID:        "same-id",
				Status:    SessionStatusCompleted,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}
	archivePath := writeArchiveFixture(t, archive)

	stores := []*Store{NewStore(path), NewStore(path)}
	ready := make(chan struct{}, len(stores))
	release := make(chan struct{})
	for _, store := range stores {
		store.beforeMutation = func() {
			ready <- struct{}{}
			<-release
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(stores))
	for _, store := range stores {
		wg.Go(func() {
			_, err := ImportSession(store, archivePath, ImportOptions{HomeDir: t.TempDir()})
			errs <- err
		})
	}
	for range stores {
		select {
		case <-ready:
		case <-time.After(2 * time.Second):
			t.Fatal("imports did not both reach record insertion")
		}
	}
	close(release)
	wg.Wait()
	close(errs)

	successes := 0
	collisions := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrSessionArchiveCollision):
			collisions++
		default:
			t.Fatalf("ImportSession error = %v", err)
		}
	}
	if successes != 1 || collisions != 1 {
		t.Fatalf("imports = %d successes, %d collisions; want one each", successes, collisions)
	}
}

func TestExportSessionWritesACPXShapedArchive(t *testing.T) {
	home := t.TempDir()
	cwd := filepath.Join(home, "repo", "project")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	created := time.Date(2026, 7, 2, 8, 30, 0, 0, time.UTC)
	completed := created.Add(time.Minute)
	if err := store.Upsert(SessionRecord{
		ID:                 "archive-source",
		ACPSessionID:       "provider-session",
		Agent:              "fixture",
		CommandFingerprint: "fp123",
		Cwd:                cwd,
		Status:             SessionStatusCompleted,
		CreatedAt:          created,
		UpdatedAt:          completed,
		LastStopReason:     "end_turn",
		Summary:            "done",
		Turns: []TurnSummary{{
			Prompt:     "hello",
			Response:   "fixture: hello",
			StopReason: "end_turn",
			CreatedAt:  completed,
		}},
		PromptQueue: []QueuedPrompt{{
			ID:          "q-1",
			Prompt:      "queued",
			Status:      QueuePromptStatusCompleted,
			CreatedAt:   created,
			CompletedAt: &completed,
			Response:    "fixture: queued",
			StopReason:  "end_turn",
		}},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	archivePath := filepath.Join(t.TempDir(), "archive.json")
	if err := ExportSession(store, "archive-source", archivePath, ExportOptions{HomeDir: home, Now: fixedArchiveClock(completed)}); err != nil {
		t.Fatalf("ExportSession: %v", err)
	}

	var archive Archive
	b, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if err := json.Unmarshal(b, &archive); err != nil {
		t.Fatalf("unmarshal archive: %v\n%s", err, b)
	}
	if archive.FormatVersion != 1 || archive.ExportedBy != "ratchet-cli" || archive.ExportedAt != completed.Format(time.RFC3339Nano) {
		t.Fatalf("archive metadata = %#v", archive)
	}
	if archive.Session.RecordID != "archive-source" || archive.Session.Agent != "fixture" {
		t.Fatalf("archive session = %#v", archive.Session)
	}
	if archive.Session.CWDRelative != filepath.Join("repo", "project") {
		t.Fatalf("CWDRelative = %q", archive.Session.CWDRelative)
	}
	if archive.Session.CWDOriginal != archive.Session.CWDRelative {
		t.Fatalf("CWDOriginal = %q, want relative cwd", archive.Session.CWDOriginal)
	}
	if archive.Session.State.ID != "archive-source" || archive.Session.State.ACPSessionID != "provider-session" {
		t.Fatalf("state = %#v", archive.Session.State)
	}
	if len(archive.History) != 2 || archive.History[0].Kind != "turn" || archive.History[1].Kind != "queue" {
		t.Fatalf("history = %#v, want turn and queue events", archive.History)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(archivePath)
		if err != nil {
			t.Fatalf("stat archive: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("archive mode = %o, want 0600", got)
		}
	}
}

func TestExportSessionRefusesActiveOwner(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 2, 8, 40, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID:        "active",
		Status:    SessionStatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	lease, err := store.AcquireOwnerLease(OwnerLock{SessionID: "active", PID: os.Getpid(), StartedAt: now})
	if err != nil {
		t.Fatalf("AcquireOwnerLease: %v", err)
	}
	defer func() { _ = lease.Release() }()

	err = ExportSession(store, "active", filepath.Join(t.TempDir(), "archive.json"), ExportOptions{Now: fixedArchiveClock(now)})
	if !errors.Is(err, ErrSessionActive) {
		t.Fatalf("ExportSession error = %v, want ErrSessionActive", err)
	}
}

func TestExportSessionHoldsOwnerLeaseAcrossSnapshot(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 13, 20, 10, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{ID: "snapshot", Status: SessionStatusCompleted, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := store.WriteEventLog("snapshot", []EventLogLine{{
		At: now, Direction: EventDirectionInbound,
		Message: json.RawMessage(`{"jsonrpc":"2.0","id":1,"result":{}}`),
	}}); err != nil {
		t.Fatalf("WriteEventLog: %v", err)
	}
	var claimErr error
	err := ExportSession(store, "snapshot", filepath.Join(t.TempDir(), "archive.json"), ExportOptions{
		HistoryMode: ArchiveHistoryModeRaw,
		Now: func() time.Time {
			other, err := store.AcquireOwnerLease(OwnerLock{SessionID: "snapshot", PID: os.Getpid(), StartedAt: now.Add(time.Second)})
			if other != nil {
				_ = other.Release()
			}
			claimErr = err
			return now
		},
	})
	if err != nil {
		t.Fatalf("ExportSession: %v", err)
	}
	if !errors.Is(claimErr, ErrOwnerLeaseBusy) {
		t.Fatalf("snapshot owner claim = %v, want ErrOwnerLeaseBusy", claimErr)
	}
	if _, err := store.Owner("snapshot"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Owner after export = %v, want os.ErrNotExist", err)
	}
}

func TestImportSessionPreservesACPXRawHistoryAndExportsRaw(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state", "sessions.json"))
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	rawHistory := []json.RawMessage{
		json.RawMessage(`{"jsonrpc":"2.0","id":"prompt-1","method":"session/prompt","params":{"sessionId":"provider-session","prompt":[]}}`),
		json.RawMessage(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"provider-session","update":{"sessionUpdate":"agent_message_chunk"}}}`),
		json.RawMessage(`{"jsonrpc":"2.0","id":"prompt-1","result":{"stopReason":"end_turn"}}`),
	}
	archivePath := writeRawArchiveFixture(t, map[string]any{
		"format_version": 1,
		"exported_at":    now.Format(time.RFC3339Nano),
		"exported_by":    "acpx",
		"session": map[string]any{
			"record_id":    "acpx-record",
			"agent":        "codex-acp",
			"agent_name":   "codex",
			"cwd_relative": "repo",
			"cwd_original": "repo",
			"created_at":   now.Format(time.RFC3339Nano),
			"updated_at":   now.Format(time.RFC3339Nano),
			"state": map[string]any{
				"id":                 "acpx-record",
				"acpSessionId":       "provider-session",
				"agent":              "codex",
				"commandFingerprint": "fp",
				"cwd":                "/source/repo",
				"status":             "completed",
				"createdAt":          now.Format(time.RFC3339Nano),
				"updatedAt":          now.Format(time.RFC3339Nano),
			},
		},
		"history": rawHistory,
	})

	imported, err := ImportSession(store, archivePath, ImportOptions{
		SessionID: "imported-raw",
		Cwd:       filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("ImportSession: %v", err)
	}
	events, err := store.ReadEventLog(imported.ID)
	if err != nil {
		t.Fatalf("ReadEventLog: %v", err)
	}
	if len(events) != len(rawHistory) {
		t.Fatalf("events len = %d, want %d", len(events), len(rawHistory))
	}
	for i := range rawHistory {
		if events[i].Seq != i+1 || events[i].Direction != EventDirectionInbound {
			t.Fatalf("event %d = %#v, want inbound seq %d", i, events[i], i+1)
		}
		if !jsonMessagesEqual(events[i].Message, rawHistory[i]) {
			t.Fatalf("event %d message = %s, want %s", i, events[i].Message, rawHistory[i])
		}
	}

	exportPath := filepath.Join(t.TempDir(), "raw-export.json")
	if err := ExportSession(store, imported.ID, exportPath, ExportOptions{
		HistoryMode: ArchiveHistoryModeRaw,
		Now:         fixedArchiveClock(now),
	}); err != nil {
		t.Fatalf("ExportSession raw: %v", err)
	}
	var exported struct {
		History        []json.RawMessage     `json:"history"`
		SummaryHistory []ArchiveHistoryEvent `json:"summary_history"`
	}
	b, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read exported archive: %v", err)
	}
	if err := json.Unmarshal(b, &exported); err != nil {
		t.Fatalf("unmarshal exported raw archive: %v\n%s", err, b)
	}
	if len(exported.SummaryHistory) != 0 {
		t.Fatalf("summary_history len = %d, want 0 for raw mode", len(exported.SummaryHistory))
	}
	if len(exported.History) != len(rawHistory) {
		t.Fatalf("raw history len = %d, want %d", len(exported.History), len(rawHistory))
	}
	for i := range rawHistory {
		if !jsonMessagesEqual(exported.History[i], rawHistory[i]) {
			t.Fatalf("raw history %d = %s, want %s", i, exported.History[i], rawHistory[i])
		}
	}
}

func TestExportSessionRawHistoryRequiresSidecar(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 2, 9, 5, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID:        "no-sidecar",
		Agent:     "fixture",
		Cwd:       t.TempDir(),
		Status:    SessionStatusCompleted,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	err := ExportSession(store, "no-sidecar", filepath.Join(t.TempDir(), "archive.json"), ExportOptions{
		HistoryMode: ArchiveHistoryModeRaw,
		Now:         fixedArchiveClock(now),
	})
	if !errors.Is(err, ErrRawHistoryUnavailable) {
		t.Fatalf("ExportSession raw error = %v, want ErrRawHistoryUnavailable", err)
	}
}

func TestImportSessionRejectsInvalidRawHistory(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 2, 9, 10, 0, 0, time.UTC)
	archivePath := writeRawArchiveFixture(t, map[string]any{
		"format_version": 1,
		"exported_at":    now.Format(time.RFC3339Nano),
		"exported_by":    "acpx",
		"session": map[string]any{
			"record_id":    "bad-raw",
			"agent":        "fixture",
			"cwd_relative": ".",
			"cwd_original": ".",
			"created_at":   now.Format(time.RFC3339Nano),
			"updated_at":   now.Format(time.RFC3339Nano),
			"state": map[string]any{
				"id":        "bad-raw",
				"agent":     "fixture",
				"cwd":       t.TempDir(),
				"status":    "completed",
				"createdAt": now.Format(time.RFC3339Nano),
				"updatedAt": now.Format(time.RFC3339Nano),
			},
		},
		"history": []json.RawMessage{json.RawMessage(`{"jsonrpc":"2.0"}`)},
	})

	_, err := ImportSession(store, archivePath, ImportOptions{})
	if !errors.Is(err, ErrInvalidSessionArchive) {
		t.Fatalf("ImportSession error = %v, want ErrInvalidSessionArchive", err)
	}
	if _, err := store.Get("bad-raw"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("invalid raw history left session behind: %v", err)
	}

	archivePath = writeRawArchiveFixture(t, map[string]any{
		"format_version": 1,
		"exported_at":    now.Format(time.RFC3339Nano),
		"exported_by":    "acpx",
		"session": map[string]any{
			"record_id":    "bad-summary",
			"agent":        "fixture",
			"cwd_relative": ".",
			"cwd_original": ".",
			"created_at":   now.Format(time.RFC3339Nano),
			"updated_at":   now.Format(time.RFC3339Nano),
			"state": map[string]any{
				"id":        "bad-summary",
				"agent":     "fixture",
				"cwd":       t.TempDir(),
				"status":    "completed",
				"createdAt": now.Format(time.RFC3339Nano),
				"updatedAt": now.Format(time.RFC3339Nano),
			},
		},
		"history": []map[string]any{{"kind": "turn", "prompt": "not raw json-rpc"}},
	})
	_, err = ImportSession(store, archivePath, ImportOptions{})
	if !errors.Is(err, ErrInvalidSessionArchive) {
		t.Fatalf("ImportSession summary-shaped acpx history error = %v, want ErrInvalidSessionArchive", err)
	}
}

func TestImportSessionEventLogFailureLeavesNoCollisionAndCanRetry(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sessions.json"))
	now := time.Date(2026, 7, 13, 20, 15, 0, 0, time.UTC)
	archivePath := writeRawArchiveFixture(t, map[string]any{
		"format_version": 1,
		"exported_at":    now.Format(time.RFC3339Nano),
		"exported_by":    "acpx",
		"session": map[string]any{
			"record_id": "retry-import", "cwd_relative": ".", "created_at": now.Format(time.RFC3339Nano), "updated_at": now.Format(time.RFC3339Nano),
			"state": map[string]any{"id": "retry-import", "status": "completed", "createdAt": now.Format(time.RFC3339Nano), "updatedAt": now.Format(time.RFC3339Nano)},
		},
		"history": []json.RawMessage{json.RawMessage(`{"jsonrpc":"2.0","id":1,"result":{}}`)},
	})
	eventsPath := filepath.Join(dir, "events")
	if err := os.WriteFile(eventsPath, []byte("blocks event directory\n"), 0o600); err != nil {
		t.Fatalf("write event directory blocker: %v", err)
	}
	if _, err := ImportSession(store, archivePath, ImportOptions{}); err == nil {
		t.Fatal("ImportSession with blocked event directory succeeded")
	}
	if _, err := store.Get("retry-import"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("failed import left session behind: %v", err)
	}
	if err := os.Remove(eventsPath); err != nil {
		t.Fatalf("remove event directory blocker: %v", err)
	}
	if _, err := ImportSession(store, archivePath, ImportOptions{}); err != nil {
		t.Fatalf("ImportSession retry: %v", err)
	}
	events, err := store.ReadEventLog("retry-import")
	if err != nil || len(events) != 1 {
		t.Fatalf("retry event log = %#v, %v; want one event", events, err)
	}
}

func TestImportSessionValidatesVersionAndCollisions(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 2, 8, 45, 0, 0, time.UTC)
	archive := Archive{
		FormatVersion: 1,
		ExportedAt:    now.Format(time.RFC3339Nano),
		ExportedBy:    "ratchet-cli",
		Session: ArchiveSession{
			RecordID:    "source",
			Agent:       "fixture",
			CWDRelative: "repo",
			CWDOriginal: "repo",
			CreatedAt:   now.Format(time.RFC3339Nano),
			UpdatedAt:   now.Format(time.RFC3339Nano),
			State: SessionRecord{
				ID:                 "source",
				ACPSessionID:       "provider-session",
				Agent:              "fixture",
				CommandFingerprint: "fp-source",
				Cwd:                "/source/repo",
				Status:             SessionStatusRunning,
				CreatedAt:          now,
				UpdatedAt:          now,
			},
		},
	}
	archivePath := writeArchiveFixture(t, archive)

	imported, err := ImportSession(store, archivePath, ImportOptions{
		SessionID:          "imported",
		Cwd:                "/dest/repo",
		Agent:              "custom",
		CommandFingerprint: "fp-dest",
	})
	if err != nil {
		t.Fatalf("ImportSession: %v", err)
	}
	if imported.ID != "imported" || imported.Cwd != "/dest/repo" || imported.Agent != "custom" || imported.CommandFingerprint != "fp-dest" {
		t.Fatalf("imported metadata = %#v", imported)
	}
	if imported.ACPSessionID != "provider-session" {
		t.Fatalf("ACPSessionID = %q", imported.ACPSessionID)
	}
	if imported.Status == SessionStatusRunning || imported.Status == SessionStatusCancelRequested {
		t.Fatalf("Status = %q, want non-running imported record", imported.Status)
	}
	if _, err := store.Get("imported"); err != nil {
		t.Fatalf("Get imported: %v", err)
	}

	_, err = ImportSession(store, archivePath, ImportOptions{SessionID: "imported"})
	if !errors.Is(err, ErrSessionArchiveCollision) {
		t.Fatalf("collision error = %v, want ErrSessionArchiveCollision", err)
	}

	archive.FormatVersion = 99
	_, err = ImportSession(store, writeArchiveFixture(t, archive), ImportOptions{SessionID: "other"})
	if !errors.Is(err, ErrUnsupportedArchiveVersion) {
		t.Fatalf("unsupported version error = %v, want ErrUnsupportedArchiveVersion", err)
	}
}

func TestImportSessionRejectsParentTraversalCWDRelative(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 2, 8, 50, 0, 0, time.UTC)
	archive := Archive{
		FormatVersion: 1,
		ExportedAt:    now.Format(time.RFC3339Nano),
		ExportedBy:    "ratchet-cli",
		Session: ArchiveSession{
			RecordID:    "source",
			Agent:       "fixture",
			CWDRelative: filepath.Join("..", "escape"),
			CWDOriginal: filepath.Join("..", "escape"),
			CreatedAt:   now.Format(time.RFC3339Nano),
			UpdatedAt:   now.Format(time.RFC3339Nano),
			State: SessionRecord{
				ID:        "source",
				Agent:     "fixture",
				Status:    SessionStatusCompleted,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}

	_, err := ImportSession(store, writeArchiveFixture(t, archive), ImportOptions{HomeDir: t.TempDir()})
	if !errors.Is(err, ErrInvalidSessionArchive) {
		t.Fatalf("ImportSession error = %v, want ErrInvalidSessionArchive", err)
	}
	if _, err := store.Get("source"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("store.Get source error = %v, want ErrSessionNotFound", err)
	}
}

func TestImportSessionNormalizesRelativeOverrideCWD(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 2, 8, 55, 0, 0, time.UTC)
	archive := Archive{
		FormatVersion: 1,
		ExportedAt:    now.Format(time.RFC3339Nano),
		ExportedBy:    "ratchet-cli",
		Session: ArchiveSession{
			RecordID:    "source",
			Agent:       "fixture",
			CWDRelative: "repo",
			CWDOriginal: "repo",
			CreatedAt:   now.Format(time.RFC3339Nano),
			UpdatedAt:   now.Format(time.RFC3339Nano),
			State: SessionRecord{
				ID:        "source",
				Agent:     "fixture",
				Status:    SessionStatusCompleted,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}

	imported, err := ImportSession(store, writeArchiveFixture(t, archive), ImportOptions{Cwd: filepath.Join("relative", "repo")})
	if err != nil {
		t.Fatalf("ImportSession: %v", err)
	}
	if !filepath.IsAbs(imported.Cwd) {
		t.Fatalf("imported.Cwd = %q, want absolute path", imported.Cwd)
	}
	if filepath.Base(imported.Cwd) != "repo" {
		t.Fatalf("imported.Cwd = %q, want repo suffix", imported.Cwd)
	}
}

func writeArchiveFixture(t *testing.T, archive Archive) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.json")
	b, err := json.MarshalIndent(archive, "", "  ")
	if err != nil {
		t.Fatalf("marshal archive: %v", err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	return path
}

func writeRawArchiveFixture(t *testing.T, archive any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.json")
	b, err := json.MarshalIndent(archive, "", "  ")
	if err != nil {
		t.Fatalf("marshal archive: %v", err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	return path
}

func jsonMessagesEqual(a, b json.RawMessage) bool {
	var av any
	var bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	return av == nil && bv == nil || jsonEqualValues(av, bv)
}

func jsonEqualValues(a, b any) bool {
	ab, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bb, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(ab) == string(bb)
}

func fixedArchiveClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}
