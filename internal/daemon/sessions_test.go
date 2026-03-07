package daemon

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_journal_mode=WAL")
	if err != nil {
		t.Fatal(err)
	}
	if err := initDB(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSessionManagerCRUD(t *testing.T) {
	db := setupTestDB(t)
	sm := NewSessionManager(db)
	ctx := context.Background()

	// Create
	s, err := sm.Create(ctx, "/tmp/project", "anthropic", "claude-sonnet-4", "fix the login bug")
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != "active" {
		t.Errorf("expected active, got %s", s.Status)
	}
	if s.ID == "" {
		t.Error("expected non-empty ID")
	}

	// List
	list, err := sm.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 session, got %d", len(list))
	}

	// Get
	got, err := sm.Get(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkingDir != "/tmp/project" {
		t.Errorf("expected /tmp/project, got %s", got.WorkingDir)
	}

	// Kill
	if err := sm.Kill(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	s2, err := sm.Get(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if s2.Status != "completed" {
		t.Errorf("expected completed, got %s", s2.Status)
	}
}

func TestSessionManagerSubscribe(t *testing.T) {
	db := setupTestDB(t)
	sm := NewSessionManager(db)
	ctx := context.Background()

	s, _ := sm.Create(ctx, "/tmp", "", "", "")
	ch := sm.Subscribe(s.ID)

	sm.Publish(s.ID, "test-event")
	got := <-ch
	if got != "test-event" {
		t.Errorf("expected test-event, got %v", got)
	}
}
