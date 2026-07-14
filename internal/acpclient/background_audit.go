package acpclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	RecordID       string    `json:"recordId"`
	At             time.Time `json:"at"`
	Action         string    `json:"action"`
	SessionID      string    `json:"sessionId"`
	Profile        string    `json:"profile"`
	DescriptorHash string    `json:"descriptorHash"`
	Outcome        string    `json:"outcome"`
}

type BackgroundAudit struct {
	path            string
	syncParent      func(string) error
	beforeAppend    func(BackgroundAuditRecord)
	writeFile       func(*os.File, []byte) (int, error)
	syncFile        func(*os.File) error
	closeFile       func(*os.File) error
	repairFile      func(*os.File, int64) error
	beforeMutation  func()
	openTransaction func(string, bool) (backgroundAuditTransaction, error)
}

func NewBackgroundAudit(path string) *BackgroundAudit {
	return &BackgroundAudit{path: backgroundAuditPath(path)}
}

func NewDefaultBackgroundAudit() (*BackgroundAudit, error) {
	store, err := NewDefaultStore()
	if err != nil {
		return nil, err
	}
	return NewBackgroundAudit(filepath.Join(filepath.Dir(store.Path()), "background-audit.jsonl")), nil
}

func backgroundAuditPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return path
	}
	if filepath.Base(filepath.Dir(path)) == ".ratchet-audit" {
		return path
	}
	return filepath.Join(filepath.Dir(path), ".ratchet-audit", filepath.Base(path))
}

func (a *BackgroundAudit) Path() string {
	if a == nil {
		return ""
	}
	return a.path
}

func (a *BackgroundAudit) Append(record BackgroundAuditRecord) error {
	if a == nil || strings.TrimSpace(a.path) == "" {
		return errors.New("acp background audit path is required")
	}
	if err := validateBackgroundAuditRecord(record); err != nil {
		return err
	}
	record.At = record.At.UTC()
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if a.beforeAppend != nil {
		a.beforeAppend(record)
	}

	tx, err := a.openAuditTransaction(true)
	if err != nil {
		return err
	}
	return func() (err error) {
		newlineWritten := false
		defer func() { err = errors.Join(err, backgroundAuditCommitError(newlineWritten, tx.Close())) }()
		f := tx.File()
		closed := false
		defer func() {
			if !closed {
				err = errors.Join(err, f.Close())
			}
		}()

		records, err := a.readRepair(tx)
		if err != nil {
			return err
		}
		for _, committed := range records {
			if committed.RecordID == record.RecordID {
				closed = true
				return f.Close()
			}
		}
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			return err
		}
		a.beforeFileMutation()
		if err := tx.ValidateForMutation(); err != nil {
			return err
		}
		written, writeErr := a.write(f, line)
		newlineWritten = written == len(line)
		if writeErr == nil && written != len(line) {
			writeErr = io.ErrShortWrite
		}
		if writeErr != nil {
			return backgroundAuditCommitError(newlineWritten, writeErr)
		}
		if err := a.sync(f); err != nil {
			return backgroundAuditCommitError(true, err)
		}
		closed = true
		if err := a.close(f); err != nil {
			return backgroundAuditCommitError(true, err)
		}
		if err := a.syncParentDir(tx); err != nil {
			return backgroundAuditCommitError(true, err)
		}
		return nil
	}()
}

func (a *BackgroundAudit) Read() (records []BackgroundAuditRecord, err error) {
	if a == nil || strings.TrimSpace(a.path) == "" {
		return nil, errors.New("acp background audit path is required")
	}
	tx, err := a.openAuditTransaction(false)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, tx.Close()) }()
	if tx.File() == nil {
		return nil, nil
	}
	records, err = a.readRepair(tx)
	err = errors.Join(err, a.close(tx.File()))
	return records, err
}

func (a *BackgroundAudit) openAuditTransaction(create bool) (backgroundAuditTransaction, error) {
	if a.openTransaction != nil {
		return a.openTransaction(a.path, create)
	}
	return backgroundOpenAuditTransaction(a.path, create)
}

func (a *BackgroundAudit) readRepair(tx backgroundAuditTransaction) ([]BackgroundAuditRecord, error) {
	f := tx.File()
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		committed := bytes.LastIndexByte(data, '\n') + 1
		a.beforeFileMutation()
		if err := tx.ValidateForMutation(); err != nil {
			return nil, err
		}
		if err := a.repair(f, int64(committed)); err != nil {
			return nil, fmt.Errorf("repair background audit %s: %w", a.path, err)
		}
		if err := a.sync(f); err != nil {
			return nil, fmt.Errorf("sync background audit repair %s: %w", a.path, err)
		}
		data = data[:committed]
	}
	if len(data) == 0 {
		return nil, nil
	}
	lines := bytes.Split(data, []byte{'\n'})
	records := make([]BackgroundAuditRecord, 0, len(lines)-1)
	for lineNumber, line := range lines[:len(lines)-1] {
		if len(bytes.TrimSpace(line)) == 0 {
			return nil, fmt.Errorf("read background audit %s line %d: empty committed record", a.path, lineNumber+1)
		}
		var record BackgroundAuditRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, fmt.Errorf("read background audit %s line %d: %w", a.path, lineNumber+1, err)
		}
		if err := validateBackgroundAuditRecord(record); err != nil {
			return nil, fmt.Errorf("read background audit %s line %d: %w", a.path, lineNumber+1, err)
		}
		records = append(records, record)
	}
	return records, nil
}

func (a *BackgroundAudit) beforeFileMutation() {
	if a.beforeMutation != nil {
		a.beforeMutation()
	}
}

func (a *BackgroundAudit) write(f *os.File, data []byte) (int, error) {
	if a.writeFile != nil {
		return a.writeFile(f, data)
	}
	return f.Write(data)
}

func (a *BackgroundAudit) sync(f *os.File) error {
	if a.syncFile != nil {
		return a.syncFile(f)
	}
	return f.Sync()
}

func (a *BackgroundAudit) close(f *os.File) error {
	if a.closeFile != nil {
		return a.closeFile(f)
	}
	return f.Close()
}

func (a *BackgroundAudit) repair(f *os.File, size int64) error {
	if a.repairFile != nil {
		return a.repairFile(f, size)
	}
	return f.Truncate(size)
}

func (a *BackgroundAudit) syncParentDir(tx backgroundAuditTransaction) error {
	if a.syncParent != nil {
		return a.syncParent(filepath.Dir(a.path))
	}
	return tx.SyncParent()
}

func validateBackgroundAuditRecord(record BackgroundAuditRecord) error {
	for field, value := range map[string]string{
		"record id":       record.RecordID,
		"action":          record.Action,
		"session id":      record.SessionID,
		"profile":         record.Profile,
		"descriptor hash": record.DescriptorHash,
		"outcome":         record.Outcome,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("acp background audit %s is required", field)
		}
	}
	if record.At.IsZero() {
		return errors.New("acp background audit time is required")
	}
	allowed := false
	switch record.Action {
	case BackgroundAuditStart:
		allowed = record.Outcome == BackgroundOutcomeStarted
	case BackgroundAuditResume:
		allowed = record.Outcome == BackgroundOutcomeResumed
	case BackgroundAuditBlock:
		allowed = record.Outcome == BackgroundOutcomeProfileUntrusted ||
			record.Outcome == BackgroundOutcomeProfileDrift ||
			record.Outcome == BackgroundOutcomeProfileMissing ||
			record.Outcome == BackgroundOutcomeSessionMissing ||
			record.Outcome == BackgroundOutcomePolicyInvalid
	case BackgroundAuditError:
		allowed = record.Outcome == BackgroundOutcomeWorkerError ||
			record.Outcome == BackgroundOutcomeWorkerPanic ||
			record.Outcome == BackgroundOutcomeStateWriteFailed ||
			record.Outcome == BackgroundOutcomeAuditAppendFailed
	case BackgroundAuditStop:
		allowed = record.Outcome == BackgroundOutcomeStopped || record.Outcome == BackgroundOutcomeCompleted
	}
	if !allowed {
		return fmt.Errorf("acp background audit action/outcome is invalid: %s/%s", record.Action, record.Outcome)
	}
	return nil
}

func backgroundAuditCommitError(newlineWritten bool, err error) error {
	if err == nil {
		return nil
	}
	if newlineWritten {
		return storeCommitUnconfirmed(err)
	}
	return err
}
