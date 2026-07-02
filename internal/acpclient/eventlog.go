package acpclient

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var (
	ErrInvalidJSONRPCMessage = errors.New("invalid acp json-rpc message")
	ErrRawHistoryUnavailable = errors.New("acp client raw event history unavailable")
)

type JSONRPCMessage json.RawMessage

type EventDirection string

const (
	EventDirectionOutbound EventDirection = "outbound"
	EventDirectionInbound  EventDirection = "inbound"
)

type EventLogLine struct {
	Seq       int             `json:"seq"`
	At        time.Time       `json:"at,omitzero"`
	Direction EventDirection  `json:"direction"`
	Message   json.RawMessage `json:"message"`
}

type EventLogMetadata struct {
	SessionID string `json:"session_id"`
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	Count     int    `json:"count"`
}

func ValidateJSONRPCMessage(message json.RawMessage) error {
	message = bytes.TrimSpace(message)
	if len(message) == 0 {
		return fmt.Errorf("%w: empty message", ErrInvalidJSONRPCMessage)
	}
	var msg struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Result  json.RawMessage `json:"result"`
		Error   json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(message, &msg); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidJSONRPCMessage, err)
	}
	if msg.JSONRPC != "2.0" {
		return fmt.Errorf("%w: jsonrpc must be 2.0", ErrInvalidJSONRPCMessage)
	}
	hasID := len(bytes.TrimSpace(msg.ID)) > 0
	hasMethod := strings.TrimSpace(msg.Method) != ""
	hasResult := len(bytes.TrimSpace(msg.Result)) > 0
	hasError := len(bytes.TrimSpace(msg.Error)) > 0
	if hasResult && hasError {
		return fmt.Errorf("%w: response cannot include both result and error", ErrInvalidJSONRPCMessage)
	}
	if hasMethod {
		if hasResult || hasError {
			return fmt.Errorf("%w: method message cannot include result or error", ErrInvalidJSONRPCMessage)
		}
		if hasID && !validJSONRPCID(msg.ID) {
			return fmt.Errorf("%w: invalid id", ErrInvalidJSONRPCMessage)
		}
		return nil
	}
	if !hasID {
		return fmt.Errorf("%w: response requires id", ErrInvalidJSONRPCMessage)
	}
	if !validJSONRPCID(msg.ID) {
		return fmt.Errorf("%w: invalid id", ErrInvalidJSONRPCMessage)
	}
	if hasError {
		if err := validateJSONRPCError(msg.Error); err != nil {
			return err
		}
		return nil
	}
	if hasResult {
		return nil
	}
	return fmt.Errorf("%w: missing method, result, or error", ErrInvalidJSONRPCMessage)
}

func validJSONRPCID(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if bytes.Equal(raw, []byte("null")) {
		return true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return true
	}
	var n json.Number
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&n); err == nil {
		return true
	}
	return false
}

func validateJSONRPCError(raw json.RawMessage) error {
	var rpcErr struct {
		Code    *int    `json:"code"`
		Message string  `json:"message"`
		Data    unknown `json:"data,omitempty"`
	}
	if err := json.Unmarshal(raw, &rpcErr); err != nil {
		return fmt.Errorf("%w: invalid error object", ErrInvalidJSONRPCMessage)
	}
	if rpcErr.Code == nil || strings.TrimSpace(rpcErr.Message) == "" {
		return fmt.Errorf("%w: invalid error object", ErrInvalidJSONRPCMessage)
	}
	return nil
}

type unknown struct{}

func (s *Store) AppendEventLog(id string, events []EventLogLine) error {
	if s == nil {
		return errors.New("acp client store is required")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("acp client session id is required")
	}
	if len(events) == 0 {
		return nil
	}
	path := s.eventLogPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	nextSeq := 1
	if existing, err := s.ReadEventLog(id); err == nil && len(existing) > 0 {
		nextSeq = existing[len(existing)-1].Seq + 1
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	for _, event := range events {
		if err := ValidateJSONRPCMessage(event.Message); err != nil {
			return err
		}
		event.Seq = nextSeq
		nextSeq = event.Seq + 1
		if event.At.IsZero() {
			event.At = time.Now().UTC()
		} else {
			event.At = event.At.UTC()
		}
		if event.Direction == "" {
			event.Direction = EventDirectionInbound
		}
		line, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	return f.Sync()
}

func (s *Store) WriteEventLog(id string, events []EventLogLine) error {
	if s == nil {
		return errors.New("acp client store is required")
	}
	path := s.eventLogPath(id)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return s.AppendEventLog(id, events)
}

func (s *Store) ReadEventLog(id string) ([]EventLogLine, error) {
	if s == nil {
		return nil, errors.New("acp client store is required")
	}
	path := s.eventLogPath(id)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck
	var events []EventLogLine
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 16*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event EventLogLine
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("read event log %s: %w", path, err)
		}
		if err := ValidateJSONRPCMessage(event.Message); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Store) CopyEventLog(id, outputPath string) error {
	if outputPath == "" {
		return errors.New("event log output path is required")
	}
	events, err := s.ReadEventLog(id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	src, err := os.Open(s.eventLogPath(id))
	if err != nil {
		return err
	}
	defer src.Close() //nolint:errcheck
	dst, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer dst.Close() //nolint:errcheck
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	_ = events
	if err := dst.Chmod(0o600); err != nil {
		return err
	}
	return dst.Sync()
}

func (s *Store) EventLogMetadata(id string) (EventLogMetadata, error) {
	if s == nil {
		return EventLogMetadata{}, errors.New("acp client store is required")
	}
	path := s.eventLogPath(id)
	meta := EventLogMetadata{SessionID: id, Path: path}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return meta, nil
		}
		return EventLogMetadata{}, err
	}
	events, err := s.ReadEventLog(id)
	if err != nil {
		return EventLogMetadata{}, err
	}
	meta.Exists = true
	meta.Count = len(events)
	return meta, nil
}

func (s *Store) eventLogPath(id string) string {
	return filepath.Join(filepath.Dir(s.path), "events", storeKey(id)+".ndjson")
}

func cloneEvents(events []EventLogLine) []EventLogLine {
	cloned := slices.Clone(events)
	for i := range cloned {
		cloned[i].Message = slices.Clone(cloned[i].Message)
	}
	return cloned
}
