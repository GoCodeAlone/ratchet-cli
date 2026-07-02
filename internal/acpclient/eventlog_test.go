package acpclient

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestValidateJSONRPCMessageAcceptsACPXShapes(t *testing.T) {
	valid := []json.RawMessage{
		json.RawMessage(`{"jsonrpc":"2.0","id":"req-1","method":"session/prompt","params":{"sessionId":"s"}}`),
		json.RawMessage(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s"}}`),
		json.RawMessage(`{"jsonrpc":"2.0","id":"req-1","result":{"stopReason":"end_turn"}}`),
		json.RawMessage(`{"jsonrpc":"2.0","id":"req-1","error":{"code":-32000,"message":"failed"}}`),
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
