package acpclient

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

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
	if err := store.WriteOwner(OwnerLock{SessionID: "active", PID: os.Getpid(), StartedAt: now}); err != nil {
		t.Fatalf("WriteOwner: %v", err)
	}

	err := ExportSession(store, "active", filepath.Join(t.TempDir(), "archive.json"), ExportOptions{Now: fixedArchiveClock(now)})
	if !errors.Is(err, ErrSessionActive) {
		t.Fatalf("ExportSession error = %v, want ErrSessionActive", err)
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

func fixedArchiveClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}
