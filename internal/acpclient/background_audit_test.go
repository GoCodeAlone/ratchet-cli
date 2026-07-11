package acpclient

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		var object map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &object); err != nil {
			t.Fatalf("Unmarshal audit object: %v", err)
		}
		allowed := map[string]bool{"at": true, "action": true, "sessionId": true, "profile": true, "descriptorHash": true, "outcome": true}
		for key := range object {
			if !allowed[key] {
				t.Fatalf("audit contains unexpected key %q: %#v", key, object)
			}
		}
		if len(object) != len(allowed) {
			t.Fatalf("audit keys = %#v, want exactly metadata keys", object)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("Scan audit: %v", err)
	}
}

func TestBackgroundAuditCoordinatesConcurrentHandles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "background-audit.jsonl")
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	const count = 64
	start := make(chan struct{})
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Go(func() {
			<-start
			errs <- NewBackgroundAudit(path).Append(BackgroundAuditRecord{
				At:             now,
				Action:         BackgroundAuditError,
				SessionID:      fmt.Sprintf("session-%02d", i),
				Profile:        "fixture",
				DescriptorHash: "descriptor-hash",
				Outcome:        BackgroundOutcomeWorkerError,
			})
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	records, err := NewBackgroundAudit(path).Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(records) != count {
		t.Fatalf("records len = %d, want %d", len(records), count)
	}
}
