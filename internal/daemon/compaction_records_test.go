package daemon

import "testing"

func TestCompactionRecordSchemaAppendList(t *testing.T) {
	db := setupTestDB(t)
	ctx := t.Context()

	columns := tableColumns(t, db, "session_compactions")
	for _, name := range []string{"id", "session_id", "summary", "reason", "messages_removed", "messages_kept", "first_kept_message_id", "created_at"} {
		if !containsString(columns, name) {
			t.Fatalf("session_compactions missing column %q; got %v", name, columns)
		}
	}

	record, err := appendCompactionRecord(ctx, db, CompactionRecord{
		SessionID:          "sess-1",
		Summary:            "short summary",
		Reason:             "manual",
		MessagesRemoved:    8,
		MessagesKept:       3,
		FirstKeptMessageID: "msg-9",
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.ID == "" || record.CreatedAt.IsZero() {
		t.Fatalf("record missing ID/CreatedAt: %+v", record)
	}

	records, err := listCompactionRecords(ctx, db, "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	got := records[0]
	if got.Summary != "short summary" || got.Reason != "manual" || got.MessagesRemoved != 8 || got.MessagesKept != 3 || got.FirstKeptMessageID != "msg-9" {
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
