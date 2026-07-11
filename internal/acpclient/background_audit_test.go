package acpclient

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackgroundAuditAppendsOwnerOnlyMetadataRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "background-audit.jsonl")
	audit := NewBackgroundAudit(path)
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	actions := []string{
		BackgroundAuditStart,
		BackgroundAuditResume,
		BackgroundAuditBlock,
		BackgroundAuditError,
		BackgroundAuditStop,
	}
	for _, action := range actions {
		if err := audit.Append(BackgroundAuditRecord{
			At:             now,
			Action:         action,
			SessionID:      "session-1",
			Profile:        "fixture",
			DescriptorHash: "descriptor-hash",
			Outcome:        "classified-outcome",
		}); err != nil {
			t.Fatalf("Append(%q): %v", action, err)
		}
	}

	records, err := audit.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(records) != len(actions) {
		t.Fatalf("records len = %d, want %d", len(records), len(actions))
	}
	for i, record := range records {
		if record.Action != actions[i] || record.SessionID != "session-1" || record.Profile != "fixture" || record.DescriptorHash != "descriptor-hash" || record.Outcome != "classified-outcome" {
			t.Fatalf("record[%d] = %#v", i, record)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("audit mode = %o, want 600", got)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, forbidden := range []string{"prompt", "response", "argv", "envValue", "credential", "secret-value"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("audit contains forbidden %q metadata: %s", forbidden, raw)
		}
	}
}
