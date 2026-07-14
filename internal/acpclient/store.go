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
	"sync"
	"time"
)

var (
	ErrSessionNotFound             = errors.New("acp client session not found")
	ErrQueuePromptNotFound         = errors.New("acp client queue prompt not found")
	ErrInvalidOwnerLock            = errors.New("acp client owner lock is invalid")
	ErrOwnerNotCancelable          = errors.New("acp client owner does not represent active execution")
	ErrOwnerLeaseBusy              = errors.New("acp client owner lease is held")
	ErrStoreProcessLockUnsupported = errors.New("acp client cross-process store locks are unsupported")
	ErrStoreCommitUnconfirmed      = errors.New("acp client store commit completed but durability confirmation failed")
)

type OwnerLease struct {
	mu       sync.Mutex
	store    *Store
	owner    OwnerLock
	release  func() error
	released bool
}

type Store struct {
	path                  string
	beforeMutation        func()
	beforeLaunchAdmission func()
	transactionLoaded     func()
	eventLogWritePaused   func()
	sessionWriter         func(storeFile) error
	eventLogWriter        func(string, []byte) error
	eventLogRemover       func(string) error
	cancelWriter          func(string, CancelRequest) error
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
	var records []SessionRecord
	err := s.readTransaction(func(data storeFile) error {
		records = slices.Clone(data.Sessions)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(records, func(a, b SessionRecord) int {
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})
	return records, nil
}

func (s *Store) Get(id string) (SessionRecord, error) {
	var record SessionRecord
	err := s.readTransaction(func(data storeFile) error {
		found, err := findSessionRecord(&data, id)
		if err != nil {
			return err
		}
		record = *found
		return nil
	})
	return record, err
}

func (s *Store) Upsert(rec SessionRecord) error {
	if strings.TrimSpace(rec.ID) == "" {
		return errors.New("acp client session id is required")
	}
	return s.transitionSession(rec.ID, nil, func(data *storeFile) (bool, error) {
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
				preserveCancellationLatch(existing, &rec)
				data.Sessions[i] = rec
				return true, nil
			}
		}
		data.Sessions = append(data.Sessions, rec)
		return true, nil
	}, nil)
}

func (s *Store) InsertSession(rec SessionRecord) error {
	if strings.TrimSpace(rec.ID) == "" {
		return errors.New("acp client session id is required")
	}
	return s.createSessionWithEvents(rec, nil)
}

func insertSessionRecord(data *storeFile, rec SessionRecord) error {
	if _, err := findSessionRecord(data, rec.ID); err == nil {
		return fmt.Errorf("%w: %s", ErrSessionArchiveCollision, rec.ID)
	} else if !errors.Is(err, ErrSessionNotFound) {
		return err
	}
	now := time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = rec.CreatedAt
	}
	data.Sessions = append(data.Sessions, rec)
	return nil
}

func (s *Store) MarkSessionStarted(rec SessionRecord) error {
	return s.updateSessionLifecycle(rec, nil, false)
}

func (s *Store) MarkSessionCompleted(rec SessionRecord, turn TurnSummary) error {
	return s.MarkSessionCompletedWithEvents(rec, turn, nil)
}

func (s *Store) updateSessionLifecycle(rec SessionRecord, turn *TurnSummary, terminal bool) error {
	if strings.TrimSpace(rec.ID) == "" {
		return errors.New("acp client session id is required")
	}
	return s.transitionSession(rec.ID, nil, func(data *storeFile) (bool, error) {
		return updateSessionLifecycleRecord(data, rec, turn, terminal)
	}, nil)
}

func (s *Store) MarkSessionCompletedWithEvents(rec SessionRecord, turn TurnSummary, events []EventLogLine) error {
	if strings.TrimSpace(rec.ID) == "" {
		return errors.New("acp client session id is required")
	}
	return s.transitionSession(rec.ID, events, func(data *storeFile) (bool, error) {
		return updateSessionLifecycleRecord(data, rec, &turn, true)
	}, nil)
}

func updateSessionLifecycleRecord(data *storeFile, rec SessionRecord, turn *TurnSummary, terminal bool) (bool, error) {
	now := time.Now().UTC()
	existing, err := findSessionRecord(data, rec.ID)
	if errors.Is(err, ErrSessionNotFound) {
		if rec.CreatedAt.IsZero() {
			rec.CreatedAt = now
		}
		if rec.UpdatedAt.IsZero() {
			rec.UpdatedAt = rec.CreatedAt
		}
		if turn != nil {
			rec.Turns = append(rec.Turns, *turn)
		}
		data.Sessions = append(data.Sessions, rec)
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if cancellationLatched(existing.Status) && !terminal {
		return false, ErrCancelRequested
	}
	if rec.ACPSessionID == "" {
		rec.ACPSessionID = existing.ACPSessionID
	}
	rec.CreatedAt = existing.CreatedAt
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = now
	}
	rec.Turns = slices.Clone(existing.Turns)
	if turn != nil {
		rec.Turns = append(rec.Turns, *turn)
	}
	rec.PendingPrompt = existing.PendingPrompt
	rec.PromptQueue = slices.Clone(existing.PromptQueue)
	preserveCancellationLatch(*existing, &rec)
	*existing = rec
	return true, nil
}

func preserveCancellationLatch(existing SessionRecord, update *SessionRecord) {
	if update == nil {
		return
	}
	switch existing.Status {
	case SessionStatusCanceled:
		update.Status = SessionStatusCanceled
	case SessionStatusCancelRequested:
		if update.Status == SessionStatusCompleted || update.Status == SessionStatusCanceled {
			update.Status = SessionStatusCanceled
			return
		}
		update.Status = SessionStatusCancelRequested
	}
}

func cancellationLatched(status string) bool {
	return status == SessionStatusCancelRequested || status == SessionStatusCanceled
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
	var result SessionRecord
	err := s.transitionSession(rec.ID, nil, func(data *storeFile) (bool, error) {
		existing, err := findSessionRecord(data, rec.ID)
		if errors.Is(err, ErrSessionNotFound) {
			data.Sessions = append(data.Sessions, rec)
			existing = &data.Sessions[len(data.Sessions)-1]
			if existing.CreatedAt.IsZero() {
				existing.CreatedAt = prompt.CreatedAt
			}
		} else if err != nil {
			return false, err
		} else {
			*existing = mergeSessionMetadata(*existing, rec)
		}
		if cancellationLatched(existing.Status) {
			return false, ErrCancelRequested
		}
		existing.Status = SessionStatusQueued
		existing.UpdatedAt = prompt.CreatedAt
		existing.PromptQueue = append(existing.PromptQueue, prompt)
		result = *existing
		return true, nil
	}, nil)
	if err != nil {
		return SessionRecord{}, err
	}
	return result, nil
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
	if strings.TrimSpace(queueID) == "" {
		return errors.New("queue prompt id is required")
	}
	if when.IsZero() {
		when = time.Now().UTC()
	}
	when = when.UTC()
	return s.transitionSession(id, nil, func(data *storeFile) (bool, error) {
		rec, err := findSessionRecord(data, id)
		if err != nil {
			return false, err
		}
		if cancellationLatched(rec.Status) {
			return false, ErrCancelRequested
		}
		prompt, err := findQueuedPrompt(rec, id, queueID)
		if err != nil {
			return false, err
		}
		if prompt.Status != QueuePromptStatusPending {
			return false, fmt.Errorf("queue prompt %s is not pending", queueID)
		}
		prompt.Status = QueuePromptStatusRunning
		prompt.StartedAt = &when
		rec.Status = SessionStatusRunning
		rec.UpdatedAt = when
		return true, nil
	}, nil)
}

func (s *Store) MarkQueueCompleted(id, queueID, response, stopReason string, when time.Time) error {
	return s.MarkQueueCompletedWithEvents(id, queueID, response, stopReason, nil, when)
}

func (s *Store) MarkQueueCompletedWithEvents(id, queueID, response, stopReason string, events []EventLogLine, when time.Time) error {
	return s.updateQueuedPromptWithEvents(id, queueID, when, events, true, func(rec *SessionRecord, prompt *QueuedPrompt, at time.Time) {
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
	return s.updateQueuedPromptWithEvents(id, queueID, when, nil, true, func(rec *SessionRecord, prompt *QueuedPrompt, at time.Time) {
		prompt.Status = QueuePromptStatusFailed
		prompt.CompletedAt = &at
		prompt.Error = message
		rec.Status = SessionStatusCompleted
		rec.Summary = summarizeStoreText(message)
	})
}

func (s *Store) CancelPendingQueue(id string, when time.Time) (int, error) {
	if when.IsZero() {
		when = time.Now().UTC()
	}
	when = when.UTC()
	count := 0
	err := s.transitionSession(id, nil, func(data *storeFile) (bool, error) {
		rec, err := findSessionRecord(data, id)
		if err != nil {
			return false, err
		}
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
		terminalized := false
		if cancellationLatched(rec.Status) && !hasRunning && rec.Status != SessionStatusCanceled {
			rec.Status = SessionStatusCanceled
			terminalized = true
		}
		if count == 0 && !terminalized {
			return false, nil
		}
		if !hasRunning {
			rec.Status = SessionStatusCanceled
		}
		rec.UpdatedAt = when
		return true, nil
	}, nil)
	return count, err
}

func (s *Store) RecoverStaleQueue(id string, when time.Time) (int, error) {
	if _, err := s.Owner(id); err == nil {
		return 0, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	count, _, err := s.recoverRunningQueueItems(id, when)
	return count, err
}

func (s *Store) recoverRunningQueueItems(id string, when time.Time) (count int, canceled bool, err error) {
	if when.IsZero() {
		when = time.Now().UTC()
	}
	when = when.UTC()
	err = s.transitionSession(id, nil, func(data *storeFile) (bool, error) {
		rec, err := findSessionRecord(data, id)
		if err != nil {
			return false, err
		}
		latched := cancellationLatched(rec.Status)
		canceled = latched
		for i := range rec.PromptQueue {
			prompt := &rec.PromptQueue[i]
			if latched {
				if prompt.Status != QueuePromptStatusRunning && prompt.Status != QueuePromptStatusPending {
					continue
				}
				prompt.Status = QueuePromptStatusCanceled
				prompt.CanceledAt = &when
				prompt.StartedAt = nil
				count++
				continue
			}
			if prompt.Status == QueuePromptStatusRunning {
				prompt.Status = QueuePromptStatusPending
				prompt.StartedAt = nil
				count++
			}
		}
		if count == 0 && !latched {
			return false, nil
		}
		if latched {
			rec.Status = SessionStatusCanceled
		} else {
			rec.Status = SessionStatusQueued
		}
		rec.UpdatedAt = when
		return true, nil
	}, nil)
	return count, canceled, err
}

func (s *Store) MarkPendingCanceled(id string, when time.Time) error {
	when = when.UTC()
	return s.transitionSession(id, nil, func(data *storeFile) (bool, error) {
		rec, err := findSessionRecord(data, id)
		if err != nil {
			return false, err
		}
		if rec.PendingPrompt == nil || rec.PendingPrompt.Status != PendingPromptStatusPending {
			return false, fmt.Errorf("session %s has no pending prompt", id)
		}
		rec.PendingPrompt.Status = PendingPromptStatusCanceled
		rec.PendingPrompt.CanceledAt = &when
		pendingID := rec.PendingPrompt.ID
		if pendingID == "" {
			pendingID = storeKey(rec.PendingPrompt.Prompt)
		}
		for i := range rec.PromptQueue {
			if rec.PromptQueue[i].ID == pendingID {
				rec.PromptQueue[i].Status = QueuePromptStatusCanceled
				rec.PromptQueue[i].CanceledAt = &when
			}
		}
		rec.Status = SessionStatusCanceled
		rec.UpdatedAt = when
		return true, nil
	}, nil)
}

func (s *Store) AcquireOwnerLease(owner OwnerLock) (*OwnerLease, error) {
	if s == nil {
		return nil, errors.New("acp client store is required")
	}
	if strings.TrimSpace(owner.SessionID) == "" {
		return nil, errors.New("owner session id is required")
	}
	owner.Kind = strings.TrimSpace(owner.Kind)
	if owner.Kind != "" && owner.Kind != OwnerKindExecution && owner.Kind != OwnerKindSnapshot {
		return nil, fmt.Errorf("%w: unsupported owner kind %q", ErrInvalidOwnerLock, owner.Kind)
	}
	if owner.StartedAt.IsZero() {
		owner.StartedAt = time.Now().UTC()
	}
	var lease *OwnerLease
	err := withStoreProcessLock(s.ownerClaimPath(owner.SessionID), func() error {
		release, acquired, err := tryStoreFileLock(s.ownerLeasePath(owner.SessionID))
		if err != nil {
			return err
		}
		if !acquired {
			return fmt.Errorf("%w: %s", ErrOwnerLeaseBusy, owner.SessionID)
		}
		if err := backgroundWriteJSONAtomic(s.ownerPath(owner.SessionID), owner); err != nil {
			return errors.Join(err, release())
		}
		lease = &OwnerLease{store: s, owner: owner, release: release}
		return nil
	})
	return lease, err
}

func (l *OwnerLease) Release() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return nil
	}
	return withStoreProcessLock(l.store.ownerClaimPath(l.owner.SessionID), func() error {
		removeErr := backgroundRemoveFile(l.store.ownerPath(l.owner.SessionID))
		releaseErr := l.release()
		l.released = true
		return errors.Join(removeErr, releaseErr)
	})
}

func (s *Store) Owner(id string) (OwnerLock, error) {
	var owner OwnerLock
	err := withStoreProcessLock(s.ownerClaimPath(id), func() error {
		var err error
		owner, err = s.liveOwnerLocked(id)
		return err
	})
	return owner, err
}

func (s *Store) liveOwnerLocked(id string) (OwnerLock, error) {
	release, acquired, err := tryStoreFileLock(s.ownerLeasePath(id))
	if err != nil {
		return OwnerLock{}, err
	}
	if acquired {
		removeErr := backgroundRemoveFile(s.ownerPath(id))
		releaseErr := release()
		if removeErr != nil || releaseErr != nil {
			return OwnerLock{}, errors.Join(removeErr, releaseErr)
		}
		return OwnerLock{}, fmt.Errorf("%w: %s", os.ErrNotExist, id)
	}
	var owner OwnerLock
	if err := readJSONFile(s.ownerPath(id), &owner); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return OwnerLock{}, fmt.Errorf("%w: live lease metadata unavailable for %s", ErrInvalidOwnerLock, id)
		}
		return OwnerLock{}, err
	}
	if owner.SessionID == "" {
		owner.SessionID = id
	}
	if owner.PID == 0 || owner.StartedAt.IsZero() {
		return OwnerLock{}, fmt.Errorf("%w: %s", ErrInvalidOwnerLock, id)
	}
	if owner.Kind != "" && owner.Kind != OwnerKindExecution && owner.Kind != OwnerKindSnapshot {
		return OwnerLock{}, fmt.Errorf("%w: unsupported owner kind %q", ErrInvalidOwnerLock, owner.Kind)
	}
	return owner, nil
}

func (s *Store) RequestCancelActiveExecution(id string, when time.Time) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("cancel session id is required")
	}
	return withStoreProcessLock(s.ownerClaimPath(id), func() error {
		owner, err := s.liveOwnerLocked(id)
		if err != nil {
			return err
		}
		if !owner.Cancelable() {
			return fmt.Errorf("%w: %s", ErrOwnerNotCancelable, id)
		}
		return s.withLaunchAdmission(id, func() error {
			return s.requestCancel(id, when)
		})
	})
}

func (s *Store) RequestCancel(id string, when time.Time) error {
	return s.withLaunchAdmission(id, func() error {
		return s.requestCancel(id, when)
	})
}

func (s *Store) requestCancel(id string, when time.Time) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("cancel session id is required")
	}
	if when.IsZero() {
		when = time.Now().UTC()
	}
	req := CancelRequest{SessionID: id, RequestedAt: when.UTC()}
	return s.transitionSession(id, nil, func(data *storeFile) (bool, error) {
		rec, err := findSessionRecord(data, id)
		if errors.Is(err, ErrSessionNotFound) {
			data.Sessions = append(data.Sessions, SessionRecord{
				ID:        id,
				Status:    SessionStatusCancelRequested,
				CreatedAt: req.RequestedAt,
				UpdatedAt: req.RequestedAt,
			})
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if rec.Status != SessionStatusCanceled {
			rec.Status = SessionStatusCancelRequested
		}
		rec.UpdatedAt = req.RequestedAt
		return true, nil
	}, func() error {
		return s.writeCancel(s.cancelPath(id), req)
	})
}

func (s *Store) writeCancel(path string, request CancelRequest) error {
	if s.cancelWriter != nil {
		return s.cancelWriter(path, request)
	}
	return backgroundWriteJSONAtomic(path, request)
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

// CheckCancellation reads the session record, which is the cancellation
// authority. The legacy sidecar remains a best-effort compatibility projection.
func (s *Store) CheckCancellation(id string) (bool, error) {
	rec, err := s.Get(id)
	if err != nil {
		return false, err
	}
	return rec.Status == SessionStatusCancelRequested || rec.Status == SessionStatusCanceled, nil
}

func (s *Store) ReconcileCancellationRequests() error {
	if s == nil {
		return errors.New("acp client store is required")
	}
	return s.withFileLock(func() error {
		data, err := s.loadUnlocked()
		if err != nil {
			return err
		}
		if s.transactionLoaded != nil {
			s.transactionLoaded()
		}
		dir := filepath.Join(filepath.Dir(s.path), "cancel-requests")
		wanted := make(map[string]CancelRequest)
		for _, rec := range data.Sessions {
			if !cancellationLatched(rec.Status) {
				continue
			}
			name := storeKey(rec.ID) + ".json"
			wanted[name] = CancelRequest{SessionID: rec.ID, RequestedAt: rec.UpdatedAt}
		}

		var projectionErr error
		for name, request := range wanted {
			if err := s.writeCancel(filepath.Join(dir, name), request); err != nil {
				projectionErr = errors.Join(projectionErr, transitionConfirmationCause(err))
			}
		}
		entries, err := os.ReadDir(dir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			projectionErr = errors.Join(projectionErr, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			if _, ok := wanted[entry.Name()]; ok {
				continue
			}
			projectionErr = errors.Join(projectionErr, transitionConfirmationCause(backgroundRemoveFile(filepath.Join(dir, entry.Name()))))
		}
		if projectionErr != nil {
			return storeCommitUnconfirmed(projectionErr)
		}
		return nil
	})
}

func (s *Store) readTransaction(read func(storeFile) error) error {
	return s.withFileLock(func() error {
		data, err := s.loadUnlocked()
		if err != nil {
			return err
		}
		return read(data)
	})
}

func (s *Store) withFileLock(operation func() error) (err error) {
	return withStoreProcessLock(s.lockPath(), operation)
}

func (s *Store) withLaunchAdmission(id string, operation func() error) error {
	if s.beforeLaunchAdmission != nil {
		s.beforeLaunchAdmission()
	}
	return withStoreProcessLock(s.launchAdmissionPath(id), operation)
}

func withStoreProcessLock(path string, operation func() error) (err error) {
	lock := backgroundPathLock(path)
	lock.Lock()
	defer lock.Unlock()
	release, err := acquireStoreFileLock(path)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, release())
	}()
	return operation()
}

func (s *Store) loadUnlocked() (storeFile, error) {
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

func (s *Store) saveUnlocked(data storeFile) error {
	normalizeRecords(data.Sessions)
	slices.SortFunc(data.Sessions, func(a, b SessionRecord) int {
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})
	if s.sessionWriter != nil {
		return s.sessionWriter(data)
	}
	return backgroundWriteJSONAtomic(s.path, data)
}

func (s *Store) lockPath() string {
	return s.path + ".lock"
}

func (s *Store) ownerPath(id string) string {
	return filepath.Join(filepath.Dir(s.path), "owners", storeKey(id)+".json")
}

func (s *Store) ownerLeasePath(id string) string {
	return filepath.Join(filepath.Dir(s.path), "owners", storeKey(id)+".lock")
}

func (s *Store) ownerClaimPath(id string) string {
	return filepath.Join(filepath.Dir(s.path), "owners", storeKey(id)+".claim.lock")
}

func (s *Store) launchAdmissionPath(id string) string {
	return filepath.Join(filepath.Dir(s.path), "launch-admissions", storeKey(id)+".lock")
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

func (s *Store) updateQueuedPromptWithEvents(id, queueID string, when time.Time, events []EventLogLine, terminal bool, update func(*SessionRecord, *QueuedPrompt, time.Time)) error {
	if strings.TrimSpace(queueID) == "" {
		return errors.New("queue prompt id is required")
	}
	if when.IsZero() {
		when = time.Now().UTC()
	}
	when = when.UTC()
	return s.transitionSession(id, events, func(data *storeFile) (bool, error) {
		rec, err := findSessionRecord(data, id)
		if err != nil {
			return false, err
		}
		prompt, err := findQueuedPrompt(rec, id, queueID)
		if err != nil {
			return false, err
		}
		if terminal && cancellationLatched(rec.Status) {
			prompt.Status = QueuePromptStatusCanceled
			prompt.CanceledAt = &when
			rec.Status = SessionStatusCanceled
			rec.UpdatedAt = when
			return true, nil
		}
		before := *rec
		update(rec, prompt, when)
		preserveCancellationLatch(before, rec)
		rec.UpdatedAt = when
		return true, nil
	}, nil)
}

func (s *Store) setACPSessionID(id, acpSessionID string) error {
	if acpSessionID == "" {
		return nil
	}
	return s.transitionSession(id, nil, func(data *storeFile) (bool, error) {
		rec, err := findSessionRecord(data, id)
		if err != nil {
			return false, err
		}
		if rec.ACPSessionID != "" {
			return false, nil
		}
		rec.ACPSessionID = acpSessionID
		return true, nil
	}, nil)
}

func findQueuedPrompt(rec *SessionRecord, sessionID, queueID string) (*QueuedPrompt, error) {
	for i := range rec.PromptQueue {
		if rec.PromptQueue[i].ID == queueID {
			return &rec.PromptQueue[i], nil
		}
	}
	return nil, fmt.Errorf("%w: session %s queue prompt %s", ErrQueuePromptNotFound, sessionID, queueID)
}

func findSessionRecord(data *storeFile, id string) (*SessionRecord, error) {
	for i := range data.Sessions {
		if data.Sessions[i].ID == id {
			return &data.Sessions[i], nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, id)
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
	runes := []rune(value)
	if len(runes) <= 200 {
		return value
	}
	return string(runes[:200])
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
