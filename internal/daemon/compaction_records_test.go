package daemon

import "testing"

func TestCompactionRecordSchemaAppendList(t *testing.T) {
	db := setupTestDB(t)
	ctx := t.Context()
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}

	columns := tableColumns(t, db, "session_compactions")
	for _, name := range []string{"id", "session_id", "summary", "reason", "messages_removed", "messages_kept", "first_kept_message_id", "archive_session_id", "created_at"} {
		if !containsString(columns, name) {
			t.Fatalf("session_compactions missing column %q; got %v", name, columns)
		}
	}

	session, err := NewSessionManager(db).Create(ctx, t.TempDir(), "mock", "mock-model", "compaction records")
	if err != nil {
		t.Fatal(err)
	}
	record, err := appendCompactionRecord(ctx, db, CompactionRecord{
		SessionID:          session.ID,
		Summary:            "short summary",
		Reason:             "manual",
		MessagesRemoved:    8,
		MessagesKept:       3,
		FirstKeptMessageID: "msg-9",
		ArchiveSessionID:   "archive-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.ID == "" || record.CreatedAt.IsZero() {
		t.Fatalf("record missing ID/CreatedAt: %+v", record)
	}

	records, err := listCompactionRecords(ctx, db, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	got := records[0]
	if got.Summary != "short summary" || got.Reason != "manual" || got.MessagesRemoved != 8 || got.MessagesKept != 3 || got.FirstKeptMessageID != "msg-9" || got.ArchiveSessionID != "archive-1" {
		t.Fatalf("record = %+v", got)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
