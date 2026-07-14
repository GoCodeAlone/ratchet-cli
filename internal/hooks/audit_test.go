package hooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	managedAuditOutputSentinel  = "managed-output-sentinel"
	managedAuditPayloadSentinel = "managed-payload-sentinel"
	managedAuditSecretSentinel  = "managed-secret-sentinel"
)

func TestDefaultHookAuditPathFailsClosedWhenHomeUnavailable(t *testing.T) {
	original := hookAuditUserHomeDir
	t.Cleanup(func() { hookAuditUserHomeDir = original })
	hookAuditUserHomeDir = func() (string, error) {
		return "", errors.New("home unavailable")
	}

	path, err := DefaultHookAuditPath()
	if err == nil || path != "" {
		t.Fatalf("DefaultHookAuditPath = %q, %v; want empty path and error", path, err)
	}
}

func TestManagedHookAuditRecordsMetadataOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "hooks.jsonl")
	audit := NewHookAudit(path)
	hook := managedAuditTestHook(managedAuditSuccessCommand())
	cfg := &HookConfig{Hooks: map[Event][]Hook{PreCommand: {hook}}}

	if err := cfg.RunWithOptions(PreCommand, map[string]string{"payload": managedAuditPayloadSentinel}, RunOptions{Audit: audit}); err != nil {
		t.Fatalf("RunWithOptions: %v", err)
	}

	records, err := audit.Read(10)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2: %+v", len(records), records)
	}
	if records[0].Result != HookAuditSuccess || records[1].Result != HookAuditStarted {
		t.Fatalf("newest-first results = %q, %q", records[0].Result, records[1].Result)
	}
	for _, record := range records {
		if record.Timestamp.IsZero() || record.Event != PreCommand || record.Hash != hook.Hash || record.Source != SourceManaged || record.DurationMS < 0 {
			t.Fatalf("invalid audit metadata: %+v", record)
		}
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var fields map[string]any
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if len(fields) != 6 {
			t.Fatalf("audit schema fields = %v, want exactly 6", fields)
		}
		for _, key := range []string{"timestamp", "event", "hash", "source", "result", "duration_ms"} {
			if _, ok := fields[key]; !ok {
				t.Fatalf("audit schema missing %q: %v", key, fields)
			}
		}
	}
	assertManagedAuditFilePrivate(t, path)
	assertManagedAuditExcludes(t, path, hook.Command, hook.CommandWindows, managedAuditPayloadSentinel, managedAuditOutputSentinel, managedAuditSecretSentinel)
}

func TestManagedHookAuditStartFailurePreventsLaunch(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "launched")
	hook := managedAuditTestHook(managedAuditMarkerCommand(marker))
	cfg := &HookConfig{Hooks: map[Event][]Hook{PreCommand: {hook}}}
	appends := 0
	audit := hookAuditWriterFunc(func(HookAuditRecord) error {
		appends++
		return errors.New(managedAuditSecretSentinel)
	})

	err := cfg.RunWithOptions(PreCommand, map[string]string{"payload": managedAuditPayloadSentinel}, RunOptions{Audit: audit})
	if !errors.Is(err, ErrHookAuditDegraded) {
		t.Fatalf("RunWithOptions error = %v, want ErrHookAuditDegraded", err)
	}
	if errors.Is(err, ErrManagedHookCommandFailed) {
		t.Fatalf("pre-launch audit failure reported command failure: %v", err)
	}
	if appends != 1 {
		t.Fatalf("append attempts = %d, want 1", appends)
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("managed command launched before durable audit: %v", statErr)
	}
	assertManagedExecutionErrorPrivate(t, err, hook.Command, hook.CommandWindows, managedAuditPayloadSentinel, managedAuditSecretSentinel)
}

func TestManagedHookAuditTerminalFailureJoinsCommandClassificationPrivately(t *testing.T) {
	hook := managedAuditTestHook(managedAuditFailureCommand())
	cfg := &HookConfig{Hooks: map[Event][]Hook{PreCommand: {hook}}}
	var records []HookAuditRecord
	audit := hookAuditWriterFunc(func(record HookAuditRecord) error {
		records = append(records, record)
		if len(records) == 2 {
			return errors.New(managedAuditSecretSentinel)
		}
		return nil
	})

	err := cfg.RunWithOptions(PreCommand, map[string]string{"payload": managedAuditPayloadSentinel}, RunOptions{Audit: audit})
	if !errors.Is(err, ErrManagedHookCommandFailed) || !errors.Is(err, ErrHookAuditDegraded) {
		t.Fatalf("RunWithOptions error = %v, want command_failed and audit_degraded", err)
	}
	if len(records) != 2 || records[0].Result != HookAuditStarted || records[1].Result != HookAuditCommandFailed {
		t.Fatalf("append attempts = %+v", records)
	}
	assertManagedExecutionErrorPrivate(t, err, hook.Command, hook.CommandWindows, managedAuditPayloadSentinel, managedAuditOutputSentinel, managedAuditSecretSentinel, "exit status")
	for _, record := range records {
		data, marshalErr := json.Marshal(record)
		if marshalErr != nil {
			t.Fatalf("Marshal: %v", marshalErr)
		}
		for _, forbidden := range []string{hook.Command, hook.CommandWindows, managedAuditPayloadSentinel, managedAuditOutputSentinel, managedAuditSecretSentinel} {
			if forbidden != "" && strings.Contains(string(data), forbidden) {
				t.Fatalf("audit record leaked %q: %s", forbidden, data)
			}
		}
	}
}

func TestManagedHookAuditTemplateFailureIsClassifiedPrivately(t *testing.T) {
	const templateSentinel = "managed-template-secret"
	hook := managedAuditTestHook("{{if "+templateSentinel, "{{if "+templateSentinel)
	cfg := &HookConfig{Hooks: map[Event][]Hook{PreCommand: {hook}}}
	var records []HookAuditRecord
	audit := hookAuditWriterFunc(func(record HookAuditRecord) error {
		records = append(records, record)
		return nil
	})

	err := cfg.RunWithOptions(PreCommand, map[string]string{"payload": managedAuditPayloadSentinel}, RunOptions{Audit: audit})
	if !errors.Is(err, ErrManagedHookCommandFailed) || errors.Is(err, ErrHookAuditDegraded) {
		t.Fatalf("RunWithOptions error = %v, want command_failed only", err)
	}
	if len(records) != 2 || records[0].Result != HookAuditStarted || records[1].Result != HookAuditCommandFailed {
		t.Fatalf("audit records = %+v", records)
	}
	assertManagedExecutionErrorPrivate(t, err, templateSentinel, managedAuditPayloadSentinel, "template:", "unclosed action")
}

func TestManagedHookAuditPersistsDegradedResultOnRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "hooks.jsonl")
	audit := NewHookAudit(path)
	if err := audit.Append(managedAuditRecord(HookAuditStarted)); err != nil {
		t.Fatalf("Append start: %v", err)
	}
	failSync := true
	audit.syncFile = func(f *os.File) error {
		if failSync {
			return errors.New(managedAuditSecretSentinel)
		}
		return f.Sync()
	}
	failed := managedAuditRecord(HookAuditCommandFailed)
	failed.DurationMS = 7
	if err := audit.Append(failed); err == nil {
		t.Fatal("terminal Append succeeded despite sync failure")
	}

	failSync = false
	recovered := managedAuditRecord(HookAuditStarted)
	recovered.Hash = strings.Repeat("b", 64)
	if err := audit.Append(recovered); err != nil {
		t.Fatalf("recovery Append: %v", err)
	}
	records, err := audit.Read(10)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	degraded := 0
	for _, record := range records {
		if record.Result == HookAuditDegraded {
			degraded++
			if record.Event != failed.Event || record.Hash != failed.Hash || record.Source != SourceManaged {
				t.Fatalf("degraded metadata = %+v, want failed append identity", record)
			}
		}
	}
	if degraded != 1 {
		t.Fatalf("audit_degraded records = %d, want 1: %+v", degraded, records)
	}
	assertManagedAuditExcludes(t, path, managedAuditSecretSentinel)
}

func TestManagedHookAuditRetriesNamespaceSyncBeforeClearingDegraded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "hooks.jsonl")
	audit := NewHookAudit(path)
	syncCalls := 0
	audit.syncDir = func(string) error {
		syncCalls++
		if syncCalls == 1 {
			return errors.New("namespace sync failed")
		}
		return nil
	}

	if err := audit.Append(managedAuditRecord(HookAuditStarted)); err == nil {
		t.Fatal("initial Append succeeded despite namespace sync failure")
	}
	recovered := managedAuditRecord(HookAuditSuccess)
	recovered.Hash = strings.Repeat("b", 64)
	if err := audit.Append(recovered); err != nil {
		t.Fatalf("recovery Append: %v", err)
	}
	if syncCalls != 2 {
		t.Fatalf("namespace sync calls = %d, want 2", syncCalls)
	}
	records, err := audit.Read(10)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(records) != 3 || records[0].Result != HookAuditSuccess || records[1].Result != HookAuditDegraded {
		t.Fatalf("recovery records = %+v, want success then degraded marker", records)
	}
}

func TestManagedHookAuditReadIsBoundedStrictAndNewestFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "hooks.jsonl")
	audit := NewHookAudit(path)
	now := time.Now().UTC()
	for i, result := range []HookAuditResult{HookAuditStarted, HookAuditSuccess, HookAuditCommandFailed} {
		if err := audit.Append(HookAuditRecord{
			Timestamp:  now.Add(time.Duration(i) * time.Second),
			Event:      PreCommand,
			Hash:       strings.Repeat(string(rune('a'+i)), 64),
			Source:     SourceManaged,
			Result:     result,
			DurationMS: int64(i),
		}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	records, err := audit.Read(2)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(records) != 2 || records[0].Result != HookAuditCommandFailed || records[1].Result != HookAuditSuccess {
		t.Fatalf("bounded newest-first records = %+v", records)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := f.WriteString(`{"timestamp":"torn`); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	records, err = audit.Read(10)
	if err != nil || len(records) != 3 {
		t.Fatalf("Read torn tail = %d, %v, want 3 records", len(records), err)
	}
}

func TestManagedHookAuditDecodeRetainsOnlyTheRequestedLimit(t *testing.T) {
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	for i := range 2_000 {
		record := managedAuditRecord(HookAuditSuccess)
		record.Hash = fmt.Sprintf("%064x", i)
		record.DurationMS = int64(i)
		if err := encoder.Encode(record); err != nil {
			t.Fatalf("Encode %d: %v", i, err)
		}
	}

	records, err := decodeHookAuditRecords(data.Bytes(), 3)
	if err != nil {
		t.Fatalf("decodeHookAuditRecords: %v", err)
	}
	if len(records) != 3 || cap(records) > 3 {
		t.Fatalf("records len/cap = %d/%d, want 3/<=3", len(records), cap(records))
	}
	if records[0].DurationMS != 1_999 || records[1].DurationMS != 1_998 || records[2].DurationMS != 1_997 {
		t.Fatalf("newest durations = %d, %d, %d", records[0].DurationMS, records[1].DurationMS, records[2].DurationMS)
	}
}

func TestManagedHookAuditRejectsMalformedCommittedAndOversizedData(t *testing.T) {
	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "malformed committed line", data: []byte("{not-json}\n")},
		{name: "oversized file", data: make([]byte, maxHookAuditBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "private", "hooks.jsonl")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(path, test.data, 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			if _, err := NewHookAudit(path).Read(10); err == nil {
				t.Fatal("Read accepted invalid committed audit data")
			}
		})
	}
}

func TestManagedHookAuditRejectsUnsafeExistingTargets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("portable FileMode does not expose Windows DACLs")
	}
	t.Run("weak parent", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "audit")
		if err := os.Mkdir(parent, 0o755); err != nil {
			t.Fatal(err)
		}
		err := NewHookAudit(filepath.Join(parent, "hooks.jsonl")).Append(managedAuditRecord(HookAuditStarted))
		if err == nil {
			t.Fatal("Append accepted weak parent permissions")
		}
	})
	t.Run("weak file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "audit", "hooks.jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := NewHookAudit(path).Append(managedAuditRecord(HookAuditStarted)); err == nil {
			t.Fatal("Append accepted weak file permissions")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "audit")
		if err := os.MkdirAll(parent, 0o700); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(parent, "target")
		if err := os.WriteFile(target, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(parent, "hooks.jsonl")
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		if err := NewHookAudit(path).Append(managedAuditRecord(HookAuditStarted)); err == nil {
			t.Fatal("Append followed symlink target")
		}
	})
	t.Run("non-regular", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "audit", "hooks.jsonl")
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := NewHookAudit(path).Append(managedAuditRecord(HookAuditStarted)); err == nil {
			t.Fatal("Append accepted non-regular audit target")
		}
	})
}

func TestManagedHookAuditWindowsAccessRequiresProtectedOwnerOnlyFullControl(t *testing.T) {
	tests := []struct {
		name         string
		ownerMatches bool
		protected    bool
		entries      []hookAuditWindowsAccessEntry
		wantErr      bool
	}{
		{
			name:         "owner-only full control",
			ownerMatches: true,
			protected:    true,
			entries:      []hookAuditWindowsAccessEntry{{allowed: true, owner: true, fullControl: true}},
		},
		{name: "wrong owner", protected: true, entries: []hookAuditWindowsAccessEntry{{allowed: true, owner: true, fullControl: true}}, wantErr: true},
		{name: "inherited DACL", ownerMatches: true, entries: []hookAuditWindowsAccessEntry{{allowed: true, owner: true, fullControl: true}}, wantErr: true},
		{name: "empty DACL", ownerMatches: true, protected: true, wantErr: true},
		{name: "other principal", ownerMatches: true, protected: true, entries: []hookAuditWindowsAccessEntry{{allowed: true, fullControl: true}}, wantErr: true},
		{name: "partial access", ownerMatches: true, protected: true, entries: []hookAuditWindowsAccessEntry{{allowed: true, owner: true}}, wantErr: true},
		{name: "deny ACE", ownerMatches: true, protected: true, entries: []hookAuditWindowsAccessEntry{{owner: true, fullControl: true}}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateHookAuditWindowsAccess(test.ownerMatches, test.protected, test.entries)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

type hookAuditWriterFunc func(HookAuditRecord) error

func (f hookAuditWriterFunc) Append(record HookAuditRecord) error { return f(record) }

func managedAuditTestHook(command, windowsCommand string) Hook {
	cfg := &HookConfig{Hooks: map[Event][]Hook{
		PreCommand: {{Command: command, CommandWindows: windowsCommand}},
	}}
	cfg.AnnotateSource(SourceMetadata{Kind: SourceManaged, ID: managedPolicySourceID, TrustByDefault: true})
	return cfg.Hooks[PreCommand][0]
}

func managedAuditSuccessCommand() (string, string) { return "exit 0", "exit 0" }

func managedAuditFailureCommand() (string, string) {
	return "printf '" + managedAuditOutputSentinel + "' >&2; exit 7", "Write-Error '" + managedAuditOutputSentinel + "'; exit 7"
}

func managedAuditMarkerCommand(path string) (string, string) {
	return "printf launched > " + shellQuote(path), "Set-Content -LiteralPath '" + strings.ReplaceAll(path, "'", "''") + "' -Value launched"
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func managedAuditRecord(result HookAuditResult) HookAuditRecord {
	return HookAuditRecord{
		Timestamp:  time.Now().UTC(),
		Event:      PreCommand,
		Hash:       strings.Repeat("a", 64),
		Source:     SourceManaged,
		Result:     result,
		DurationMS: 0,
	}
}

func assertManagedAuditFilePrivate(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	fileInfo, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat audit: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("audit mode = %#o, want 0600", got)
	}
	parentInfo, err := os.Lstat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Lstat audit parent: %v", err)
	}
	if got := parentInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("audit parent mode = %#o, want 0700", got)
	}
}

func assertManagedAuditExcludes(t *testing.T, path string, forbidden ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, value := range forbidden {
		if value != "" && strings.Contains(string(data), value) {
			t.Fatalf("audit leaked %q: %s", value, data)
		}
	}
}

func assertManagedExecutionErrorPrivate(t *testing.T, err error, forbidden ...string) {
	t.Helper()
	for _, value := range forbidden {
		if value != "" && strings.Contains(err.Error(), value) {
			t.Fatalf("managed execution error leaked %q: %v", value, err)
		}
	}
}
