package daemon

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type CompactionRecord struct {
	ID                 string
	SessionID          string
	Summary            string
	Reason             string
	MessagesRemoved    int
	MessagesKept       int
	FirstKeptMessageID string
	CreatedAt          time.Time
}

func appendCompactionRecord(ctx context.Context, db *sql.DB, record CompactionRecord) (*CompactionRecord, error) {
	record.ID = uuid.New().String()
	record.CreatedAt = time.Now().UTC()
	_, err := db.ExecContext(ctx,
		`INSERT INTO session_compactions
		 (id, session_id, summary, reason, messages_removed, messages_kept, first_kept_message_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID,
		record.SessionID,
		record.Summary,
		record.Reason,
		record.MessagesRemoved,
		record.MessagesKept,
		record.FirstKeptMessageID,
		record.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func listCompactionRecords(ctx context.Context, db *sql.DB, sessionID string) ([]CompactionRecord, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, session_id, summary, reason, messages_removed, messages_kept, COALESCE(first_kept_message_id, ''), created_at
		 FROM session_compactions
		 WHERE session_id = ?
		 ORDER BY created_at DESC, id DESC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []CompactionRecord
	for rows.Next() {
		var record CompactionRecord
		if err := rows.Scan(
			&record.ID,
			&record.SessionID,
			&record.Summary,
			&record.Reason,
			&record.MessagesRemoved,
			&record.MessagesKept,
			&record.FirstKeptMessageID,
			&record.CreatedAt,
		); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func firstKeptMessageID(messages []SessionHistoryMessage, preserveCount int) string {
	if preserveCount <= 0 || len(messages) <= preserveCount {
		return ""
	}
	return messages[len(messages)-preserveCount].ID
}

func compactionReplacementMessageIDs(messages []SessionHistoryMessage, preserveCount, compressedCount int) []string {
	if compressedCount <= 0 {
		return nil
	}
	ids := make([]string, compressedCount)
	if preserveCount <= 0 || len(messages) <= preserveCount {
		return ids
	}
	kept := messages[len(messages)-preserveCount:]
	for i, msg := range kept {
		// compressed[0] is the generated summary, so preserved messages start at index 1.
		idx := i + 1
		if idx >= len(ids) {
			break
		}
		ids[idx] = msg.ID
	}
	return ids
}
