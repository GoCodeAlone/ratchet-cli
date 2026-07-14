package acpclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	BackgroundOutcomeCompleted         = "completed"
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
	ErrBackgroundTransitionBusy          = errors.New("acp background session transition is busy")
	ErrBackgroundPersistenceDegraded     = errors.New("acp background persistence is degraded")
)

type BackgroundPolicy struct {
	SessionID           string    `json:"sessionId"`
	Profile             string    `json:"profile"`
	DescriptorHash      string    `json:"descriptorHash"`
	PolicyVersion       int       `json:"policyVersion"`
	AcknowledgedAt      time.Time `json:"acknowledgedAt"`
	Enabled             bool      `json:"enabled"`
	State               string    `json:"state"`
	Outcome             string    `json:"outcome"`
	PersistenceDegraded bool      `json:"persistenceDegraded,omitzero"`
	StartedAt           time.Time `json:"startedAt,omitzero"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type BackgroundStatus struct {
	SessionID           string
	Profile             string
	DescriptorHash      string
	PolicyVersion       int
	AcknowledgedAt      time.Time
	Enabled             bool
	State               string
	Outcome             string
	PersistenceDegraded bool
	StartedAt           time.Time
	UpdatedAt           time.Time
}

type BackgroundStore struct {
	path                   string
	afterListTransitionIDs func()
}

type backgroundFile struct {
	Policies []BackgroundPolicy `json:"policies"`
}

type backgroundTransition struct {
	Policy  BackgroundPolicy `json:"policy"`
	Action  string           `json:"action"`
	EventID string           `json:"eventId"`
}

type backgroundTransitionFile struct {
	Transitions []backgroundTransition `json:"transitions"`
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
	var policies []BackgroundPolicy
	err := withStoreProcessLock(s.path+".lock", func() error {
		data, err := s.load(s.path)
		if err != nil {
			return err
		}
		policies = slices.Clone(data.Policies)
		return nil
	})
	if err != nil {
		return nil, err
	}
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
	return withStoreProcessLock(s.path+".lock", func() error {
		data, err := s.load(s.path)
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
	})
}

func (s *BackgroundStore) load(path string) (backgroundFile, error) {
	var data backgroundFile
	b, err := os.ReadFile(path)
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
		return backgroundFile{}, fmt.Errorf("read background policies %s: %w", path, err)
	}
	return data, nil
}

func (s *BackgroundStore) save(data backgroundFile) error {
	slices.SortFunc(data.Policies, func(a, b BackgroundPolicy) int {
		return strings.Compare(a.SessionID, b.SessionID)
	})
	return backgroundWriteJSONAtomic(s.path, data)
}

func (s *BackgroundStore) putTransition(transition backgroundTransition) error {
	if strings.TrimSpace(transition.EventID) == "" {
		return errors.New("acp background transition event id is required")
	}
	path := s.transitionPath()
	return withStoreProcessLock(path+".lock", func() error {
		data, err := s.loadTransitions(path)
		if err != nil {
			return err
		}
		for i := range data.Transitions {
			if data.Transitions[i].Policy.SessionID == transition.Policy.SessionID {
				data.Transitions[i] = transition
				return backgroundWriteJSONAtomic(path, data)
			}
		}
		data.Transitions = append(data.Transitions, transition)
		slices.SortFunc(data.Transitions, func(a, b backgroundTransition) int {
			return strings.Compare(a.Policy.SessionID, b.Policy.SessionID)
		})
		return backgroundWriteJSONAtomic(path, data)
	})
}

func (s *BackgroundStore) listTransitionIDs() ([]string, error) {
	path := s.transitionPath()
	var sessionIDs []string
	err := withStoreProcessLock(path+".lock", func() error {
		data, err := s.loadTransitions(path)
		if err != nil {
			return err
		}
		sessionIDs = make([]string, 0, len(data.Transitions))
		for _, transition := range data.Transitions {
			sessionIDs = append(sessionIDs, transition.Policy.SessionID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if s.afterListTransitionIDs != nil {
		s.afterListTransitionIDs()
	}
	return sessionIDs, nil
}

func (s *BackgroundStore) getTransition(sessionID string) (backgroundTransition, bool, error) {
	path := s.transitionPath()
	var current backgroundTransition
	var found bool
	err := withStoreProcessLock(path+".lock", func() error {
		data, err := s.loadTransitions(path)
		if err != nil {
			return err
		}
		for _, transition := range data.Transitions {
			if transition.Policy.SessionID == sessionID {
				current = transition
				found = true
				break
			}
		}
		return nil
	})
	return current, found, err
}

func (s *BackgroundStore) removeTransition(sessionID string) error {
	path := s.transitionPath()
	return withStoreProcessLock(path+".lock", func() error {
		data, err := s.loadTransitions(path)
		if err != nil {
			return err
		}
		next := data.Transitions[:0]
		for _, transition := range data.Transitions {
			if transition.Policy.SessionID != sessionID {
				next = append(next, transition)
			}
		}
		if len(next) == len(data.Transitions) {
			return nil
		}
		if len(next) == 0 {
			return backgroundRemoveFile(path)
		}
		data.Transitions = next
		return backgroundWriteJSONAtomic(path, data)
	})
}

func (s *BackgroundStore) transitionPath() string {
	ext := filepath.Ext(s.path)
	base := strings.TrimSuffix(s.path, ext)
	return base + "-transitions" + ext
}

func (s *BackgroundStore) loadTransitions(path string) (backgroundTransitionFile, error) {
	var data backgroundTransitionFile
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return data, nil
	}
	if err != nil {
		return backgroundTransitionFile{}, err
	}
	if len(b) == 0 {
		return data, nil
	}
	if err := json.Unmarshal(b, &data); err != nil {
		return backgroundTransitionFile{}, fmt.Errorf("read background transitions %s: %w", path, err)
	}
	return data, nil
}

type ResolvedBackgroundProfile struct {
	Spec               AgentSpec
	Options            RunOptions
	DescriptorHash     string
	TrustValid         bool
	WithTrustedProfile func(string, func(Profile) error) error
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
			WithTrustedProfile: func(pinnedHash string, callback func(Profile) error) error {
				return profiles.WithTrustedProfile(name, pinnedHash, callback)
			},
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
	ctx            context.Context
	cancel         context.CancelCauseFunc
	done           chan struct{}
	outcome        string
	err            error
	releaseLease   func() error
}

type backgroundStopOwner struct {
	handoff chan bool
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

	ctx             context.Context
	cancel          context.CancelCauseFunc
	stopParent      func() bool
	mu              sync.Mutex
	resumeMu        sync.Mutex
	closed          bool
	active          map[string]*backgroundWorker
	transitions     map[string]struct{}
	transitionLease map[string]func() error
	stopping        map[string]*backgroundStopOwner
	terminal        map[string]backgroundTerminalGuard
	lifecycle       sync.WaitGroup
	wg              sync.WaitGroup
}

var (
	errBackgroundStop     = errors.New("acp background stop requested")
	errBackgroundShutdown = errors.New("acp background shutdown requested")
)

func NewBackgroundManager(sessions *Store, store *BackgroundStore, audit *BackgroundAudit, opts BackgroundManagerOptions) *BackgroundManager {
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	parent := ctx
	ctx, cancel := context.WithCancelCause(context.WithoutCancel(parent))
	stopParent := context.AfterFunc(parent, func() {
		cancel(errBackgroundShutdown)
	})
	if parent.Err() != nil {
		cancel(errBackgroundShutdown)
	}
	watcher := opts.Watcher
	if watcher == nil {
		watcher = WatchQueue
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &BackgroundManager{
		sessions:        sessions,
		store:           store,
		audit:           audit,
		resolver:        opts.Resolver,
		watcher:         watcher,
		watch:           opts.WatchOptions,
		now:             now,
		ctx:             ctx,
		cancel:          cancel,
		stopParent:      stopParent,
		active:          make(map[string]*backgroundWorker),
		transitions:     make(map[string]struct{}),
		transitionLease: make(map[string]func() error),
		stopping:        make(map[string]*backgroundStopOwner),
		terminal:        make(map[string]backgroundTerminalGuard),
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

func (m *BackgroundManager) Start(sessionID, profile string, acknowledged bool) (status BackgroundStatus, err error) {
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
	if err := m.admitLifecycle(); err != nil {
		return BackgroundStatus{}, err
	}
	defer m.lifecycle.Done()
	if err := m.reconcileCancellationProjection(); err != nil {
		return BackgroundStatus{}, err
	}
	canceled, cancelErr := m.recoverCanceledSession(sessionID)
	if cancelErr != nil && !canceled {
		return BackgroundStatus{}, cancelErr
	}
	if canceled {
		return BackgroundStatus{}, errors.Join(ErrCancelRequested, cancelErr)
	}
	resolved, err := m.resolver(profile)
	if err != nil {
		return BackgroundStatus{}, err
	}

	m.mu.Lock()
	if _, busy := m.transitions[sessionID]; busy {
		m.mu.Unlock()
		return BackgroundStatus{}, fmt.Errorf("%w: %s", ErrBackgroundTransitionBusy, sessionID)
	}
	if worker, ok := m.active[sessionID]; ok {
		m.mu.Unlock()
		if worker.profile == profile && worker.descriptorHash == resolved.DescriptorHash && resolved.TrustValid {
			policy, err := m.store.Get(sessionID)
			return backgroundStatus(policy), err
		}
		return BackgroundStatus{}, fmt.Errorf("%w: %s", ErrBackgroundPolicyConflict, sessionID)
	}
	m.mu.Unlock()
	if err := m.reserveTransition(sessionID); err != nil {
		return BackgroundStatus{}, err
	}
	transitionReserved := true
	defer func() {
		if transitionReserved {
			err = errors.Join(err, m.releaseTransition(sessionID))
		}
	}()

	now := m.currentTime()
	policy := BackgroundPolicy{
		SessionID:      sessionID,
		Profile:        profile,
		DescriptorHash: resolved.DescriptorHash,
		PolicyVersion:  BackgroundPolicyVersion,
		AcknowledgedAt: now,
		UpdatedAt:      now,
	}
	if !resolved.TrustValid || strings.TrimSpace(resolved.DescriptorHash) == "" {
		policy.Enabled = false
		policy.State = BackgroundStateBlocked
		policy.Outcome = BackgroundOutcomeProfileUntrusted
		result := m.persistTerminal(policy, BackgroundAuditBlock)
		m.rememberTerminal(result)
		return backgroundStatus(result.policy), errors.Join(ErrBackgroundProfileUntrusted, result.err)
	}
	policy.Enabled = true
	policy.State = BackgroundStateRunning
	policy.Outcome = BackgroundOutcomeStarted
	policy.StartedAt = now
	persisted, resolved, err := m.persistBeforeResolvedLaunch(policy, BackgroundAuditStart, resolved)
	if err != nil {
		if outcome, ok := backgroundProfileLeaseFailure(err); ok {
			policy.Enabled = false
			policy.State = BackgroundStateBlocked
			policy.Outcome = outcome
			result := m.persistTerminal(policy, BackgroundAuditBlock)
			m.rememberTerminal(result)
			return backgroundStatus(result.policy), errors.Join(ErrBackgroundProfileUntrusted, err, result.err)
		}
		return backgroundStatus(persisted), err
	}
	launched, canceled, launchErr := m.launchPersistedPolicy(persisted, resolved)
	if launched {
		transitionReserved = false
		return backgroundStatus(persisted), launchErr
	}
	if canceled {
		_, cancelErr := m.recoverCanceledSession(sessionID)
		persisted.Enabled = false
		persisted.State = BackgroundStateDisabled
		persisted.Outcome = BackgroundOutcomeStopped
		persisted.UpdatedAt = m.currentTime()
		result := m.persistTerminal(persisted, BackgroundAuditStop)
		m.rememberTerminal(result)
		return backgroundStatus(result.policy), errors.Join(ErrCancelRequested, launchErr, cancelErr, result.err)
	}
	if errors.Is(launchErr, ErrBackgroundManagerClosed) {
		persisted.Enabled = false
		persisted.State = BackgroundStateDisabled
		persisted.Outcome = BackgroundOutcomeStopped
		persisted.UpdatedAt = m.currentTime()
		result := m.persistTerminal(persisted, BackgroundAuditStop)
		m.rememberTerminal(result)
		return backgroundStatus(result.policy), errors.Join(launchErr, result.err)
	}
	return backgroundStatus(persisted), launchErr
}

func (m *BackgroundManager) Stop(sessionID string) (status BackgroundStatus, err error) {
	if err := m.validate(); err != nil {
		return BackgroundStatus{}, err
	}
	if err := m.admitLifecycle(); err != nil {
		return BackgroundStatus{}, err
	}
	defer m.lifecycle.Done()
	sessionID = strings.TrimSpace(sessionID)
	worker, stopOwner, err := m.reserveStopTransition(sessionID)
	if err != nil {
		return BackgroundStatus{}, err
	}
	defer func() {
		err = errors.Join(err, m.releaseStopTransition(sessionID, stopOwner))
	}()
	policy, err := m.store.Get(sessionID)
	if err != nil {
		return BackgroundStatus{}, err
	}
	policy.Enabled = false
	policy.State = BackgroundStateDisabled
	policy.Outcome = BackgroundOutcomeStopped
	policy.UpdatedAt = m.currentTime()
	result := m.persistTerminal(policy, BackgroundAuditStop)
	if result.err != nil {
		m.rememberTerminal(result)
		if worker != nil {
			stopOwner.handoff <- true
		}
		return backgroundStatus(result.policy), result.err
	}
	m.mu.Lock()
	delete(m.terminal, sessionID)
	m.mu.Unlock()
	if worker != nil {
		stopOwner.handoff <- false
		worker.cancel(errBackgroundStop)
		<-worker.done
		if worker.err != nil && !isExplicitCancellation(worker.ctx, worker.err) {
			policy := result.policy
			policy.State = BackgroundStateError
			policy.Outcome = worker.outcome
			policy.UpdatedAt = m.currentTime()
			result = m.persistTerminal(policy, BackgroundAuditError)
			m.rememberTerminal(result)
			if result.err != nil {
				return backgroundStatus(result.policy), result.err
			}
		}
	}
	status, err = m.Get(sessionID)
	if err != nil {
		return backgroundStatus(result.policy), err
	}
	return status, nil
}

func (m *BackgroundManager) Get(sessionID string) (BackgroundStatus, error) {
	if err := m.validate(); err != nil {
		return BackgroundStatus{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	guard, ok := m.terminal[sessionID]
	m.mu.Unlock()
	if ok {
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
	terminal := make(map[string]backgroundTerminalGuard, len(m.terminal))
	for sessionID, guard := range m.terminal {
		terminal[sessionID] = guard
	}
	m.mu.Unlock()
	policies, err := m.store.List()
	if err != nil {
		return nil, err
	}
	statuses := make([]BackgroundStatus, 0, len(policies)+len(terminal))
	seen := make(map[string]struct{}, len(policies))
	for _, policy := range policies {
		seen[policy.SessionID] = struct{}{}
		if guard, ok := terminal[policy.SessionID]; ok {
			statuses = append(statuses, guard.status)
			continue
		}
		statuses = append(statuses, backgroundStatus(policy))
	}
	for sessionID, guard := range terminal {
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
	if err := m.admitLifecycle(); err != nil {
		return err
	}
	defer m.lifecycle.Done()
	m.resumeMu.Lock()
	defer m.resumeMu.Unlock()
	if m.managerDone() {
		return ErrBackgroundManagerClosed
	}
	if err := m.reconcileCancellationProjection(); err != nil {
		return err
	}
	if err := m.reconcileTerminalAudits(); err != nil {
		return err
	}
	if err := m.reconcileTransitions(); err != nil {
		return err
	}
	policies, err := m.store.List()
	if err != nil {
		return err
	}
	for _, policy := range policies {
		if !policy.Enabled {
			continue
		}
		if m.hasActiveWorker(policy.SessionID) {
			continue
		}
		if err := m.reserveTransition(policy.SessionID); err != nil {
			if errors.Is(err, ErrBackgroundTransitionBusy) {
				continue
			}
			return err
		}
		current, err := m.store.Get(policy.SessionID)
		if errors.Is(err, ErrBackgroundPolicyNotFound) {
			if err := m.releaseTransition(policy.SessionID); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return errors.Join(err, m.releaseTransition(policy.SessionID))
		}
		policy = current
		if !policy.Enabled {
			if err := m.releaseTransition(policy.SessionID); err != nil {
				return err
			}
			continue
		}
		if policy.State != BackgroundStateRunning || policy.PolicyVersion != BackgroundPolicyVersion || policy.AcknowledgedAt.IsZero() {
			if err := m.block(policy, BackgroundOutcomePolicyInvalid); err != nil {
				return errors.Join(err, m.releaseTransition(policy.SessionID))
			}
			if err := m.releaseTransition(policy.SessionID); err != nil {
				return err
			}
			continue
		}
		canceled, cancelErr := m.recoverCanceledSession(policy.SessionID)
		if cancelErr != nil && !canceled {
			if errors.Is(cancelErr, ErrSessionNotFound) {
				if err := m.block(policy, BackgroundOutcomeSessionMissing); err != nil {
					return errors.Join(err, m.releaseTransition(policy.SessionID))
				}
				if err := m.releaseTransition(policy.SessionID); err != nil {
					return err
				}
				continue
			}
			return errors.Join(cancelErr, m.releaseTransition(policy.SessionID))
		}
		if canceled {
			policy.Enabled = false
			policy.State = BackgroundStateDisabled
			policy.Outcome = BackgroundOutcomeStopped
			policy.UpdatedAt = m.currentTime()
			result := m.persistTerminal(policy, BackgroundAuditStop)
			m.rememberTerminal(result)
			releaseErr := m.releaseTransition(policy.SessionID)
			if cancelErr != nil || result.err != nil || releaseErr != nil {
				return errors.Join(cancelErr, result.err, releaseErr)
			}
			continue
		}
		resolved, err := m.resolver(policy.Profile)
		if err != nil {
			if errors.Is(err, ErrProfileNotFound) || errors.Is(err, ErrUnknownAgent) {
				if err := m.block(policy, BackgroundOutcomeProfileMissing); err != nil {
					return errors.Join(err, m.releaseTransition(policy.SessionID))
				}
				if err := m.releaseTransition(policy.SessionID); err != nil {
					return err
				}
				continue
			}
			return errors.Join(err, m.releaseTransition(policy.SessionID))
		}
		if !resolved.TrustValid {
			if err := m.block(policy, BackgroundOutcomeProfileUntrusted); err != nil {
				return errors.Join(err, m.releaseTransition(policy.SessionID))
			}
			if err := m.releaseTransition(policy.SessionID); err != nil {
				return err
			}
			continue
		}
		if resolved.DescriptorHash != policy.DescriptorHash {
			if err := m.block(policy, BackgroundOutcomeProfileDrift); err != nil {
				return errors.Join(err, m.releaseTransition(policy.SessionID))
			}
			if err := m.releaseTransition(policy.SessionID); err != nil {
				return err
			}
			continue
		}
		now := m.currentTime()
		policy.State = BackgroundStateRunning
		policy.Outcome = BackgroundOutcomeResumed
		policy.StartedAt = now
		policy.UpdatedAt = now
		persisted, resolved, err := m.persistBeforeResolvedLaunch(policy, BackgroundAuditResume, resolved)
		if err != nil {
			if outcome, ok := backgroundProfileLeaseFailure(err); ok {
				if blockErr := m.block(policy, outcome); blockErr != nil {
					return errors.Join(err, blockErr, m.releaseTransition(policy.SessionID))
				}
				if releaseErr := m.releaseTransition(policy.SessionID); releaseErr != nil {
					return releaseErr
				}
				continue
			}
			return errors.Join(err, m.releaseTransition(policy.SessionID))
		}
		launched, canceled, launchErr := m.launchPersistedPolicy(persisted, resolved)
		if launched {
			if launchErr != nil {
				return launchErr
			}
			continue
		}
		if canceled {
			_, cancelErr := m.recoverCanceledSession(policy.SessionID)
			persisted.Enabled = false
			persisted.State = BackgroundStateDisabled
			persisted.Outcome = BackgroundOutcomeStopped
			persisted.UpdatedAt = m.currentTime()
			result := m.persistTerminal(persisted, BackgroundAuditStop)
			m.rememberTerminal(result)
			releaseErr := m.releaseTransition(policy.SessionID)
			if launchErr != nil || cancelErr != nil || result.err != nil || releaseErr != nil {
				return errors.Join(launchErr, cancelErr, result.err, releaseErr)
			}
			continue
		}
		if errors.Is(launchErr, ErrBackgroundManagerClosed) {
			persisted.Enabled = false
			persisted.State = BackgroundStateDisabled
			persisted.Outcome = BackgroundOutcomeStopped
			persisted.UpdatedAt = m.currentTime()
			result := m.persistTerminal(persisted, BackgroundAuditStop)
			m.rememberTerminal(result)
			return errors.Join(launchErr, result.err, m.releaseTransition(policy.SessionID))
		}
		return errors.Join(launchErr, m.releaseTransition(policy.SessionID))
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
		if m.stopParent != nil {
			m.stopParent()
			m.stopParent = nil
		}
		m.cancel(errBackgroundShutdown)
		for _, worker := range m.active {
			worker.cancel(errBackgroundShutdown)
		}
	}
	m.mu.Unlock()
	m.lifecycle.Wait()
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

func (m *BackgroundManager) recoverCanceledSession(sessionID string) (bool, error) {
	session, err := m.sessions.Get(sessionID)
	if err != nil {
		return false, err
	}
	if !cancellationLatched(session.Status) {
		return false, nil
	}
	_, err = m.sessions.RecoverStaleQueue(sessionID, m.currentTime())
	return true, err
}

func (m *BackgroundManager) reconcileCancellationProjection() error {
	if err := m.sessions.ReconcileCancellationRequests(); err != nil {
		return errors.Join(ErrBackgroundPersistenceDegraded, err)
	}
	return nil
}

func (m *BackgroundManager) launchLocked(policy BackgroundPolicy, resolved ResolvedBackgroundProfile) {
	ctx, cancel := context.WithCancelCause(m.ctx)
	worker := &backgroundWorker{
		profile:        policy.Profile,
		descriptorHash: policy.DescriptorHash,
		policy:         policy,
		ctx:            ctx,
		cancel:         cancel,
		done:           make(chan struct{}),
		releaseLease:   m.transitionLease[policy.SessionID],
	}
	delete(m.transitionLease, policy.SessionID)
	delete(m.transitions, policy.SessionID)
	m.active[policy.SessionID] = worker
	m.wg.Go(func() {
		outcome, err := m.watchWorker(ctx, resolved, policy.SessionID)
		m.workerDone(policy.SessionID, worker, ctx, outcome, err)
	})
}

func (m *BackgroundManager) launchPersistedPolicy(policy BackgroundPolicy, resolved ResolvedBackgroundProfile) (launched, canceled bool, err error) {
	err = m.sessions.withLaunchAdmission(policy.SessionID, func() error {
		session, getErr := m.sessions.Get(policy.SessionID)
		if getErr != nil {
			return getErr
		}
		if cancellationLatched(session.Status) {
			canceled = true
			return nil
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.closed || m.ctx.Err() != nil {
			return ErrBackgroundManagerClosed
		}
		delete(m.terminal, policy.SessionID)
		m.launchLocked(policy, resolved)
		launched = true
		return nil
	})
	return launched, canceled, err
}

func (m *BackgroundManager) watchWorker(ctx context.Context, resolved ResolvedBackgroundProfile, sessionID string) (outcome string, err error) {
	defer func() {
		if recover() != nil {
			outcome = BackgroundOutcomeWorkerPanic
			err = errors.New("acp background watcher panic")
		}
	}()
	watch := m.watch
	if resolved.WithTrustedProfile != nil {
		start := watch.StartRunner
		if start == nil {
			start = defaultDrainStartRunner
		}
		watch.StartRunner = resolved.trustedStartRunner(start)
	}
	_, err = m.watcher(ctx, m.sessions, resolved.Spec, resolved.Options, sessionID, watch, nil)
	return BackgroundOutcomeWorkerError, err
}

func (resolved ResolvedBackgroundProfile) trustedStartRunner(start func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error)) func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error) {
	return func(ctx context.Context, _ AgentSpec, opts RunOptions, existingID string) (runner DrainPromptRunner, closeRunner func() error, err error) {
		err = resolved.WithTrustedProfile(resolved.DescriptorHash, func(profile Profile) error {
			spec := cloneSpec(profile.Spec)
			if validateErr := spec.Validate(); validateErr != nil {
				return fmt.Errorf("%w: %s", ErrBackgroundProfileIneligible, profile.Name)
			}
			opts.Agent = profile.Name
			opts.Command = ""
			opts.Args = nil
			opts.Cwd = profile.Cwd
			runner, closeRunner, err = start(ctx, spec, opts, existingID)
			return err
		})
		return runner, closeRunner, err
	}
}

func (m *BackgroundManager) workerDone(sessionID string, worker *backgroundWorker, ctx context.Context, outcome string, workerErr error) {
	m.mu.Lock()
	stopOwner, stopOwns := m.stopping[sessionID]
	_, transitionReserved := m.transitions[sessionID]
	workerOwnsTransition := !stopOwns && !transitionReserved
	if workerOwnsTransition {
		m.transitions[sessionID] = struct{}{}
	}
	worker.outcome = outcome
	worker.err = workerErr
	m.mu.Unlock()

	managerCancellation := isManagerCancellation(ctx, workerErr)
	sessionCancellation := isSessionCancellation(workerErr)
	var result backgroundRecordingResult
	workerOwnsPersistence := true
	if stopOwns {
		workerOwnsPersistence = <-stopOwner.handoff
	}
	if workerOwnsPersistence {
		admissionErr := m.sessions.withLaunchAdmission(sessionID, func() error {
			cancelRequested, authorityErr := m.sessions.CheckCancellation(sessionID)
			if authorityErr != nil {
				workerErr = errors.Join(workerErr, authorityErr)
				outcome = BackgroundOutcomeWorkerError
				managerCancellation = false
				sessionCancellation = false
			} else if cancelRequested && (workerErr == nil || backgroundCancellationOnly(workerErr)) {
				managerCancellation = false
				sessionCancellation = true
			}
			if managerCancellation {
				return nil
			}

			policy := worker.policy
			policy.Enabled = false
			policy.UpdatedAt = m.currentTime()
			action := BackgroundAuditError
			switch {
			case sessionCancellation:
				policy.State = BackgroundStateDisabled
				policy.Outcome = BackgroundOutcomeStopped
				action = BackgroundAuditStop
			case workerErr == nil:
				policy.State = BackgroundStateDisabled
				policy.Outcome = BackgroundOutcomeCompleted
				action = BackgroundAuditStop
			default:
				policy.State = BackgroundStateError
				policy.Outcome = outcome
			}
			result = m.persistTerminal(policy, action)
			return nil
		})
		if admissionErr != nil {
			workerErr = errors.Join(workerErr, admissionErr)
			outcome = BackgroundOutcomeWorkerError
			managerCancellation = false
			result.policy = worker.policy
			result.policy.Enabled = false
			result.policy.State = BackgroundStateError
			result.policy.Outcome = outcome
			result.policy.PersistenceDegraded = true
			result.policy.UpdatedAt = m.currentTime()
			result.stateRecorded = false
			result.auditRecorded = false
			result.err = errors.Join(result.err, admissionErr)
		}
	}
	if stopOwns && !workerOwnsPersistence && worker.releaseLease != nil {
		m.mu.Lock()
		m.transitionLease[sessionID] = worker.releaseLease
		worker.releaseLease = nil
		m.mu.Unlock()
	}
	if worker.releaseLease != nil {
		if releaseErr := worker.releaseLease(); releaseErr != nil {
			workerErr = errors.Join(workerErr, releaseErr)
			outcome = BackgroundOutcomeWorkerError
			managerCancellation = false
			result.policy = worker.policy
			result.policy.Enabled = false
			result.policy.State = BackgroundStateError
			result.policy.Outcome = outcome
			result.policy.PersistenceDegraded = true
			result.policy.UpdatedAt = m.currentTime()
			result.err = errors.Join(result.err, releaseErr)
		}
		worker.releaseLease = nil
	}

	m.mu.Lock()
	worker.outcome = outcome
	worker.err = workerErr
	if current, ok := m.active[sessionID]; ok && current == worker {
		delete(m.active, sessionID)
	}
	if workerOwnsTransition {
		delete(m.transitions, sessionID)
	}
	if workerOwnsPersistence && !managerCancellation {
		m.terminal[sessionID] = backgroundTerminalGuard{
			status:        backgroundStatus(result.policy),
			stateRecorded: result.stateRecorded,
			auditRecorded: result.auditRecorded,
		}
	}
	close(worker.done)
	m.mu.Unlock()
}

func (m *BackgroundManager) block(policy BackgroundPolicy, outcome string) error {
	policy.Enabled = false
	policy.State = BackgroundStateBlocked
	policy.Outcome = outcome
	policy.UpdatedAt = m.currentTime()
	result := m.persistTerminal(policy, BackgroundAuditBlock)
	m.rememberTerminal(result)
	return result.err
}

func (m *BackgroundManager) persistBeforeLaunch(policy BackgroundPolicy, action string) (BackgroundPolicy, error) {
	transition, err := newBackgroundTransition(policy, action)
	if err != nil {
		return policy, err
	}
	if err := m.store.putTransition(transition); err != nil {
		failed := m.recordingFailure(policy, BackgroundOutcomeStateWriteFailed)
		result := m.persistTerminal(failed, BackgroundAuditError)
		m.rememberTerminal(result)
		return result.policy, errors.Join(err, result.err)
	}
	if err := m.store.Upsert(policy); err != nil {
		failed := m.recordingFailure(policy, BackgroundOutcomeStateWriteFailed)
		result := m.persistTerminal(failed, BackgroundAuditError)
		m.rememberTerminal(result)
		return result.policy, errors.Join(err, result.err)
	}
	if err := m.appendAudit(transition); err != nil {
		failed := m.recordingFailure(policy, BackgroundOutcomeAuditAppendFailed)
		result := m.persistTerminal(failed, BackgroundAuditError)
		m.rememberTerminal(result)
		return result.policy, errors.Join(err, result.err)
	}
	if err := m.store.removeTransition(policy.SessionID); err != nil {
		failed := m.recordingFailure(policy, BackgroundOutcomeStateWriteFailed)
		result := m.persistTerminal(failed, BackgroundAuditError)
		m.rememberTerminal(result)
		return result.policy, errors.Join(err, result.err)
	}
	return policy, nil
}

func (m *BackgroundManager) persistBeforeResolvedLaunch(policy BackgroundPolicy, action string, resolved ResolvedBackgroundProfile) (BackgroundPolicy, ResolvedBackgroundProfile, error) {
	if resolved.WithTrustedProfile == nil {
		if err := resolved.Spec.Validate(); err != nil {
			return BackgroundPolicy{}, resolved, fmt.Errorf("%w: %s", ErrBackgroundProfileIneligible, policy.Profile)
		}
		persisted, err := m.persistBeforeLaunch(policy, action)
		return persisted, resolved, err
	}

	current := resolved
	var persisted BackgroundPolicy
	err := resolved.WithTrustedProfile(policy.DescriptorHash, func(profile Profile) error {
		current.Spec = cloneSpec(profile.Spec)
		current.Options.Agent = profile.Name
		current.Options.Command = ""
		current.Options.Args = nil
		current.Options.Cwd = profile.Cwd
		current.DescriptorHash = profile.DescriptorHash()
		current.TrustValid = true
		if err := current.Spec.Validate(); err != nil {
			return fmt.Errorf("%w: %s", ErrBackgroundProfileIneligible, profile.Name)
		}
		var err error
		persisted, err = m.persistBeforeLaunch(policy, action)
		return err
	})
	return persisted, current, err
}

func backgroundProfileLeaseFailure(err error) (string, bool) {
	switch {
	case errors.Is(err, ErrProfileNotFound):
		return BackgroundOutcomeProfileMissing, true
	case errors.Is(err, errProfileTrustInvalid):
		return BackgroundOutcomeProfileUntrusted, true
	case errors.Is(err, errProfileHashMismatch):
		return BackgroundOutcomeProfileDrift, true
	default:
		return "", false
	}
}

func (m *BackgroundManager) recordingFailure(policy BackgroundPolicy, outcome string) BackgroundPolicy {
	policy.Enabled = false
	policy.State = BackgroundStateError
	policy.Outcome = outcome
	policy.UpdatedAt = m.currentTime()
	return policy
}

type backgroundRecordingResult struct {
	policy        BackgroundPolicy
	stateRecorded bool
	auditRecorded bool
	err           error
}

func (m *BackgroundManager) persistTerminal(policy BackgroundPolicy, action string) backgroundRecordingResult {
	transition, err := newBackgroundTransition(policy, action)
	if err != nil {
		policy.PersistenceDegraded = true
		return backgroundRecordingResult{policy: policy, err: errors.Join(ErrBackgroundPersistenceDegraded, err)}
	}
	return m.persistTerminalTransition(transition, true)
}

func (m *BackgroundManager) persistTerminalTransition(transition backgroundTransition, persistTransition bool) backgroundRecordingResult {
	policy := transition.Policy
	policy.Enabled = false
	policy.PersistenceDegraded = true
	transition.Policy = policy
	var transitionErr error
	if persistTransition {
		transitionErr = m.store.putTransition(transition)
		if transitionErr != nil {
			return backgroundRecordingResult{
				policy: policy,
				err:    errors.Join(ErrBackgroundPersistenceDegraded, transitionErr),
			}
		}
	}
	stateErr := m.store.Upsert(policy)
	auditErr := m.appendAudit(transition)
	result := backgroundRecordingResult{
		policy:        policy,
		stateRecorded: stateErr == nil,
		auditRecorded: auditErr == nil,
	}
	if stateErr != nil || auditErr != nil {
		result.err = errors.Join(ErrBackgroundPersistenceDegraded, transitionErr, stateErr, auditErr)
		return result
	}
	policy.PersistenceDegraded = false
	finalStateErr := m.store.Upsert(policy)
	if finalStateErr != nil {
		policy.PersistenceDegraded = true
		result.policy = policy
		result.err = errors.Join(ErrBackgroundPersistenceDegraded, finalStateErr)
		return result
	}
	clearErr := m.store.removeTransition(policy.SessionID)
	if clearErr != nil {
		policy.PersistenceDegraded = true
		degradedStateErr := m.store.Upsert(policy)
		result.policy = policy
		result.stateRecorded = degradedStateErr == nil
		result.err = errors.Join(ErrBackgroundPersistenceDegraded, clearErr, degradedStateErr)
		return result
	}
	result.policy = policy
	return result
}

func (m *BackgroundManager) rememberTerminal(result backgroundRecordingResult) {
	m.mu.Lock()
	m.terminal[result.policy.SessionID] = backgroundTerminalGuard{
		status:        backgroundStatus(result.policy),
		stateRecorded: result.stateRecorded,
		auditRecorded: result.auditRecorded,
	}
	m.mu.Unlock()
}

func (m *BackgroundManager) reconcileTransitions() error {
	sessionIDs, err := m.store.listTransitionIDs()
	if err != nil {
		return errors.Join(ErrBackgroundPersistenceDegraded, err)
	}
	var reconcileErr error
	for _, sessionID := range sessionIDs {
		err := m.reconcileTransition(sessionID)
		if errors.Is(err, ErrBackgroundTransitionBusy) {
			continue
		}
		reconcileErr = errors.Join(reconcileErr, err)
	}
	return reconcileErr
}

func (m *BackgroundManager) reconcileTransition(sessionID string) (err error) {
	if err := m.reserveTransition(sessionID); err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, m.releaseTransition(sessionID))
	}()
	transition, found, err := m.store.getTransition(sessionID)
	if err != nil {
		return errors.Join(ErrBackgroundPersistenceDegraded, err)
	}
	if !found {
		return nil
	}
	result := m.persistTerminalTransition(transition, false)
	m.rememberTerminal(result)
	return result.err
}

func (m *BackgroundManager) reconcileTerminalAudits() error {
	policies, err := m.store.List()
	if err != nil {
		return errors.Join(ErrBackgroundPersistenceDegraded, err)
	}
	var reconcileErr error
	for _, policy := range policies {
		err := m.reconcileTerminalAudit(policy.SessionID)
		if errors.Is(err, ErrBackgroundTransitionBusy) {
			continue
		}
		reconcileErr = errors.Join(reconcileErr, err)
	}
	return reconcileErr
}

func (m *BackgroundManager) reconcileTerminalAudit(sessionID string) (err error) {
	if err := m.reserveTransition(sessionID); err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, m.releaseTransition(sessionID))
	}()
	policy, err := m.store.Get(sessionID)
	if errors.Is(err, ErrBackgroundPolicyNotFound) {
		return nil
	}
	if err != nil {
		return errors.Join(ErrBackgroundPersistenceDegraded, err)
	}
	records, readErr := m.audit.Read()
	if readErr != nil {
		if !policy.Enabled {
			return errors.Join(ErrBackgroundPersistenceDegraded, readErr)
		}
		failed := m.recordingFailure(policy, BackgroundOutcomeAuditAppendFailed)
		result := m.persistTerminal(failed, BackgroundAuditError)
		m.rememberTerminal(result)
		return errors.Join(ErrBackgroundPersistenceDegraded, readErr, result.err)
	}
	var record BackgroundAuditRecord
	found := false
	for _, candidate := range records {
		if candidate.SessionID == sessionID {
			record = candidate
			found = true
		}
	}
	if !found || !backgroundAuditTerminal(record.Action) {
		return nil
	}
	if record.Profile != policy.Profile || record.DescriptorHash != policy.DescriptorHash {
		return nil
	}
	policy.Enabled = false
	policy.PersistenceDegraded = false
	policy.Outcome = record.Outcome
	policy.UpdatedAt = record.At
	switch record.Action {
	case BackgroundAuditBlock:
		policy.State = BackgroundStateBlocked
	case BackgroundAuditError:
		policy.State = BackgroundStateError
	case BackgroundAuditStop:
		policy.State = BackgroundStateDisabled
	}
	if err := m.store.Upsert(policy); err != nil {
		return errors.Join(ErrBackgroundPersistenceDegraded, err)
	}
	m.rememberTerminal(backgroundRecordingResult{
		policy:        policy,
		stateRecorded: true,
		auditRecorded: true,
	})
	return nil
}

func backgroundAuditTerminal(action string) bool {
	return action == BackgroundAuditBlock || action == BackgroundAuditError || action == BackgroundAuditStop
}

func (m *BackgroundManager) reserveTransition(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.ctx.Err() != nil {
		return ErrBackgroundManagerClosed
	}
	if _, busy := m.transitions[sessionID]; busy {
		return fmt.Errorf("%w: %s", ErrBackgroundTransitionBusy, sessionID)
	}
	release, acquired, err := tryStoreFileLock(m.workerLeasePath(sessionID))
	if err != nil {
		return err
	}
	if !acquired {
		return fmt.Errorf("%w: %s", ErrBackgroundTransitionBusy, sessionID)
	}
	m.transitionLease[sessionID] = release
	m.transitions[sessionID] = struct{}{}
	return nil
}

func (m *BackgroundManager) admitLifecycle() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.ctx.Err() != nil {
		return ErrBackgroundManagerClosed
	}
	m.lifecycle.Add(1)
	return nil
}

func (m *BackgroundManager) reserveStopTransition(sessionID string) (*backgroundWorker, *backgroundStopOwner, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.ctx.Err() != nil {
		return nil, nil, ErrBackgroundManagerClosed
	}
	if _, busy := m.transitions[sessionID]; busy {
		return nil, nil, fmt.Errorf("%w: %s", ErrBackgroundTransitionBusy, sessionID)
	}
	worker := m.active[sessionID]
	if worker == nil {
		release, acquired, err := tryStoreFileLock(m.workerLeasePath(sessionID))
		if err != nil {
			return nil, nil, err
		}
		if !acquired {
			return nil, nil, fmt.Errorf("%w: %s", ErrBackgroundTransitionBusy, sessionID)
		}
		m.transitionLease[sessionID] = release
	}
	owner := &backgroundStopOwner{handoff: make(chan bool, 1)}
	m.transitions[sessionID] = struct{}{}
	m.stopping[sessionID] = owner
	return worker, owner, nil
}

func (m *BackgroundManager) releaseStopTransition(sessionID string, owner *backgroundStopOwner) error {
	m.mu.Lock()
	var release func() error
	if m.stopping[sessionID] == owner {
		delete(m.stopping, sessionID)
		delete(m.transitions, sessionID)
		release = m.transitionLease[sessionID]
		delete(m.transitionLease, sessionID)
	}
	m.mu.Unlock()
	if release != nil {
		return release()
	}
	return nil
}

func (m *BackgroundManager) releaseTransition(sessionID string) error {
	m.mu.Lock()
	delete(m.transitions, sessionID)
	release := m.transitionLease[sessionID]
	delete(m.transitionLease, sessionID)
	m.mu.Unlock()
	if release != nil {
		return release()
	}
	return nil
}

func (m *BackgroundManager) workerLeasePath(sessionID string) string {
	return filepath.Join(filepath.Dir(m.sessions.Path()), "background-workers", storeKey(sessionID)+".lock")
}

func (m *BackgroundManager) hasActiveWorker(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.active[sessionID]
	return ok
}

func (m *BackgroundManager) managerDone() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed || m.ctx.Err() != nil
}

func isExplicitCancellation(ctx context.Context, workerErr error) bool {
	return isManagerCancellation(ctx, workerErr) || isSessionCancellation(workerErr)
}

func isManagerCancellation(ctx context.Context, workerErr error) bool {
	if !backgroundCancellationOnly(workerErr) {
		return false
	}
	cause := context.Cause(ctx)
	return errors.Is(cause, errBackgroundStop) || errors.Is(cause, errBackgroundShutdown)
}

func isSessionCancellation(workerErr error) bool {
	return backgroundCancellationOnly(workerErr) && errors.Is(workerErr, ErrCancelRequested)
}

func backgroundCancellationOnly(err error) bool {
	if err == nil {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		errs := joined.Unwrap()
		if len(errs) == 0 {
			return false
		}
		for _, nested := range errs {
			if !backgroundCancellationOnly(nested) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return backgroundCancellationOnly(wrapped.Unwrap())
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, ErrCancelRequested)
}

func (m *BackgroundManager) appendAudit(transition backgroundTransition) error {
	policy := transition.Policy
	return m.audit.Append(BackgroundAuditRecord{
		RecordID:       transition.EventID,
		At:             policy.UpdatedAt,
		Action:         transition.Action,
		SessionID:      policy.SessionID,
		Profile:        policy.Profile,
		DescriptorHash: policy.DescriptorHash,
		Outcome:        policy.Outcome,
	})
}

func newBackgroundTransition(policy BackgroundPolicy, action string) (backgroundTransition, error) {
	var eventID [16]byte
	if _, err := rand.Read(eventID[:]); err != nil {
		return backgroundTransition{}, fmt.Errorf("generate acp background audit event id: %w", err)
	}
	return backgroundTransition{Policy: policy, Action: action, EventID: hex.EncodeToString(eventID[:])}, nil
}

func (m *BackgroundManager) currentTime() time.Time {
	return m.now().UTC()
}

func backgroundStatus(policy BackgroundPolicy) BackgroundStatus {
	return BackgroundStatus(policy)
}
