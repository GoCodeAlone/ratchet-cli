package daemon

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/tochemey/goakt/v4/actor"
	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := initDB(db); err != nil {
		db.Close()
		t.Fatalf("init test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestActorManager_Init(t *testing.T) {
	db := openTestDB(t)
	am, err := NewActorManager(context.Background(), db)
	if err != nil {
		t.Fatalf("NewActorManager: %v", err)
	}
	defer am.Close(context.Background())

	if am.system == nil {
		t.Fatal("expected non-nil actor system")
	}
	if am.sessions == nil {
		t.Fatal("expected non-nil sessions map")
	}
}

func TestActorManager_SessionActor_Create(t *testing.T) {
	db := openTestDB(t)
	am, err := NewActorManager(context.Background(), db)
	if err != nil {
		t.Fatalf("NewActorManager: %v", err)
	}
	defer am.Close(context.Background())

	pid, err := am.SpawnSession(context.Background(), "sess-test-1", "/tmp")
	if err != nil {
		t.Fatalf("SpawnSession: %v", err)
	}
	if pid == nil {
		t.Fatal("expected non-nil PID")
	}

	// Spawning the same session again should return the cached PID.
	pid2, err := am.SpawnSession(context.Background(), "sess-test-1", "/tmp")
	if err != nil {
		t.Fatalf("SpawnSession (duplicate): %v", err)
	}
	if pid != pid2 {
		t.Error("expected same PID for duplicate session spawn")
	}
}

func TestActorManager_SessionActor_Persistence(t *testing.T) {
	db := openTestDB(t)

	// Insert an active session into SQLite.
	_, err := db.Exec(
		`INSERT INTO sessions (id, name, status, working_dir) VALUES (?, ?, ?, ?)`,
		"sess-persist-1", "test-session", "active", "/workspace",
	)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}

	am, err := NewActorManager(context.Background(), db)
	if err != nil {
		t.Fatalf("NewActorManager: %v", err)
	}
	defer am.Close(context.Background())

	// Verify the session was rehydrated (pid exists in map).
	am.mu.RLock()
	pid, ok := am.sessions["sess-persist-1"]
	am.mu.RUnlock()
	if !ok {
		t.Fatal("expected session actor to be rehydrated from SQLite")
	}
	if pid == nil {
		t.Fatal("expected non-nil rehydrated PID")
	}
}

func TestActorManager_ApprovalFlow(t *testing.T) {
	db := openTestDB(t)
	am, err := NewActorManager(context.Background(), db)
	if err != nil {
		t.Fatalf("NewActorManager: %v", err)
	}
	defer am.Close(context.Background())

	pid, err := am.SpawnApproval(context.Background(), "req-001")
	if err != nil {
		t.Fatalf("SpawnApproval: %v", err)
	}

	// Send an ApprovalRequest via Ask; actor returns denied response immediately.
	resp, err := actor.Ask(context.Background(), pid, ApprovalRequest{
		ToolName: "bash",
		Input:    "rm -rf /tmp/test",
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("Ask ApprovalRequest: %v", err)
	}

	ar, ok := resp.(ApprovalResponse)
	if !ok {
		t.Fatalf("expected ApprovalResponse, got %T", resp)
	}
	// Default behavior: denied (no TUI present in tests).
	if ar.Approved {
		t.Error("expected Approved=false for unanswered approval request")
	}
}
