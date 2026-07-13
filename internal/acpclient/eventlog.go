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
		Code    *int            `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data,omitempty"`
	}
	if err := json.Unmarshal(raw, &rpcErr); err != nil {
		return fmt.Errorf("%w: invalid error object", ErrInvalidJSONRPCMessage)
	}
	if rpcErr.Code == nil || strings.TrimSpace(rpcErr.Message) == "" {
		return fmt.Errorf("%w: invalid error object", ErrInvalidJSONRPCMessage)
	}
	return nil
}

func (s *Store) AppendEventLog(id string, events []EventLogLine) (err error) {
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
	return s.withEventLogLock(id, func(path string) error {
		nextSeq := 1
		existing, readErr := readEventLogPath(path)
		if readErr == nil && len(existing) > 0 {
			nextSeq = existing[len(existing)-1].Seq + 1
		} else if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return readErr
		}
		appendBytes, err := encodeEventLog(events, nextSeq)
		if err != nil {
			return err
		}
		f, err := backgroundOpenPrivateAppend(path)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, bytes.NewReader(appendBytes)); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Sync(); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		return backgroundSyncParentDir(filepath.Dir(path))
	})
}

func (s *Store) WriteEventLog(id string, events []EventLogLine) error {
	return s.withEventLogLock(id, func(path string) error {
		data, err := encodeEventLog(events, 1)
		if err != nil {
			return err
		}
		if s.eventLogWritePaused != nil {
			s.eventLogWritePaused()
		}
		return backgroundWriteFileAtomic(path, data)
	})
}

func (s *Store) ReadEventLog(id string) ([]EventLogLine, error) {
	var events []EventLogLine
	err := s.withEventLogLock(id, func(path string) error {
		var err error
		events, err = readEventLogPath(path)
		return err
	})
	return events, err
}

func readEventLogPath(path string) ([]EventLogLine, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck
	return readEventLog(f, path)
}

func readEventLog(r io.Reader, path string) ([]EventLogLine, error) {
	var events []EventLogLine
	scanner := bufio.NewScanner(r)
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

func (s *Store) CopyEventLog(id, outputPath string) (err error) {
	if outputPath == "" {
		return errors.New("event log output path is required")
	}
	var snapshot []byte
	if err := s.withEventLogLock(id, func(path string) error {
		var err error
		snapshot, err = os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = readEventLog(bytes.NewReader(snapshot), path)
		return err
	}); err != nil {
		return err
	}
	return backgroundWriteFileAtomic(outputPath, snapshot)
}

func (s *Store) EventLogMetadata(id string) (EventLogMetadata, error) {
	meta := EventLogMetadata{SessionID: strings.TrimSpace(id)}
	err := s.withEventLogLock(id, func(path string) error {
		meta.Path = path
		f, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer f.Close() //nolint:errcheck
		if _, err := f.Stat(); err != nil {
			return err
		}
		events, err := readEventLog(f, path)
		if err != nil {
			return err
		}
		meta.Exists = true
		meta.Count = len(events)
		return nil
	})
	if err != nil {
		return EventLogMetadata{}, err
	}
	return meta, nil
}

func (s *Store) withEventLogLock(id string, operation func(string) error) (err error) {
	if s == nil {
		return errors.New("acp client store is required")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("acp client session id is required")
	}
	path := s.eventLogPath(id)
	lockPath := path + ".lock"
	lock := backgroundPathLock(lockPath)
	lock.Lock()
	defer lock.Unlock()
	release, err := acquireStoreFileLock(lockPath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, release()) }()
	return operation(path)
}

func encodeEventLog(events []EventLogLine, nextSeq int) ([]byte, error) {
	var data bytes.Buffer
	for _, event := range events {
		if err := ValidateJSONRPCMessage(event.Message); err != nil {
			return nil, err
		}
		event.Seq = nextSeq
		nextSeq++
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
			return nil, err
		}
		data.Write(line)
		data.WriteByte('\n')
	}
	return data.Bytes(), nil
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
