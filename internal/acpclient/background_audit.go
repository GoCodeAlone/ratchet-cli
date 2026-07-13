package acpclient

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	path       string
	syncParent func(string) error
}

func NewBackgroundAudit(path string) *BackgroundAudit {
	return &BackgroundAudit{path: path, syncParent: backgroundSyncParentDir}
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

	lock := backgroundPathLock(a.path)
	lock.Lock()
	defer lock.Unlock()
	_, statErr := os.Stat(a.path)
	created := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !created {
		return statErr
	}
	f, err := backgroundOpenPrivateAppend(a.path)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
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
	if created {
		return a.syncParent(filepath.Dir(a.path))
	}
	return nil
}

func (a *BackgroundAudit) Read() ([]BackgroundAuditRecord, error) {
	if a == nil || strings.TrimSpace(a.path) == "" {
		return nil, errors.New("acp background audit path is required")
	}
	lock := backgroundPathLock(a.path)
	lock.Lock()
	defer lock.Unlock()
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
