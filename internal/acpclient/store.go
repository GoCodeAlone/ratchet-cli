package acpclient

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var ErrSessionNotFound = errors.New("acp client session not found")

type Store struct {
	path string
}

type storeFile struct {
	Sessions []SessionRecord `json:"sessions"`
}

func NewDefaultStore() (*Store, error) {
	stateHome := strings.TrimSpace(os.Getenv("XDG_STATE_HOME"))
	if stateHome == "" {
		home := strings.TrimSpace(os.Getenv("HOME"))
		if home == "" {
			var err error
			home, err = os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("resolve home directory: %w", err)
			}
		}
		stateHome = filepath.Join(home, ".local", "state")
	}
	return NewStore(filepath.Join(stateHome, "ratchet", "acp-client", "sessions.json")), nil
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Store) List() ([]SessionRecord, error) {
	data, err := s.load()
	if err != nil {
		return nil, err
	}
	records := slices.Clone(data.Sessions)
	normalizeRecords(records)
	slices.SortFunc(records, func(a, b SessionRecord) int {
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})
	return records, nil
}

func (s *Store) Get(id string) (SessionRecord, error) {
	records, err := s.List()
	if err != nil {
		return SessionRecord{}, err
	}
	for _, rec := range records {
		if rec.ID == id {
			return rec, nil
		}
	}
	return SessionRecord{}, fmt.Errorf("%w: %s", ErrSessionNotFound, id)
}

func (s *Store) Upsert(rec SessionRecord) error {
	if strings.TrimSpace(rec.ID) == "" {
		return errors.New("acp client session id is required")
	}
	data, err := s.load()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	createdAtZero := rec.CreatedAt.IsZero()
	if createdAtZero {
		rec.CreatedAt = now
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = rec.CreatedAt
	}
	for i, existing := range data.Sessions {
		if existing.ID == rec.ID {
			if createdAtZero {
				rec.CreatedAt = existing.CreatedAt
			}
			data.Sessions[i] = rec
			return s.save(data)
		}
	}
	data.Sessions = append(data.Sessions, rec)
	return s.save(data)
}

func (s *Store) AppendQueuedPrompt(rec SessionRecord, prompt QueuedPrompt) (SessionRecord, error) {
	if strings.TrimSpace(rec.ID) == "" {
		return SessionRecord{}, errors.New("acp client session id is required")
	}
	now := time.Now().UTC()
	if prompt.ID == "" {
		prompt.ID = newQueueID(prompt.Prompt, now)
	}
	if prompt.CreatedAt.IsZero() {
		prompt.CreatedAt = now
	}
	if prompt.Status == "" {
		prompt.Status = QueuePromptStatusPending
	}
	existing, err := s.Get(rec.ID)
	if err != nil {
		if !errors.Is(err, ErrSessionNotFound) {
			return SessionRecord{}, err
		}
		existing = rec
		if existing.CreatedAt.IsZero() {
			existing.CreatedAt = prompt.CreatedAt
		}
	} else {
		existing = mergeSessionMetadata(existing, rec)
	}
	existing.Status = SessionStatusQueued
	existing.UpdatedAt = prompt.CreatedAt
	existing.PromptQueue = append(existing.PromptQueue, prompt)
	if err := s.Upsert(existing); err != nil {
		return SessionRecord{}, err
	}
	return s.Get(existing.ID)
}

func (s *Store) NextQueuedPrompt(id string) (QueuedPrompt, bool, error) {
	rec, err := s.Get(id)
	if err != nil {
		return QueuedPrompt{}, false, err
	}
	for _, prompt := range rec.PromptQueue {
		if prompt.Status == QueuePromptStatusPending {
			return prompt, true, nil
		}
	}
	return QueuedPrompt{}, false, nil
}

func (s *Store) MarkQueueRunning(id, queueID string, when time.Time) error {
	return s.updateQueuedPrompt(id, queueID, when, func(rec *SessionRecord, prompt *QueuedPrompt, at time.Time) {
		prompt.Status = QueuePromptStatusRunning
		prompt.StartedAt = &at
		rec.Status = SessionStatusRunning
	})
}

func (s *Store) MarkQueueCompleted(id, queueID, response, stopReason string, when time.Time) error {
	return s.updateQueuedPrompt(id, queueID, when, func(rec *SessionRecord, prompt *QueuedPrompt, at time.Time) {
		prompt.Status = QueuePromptStatusCompleted
		prompt.CompletedAt = &at
		prompt.Response = response
		prompt.StopReason = stopReason
		rec.Status = SessionStatusCompleted
		rec.LastStopReason = stopReason
		rec.Summary = summarizeStoreText(response)
		rec.Turns = append(rec.Turns, TurnSummary{
			Prompt:     summarizeStoreText(prompt.Prompt),
			Response:   summarizeStoreText(response),
			StopReason: stopReason,
			CreatedAt:  at,
		})
	})
}

func (s *Store) MarkQueueFailed(id, queueID, message string, when time.Time) error {
	return s.updateQueuedPrompt(id, queueID, when, func(rec *SessionRecord, prompt *QueuedPrompt, at time.Time) {
		prompt.Status = QueuePromptStatusFailed
		prompt.CompletedAt = &at
		prompt.Error = message
		rec.Status = SessionStatusCompleted
		rec.Summary = summarizeStoreText(message)
	})
}

func (s *Store) CancelPendingQueue(id string, when time.Time) (int, error) {
	rec, err := s.Get(id)
	if err != nil {
		return 0, err
	}
	if when.IsZero() {
		when = time.Now().UTC()
	}
	when = when.UTC()
	count := 0
	hasRunning := false
	for i := range rec.PromptQueue {
		switch rec.PromptQueue[i].Status {
		case QueuePromptStatusPending:
			rec.PromptQueue[i].Status = QueuePromptStatusCanceled
			rec.PromptQueue[i].CanceledAt = &when
			count++
		case QueuePromptStatusRunning:
			hasRunning = true
		}
	}
	if count == 0 {
		return 0, nil
	}
	if !hasRunning {
		rec.Status = SessionStatusCanceled
	}
	rec.UpdatedAt = when
	return count, s.Upsert(rec)
}

func (s *Store) RecoverStaleQueue(id string, when time.Time) (int, error) {
	if _, err := s.Owner(id); err == nil {
		return 0, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	rec, err := s.Get(id)
	if err != nil {
		return 0, err
	}
	if when.IsZero() {
		when = time.Now().UTC()
	}
	when = when.UTC()
	count := 0
	for i := range rec.PromptQueue {
		if rec.PromptQueue[i].Status != QueuePromptStatusRunning {
			continue
		}
		rec.PromptQueue[i].Status = QueuePromptStatusPending
		rec.PromptQueue[i].StartedAt = nil
		count++
	}
	if count == 0 {
		return 0, nil
	}
	rec.Status = SessionStatusQueued
	rec.UpdatedAt = when
	return count, s.Upsert(rec)
}

func (s *Store) MarkPendingCanceled(id string, when time.Time) error {
	rec, err := s.Get(id)
	if err != nil {
		return err
	}
	if rec.PendingPrompt == nil || rec.PendingPrompt.Status != PendingPromptStatusPending {
		return fmt.Errorf("session %s has no pending prompt", id)
	}
	when = when.UTC()
	rec.PendingPrompt.Status = PendingPromptStatusCanceled
	rec.PendingPrompt.CanceledAt = &when
	rec.Status = SessionStatusCanceled
	rec.UpdatedAt = when
	return s.Upsert(rec)
}

func (s *Store) WriteOwner(owner OwnerLock) error {
	if strings.TrimSpace(owner.SessionID) == "" {
		return errors.New("owner session id is required")
	}
	if owner.StartedAt.IsZero() {
		owner.StartedAt = time.Now().UTC()
	}
	return writeJSONFileAtomic(s.ownerPath(owner.SessionID), owner, 0o600)
}

func (s *Store) Owner(id string) (OwnerLock, error) {
	var owner OwnerLock
	if err := readJSONFile(s.ownerPath(id), &owner); err != nil {
		return OwnerLock{}, err
	}
	if owner.SessionID == "" {
		owner.SessionID = id
	}
	return owner, nil
}

func (s *Store) ClearOwner(id string) error {
	err := os.Remove(s.ownerPath(id))
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Store) RequestCancel(id string, when time.Time) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("cancel session id is required")
	}
	if when.IsZero() {
		when = time.Now().UTC()
	}
	req := CancelRequest{SessionID: id, RequestedAt: when.UTC()}
	if err := writeJSONFileAtomic(s.cancelPath(id), req, 0o600); err != nil {
		return err
	}
	rec, err := s.Get(id)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return nil
		}
		return err
	}
	rec.Status = SessionStatusCancelRequested
	rec.UpdatedAt = req.RequestedAt
	return s.Upsert(rec)
}

func (s *Store) CancelRequest(id string) (CancelRequest, error) {
	var req CancelRequest
	if err := readJSONFile(s.cancelPath(id), &req); err != nil {
		return CancelRequest{}, err
	}
	if req.SessionID == "" {
		req.SessionID = id
	}
	return req, nil
}

func (s *Store) load() (storeFile, error) {
	var data storeFile
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return data, nil
		}
		return storeFile{}, err
	}
	if len(b) == 0 {
		return data, nil
	}
	if err := json.Unmarshal(b, &data); err != nil {
		var legacy []SessionRecord
		if legacyErr := json.Unmarshal(b, &legacy); legacyErr == nil {
			data.Sessions = legacy
		} else {
			return storeFile{}, err
		}
	}
	normalizeRecords(data.Sessions)
	return data, nil
}

func (s *Store) save(data storeFile) error {
	normalizeRecords(data.Sessions)
	slices.SortFunc(data.Sessions, func(a, b SessionRecord) int {
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})
	return writeJSONFileAtomic(s.path, data, 0o600)
}

func (s *Store) ownerPath(id string) string {
	return filepath.Join(filepath.Dir(s.path), "owners", storeKey(id)+".json")
}

func (s *Store) cancelPath(id string) string {
	return filepath.Join(filepath.Dir(s.path), "cancel-requests", storeKey(id)+".json")
}

func normalizeRecords(records []SessionRecord) {
	now := time.Now().UTC()
	for i := range records {
		if records[i].CreatedAt.IsZero() {
			records[i].CreatedAt = now
		}
		if records[i].UpdatedAt.IsZero() {
			records[i].UpdatedAt = records[i].CreatedAt
		}
		if records[i].Status == "" {
			records[i].Status = SessionStatusCompleted
		}
		normalizePromptQueue(&records[i])
	}
}

func normalizePromptQueue(rec *SessionRecord) {
	if rec == nil {
		return
	}
	for i := range rec.PromptQueue {
		if rec.PromptQueue[i].Status == "" {
			rec.PromptQueue[i].Status = QueuePromptStatusPending
		}
		if rec.PromptQueue[i].CreatedAt.IsZero() {
			rec.PromptQueue[i].CreatedAt = rec.CreatedAt
		}
	}
	if rec.PendingPrompt == nil {
		return
	}
	pendingID := rec.PendingPrompt.ID
	if pendingID == "" {
		pendingID = storeKey(rec.PendingPrompt.Prompt)
	}
	for _, queued := range rec.PromptQueue {
		if queued.ID == pendingID {
			return
		}
	}
	status := QueuePromptStatusPending
	if rec.PendingPrompt.Status == PendingPromptStatusCanceled {
		status = QueuePromptStatusCanceled
	}
	rec.PromptQueue = append(rec.PromptQueue, QueuedPrompt{
		ID:         pendingID,
		Prompt:     rec.PendingPrompt.Prompt,
		Status:     status,
		CreatedAt:  rec.PendingPrompt.CreatedAt,
		CanceledAt: rec.PendingPrompt.CanceledAt,
	})
}

func (s *Store) updateQueuedPrompt(id, queueID string, when time.Time, update func(*SessionRecord, *QueuedPrompt, time.Time)) error {
	if strings.TrimSpace(queueID) == "" {
		return errors.New("queue prompt id is required")
	}
	rec, err := s.Get(id)
	if err != nil {
		return err
	}
	if when.IsZero() {
		when = time.Now().UTC()
	}
	when = when.UTC()
	for i := range rec.PromptQueue {
		if rec.PromptQueue[i].ID == queueID {
			update(&rec, &rec.PromptQueue[i], when)
			rec.UpdatedAt = when
			return s.Upsert(rec)
		}
	}
	return fmt.Errorf("%w: queue prompt %s", ErrSessionNotFound, queueID)
}

func mergeSessionMetadata(existing, update SessionRecord) SessionRecord {
	if update.ACPSessionID != "" {
		existing.ACPSessionID = update.ACPSessionID
	}
	if update.Agent != "" {
		existing.Agent = update.Agent
	}
	if update.CommandFingerprint != "" {
		existing.CommandFingerprint = update.CommandFingerprint
	}
	if update.Cwd != "" {
		existing.Cwd = update.Cwd
	}
	if update.CreatedAt.IsZero() {
		return existing
	}
	existing.CreatedAt = update.CreatedAt
	return existing
}

func newQueueID(prompt string, when time.Time) string {
	return storeKey(fmt.Sprintf("%s:%s", when.UTC().Format(time.RFC3339Nano), prompt))
}

func summarizeStoreText(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 200 {
		return value
	}
	return value[:200]
}

func storeKey(id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(id))
}

func readJSONFile(path string, dst any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

func writeJSONFileAtomic(path string, value any, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return err
		}
		if retryErr := os.Rename(tmpName, path); retryErr != nil {
			return retryErr
		}
	}
	return nil
}
