package acpclient

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	path = audit.Path()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	records := []BackgroundAuditRecord{
		backgroundAuditTestRecord("event-start", BackgroundAuditStart, BackgroundOutcomeStarted),
		backgroundAuditTestRecord("event-resume", BackgroundAuditResume, BackgroundOutcomeResumed),
		backgroundAuditTestRecord("event-block", BackgroundAuditBlock, BackgroundOutcomeProfileDrift),
		backgroundAuditTestRecord("event-error", BackgroundAuditError, BackgroundOutcomeWorkerError),
		backgroundAuditTestRecord("event-stop", BackgroundAuditStop, BackgroundOutcomeStopped),
	}
	for _, record := range records {
		record.At = now
		if err := audit.Append(record); err != nil {
			t.Fatalf("Append(%q): %v", record.Action, err)
		}
	}

	records, err := audit.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(records) != 5 {
		t.Fatalf("records len = %d, want 5", len(records))
	}
	for i, record := range records {
		if record.SessionID != "session-1" || record.Profile != "fixture" || record.DescriptorHash != "descriptor-hash" {
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
	lockInfo, err := os.Stat(requireStoreLockPhysicalPath(t, path+".lock"))
	if err != nil {
		t.Fatalf("Stat audit lock: %v", err)
	}
	if got := lockInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("audit lock mode = %o, want 600", got)
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
		allowed := map[string]bool{"recordId": true, "at": true, "action": true, "sessionId": true, "profile": true, "descriptorHash": true, "outcome": true}
		for key := range object {
			if !allowed[key] {
				t.Fatalf("audit contains unexpected key %q: %#v", key, object)
			}
		}
		if len(object) != len(allowed) {
			t.Fatalf("audit keys = %#v, want exactly metadata keys", object)
		}
		if recordID, _ := object["recordId"].(string); recordID == "" {
			t.Fatalf("audit record ID is empty: %#v", object)
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
			record := backgroundAuditTestRecord(fmt.Sprintf("event-%02d", i), BackgroundAuditError, BackgroundOutcomeWorkerError)
			record.At = now
			record.SessionID = fmt.Sprintf("session-%02d", i)
			errs <- NewBackgroundAudit(path).Append(record)
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

func TestBackgroundAuditEveryAppendRequiresParentSync(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "background-audit.jsonl")
	syncErr := errors.New("parent sync failed")
	syncCalls := 0
	audit := NewBackgroundAudit(path)
	path = audit.Path()
	audit.syncParent = func(gotDir string) error {
		syncCalls++
		if gotDir != filepath.Dir(path) {
			t.Fatalf("sync parent = %q, want %q", gotDir, filepath.Dir(path))
		}
		if syncCalls <= 2 {
			return syncErr
		}
		return nil
	}
	record := BackgroundAuditRecord{
		RecordID:       "event-error",
		At:             time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		Action:         BackgroundAuditError,
		SessionID:      "session-1",
		Profile:        "fixture",
		DescriptorHash: "descriptor-hash",
		Outcome:        BackgroundOutcomeWorkerError,
	}
	if err := audit.Append(record); !errors.Is(err, syncErr) || !errors.Is(err, ErrStoreCommitUnconfirmed) {
		t.Fatalf("first Append error = %v, want commit-unconfirmed parent sync failure", err)
	}
	if syncCalls != 1 {
		t.Fatalf("parent sync calls = %d, want 1", syncCalls)
	}
	records, err := audit.Read()
	if err != nil {
		t.Fatalf("Read after sync failure: %v", err)
	}
	if len(records) != 1 || records[0].Action != BackgroundAuditError {
		t.Fatalf("records after sync failure = %#v", records)
	}

	record.Action = BackgroundAuditStop
	record.RecordID = "event-stop"
	record.Outcome = BackgroundOutcomeStopped
	if err := audit.Append(record); !errors.Is(err, syncErr) || !errors.Is(err, ErrStoreCommitUnconfirmed) {
		t.Fatalf("retry Append error = %v, want commit-unconfirmed parent sync failure", err)
	}
	if syncCalls != 2 {
		t.Fatalf("parent sync calls after retry = %d, want 2", syncCalls)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove audit WAL: %v", err)
	}
	record.Action = BackgroundAuditResume
	record.RecordID = "event-resume"
	record.Outcome = BackgroundOutcomeResumed
	if err := audit.Append(record); err != nil {
		t.Fatalf("Append after deletion: %v", err)
	}
	if syncCalls != 3 {
		t.Fatalf("parent sync calls after recreation = %d, want 3", syncCalls)
	}
}

func TestBackgroundAuditRequiresRecordID(t *testing.T) {
	record := backgroundAuditTestRecord("", BackgroundAuditStart, BackgroundOutcomeStarted)
	err := NewBackgroundAudit(filepath.Join(t.TempDir(), "background-audit.jsonl")).Append(record)
	if err == nil || !strings.Contains(err.Error(), "record id") {
		t.Fatalf("Append error = %v, want required record ID", err)
	}
}

func TestBackgroundAuditRequiresPath(t *testing.T) {
	err := NewBackgroundAudit("").Append(backgroundAuditTestRecord("event-path", BackgroundAuditStart, BackgroundOutcomeStarted))
	if err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("Append error = %v, want required audit path", err)
	}
}

func TestBackgroundAuditValidatesRequiredFieldsAndActionOutcomeMatrix(t *testing.T) {
	valid := backgroundAuditTestRecord("event-valid", BackgroundAuditStart, BackgroundOutcomeStarted)
	cases := map[string]func(*BackgroundAuditRecord){
		"at":              func(record *BackgroundAuditRecord) { record.At = time.Time{} },
		"action":          func(record *BackgroundAuditRecord) { record.Action = "future" },
		"sessionId":       func(record *BackgroundAuditRecord) { record.SessionID = "" },
		"profile":         func(record *BackgroundAuditRecord) { record.Profile = "" },
		"descriptorHash":  func(record *BackgroundAuditRecord) { record.DescriptorHash = "" },
		"outcome":         func(record *BackgroundAuditRecord) { record.Outcome = "" },
		"action/outcome":  func(record *BackgroundAuditRecord) { record.Outcome = BackgroundOutcomeStopped },
		"unknown outcome": func(record *BackgroundAuditRecord) { record.Outcome = "future" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			record := valid
			mutate(&record)
			if err := NewBackgroundAudit(filepath.Join(t.TempDir(), "audit.jsonl")).Append(record); err == nil {
				t.Fatalf("Append(%#v) succeeded", record)
			}
		})
	}
}

func TestBackgroundAuditReadRepairsOnlyIncompleteTail(t *testing.T) {
	audit := NewBackgroundAudit(filepath.Join(t.TempDir(), "background-audit.jsonl"))
	path := audit.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	committed := backgroundAuditTestJSON(t, backgroundAuditTestRecord("event-1", BackgroundAuditStart, BackgroundOutcomeStarted))
	if err := os.WriteFile(path, append(append(committed, '\n'), []byte(`{"recordId":"torn`)...), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	records, err := audit.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(records) != 1 || records[0].RecordID != "event-1" {
		t.Fatalf("records = %#v, want committed prefix", records)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := append(committed, '\n')
	if string(raw) != string(want) {
		t.Fatalf("repaired audit = %q, want %q", raw, want)
	}
}

func TestBackgroundAuditRejectsMalformedCommittedRecord(t *testing.T) {
	audit := NewBackgroundAudit(filepath.Join(t.TempDir(), "background-audit.jsonl"))
	path := audit.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("{malformed}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := audit.Read(); err == nil {
		t.Fatal("Read accepted malformed newline-committed record")
	}
	record := backgroundAuditTestRecord("event-2", BackgroundAuditStop, BackgroundOutcomeStopped)
	if err := audit.Append(record); err == nil {
		t.Fatal("Append accepted malformed newline-committed record")
	}
}

func TestBackgroundAuditIgnoresUnknownJSONFields(t *testing.T) {
	audit := NewBackgroundAudit(filepath.Join(t.TempDir(), "background-audit.jsonl"))
	path := audit.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	record := backgroundAuditTestRecord("event-1", BackgroundAuditStart, BackgroundOutcomeStarted)
	line := strings.TrimSuffix(string(backgroundAuditTestJSON(t, record)), "}") + `,"futureFormatField":{"enabled":true}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	records, err := audit.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(records) != 1 || records[0] != record {
		t.Fatalf("records = %#v, want %#v", records, record)
	}
}

func TestBackgroundAuditDeduplicatesCommittedRecordIDAcrossInterleaving(t *testing.T) {
	audit := NewBackgroundAudit(filepath.Join(t.TempDir(), "background-audit.jsonl"))
	first := backgroundAuditTestRecord("event-a", BackgroundAuditError, BackgroundOutcomeWorkerError)
	interleaved := backgroundAuditTestRecord("event-b", BackgroundAuditStop, BackgroundOutcomeStopped)
	for _, record := range []BackgroundAuditRecord{first, interleaved, first} {
		if err := audit.Append(record); err != nil {
			t.Fatalf("Append(%s): %v", record.RecordID, err)
		}
	}
	records, err := audit.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(records) != 2 || records[0].RecordID != "event-a" || records[1].RecordID != "event-b" {
		t.Fatalf("records = %#v, want one committed record per ID", records)
	}
}

func TestBackgroundAuditKeepsDistinctIDsWithIdenticalMetadata(t *testing.T) {
	audit := NewBackgroundAudit(filepath.Join(t.TempDir(), "background-audit.jsonl"))
	first := backgroundAuditTestRecord("event-a", BackgroundAuditStop, BackgroundOutcomeStopped)
	second := first
	second.RecordID = "event-b"
	for _, record := range []BackgroundAuditRecord{first, second} {
		if err := audit.Append(record); err != nil {
			t.Fatalf("Append(%s): %v", record.RecordID, err)
		}
	}
	records, err := audit.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %#v, want two logical events", records)
	}
}

func TestBackgroundAuditAppendFailureStagesReconcileByRecordID(t *testing.T) {
	stageErr := errors.New("injected audit stage failure")
	tests := []struct {
		name   string
		inject func(*BackgroundAudit)
		want   error
	}{
		{
			name: "partial write",
			inject: func(audit *BackgroundAudit) {
				audit.writeFile = func(f *os.File, data []byte) (int, error) {
					return f.Write(data[:len(data)/2])
				}
			},
			want: io.ErrShortWrite,
		},
		{
			name: "complete write error",
			inject: func(audit *BackgroundAudit) {
				audit.writeFile = func(f *os.File, data []byte) (int, error) {
					n, err := f.Write(data)
					return n, errors.Join(err, stageErr)
				}
			},
			want: ErrStoreCommitUnconfirmed,
		},
		{
			name: "sync",
			inject: func(audit *BackgroundAudit) {
				audit.syncFile = func(*os.File) error { return stageErr }
			},
			want: ErrStoreCommitUnconfirmed,
		},
		{
			name: "close",
			inject: func(audit *BackgroundAudit) {
				audit.closeFile = func(f *os.File) error { return errors.Join(f.Close(), stageErr) }
			},
			want: ErrStoreCommitUnconfirmed,
		},
		{
			name: "parent sync",
			inject: func(audit *BackgroundAudit) {
				audit.syncParent = func(string) error { return stageErr }
			},
			want: ErrStoreCommitUnconfirmed,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			audit := NewBackgroundAudit(filepath.Join(t.TempDir(), "background-audit.jsonl"))
			tc.inject(audit)
			record := backgroundAuditTestRecord("event-retry", BackgroundAuditError, BackgroundOutcomeWorkerError)
			if err := audit.Append(record); !errors.Is(err, tc.want) {
				t.Fatalf("Append error = %v, want %v", err, tc.want)
			}
			audit.writeFile = nil
			audit.syncFile = nil
			audit.closeFile = nil
			audit.syncParent = nil
			if err := audit.Append(record); err != nil {
				t.Fatalf("retry Append: %v", err)
			}
			records, err := audit.Read()
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if len(records) != 1 || records[0].RecordID != record.RecordID {
				t.Fatalf("records = %#v, want one reconciled ID", records)
			}
		})
	}
}

func TestBackgroundAuditRepairFailurePreventsReadAndAppend(t *testing.T) {
	audit := NewBackgroundAudit(filepath.Join(t.TempDir(), "background-audit.jsonl"))
	path := audit.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	committed := backgroundAuditTestJSON(t, backgroundAuditTestRecord("event-1", BackgroundAuditStart, BackgroundOutcomeStarted))
	original := append(append(committed, '\n'), []byte(`{"recordId":"torn`)...)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	repairErr := errors.New("injected repair failure")
	audit.repairFile = func(*os.File, int64) error { return repairErr }
	if _, err := audit.Read(); !errors.Is(err, repairErr) {
		t.Fatalf("Read error = %v, want repair failure", err)
	}
	record := backgroundAuditTestRecord("event-2", BackgroundAuditStop, BackgroundOutcomeStopped)
	if err := audit.Append(record); !errors.Is(err, repairErr) {
		t.Fatalf("Append error = %v, want repair failure", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(raw) != string(original) {
		t.Fatalf("failed repair mutated audit = %q, want %q", raw, original)
	}
}

func TestBackgroundAuditUnsupportedProcessLockFailsBeforeMutation(t *testing.T) {
	audit := NewBackgroundAudit(filepath.Join(t.TempDir(), "background-audit.jsonl"))
	mutated := false
	audit.openTransaction = func(string, bool) (backgroundAuditTransaction, error) {
		return nil, ErrStoreProcessLockUnsupported
	}
	audit.beforeMutation = func() { mutated = true }
	if err := audit.Append(backgroundAuditTestRecord("unsupported-lock", BackgroundAuditError, BackgroundOutcomeWorkerError)); !errors.Is(err, ErrStoreProcessLockUnsupported) {
		t.Fatalf("Append error = %v, want ErrStoreProcessLockUnsupported", err)
	}
	if mutated {
		t.Fatal("audit mutation began before unsupported process lock failure")
	}
	if _, err := os.Stat(filepath.Dir(audit.Path())); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("audit namespace after unsupported process lock = %v, want not exist", err)
	}
}

func TestBackgroundAuditPersistsRecordID(t *testing.T) {
	audit := NewBackgroundAudit(filepath.Join(t.TempDir(), "background-audit.jsonl"))
	record := BackgroundAuditRecord{
		RecordID:       "event-123",
		At:             time.Date(2026, 7, 13, 13, 0, 0, 0, time.UTC),
		Action:         BackgroundAuditStop,
		SessionID:      "session-1",
		Profile:        "fixture",
		DescriptorHash: "descriptor-hash",
		Outcome:        BackgroundOutcomeStopped,
	}
	if err := audit.Append(record); err != nil {
		t.Fatalf("Append: %v", err)
	}
	records, err := audit.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(records) != 1 || records[0].RecordID != record.RecordID {
		t.Fatalf("records = %#v, want persisted record ID", records)
	}
}

func backgroundAuditTestRecord(recordID, action, outcome string) BackgroundAuditRecord {
	return BackgroundAuditRecord{
		RecordID:       recordID,
		At:             time.Date(2026, 7, 13, 13, 0, 0, 0, time.UTC),
		Action:         action,
		SessionID:      "session-1",
		Profile:        "fixture",
		DescriptorHash: "descriptor-hash",
		Outcome:        outcome,
	}
}

func backgroundAuditTestJSON(t *testing.T, record BackgroundAuditRecord) []byte {
	t.Helper()
	line, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return line
}
