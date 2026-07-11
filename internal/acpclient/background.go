package acpclient

import (
	"context"
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

const (
	BackgroundPolicyVersion = 1

	BackgroundStateRunning  = "running"
	BackgroundStateBlocked  = "blocked"
	BackgroundStateError    = "error"
	BackgroundStateDisabled = "disabled"

	BackgroundOutcomeStarted           = "started"
	BackgroundOutcomeResumed           = "resumed"
	BackgroundOutcomeStopped           = "stopped"
	BackgroundOutcomeProfileUntrusted  = "profile_untrusted"
	BackgroundOutcomeProfileDrift      = "profile_drift"
	BackgroundOutcomeProfileMissing    = "profile_missing"
	BackgroundOutcomeSessionMissing    = "session_missing"
	BackgroundOutcomePolicyInvalid     = "policy_invalid"
	BackgroundOutcomeWorkerError       = "worker_error"
	BackgroundOutcomeWorkerPanic       = "worker_panic"
	BackgroundOutcomeStateWriteFailed  = "state_write_failed"
	BackgroundOutcomeAuditAppendFailed = "audit_append_failed"
)

var (
	ErrBackgroundPolicyNotFound          = errors.New("acp background policy not found")
	ErrBackgroundPolicyConflict          = errors.New("acp background policy conflicts with active worker")
	ErrBackgroundAcknowledgementRequired = errors.New("acp background unattended execution acknowledgement is required")
	ErrBackgroundProfileUntrusted        = errors.New("acp background profile trust is invalid")
	ErrBackgroundProfileIneligible       = errors.New("acp background profile is ineligible")
	ErrBackgroundManagerClosed           = errors.New("acp background manager is closed")
)

type BackgroundPolicy struct {
	SessionID      string    `json:"sessionId"`
	Profile        string    `json:"profile"`
	DescriptorHash string    `json:"descriptorHash"`
	PolicyVersion  int       `json:"policyVersion"`
	AcknowledgedAt time.Time `json:"acknowledgedAt"`
	Enabled        bool      `json:"enabled"`
	State          string    `json:"state"`
	Outcome        string    `json:"outcome"`
	StartedAt      time.Time `json:"startedAt,omitzero"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type BackgroundStatus struct {
	SessionID      string
	Profile        string
	DescriptorHash string
	PolicyVersion  int
	AcknowledgedAt time.Time
	Enabled        bool
	State          string
	Outcome        string
	StartedAt      time.Time
	UpdatedAt      time.Time
}

type BackgroundStore struct {
	path string
	mu   sync.Mutex
}

type backgroundFile struct {
	Policies []BackgroundPolicy `json:"policies"`
}

func NewBackgroundStore(path string) *BackgroundStore {
	return &BackgroundStore{path: path}
}

func NewDefaultBackgroundStore() (*BackgroundStore, error) {
	store, err := NewDefaultStore()
	if err != nil {
		return nil, err
	}
	return NewBackgroundStore(filepath.Join(filepath.Dir(store.Path()), "background.json")), nil
}

func (s *BackgroundStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *BackgroundStore) List() ([]BackgroundPolicy, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil, errors.New("acp background policy path is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.load()
	if err != nil {
		return nil, err
	}
	policies := slices.Clone(data.Policies)
	slices.SortFunc(policies, func(a, b BackgroundPolicy) int {
		return strings.Compare(a.SessionID, b.SessionID)
	})
	return policies, nil
}

func (s *BackgroundStore) Get(sessionID string) (BackgroundPolicy, error) {
	policies, err := s.List()
	if err != nil {
		return BackgroundPolicy{}, err
	}
	for _, policy := range policies {
		if policy.SessionID == sessionID {
			return policy, nil
		}
	}
	return BackgroundPolicy{}, fmt.Errorf("%w: %s", ErrBackgroundPolicyNotFound, sessionID)
}

func (s *BackgroundStore) Upsert(policy BackgroundPolicy) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return errors.New("acp background policy path is required")
	}
	policy.SessionID = strings.TrimSpace(policy.SessionID)
	policy.Profile = strings.TrimSpace(policy.Profile)
	if policy.SessionID == "" {
		return errors.New("acp background session id is required")
	}
	if policy.Profile == "" {
		return errors.New("acp background profile is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.load()
	if err != nil {
		return err
	}
	for i := range data.Policies {
		if data.Policies[i].SessionID == policy.SessionID {
			data.Policies[i] = policy
			return s.save(data)
		}
	}
	data.Policies = append(data.Policies, policy)
	return s.save(data)
}

func (s *BackgroundStore) load() (backgroundFile, error) {
	var data backgroundFile
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return data, nil
	}
	if err != nil {
		return backgroundFile{}, err
	}
	if len(b) == 0 {
		return data, nil
	}
	if err := json.Unmarshal(b, &data); err != nil {
		return backgroundFile{}, fmt.Errorf("read background policies %s: %w", s.path, err)
	}
	return data, nil
}

func (s *BackgroundStore) save(data backgroundFile) error {
	slices.SortFunc(data.Policies, func(a, b BackgroundPolicy) int {
		return strings.Compare(a.SessionID, b.SessionID)
	})
	return writeJSONFileAtomic(s.path, data, 0o600)
}

type ResolvedBackgroundProfile struct {
	Spec           AgentSpec
	Options        RunOptions
	DescriptorHash string
	TrustValid     bool
}

type BackgroundProfileResolver func(string) (ResolvedBackgroundProfile, error)

func NewBackgroundProfileResolver(registry Registry, profiles *ProfileStore) BackgroundProfileResolver {
	return func(name string) (ResolvedBackgroundProfile, error) {
		name = strings.TrimSpace(name)
		if name == "" {
			return ResolvedBackgroundProfile{}, fmt.Errorf("%w: profile name is required", ErrProfileNotFound)
		}
		if spec, ok := registry.Lookup(name); ok {
			if err := spec.Validate(); err != nil {
				return ResolvedBackgroundProfile{}, fmt.Errorf("%w: %s", ErrBackgroundProfileIneligible, name)
			}
			return ResolvedBackgroundProfile{
				Spec:           spec,
				DescriptorHash: spec.Fingerprint(),
				TrustValid:     true,
			}, nil
		}
		if profiles == nil {
			return ResolvedBackgroundProfile{}, fmt.Errorf("%w: %s", ErrProfileNotFound, name)
		}
		profile, err := profiles.Get(name)
		if err != nil {
			return ResolvedBackgroundProfile{}, err
		}
		return ResolvedBackgroundProfile{
			Spec:           cloneSpec(profile.Spec),
			Options:        RunOptions{Agent: name, Cwd: profile.Cwd},
			DescriptorHash: profile.DescriptorHash(),
			TrustValid:     profile.TrustValid(),
		}, nil
	}
}

type BackgroundWatcher func(context.Context, *Store, AgentSpec, RunOptions, string, WatchOptions, func(WatchCycle)) (WatchResult, error)

type BackgroundManagerOptions struct {
	Context      context.Context
	Now          func() time.Time
	Resolver     BackgroundProfileResolver
	Watcher      BackgroundWatcher
	WatchOptions WatchOptions
}

type backgroundWorker struct {
	profile        string
	descriptorHash string
	policy         BackgroundPolicy
	cancel         context.CancelFunc
}

type backgroundTerminalGuard struct {
	status        BackgroundStatus
	stateRecorded bool
	auditRecorded bool
}

type BackgroundManager struct {
	sessions *Store
	store    *BackgroundStore
	audit    *BackgroundAudit
	resolver BackgroundProfileResolver
	watcher  BackgroundWatcher
	watch    WatchOptions
	now      func() time.Time

	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	closed   bool
	active   map[string]backgroundWorker
	terminal map[string]backgroundTerminalGuard
	wg       sync.WaitGroup
}

func NewBackgroundManager(sessions *Store, store *BackgroundStore, audit *BackgroundAudit, opts BackgroundManagerOptions) *BackgroundManager {
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	watcher := opts.Watcher
	if watcher == nil {
		watcher = WatchQueue
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &BackgroundManager{
		sessions: sessions,
		store:    store,
		audit:    audit,
		resolver: opts.Resolver,
		watcher:  watcher,
		watch:    opts.WatchOptions,
		now:      now,
		ctx:      ctx,
		cancel:   cancel,
		active:   make(map[string]backgroundWorker),
		terminal: make(map[string]backgroundTerminalGuard),
	}
}

func NewDefaultBackgroundManager(ctx context.Context, sessions *Store, profiles *ProfileStore, registry Registry) (*BackgroundManager, error) {
	if sessions == nil || strings.TrimSpace(sessions.Path()) == "" {
		return nil, errors.New("acp client store is required")
	}
	dir := filepath.Dir(sessions.Path())
	return NewBackgroundManager(
		sessions,
		NewBackgroundStore(filepath.Join(dir, "background.json")),
		NewBackgroundAudit(filepath.Join(dir, "background-audit.jsonl")),
		BackgroundManagerOptions{
			Context:  ctx,
			Resolver: NewBackgroundProfileResolver(registry, profiles),
		},
	), nil
}

func (m *BackgroundManager) Start(sessionID, profile string, acknowledged bool) (BackgroundStatus, error) {
	if !acknowledged {
		return BackgroundStatus{}, ErrBackgroundAcknowledgementRequired
	}
	sessionID = strings.TrimSpace(sessionID)
	profile = strings.TrimSpace(profile)
	if sessionID == "" {
		return BackgroundStatus{}, errors.New("acp background session id is required")
	}
	if profile == "" {
		return BackgroundStatus{}, errors.New("acp background profile is required")
	}
	if err := m.validate(); err != nil {
		return BackgroundStatus{}, err
	}
	if _, err := m.sessions.Get(sessionID); err != nil {
		return BackgroundStatus{}, err
	}
	resolved, err := m.resolver(profile)
	if err != nil {
		return BackgroundStatus{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return BackgroundStatus{}, ErrBackgroundManagerClosed
	}
	if worker, ok := m.active[sessionID]; ok {
		if worker.profile == profile && worker.descriptorHash == resolved.DescriptorHash && resolved.TrustValid {
			policy, err := m.store.Get(sessionID)
			return backgroundStatus(policy), err
		}
		return BackgroundStatus{}, fmt.Errorf("%w: %s", ErrBackgroundPolicyConflict, sessionID)
	}

	now := m.currentTime()
	policy := BackgroundPolicy{
		SessionID:      sessionID,
		Profile:        profile,
		DescriptorHash: resolved.DescriptorHash,
		PolicyVersion:  BackgroundPolicyVersion,
		AcknowledgedAt: now,
		Enabled:        true,
		UpdatedAt:      now,
	}
	if !resolved.TrustValid || strings.TrimSpace(resolved.DescriptorHash) == "" {
		policy.State = BackgroundStateBlocked
		policy.Outcome = BackgroundOutcomeProfileUntrusted
		if err := m.persistAndAudit(policy, BackgroundAuditBlock); err != nil {
			return backgroundStatus(policy), err
		}
		return backgroundStatus(policy), ErrBackgroundProfileUntrusted
	}
	if err := resolved.Spec.Validate(); err != nil {
		return BackgroundStatus{}, fmt.Errorf("%w: %s", ErrBackgroundProfileIneligible, profile)
	}
	policy.State = BackgroundStateRunning
	policy.Outcome = BackgroundOutcomeStarted
	policy.StartedAt = now
	persisted, err := m.persistBeforeLaunch(policy, BackgroundAuditStart)
	if err != nil {
		m.terminal[sessionID] = backgroundTerminalGuard{status: backgroundStatus(persisted)}
		return backgroundStatus(persisted), err
	}
	delete(m.terminal, sessionID)
	m.launch(persisted, resolved)
	return backgroundStatus(persisted), nil
}

func (m *BackgroundManager) Stop(sessionID string) (BackgroundStatus, error) {
	if err := m.validate(); err != nil {
		return BackgroundStatus{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	policy, err := m.store.Get(sessionID)
	if err != nil {
		return BackgroundStatus{}, err
	}
	policy.Enabled = false
	policy.State = BackgroundStateDisabled
	policy.Outcome = BackgroundOutcomeStopped
	policy.UpdatedAt = m.currentTime()
	if err := m.store.Upsert(policy); err != nil {
		return backgroundStatus(policy), err
	}
	auditErr := m.appendAudit(policy, BackgroundAuditStop)
	delete(m.terminal, sessionID)
	if worker, ok := m.active[sessionID]; ok {
		worker.cancel()
	}
	return backgroundStatus(policy), auditErr
}

func (m *BackgroundManager) Get(sessionID string) (BackgroundStatus, error) {
	if err := m.validate(); err != nil {
		return BackgroundStatus{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if guard, ok := m.terminal[sessionID]; ok {
		return guard.status, nil
	}
	policy, err := m.store.Get(sessionID)
	return backgroundStatus(policy), err
}

func (m *BackgroundManager) List() ([]BackgroundStatus, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	policies, err := m.store.List()
	if err != nil {
		return nil, err
	}
	statuses := make([]BackgroundStatus, 0, len(policies)+len(m.terminal))
	seen := make(map[string]struct{}, len(policies))
	for _, policy := range policies {
		seen[policy.SessionID] = struct{}{}
		if guard, ok := m.terminal[policy.SessionID]; ok {
			statuses = append(statuses, guard.status)
			continue
		}
		statuses = append(statuses, backgroundStatus(policy))
	}
	for sessionID, guard := range m.terminal {
		if _, ok := seen[sessionID]; !ok {
			statuses = append(statuses, guard.status)
		}
	}
	slices.SortFunc(statuses, func(a, b BackgroundStatus) int { return strings.Compare(a.SessionID, b.SessionID) })
	return statuses, nil
}

func (m *BackgroundManager) Resume() error {
	if err := m.validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrBackgroundManagerClosed
	}
	policies, err := m.store.List()
	if err != nil {
		return err
	}
	for _, policy := range policies {
		if !policy.Enabled {
			continue
		}
		if _, guarded := m.terminal[policy.SessionID]; guarded {
			continue
		}
		if _, ok := m.active[policy.SessionID]; ok {
			continue
		}
		if policy.PolicyVersion != BackgroundPolicyVersion || policy.AcknowledgedAt.IsZero() {
			if err := m.block(policy, BackgroundOutcomePolicyInvalid); err != nil {
				return err
			}
			continue
		}
		if _, err := m.sessions.Get(policy.SessionID); err != nil {
			if errors.Is(err, ErrSessionNotFound) {
				if err := m.block(policy, BackgroundOutcomeSessionMissing); err != nil {
					return err
				}
				continue
			}
			return err
		}
		resolved, err := m.resolver(policy.Profile)
		if err != nil {
			if errors.Is(err, ErrProfileNotFound) || errors.Is(err, ErrUnknownAgent) {
				if err := m.block(policy, BackgroundOutcomeProfileMissing); err != nil {
					return err
				}
				continue
			}
			return err
		}
		if !resolved.TrustValid {
			if err := m.block(policy, BackgroundOutcomeProfileUntrusted); err != nil {
				return err
			}
			continue
		}
		if resolved.DescriptorHash != policy.DescriptorHash {
			if err := m.block(policy, BackgroundOutcomeProfileDrift); err != nil {
				return err
			}
			continue
		}
		if err := resolved.Spec.Validate(); err != nil {
			if err := m.block(policy, BackgroundOutcomeProfileUntrusted); err != nil {
				return err
			}
			continue
		}
		now := m.currentTime()
		policy.State = BackgroundStateRunning
		policy.Outcome = BackgroundOutcomeResumed
		policy.StartedAt = now
		policy.UpdatedAt = now
		persisted, err := m.persistBeforeLaunch(policy, BackgroundAuditResume)
		if err != nil {
			m.terminal[policy.SessionID] = backgroundTerminalGuard{status: backgroundStatus(persisted)}
			return err
		}
		delete(m.terminal, policy.SessionID)
		m.launch(persisted, resolved)
	}
	return nil
}

func (m *BackgroundManager) Shutdown() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		m.cancel()
		for _, worker := range m.active {
			worker.cancel()
		}
	}
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *BackgroundManager) validate() error {
	if m == nil {
		return errors.New("acp background manager is required")
	}
	if m.sessions == nil {
		return errors.New("acp client store is required")
	}
	if m.store == nil {
		return errors.New("acp background policy store is required")
	}
	if m.audit == nil {
		return errors.New("acp background audit is required")
	}
	if m.resolver == nil {
		return errors.New("acp background profile resolver is required")
	}
	return nil
}

func (m *BackgroundManager) launch(policy BackgroundPolicy, resolved ResolvedBackgroundProfile) {
	ctx, cancel := context.WithCancel(m.ctx)
	m.active[policy.SessionID] = backgroundWorker{
		profile:        policy.Profile,
		descriptorHash: policy.DescriptorHash,
		policy:         policy,
		cancel:         cancel,
	}
	m.wg.Go(func() {
		outcome, err := m.watchWorker(ctx, resolved, policy.SessionID)
		m.workerDone(policy.SessionID, ctx, outcome, err)
	})
}

func (m *BackgroundManager) watchWorker(ctx context.Context, resolved ResolvedBackgroundProfile, sessionID string) (outcome string, err error) {
	defer func() {
		if recover() != nil {
			outcome = BackgroundOutcomeWorkerPanic
			err = errors.New("acp background watcher panic")
		}
	}()
	_, err = m.watcher(ctx, m.sessions, resolved.Spec, resolved.Options, sessionID, m.watch, nil)
	return BackgroundOutcomeWorkerError, err
}

func (m *BackgroundManager) workerDone(sessionID string, ctx context.Context, outcome string, workerErr error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	worker, ok := m.active[sessionID]
	delete(m.active, sessionID)
	if workerErr == nil || errors.Is(workerErr, context.Canceled) || ctx.Err() != nil {
		return
	}
	if !ok {
		return
	}
	policy := worker.policy
	policy.Enabled = false
	policy.State = BackgroundStateError
	policy.Outcome = outcome
	policy.UpdatedAt = m.currentTime()
	stateErr := m.store.Upsert(policy)
	auditErr := m.appendAudit(policy, BackgroundAuditError)
	m.terminal[sessionID] = backgroundTerminalGuard{
		status:        backgroundStatus(policy),
		stateRecorded: stateErr == nil,
		auditRecorded: auditErr == nil,
	}
}

func (m *BackgroundManager) block(policy BackgroundPolicy, outcome string) error {
	policy.State = BackgroundStateBlocked
	policy.Outcome = outcome
	policy.UpdatedAt = m.currentTime()
	return m.persistAndAudit(policy, BackgroundAuditBlock)
}

func (m *BackgroundManager) persistAndAudit(policy BackgroundPolicy, action string) error {
	if err := m.store.Upsert(policy); err != nil {
		return err
	}
	return m.audit.Append(BackgroundAuditRecord{
		At:             policy.UpdatedAt,
		Action:         action,
		SessionID:      policy.SessionID,
		Profile:        policy.Profile,
		DescriptorHash: policy.DescriptorHash,
		Outcome:        policy.Outcome,
	})
}

func (m *BackgroundManager) persistBeforeLaunch(policy BackgroundPolicy, action string) (BackgroundPolicy, error) {
	if err := m.store.Upsert(policy); err != nil {
		return m.recordingFailure(policy, BackgroundOutcomeStateWriteFailed), err
	}
	if err := m.appendAudit(policy, action); err != nil {
		failed := m.recordingFailure(policy, BackgroundOutcomeAuditAppendFailed)
		return failed, errors.Join(err, m.store.Upsert(failed))
	}
	return policy, nil
}

func (m *BackgroundManager) recordingFailure(policy BackgroundPolicy, outcome string) BackgroundPolicy {
	policy.Enabled = false
	policy.State = BackgroundStateError
	policy.Outcome = outcome
	policy.UpdatedAt = m.currentTime()
	return policy
}

func (m *BackgroundManager) appendAudit(policy BackgroundPolicy, action string) error {
	return m.audit.Append(BackgroundAuditRecord{
		At:             policy.UpdatedAt,
		Action:         action,
		SessionID:      policy.SessionID,
		Profile:        policy.Profile,
		DescriptorHash: policy.DescriptorHash,
		Outcome:        policy.Outcome,
	})
}

func (m *BackgroundManager) currentTime() time.Time {
	return m.now().UTC()
}

func backgroundStatus(policy BackgroundPolicy) BackgroundStatus {
	return BackgroundStatus(policy)
}
