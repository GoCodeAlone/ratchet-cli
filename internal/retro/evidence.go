package retro

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/GoCodeAlone/workflow/secrets"
)

// EvidenceStore persists local retro evidence as JSONL.
type EvidenceStore struct {
	path     string
	redactor *secrets.Redactor
}

func NewEvidenceStore(path string, redactor *secrets.Redactor) *EvidenceStore {
	return &EvidenceStore{path: path, redactor: redactor}
}

func (s *EvidenceStore) Append(event Event) (err error) {
	if s == nil {
		return nil
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	event.Message = s.redact(event.Message)
	event.Command = s.redact(event.Command)
	event.Outcome = s.redact(event.Outcome)
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}()
	_, err = f.Write(data)
	return err
}

func (s *EvidenceStore) Load() ([]Event, error) {
	if s == nil {
		return nil, nil
	}
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	var events []Event
	for {
		line, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) && len(line) == 0 {
			break
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		var event Event
		if err := json.Unmarshal(line, &event); err == nil {
			events = append(events, event)
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}
	return events, nil
}

func (s *EvidenceStore) redact(text string) string {
	if s == nil || s.redactor == nil {
		return text
	}
	return s.redactor.Redact(text)
}

// Recorder wraps an EvidenceStore for daemon lifecycle call sites.
type Recorder struct {
	store *EvidenceStore
}

func NewRecorder(store *EvidenceStore) *Recorder {
	return &Recorder{store: store}
}

func (r *Recorder) Record(event Event) error {
	if r == nil || r.store == nil {
		return nil
	}
	return r.store.Append(event)
}
