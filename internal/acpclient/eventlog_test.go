package acpclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	eventLogHelperEnv      = "RATCHET_EVENT_LOG_HELPER"
	eventLogStorePathEnv   = "RATCHET_EVENT_LOG_STORE_PATH"
	eventLogSessionIDEnv   = "RATCHET_EVENT_LOG_SESSION_ID"
	eventLogEventIDEnv     = "RATCHET_EVENT_LOG_EVENT_ID"
	eventLogReadyPathEnv   = "RATCHET_EVENT_LOG_READY_PATH"
	eventLogReleasePathEnv = "RATCHET_EVENT_LOG_RELEASE_PATH"
)

func TestEventLogConcurrentHandlesProduceContiguousSequence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	const writers = 24
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := range writers {
		wg.Go(func() {
			<-start
			store := NewStore(path)
			errs <- store.AppendEventLog("concurrent", []EventLogLine{{
				Direction: EventDirectionInbound,
				Message:   json.RawMessage(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{}}`, i)),
			}})
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("AppendEventLog: %v", err)
		}
	}
	assertContiguousEventLog(t, NewStore(path), "concurrent", writers)
}

func TestEventLogWriteSerializesMetadataAndCopyWithoutBlockingSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	seed := NewStore(path)
	if err := seed.WriteEventLog("replace", []EventLogLine{{
		Direction: EventDirectionInbound,
		Message:   json.RawMessage(`{"jsonrpc":"2.0","id":"old","result":{}}`),
	}}); err != nil {
		t.Fatalf("seed WriteEventLog: %v", err)
	}

	writer := NewStore(path)
	paused := make(chan struct{})
	release := make(chan struct{})
	writer.eventLogWritePaused = func() {
		close(paused)
		<-release
	}
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- writer.WriteEventLog("replace", []EventLogLine{
			{Direction: EventDirectionInbound, Message: json.RawMessage(`{"jsonrpc":"2.0","id":"new-1","result":{}}`)},
			{Direction: EventDirectionInbound, Message: json.RawMessage(`{"jsonrpc":"2.0","id":"new-2","result":{}}`)},
		})
	}()
	select {
	case <-paused:
	case <-time.After(2 * time.Second):
		t.Fatal("WriteEventLog did not reach replacement pause")
	}
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()

	reader := NewStore(path)
	metaDone := make(chan error, 1)
	go func() {
		meta, err := reader.EventLogMetadata("replace")
		if err == nil && (!meta.Exists || meta.Count != 2) {
			err = fmt.Errorf("metadata = %#v, want complete replacement", meta)
		}
		metaDone <- err
	}()
	copyPath := filepath.Join(t.TempDir(), "copy.ndjson")
	copyDone := make(chan error, 1)
	go func() { copyDone <- reader.CopyEventLog("replace", copyPath) }()
	sessionDone := make(chan error, 1)
	go func() { sessionDone <- reader.Upsert(SessionRecord{ID: "unrelated"}) }()

	select {
	case err := <-sessionDone:
		if err != nil {
			t.Fatalf("unrelated Upsert: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("event-log transaction blocked unrelated sessions transaction")
	}
	select {
	case err := <-metaDone:
		t.Fatalf("EventLogMetadata completed during replacement: %v", err)
	case err := <-copyDone:
		t.Fatalf("CopyEventLog completed during replacement: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	released = true
	for name, done := range map[string]<-chan error{"write": writeDone, "metadata": metaDone, "copy": copyDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s after release: %v", name, err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s remained blocked after release", name)
		}
	}
	assertEventLogFileCount(t, copyPath, 2)
}

func TestEventLogConcurrentSubprocessAppendsProduceContiguousSequence(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "sessions.json")
	releasePath := filepath.Join(dir, "release")
	const writers = 8
	type child struct {
		cmd    *exec.Cmd
		output bytes.Buffer
	}
	children := make([]child, writers)
	for i := range writers {
		readyPath := filepath.Join(dir, fmt.Sprintf("ready-%d", i))
		children[i].cmd = exec.Command(os.Args[0], "-test.run=^TestEventLogSubprocessHelper$")
		children[i].cmd.Env = append(os.Environ(),
			eventLogHelperEnv+"=1",
			eventLogStorePathEnv+"="+storePath,
			eventLogSessionIDEnv+"=subprocess",
			eventLogEventIDEnv+"="+fmt.Sprint(i),
			eventLogReadyPathEnv+"="+readyPath,
			eventLogReleasePathEnv+"="+releasePath,
		)
		children[i].cmd.Stdout = &children[i].output
		children[i].cmd.Stderr = &children[i].output
		if err := children[i].cmd.Start(); err != nil {
			t.Fatalf("start helper %d: %v", i, err)
		}
		waitForStoreLockHelperReady(t, readyPath)
	}
	if err := os.WriteFile(releasePath, []byte("release\n"), 0o600); err != nil {
		t.Fatalf("release helpers: %v", err)
	}
	for i := range children {
		if err := children[i].cmd.Wait(); err != nil {
			t.Fatalf("helper %d: %v\n%s", i, err, children[i].output.String())
		}
	}
	assertContiguousEventLog(t, NewStore(storePath), "subprocess", writers)
}

func TestEventLogSubprocessHelper(t *testing.T) {
	if os.Getenv(eventLogHelperEnv) != "1" {
		return
	}
	if err := os.WriteFile(os.Getenv(eventLogReadyPathEnv), []byte("ready\n"), 0o600); err != nil {
		t.Fatalf("write ready marker: %v", err)
	}
	for {
		if _, err := os.Stat(os.Getenv(eventLogReleasePathEnv)); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	id := os.Getenv(eventLogEventIDEnv)
	if err := NewStore(os.Getenv(eventLogStorePathEnv)).AppendEventLog(os.Getenv(eventLogSessionIDEnv), []EventLogLine{{
		Direction: EventDirectionInbound,
		Message:   json.RawMessage(fmt.Sprintf(`{"jsonrpc":"2.0","id":%q,"result":{}}`, id)),
	}}); err != nil {
		t.Fatalf("AppendEventLog: %v", err)
	}
}

func assertContiguousEventLog(t *testing.T, store *Store, sessionID string, want int) {
	t.Helper()
	events, err := store.ReadEventLog(sessionID)
	if err != nil {
		t.Fatalf("ReadEventLog: %v", err)
	}
	if len(events) != want {
		t.Fatalf("events len = %d, want %d", len(events), want)
	}
	for i, event := range events {
		if event.Seq != i+1 {
			t.Fatalf("event %d sequence = %d, want %d", i, event.Seq, i+1)
		}
	}
}

func assertEventLogFileCount(t *testing.T, path string, want int) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	if len(lines) != want {
		t.Fatalf("copied event lines = %d, want %d", len(lines), want)
	}
	for _, line := range lines {
		var event EventLogLine
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatalf("copied event is incomplete: %v", err)
		}
	}
}

func TestValidateJSONRPCMessageAcceptsACPXShapes(t *testing.T) {
	valid := []json.RawMessage{
		json.RawMessage(`{"jsonrpc":"2.0","id":"req-1","method":"session/prompt","params":{"sessionId":"s"}}`),
		json.RawMessage(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s"}}`),
		json.RawMessage(`{"jsonrpc":"2.0","id":"req-1","result":{"stopReason":"end_turn"}}`),
		json.RawMessage(`{"jsonrpc":"2.0","id":"req-1","error":{"code":-32000,"message":"failed"}}`),
		json.RawMessage(`{"jsonrpc":"2.0","id":"req-1","error":{"code":-32000,"message":"failed","data":"detail"}}`),
		json.RawMessage(`{"jsonrpc":"2.0","id":"req-1","error":{"code":-32000,"message":"failed","data":["detail"]}}`),
	}
	for _, msg := range valid {
		if err := ValidateJSONRPCMessage(msg); err != nil {
			t.Fatalf("ValidateJSONRPCMessage(%s) = %v, want nil", msg, err)
		}
	}
}

func TestValidateJSONRPCMessageRejectsInvalidShapes(t *testing.T) {
	invalid := []json.RawMessage{
		json.RawMessage(`{"id":"req-1","method":"session/prompt"}`),
		json.RawMessage(`{"jsonrpc":"2.0","method":""}`),
		json.RawMessage(`{"jsonrpc":"2.0","id":"req-1","result":{},"error":{"code":-32000,"message":"failed"}}`),
		json.RawMessage(`{"jsonrpc":"2.0","id":"req-1","error":{"message":"failed"}}`),
	}
	for _, msg := range invalid {
		if err := ValidateJSONRPCMessage(msg); !errors.Is(err, ErrInvalidJSONRPCMessage) {
			t.Fatalf("ValidateJSONRPCMessage(%s) = %v, want ErrInvalidJSONRPCMessage", msg, err)
		}
	}
}

func TestEventLogStoreWritesAndReadsJSONRPCEvents(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state", "sessions.json"))
	at := time.Date(2026, 7, 2, 12, 30, 0, 0, time.UTC)
	events := []EventLogLine{
		{
			Seq:       1,
			At:        at,
			Direction: EventDirectionOutbound,
			Message:   json.RawMessage(`{"jsonrpc":"2.0","id":"prompt-1","method":"session/prompt","params":{"sessionId":"s"}}`),
		},
		{
			Seq:       2,
			At:        at.Add(time.Second),
			Direction: EventDirectionInbound,
			Message:   json.RawMessage(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s"}}`),
		},
	}
	if err := store.AppendEventLog("session/with spaces", events); err != nil {
		t.Fatalf("AppendEventLog: %v", err)
	}
	got, err := store.ReadEventLog("session/with spaces")
	if err != nil {
		t.Fatalf("ReadEventLog: %v", err)
	}
	if len(got) != len(events) {
		t.Fatalf("events len = %d, want %d", len(got), len(events))
	}
	for i := range events {
		if got[i].Seq != events[i].Seq || !got[i].At.Equal(events[i].At) || got[i].Direction != events[i].Direction {
			t.Fatalf("event %d metadata = %#v, want %#v", i, got[i], events[i])
		}
		if !jsonMessagesEqual(got[i].Message, events[i].Message) {
			t.Fatalf("event %d message = %s, want %s", i, got[i].Message, events[i].Message)
		}
	}
	meta, err := store.EventLogMetadata("session/with spaces")
	if err != nil {
		t.Fatalf("EventLogMetadata: %v", err)
	}
	if !meta.Exists || meta.Count != 2 {
		t.Fatalf("metadata = %#v, want existing count 2", meta)
	}
	if strings.Contains(filepath.Base(meta.Path), "/") || strings.Contains(filepath.Base(meta.Path), `\`) {
		t.Fatalf("event log path %q is not escaped", meta.Path)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(meta.Path)
		if err != nil {
			t.Fatalf("stat event log: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("event log mode = %o, want 0600", got)
		}
	}
}

func TestEventLogStoreRejectsInvalidJSONRPCMessages(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state", "sessions.json"))
	if err := store.AppendEventLog("session-bad", []EventLogLine{{
		Seq:       1,
		At:        time.Date(2026, 7, 2, 12, 35, 0, 0, time.UTC),
		Direction: EventDirectionInbound,
		Message:   json.RawMessage(`{"jsonrpc":"2.0","method":"session/update"}`),
	}}); err != nil {
		t.Fatalf("AppendEventLog good event: %v", err)
	}
	meta, err := store.EventLogMetadata("session-bad")
	if err != nil {
		t.Fatalf("EventLogMetadata: %v", err)
	}
	f, err := os.OpenFile(meta.Path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	if _, err := f.WriteString(`{"seq":2,"at":"2026-07-02T12:35:01Z","direction":"inbound","message":{"jsonrpc":"2.0"}}` + "\n"); err != nil {
		_ = f.Close()
		t.Fatalf("corrupt event log: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close event log: %v", err)
	}
	_, err = store.ReadEventLog("session-bad")
	if !errors.Is(err, ErrInvalidJSONRPCMessage) {
		t.Fatalf("ReadEventLog error = %v, want ErrInvalidJSONRPCMessage", err)
	}
}
