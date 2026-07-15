package hooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
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

func TestDefaultHookAuditPathRejectsRelativeHome(t *testing.T) {
	original := hookAuditUserHomeDir
	t.Cleanup(func() { hookAuditUserHomeDir = original })
	hookAuditUserHomeDir = func() (string, error) { return filepath.Join("relative", "home"), nil }

	path, err := DefaultHookAuditPath()
	if err == nil || path != "" {
		t.Fatalf("DefaultHookAuditPath = %q, %v; want empty path and error", path, err)
	}
}

func TestManagedHookAuditRejectsRelativePath(t *testing.T) {
	t.Chdir(t.TempDir())
	audit := NewHookAudit(filepath.Join(".ratchet", "audit", "hooks.jsonl"))
	for name, operation := range map[string]func() error{
		"Append": func() error { return audit.Append(managedAuditRecord(HookAuditStarted)) },
		"Read":   func() error { _, err := audit.Read(1); return err },
	} {
		t.Run(name, func(t *testing.T) {
			err := operation()
			if err == nil || !strings.Contains(err.Error(), "absolute path") {
				t.Fatalf("%s error = %v, want absolute-path requirement", name, err)
			}
		})
	}
}

func TestManagedHookAuditRejectsDeepMissingNamespace(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "one", "two", "three", "hooks.jsonl")
	err := NewHookAudit(path).Append(managedAuditRecord(HookAuditStarted))
	if err == nil || !strings.Contains(err.Error(), "trusted anchor") {
		t.Fatalf("Append error = %v, want trusted-anchor requirement", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "one")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("deep namespace was partially created: %v", statErr)
	}
}

func TestManagedHookAuditRecordsMetadataOnly(t *testing.T) {
	path := managedAuditTestPath(t)
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

func TestManagedHookCommandDiscardsOutput(t *testing.T) {
	command, windowsCommand := managedAuditFailureCommand()
	if runtime.GOOS == "windows" {
		command = windowsCommand
	}
	cmd := execHookCommand(command)
	if err := runManagedHookCommand(cmd); err == nil {
		t.Fatal("runManagedHookCommand succeeded for failing command")
	}
	if cmd.Stdout != io.Discard || cmd.Stderr != io.Discard {
		t.Fatalf("managed command output writers = %T/%T, want io.Discard", cmd.Stdout, cmd.Stderr)
	}
}

func TestManagedHookDurationKeepsTheOriginalClockReading(t *testing.T) {
	originalNow, originalSince := managedHookNow, managedHookSince
	t.Cleanup(func() {
		managedHookNow = originalNow
		managedHookSince = originalSince
	})
	start := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.FixedZone("test", -4*60*60))
	nowCalls := 0
	managedHookNow = func() time.Time {
		nowCalls++
		if nowCalls == 1 {
			return start
		}
		return start.Add(time.Second)
	}
	var elapsedFrom time.Time
	managedHookSince = func(got time.Time) time.Duration {
		elapsedFrom = got
		return 17 * time.Millisecond
	}

	var records []HookAuditRecord
	audit := hookAuditWriterFunc(func(record HookAuditRecord) error {
		records = append(records, record)
		return nil
	})
	hook := managedAuditTestHook(managedAuditSuccessCommand())
	cfg := &HookConfig{Hooks: map[Event][]Hook{PreCommand: {hook}}}
	if err := cfg.RunWithOptions(PreCommand, nil, RunOptions{Audit: audit}); err != nil {
		t.Fatalf("RunWithOptions: %v", err)
	}
	if elapsedFrom != start {
		t.Fatalf("duration start = %v, want original %v", elapsedFrom, start)
	}
	if len(records) != 2 || records[0].Timestamp.Location() != time.UTC || records[1].Timestamp.Location() != time.UTC || records[1].DurationMS != 17 {
		t.Fatalf("audit timing records = %+v", records)
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
	path := managedAuditTestPath(t)
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
	path := managedAuditTestPath(t)
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
	if syncCalls != 4 {
		t.Fatalf("namespace sync calls = %d, want 4", syncCalls)
	}
	records, err := audit.Read(10)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(records) != 3 || records[0].Result != HookAuditSuccess || records[1].Result != HookAuditDegraded {
		t.Fatalf("recovery records = %+v, want success then degraded marker", records)
	}
}

func TestManagedHookAuditSyncsFixedNamespaceChain(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ratchet", "audit", "hooks.jsonl")
	audit := NewHookAudit(path)
	var synced []string
	audit.syncDir = func(path string) error {
		synced = append(synced, path)
		return nil
	}
	if err := audit.Append(managedAuditRecord(HookAuditStarted)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	want := []string{filepath.Dir(path), filepath.Dir(filepath.Dir(path)), filepath.Dir(filepath.Dir(filepath.Dir(path)))}
	if !slices.Equal(synced, want) {
		t.Fatalf("synced directories = %v, want %v", synced, want)
	}
}

func TestManagedHookAuditRejectsParentReplacementDuringAppend(t *testing.T) {
	path := managedAuditTestPath(t)
	audit := NewHookAudit(path)
	if err := audit.Append(managedAuditRecord(HookAuditStarted)); err != nil {
		t.Fatalf("seed Append: %v", err)
	}

	parent := filepath.Dir(path)
	displaced := parent + ".displaced"
	audit.syncFile = func(file *os.File) error {
		if err := file.Sync(); err != nil {
			return err
		}
		if err := os.Rename(parent, displaced); err != nil {
			return fmt.Errorf("replace audit parent: %w", err)
		}
		replacement, _, err := openHookAuditFile(path, true)
		if err != nil {
			return fmt.Errorf("create replacement audit: %w", err)
		}
		return replacement.Close()
	}

	err := audit.Append(managedAuditRecord(HookAuditSuccess))
	if err == nil || !strings.Contains(err.Error(), "target changed during open") {
		t.Fatalf("Append error = %v, want parent-replacement identity failure", err)
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("Stat replacement: %v", statErr)
	}
	if info.Size() != 0 {
		t.Fatalf("replacement audit size = %d, want no committed record", info.Size())
	}
}

func TestManagedHookAuditRejectsAnchorReplacementDuringAppend(t *testing.T) {
	base := t.TempDir()
	anchor := filepath.Join(base, "home")
	if err := os.Mkdir(anchor, 0o700); err != nil {
		t.Fatalf("Mkdir anchor: %v", err)
	}
	path := filepath.Join(anchor, ".ratchet", "audit", "hooks.jsonl")
	audit := NewHookAudit(path)
	if err := audit.Append(managedAuditRecord(HookAuditStarted)); err != nil {
		t.Fatalf("seed Append: %v", err)
	}

	displaced := anchor + ".displaced"
	audit.syncFile = func(file *os.File) error {
		if err := file.Sync(); err != nil {
			return err
		}
		if err := os.Rename(anchor, displaced); err != nil {
			return fmt.Errorf("replace audit anchor: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("create replacement namespace: %w", err)
		}
		if err := os.Chmod(filepath.Dir(filepath.Dir(path)), 0o700); err != nil {
			return fmt.Errorf("secure replacement namespace root: %w", err)
		}
		if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("secure replacement namespace: %w", err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			return fmt.Errorf("create replacement audit: %w", err)
		}
		return nil
	}

	err := audit.Append(managedAuditRecord(HookAuditSuccess))
	if err == nil {
		t.Fatal("Append succeeded after trusted-anchor replacement attempt")
	}
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("Stat replacement audit: %v", statErr)
		}
		if info.Size() != 0 {
			t.Fatalf("replacement audit size = %d, want no canonical record", info.Size())
		}
	}
}

func TestManagedHookAuditRejectsAnchorReplacementBeforeReadUnlock(t *testing.T) {
	base := t.TempDir()
	anchor := filepath.Join(base, "home")
	if err := os.Mkdir(anchor, 0o700); err != nil {
		t.Fatalf("Mkdir anchor: %v", err)
	}
	path := filepath.Join(anchor, ".ratchet", "audit", "hooks.jsonl")
	audit := NewHookAudit(path)
	if err := audit.Append(managedAuditRecord(HookAuditStarted)); err != nil {
		t.Fatalf("seed Append: %v", err)
	}
	displaced := anchor + ".displaced"
	audit.beforeProcessUnlock = func() error {
		if err := os.Rename(anchor, displaced); err != nil {
			return fmt.Errorf("replace audit anchor: %w", err)
		}
		if err := os.Mkdir(anchor, 0o700); err != nil {
			return fmt.Errorf("create replacement anchor: %w", err)
		}
		return nil
	}

	if _, err := audit.Read(1); err == nil {
		t.Fatal("Read succeeded after trusted-anchor replacement attempt")
	}
}

func TestManagedHookAuditSerializesAcrossProcesses(t *testing.T) {
	if os.Getenv("RATCHET_AUDIT_PROCESS_CHILD") == "1" {
		audit := NewHookAudit(os.Getenv("RATCHET_AUDIT_PROCESS_PATH"))
		audit.beforeProcessLock = func() error {
			return os.WriteFile(os.Getenv("RATCHET_AUDIT_PROCESS_ATTEMPTED"), nil, 0o600)
		}
		audit.afterProcessLock = func() error {
			return os.WriteFile(os.Getenv("RATCHET_AUDIT_PROCESS_ACQUIRED"), nil, 0o600)
		}
		if err := audit.Append(managedAuditRecord(HookAuditSuccess)); err != nil {
			t.Fatalf("child Append: %v", err)
		}
		return
	}

	root := t.TempDir()
	path := filepath.Join(root, ".ratchet", "audit", "hooks.jsonl")
	attempted := filepath.Join(root, "child-attempted")
	acquired := filepath.Join(root, "child-acquired")
	audit := NewHookAudit(path)
	entered := make(chan struct{})
	release := make(chan struct{})
	audit.syncFile = func(file *os.File) error {
		if err := file.Sync(); err != nil {
			return err
		}
		close(entered)
		<-release
		return nil
	}
	parentDone := make(chan error, 1)
	go func() { parentDone <- audit.Append(managedAuditRecord(HookAuditStarted)) }()
	<-entered

	command := exec.Command(os.Args[0], "-test.run=^TestManagedHookAuditSerializesAcrossProcesses$")
	command.Env = append(os.Environ(),
		"RATCHET_AUDIT_PROCESS_CHILD=1",
		"RATCHET_AUDIT_PROCESS_PATH="+path,
		"RATCHET_AUDIT_PROCESS_ATTEMPTED="+attempted,
		"RATCHET_AUDIT_PROCESS_ACQUIRED="+acquired,
	)
	if err := command.Start(); err != nil {
		close(release)
		t.Fatalf("start audit child: %v", err)
	}
	childDone := make(chan error, 1)
	go func() { childDone <- command.Wait() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(attempted); err == nil {
			break
		}
		if time.Now().After(deadline) {
			close(release)
			t.Fatal("timed out waiting for child to reach the OS lock syscall")
		}
		time.Sleep(10 * time.Millisecond)
	}

	var childErr error
	childFinishedEarly := false
	select {
	case childErr = <-childDone:
		childFinishedEarly = true
	case <-time.After(200 * time.Millisecond):
	}
	if _, err := os.Stat(acquired); err == nil {
		close(release)
		t.Fatal("child acquired the process lock while parent transaction was held")
	} else if !errors.Is(err, os.ErrNotExist) {
		close(release)
		t.Fatalf("inspect child acquired marker: %v", err)
	}
	close(release)
	if err := <-parentDone; err != nil {
		t.Fatalf("parent Append: %v", err)
	}
	if !childFinishedEarly {
		childErr = <-childDone
	}
	if childErr != nil {
		t.Fatalf("child process: %v", childErr)
	}
	if childFinishedEarly {
		t.Fatal("child audit transaction completed while parent transaction was held")
	}
	if _, err := os.Stat(acquired); err != nil {
		t.Fatalf("child never acquired process lock after release: %v", err)
	}
}

func TestManagedHookExecutionRecomputesAuditHash(t *testing.T) {
	path := managedAuditTestPath(t)
	hook := managedAuditTestHook(managedAuditSuccessCommand())
	staleHash := strings.Repeat("f", 64)
	hook.Hash = staleHash
	cfg := &HookConfig{Hooks: map[Event][]Hook{PreCommand: {hook}}}

	if err := cfg.RunWithOptions(PreCommand, nil, RunOptions{Audit: NewHookAudit(path)}); err != nil {
		t.Fatalf("RunWithOptions: %v", err)
	}
	records, err := NewHookAudit(path).Read(2)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	wantHash := hook.DescriptorHash()
	for _, record := range records {
		if record.Hash != wantHash || record.Hash == staleHash {
			t.Fatalf("audit hash = %q, want current descriptor %q", record.Hash, wantHash)
		}
	}
}

func TestHookAuditNamespaceSyncDirectoriesIncludesRootHome(t *testing.T) {
	path := filepath.Join(string(filepath.Separator), ".ratchet", "audit", "hooks.jsonl")
	want := []string{
		filepath.Join(string(filepath.Separator), ".ratchet", "audit"),
		filepath.Join(string(filepath.Separator), ".ratchet"),
		string(filepath.Separator),
	}
	if got := hookAuditNamespaceSyncDirectories(path); !slices.Equal(got, want) {
		t.Fatalf("sync directories = %v, want %v", got, want)
	}
}

func TestManagedHookAuditStartedDurationDiagnostic(t *testing.T) {
	record := managedAuditRecord(HookAuditStarted)
	record.DurationMS = 1

	err := validateHookAuditRecord(record)
	if err == nil || !strings.Contains(err.Error(), "started duration must be zero") {
		t.Fatalf("validateHookAuditRecord error = %v, want started-duration diagnostic", err)
	}
}

func TestManagedHookAuditReadIsBoundedStrictAndNewestFirst(t *testing.T) {
	path := managedAuditTestPath(t)
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

func TestManagedHookAuditReadLocksExistingGenerations(t *testing.T) {
	path := managedAuditTestPath(t)
	audit := NewHookAudit(path)
	if err := audit.Append(managedAuditRecord(HookAuditStarted)); err != nil {
		t.Fatalf("seed Append: %v", err)
	}
	before := false
	after := false
	audit.beforeProcessLock = func() error {
		before = true
		return nil
	}
	audit.afterProcessLock = func() error {
		after = true
		return nil
	}
	if _, err := audit.Read(1); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !before || !after {
		t.Fatalf("read process-lock observations = before %v, after %v; want both", before, after)
	}
}

func TestManagedHookAuditReadHoldsProcessLockThroughGenerationRead(t *testing.T) {
	if os.Getenv("RATCHET_AUDIT_READ_LOCK_CHILD") == "1" {
		audit := NewHookAudit(os.Getenv("RATCHET_AUDIT_READ_LOCK_PATH"))
		audit.beforeProcessLock = func() error {
			return os.WriteFile(os.Getenv("RATCHET_AUDIT_READ_LOCK_ATTEMPTED"), nil, 0o600)
		}
		audit.afterProcessLock = func() error {
			return os.WriteFile(os.Getenv("RATCHET_AUDIT_READ_LOCK_ACQUIRED"), nil, 0o600)
		}
		if err := audit.Append(managedAuditRecord(HookAuditSuccess)); err != nil {
			t.Fatalf("child Append: %v", err)
		}
		return
	}

	root := t.TempDir()
	path := filepath.Join(root, ".ratchet", "audit", "hooks.jsonl")
	audit := NewHookAudit(path)
	if err := audit.Append(managedAuditRecord(HookAuditStarted)); err != nil {
		t.Fatalf("seed Append: %v", err)
	}
	readParsed := make(chan struct{})
	releaseRead := make(chan struct{})
	audit.beforeProcessUnlock = func() error {
		close(readParsed)
		<-releaseRead
		return nil
	}
	readDone := make(chan error, 1)
	go func() {
		_, err := audit.Read(1)
		readDone <- err
	}()
	<-readParsed

	attempted := filepath.Join(root, "writer-attempted")
	acquired := filepath.Join(root, "writer-acquired")
	command := exec.Command(os.Args[0], "-test.run=^TestManagedHookAuditReadHoldsProcessLockThroughGenerationRead$")
	command.Env = append(os.Environ(),
		"RATCHET_AUDIT_READ_LOCK_CHILD=1",
		"RATCHET_AUDIT_READ_LOCK_PATH="+path,
		"RATCHET_AUDIT_READ_LOCK_ATTEMPTED="+attempted,
		"RATCHET_AUDIT_READ_LOCK_ACQUIRED="+acquired,
	)
	if err := command.Start(); err != nil {
		close(releaseRead)
		t.Fatalf("start audit child: %v", err)
	}
	childDone := make(chan error, 1)
	go func() { childDone <- command.Wait() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(attempted); err == nil {
			break
		}
		if time.Now().After(deadline) {
			close(releaseRead)
			t.Fatal("timed out waiting for writer to reach the OS lock syscall")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(acquired); err == nil {
		close(releaseRead)
		t.Fatal("writer acquired process lock before generation read released it")
	} else if !errors.Is(err, os.ErrNotExist) {
		close(releaseRead)
		t.Fatalf("inspect writer acquired marker: %v", err)
	}
	select {
	case err := <-childDone:
		close(releaseRead)
		t.Fatalf("writer exited while read lock was held: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	close(releaseRead)
	if err := <-readDone; err != nil {
		t.Fatalf("Read: %v", err)
	}
	if err := <-childDone; err != nil {
		t.Fatalf("child process: %v", err)
	}
	if _, err := os.Stat(acquired); err != nil {
		t.Fatalf("writer never acquired process lock after read release: %v", err)
	}
}

func TestManagedHookAuditReadDoesNotCreateAbsentNamespace(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".ratchet", "audit", "hooks.jsonl")
	if records, err := NewHookAudit(path).Read(1); err != nil || len(records) != 0 {
		t.Fatalf("Read absent = %+v, %v", records, err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absent Read created namespace: %v", err)
	}
}

func TestManagedHookAuditReadDoesNotCreateLockForEmptyNamespace(t *testing.T) {
	path := managedAuditTestPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if records, err := NewHookAudit(path).Read(1); err != nil || len(records) != 0 {
		t.Fatalf("Read empty namespace = %+v, %v", records, err)
	}
	if _, err := os.Stat(path + hookAuditProcessLockSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty Read created process lock: %v", err)
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

func TestManagedHookAuditRotatesBeforeCapacityDisablesExecution(t *testing.T) {
	path := managedAuditTestPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	seed := managedAuditRecord(HookAuditSuccess)
	encoded, err := json.Marshal(seed)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	line := append(encoded, '\n')
	data := bytes.Repeat(line, maxHookAuditBytes/len(line))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	next := managedAuditRecord(HookAuditStarted)
	next.Hash = strings.Repeat("b", 64)
	audit := NewHookAudit(path)
	if err := audit.Append(next); err != nil {
		t.Fatalf("Append at capacity: %v", err)
	}
	records, err := audit.Read(10)
	if err != nil {
		t.Fatalf("Read active audit: %v", err)
	}
	if len(records) != 10 || records[0].Hash != next.Hash || records[1].Hash != seed.Hash {
		t.Fatalf("combined records = %+v, want active followed by archive", records)
	}
	archivePath := path + ".1"
	archiveRecords, err := NewHookAudit(archivePath).Read(1)
	if err != nil {
		t.Fatalf("Read archive: %v", err)
	}
	if len(archiveRecords) != 1 || archiveRecords[0].Hash != seed.Hash {
		t.Fatalf("archive records = %+v, want prior audit", archiveRecords)
	}
	assertManagedAuditFilePrivate(t, path)
	assertManagedAuditFilePrivate(t, archivePath)
}

func TestManagedHookAuditFailedRotationPreservesPriorArchive(t *testing.T) {
	path := managedAuditTestPath(t)
	audit := NewHookAudit(path)
	if err := audit.Append(managedAuditRecord(HookAuditStarted)); err != nil {
		t.Fatalf("seed Append: %v", err)
	}
	archivePath := path + hookAuditArchiveSuffix
	wantArchive := []byte("prior archive\n")
	if err := os.WriteFile(archivePath, wantArchive, 0o600); err != nil {
		t.Fatalf("WriteFile archive: %v", err)
	}
	audit.rotateFile = func(string, string) error { return errors.New("forced rotation failure") }
	current, _, err := openHookAuditFile(path, true)
	if err != nil {
		t.Fatalf("open current audit: %v", err)
	}
	if _, err := audit.rotate(current); err == nil {
		t.Fatal("rotation succeeded despite forced replacement failure")
	}
	gotArchive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("ReadFile archive: %v", err)
	}
	if !bytes.Equal(gotArchive, wantArchive) {
		t.Fatalf("archive = %q, want %q", gotArchive, wantArchive)
	}
}

func TestManagedHookAuditRejectsMalformedCommittedAndOversizedData(t *testing.T) {
	strictTimestamp := "2026-07-14T12:00:00Z"
	strictHash := strings.Repeat("a", 64)
	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "malformed committed line", data: []byte("{not-json}\n")},
		{
			name: "missing required duration",
			data: fmt.Appendf(nil, `{"timestamp":%q,"event":"pre-command","hash":%q,"source":"managed","result":"started"}`+"\n", strictTimestamp, strictHash),
		},
		{
			name: "duplicate result field",
			data: fmt.Appendf(nil, `{"timestamp":%q,"event":"pre-command","hash":%q,"source":"managed","result":"started","result":"success","duration_ms":0}`+"\n", strictTimestamp, strictHash),
		},
		{name: "oversized file", data: make([]byte, maxHookAuditBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := managedAuditTestPath(t)
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
		parent := filepath.Dir(managedAuditTestPath(t))
		if err := os.Mkdir(filepath.Dir(parent), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(parent, 0o755); err != nil {
			t.Fatal(err)
		}
		err := NewHookAudit(filepath.Join(parent, "hooks.jsonl")).Append(managedAuditRecord(HookAuditStarted))
		if err == nil {
			t.Fatal("Append accepted weak parent permissions")
		}
	})
	t.Run("weak file", func(t *testing.T) {
		path := managedAuditTestPath(t)
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
		parent := filepath.Dir(managedAuditTestPath(t))
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
		path := managedAuditTestPath(t)
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
		{name: "inherit-only owner ACE", ownerMatches: true, protected: true, entries: []hookAuditWindowsAccessEntry{{allowed: true, owner: true, fullControl: true, inheritOnly: true}}, wantErr: true},
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

func TestManagedHookAuditWindowsAnchorAccessRejectsUntrustedMutation(t *testing.T) {
	tests := []struct {
		name         string
		ownerTrusted bool
		daclPresent  bool
		entries      []hookAuditWindowsAnchorAccessEntry
		wantErr      bool
	}{
		{
			name:         "trusted mutation and untrusted read",
			ownerTrusted: true,
			daclPresent:  true,
			entries: []hookAuditWindowsAnchorAccessEntry{
				{allowed: true, trusted: true, mutating: true},
				{allowed: true},
			},
		},
		{
			name:         "untrusted mutation",
			ownerTrusted: true,
			daclPresent:  true,
			entries:      []hookAuditWindowsAnchorAccessEntry{{allowed: true, mutating: true}},
			wantErr:      true,
		},
		{name: "untrusted owner", daclPresent: true, wantErr: true},
		{name: "null DACL", ownerTrusted: true, wantErr: true},
		{
			name:         "deny does not grant mutation",
			ownerTrusted: true,
			daclPresent:  true,
			entries:      []hookAuditWindowsAnchorAccessEntry{{mutating: true}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateHookAuditWindowsAnchorAccess(test.ownerTrusted, test.daclPresent, test.entries)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestManagedHookAuditLinuxACLNamesRejectUnsupportedModels(t *testing.T) {
	for _, test := range []struct {
		name    string
		xattrs  []string
		wantErr bool
	}{
		{name: "no ACL", xattrs: []string{"user.comment"}},
		{name: "POSIX access and default", xattrs: []string{"system.posix_acl_access", "system.posix_acl_default"}},
		{name: "NFSv4 ACL", xattrs: []string{"system.nfs4_acl"}, wantErr: true},
		{name: "rich ACL", xattrs: []string{"system.richacl"}, wantErr: true},
		{name: "Samba ACL", xattrs: []string{"security.NTACL"}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateHookAuditLinuxACLNames(test.xattrs)
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

func managedAuditTestPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), ".ratchet", "audit", "hooks.jsonl")
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
