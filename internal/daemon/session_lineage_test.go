package daemon

import (
	"database/sql"
	"slices"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	_ "modernc.org/sqlite"
)

func TestSessionLineageMigrationAddsColumns(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:?_journal_mode=WAL")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			name TEXT,
			status TEXT DEFAULT 'active',
			working_dir TEXT,
			provider TEXT,
			model TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE messages (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT,
			tool_name TEXT,
			tool_call_id TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		);
		INSERT INTO sessions (id, name, status, working_dir, provider, model)
		VALUES ('sess-old', 'old', 'active', '/tmp/project', 'mock', 'mock-model');
	`)
	if err != nil {
		t.Fatal(err)
	}

	if err := initDB(db); err != nil {
		t.Fatal(err)
	}

	columns := tableColumns(t, db, "sessions")
	for _, name := range []string{"parent_id", "root_id", "forked_from_message_id", "fork_reason"} {
		if !slices.Contains(columns, name) {
			t.Fatalf("expected sessions column %q after migration; got %v", name, columns)
		}
	}

	var rootID string
	if err := db.QueryRow(`SELECT root_id FROM sessions WHERE id = 'sess-old'`).Scan(&rootID); err != nil {
		t.Fatal(err)
	}
	if rootID != "sess-old" {
		t.Fatalf("root_id = %q, want sess-old", rootID)
	}
}

func TestEnsureColumnQuotesSQLiteIdentifiers(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:?_journal_mode=WAL")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`CREATE TABLE "needs quote" (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := ensureColumn(db, "needs quote", "new column", "TEXT"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO "needs quote" (id, "new column") VALUES ('id-1', 'value-1')`); err != nil {
		t.Fatal(err)
	}

	var got string
	if err := db.QueryRow(`SELECT "new column" FROM "needs quote" WHERE id = 'id-1'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "value-1" {
		t.Fatalf(`"new column" = %q, want value-1`, got)
	}
}

func TestSessionManagerLineageCreateCloneForkTree(t *testing.T) {
	db := setupTestDB(t)
	sm := NewSessionManager(db)
	ctx := t.Context()

	source, err := sm.Create(ctx, "/tmp/project", "mock", "mock-model", "investigate lineage")
	if err != nil {
		t.Fatal(err)
	}
	if source.RootID != source.ID {
		t.Fatalf("source RootID = %q, want %q", source.RootID, source.ID)
	}

	firstID := insertMessage(t, db, source.ID, "user", "first")
	secondID := insertMessage(t, db, source.ID, "assistant", "second")
	insertMessage(t, db, source.ID, "user", "third")

	clone, err := sm.Clone(ctx, source.ID, "parallel attempt")
	if err != nil {
		t.Fatal(err)
	}
	if clone.ParentID != source.ID {
		t.Fatalf("clone ParentID = %q, want %q", clone.ParentID, source.ID)
	}
	if clone.RootID != source.ID {
		t.Fatalf("clone RootID = %q, want %q", clone.RootID, source.ID)
	}
	if got := countMessages(t, db, clone.ID); got != 3 {
		t.Fatalf("clone message count = %d, want 3", got)
	}

	fork, err := sm.Fork(ctx, source.ID, secondID, "branch from second")
	if err != nil {
		t.Fatal(err)
	}
	if fork.ParentID != source.ID {
		t.Fatalf("fork ParentID = %q, want %q", fork.ParentID, source.ID)
	}
	if fork.RootID != source.ID {
		t.Fatalf("fork RootID = %q, want %q", fork.RootID, source.ID)
	}
	if fork.ForkedFromMessageID != secondID {
		t.Fatalf("fork ForkedFromMessageID = %q, want %q", fork.ForkedFromMessageID, secondID)
	}
	if got := countMessages(t, db, fork.ID); got != 2 {
		t.Fatalf("fork message count = %d, want 2", got)
	}

	if _, err := sm.Fork(ctx, source.ID, "missing-message", "bad branch"); err == nil {
		t.Fatal("expected missing message fork to fail")
	}

	tree, err := sm.ListTree(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 3 {
		t.Fatalf("tree length = %d, want 3", len(tree))
	}
	gotIDs := []string{tree[0].ID, tree[1].ID, tree[2].ID}
	for _, want := range []string{source.ID, clone.ID, fork.ID} {
		if !slices.Contains(gotIDs, want) {
			t.Fatalf("tree IDs %v missing %s", gotIDs, want)
		}
	}

	if firstID == "" {
		t.Fatal("test setup did not create first message")
	}
}

func TestServiceReturnsSessionLineageMetadata(t *testing.T) {
	h := newE2EHarness(t)
	ctx := t.Context()

	session, err := h.Svc.CreateSession(ctx, &pb.CreateSessionReq{
		WorkingDir:    t.TempDir(),
		Provider:      "e2e-mock",
		Model:         "mock-model",
		InitialPrompt: "lineage metadata",
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.RootId != session.Id {
		t.Fatalf("RootId = %q, want %q", session.RootId, session.Id)
	}

	got, err := h.Svc.ListSessions(ctx, &pb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sessions) != 1 {
		t.Fatalf("session count = %d, want 1", len(got.Sessions))
	}
	if got.Sessions[0].RootId != session.Id {
		t.Fatalf("listed RootId = %q, want %q", got.Sessions[0].RootId, session.Id)
	}
}

func tableColumns(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return columns
}

func insertMessage(t *testing.T, db *sql.DB, sessionID, role, content string) string {
	t.Helper()
	id := content + "-id"
	_, err := db.Exec(
		`INSERT INTO messages (id, session_id, role, content, tool_name, tool_call_id) VALUES (?, ?, ?, ?, '', '')`,
		id, sessionID, role, content,
	)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func countMessages(t *testing.T, db *sql.DB, sessionID string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, sessionID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
