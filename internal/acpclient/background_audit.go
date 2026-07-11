package acpclient

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	BackgroundAuditStart  = "start"
	BackgroundAuditResume = "resume"
	BackgroundAuditBlock  = "block"
	BackgroundAuditError  = "error"
	BackgroundAuditStop   = "stop"
)

type BackgroundAuditRecord struct {
	At             time.Time `json:"at"`
	Action         string    `json:"action"`
	SessionID      string    `json:"sessionId"`
	Profile        string    `json:"profile"`
	DescriptorHash string    `json:"descriptorHash"`
	Outcome        string    `json:"outcome"`
}

type BackgroundAudit struct {
	path string
	mu   sync.Mutex
}

func NewBackgroundAudit(path string) *BackgroundAudit {
	return &BackgroundAudit{path: path}
}

func NewDefaultBackgroundAudit() (*BackgroundAudit, error) {
	store, err := NewDefaultStore()
	if err != nil {
		return nil, err
	}
	return NewBackgroundAudit(filepath.Join(filepath.Dir(store.Path()), "background-audit.jsonl")), nil
}

func (a *BackgroundAudit) Path() string {
	if a == nil {
		return ""
	}
	return a.path
}

func (a *BackgroundAudit) Append(record BackgroundAuditRecord) (err error) {
	if a == nil || strings.TrimSpace(a.path) == "" {
		return errors.New("acp background audit path is required")
	}
	if strings.TrimSpace(record.Action) == "" {
		return errors.New("acp background audit action is required")
	}
	if strings.TrimSpace(record.SessionID) == "" {
		return errors.New("acp background audit session id is required")
	}
	if record.At.IsZero() {
		record.At = time.Now().UTC()
	} else {
		record.At = record.At.UTC()
	}
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(a.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func (a *BackgroundAudit) Read() ([]BackgroundAuditRecord, error) {
	if a == nil || strings.TrimSpace(a.path) == "" {
		return nil, errors.New("acp background audit path is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	f, err := os.Open(a.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var records []BackgroundAuditRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var record BackgroundAuditRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, fmt.Errorf("read background audit %s: %w", a.path, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read background audit %s: %w", a.path, err)
	}
	return records, nil
}
