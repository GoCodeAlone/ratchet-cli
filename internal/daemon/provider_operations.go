package daemon

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GoCodeAlone/workflow/secrets"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

const (
	providerOperationPending   = "pending"
	providerOperationApplied   = "applied"
	providerOperationCommitted = "committed"
	providerOperationFailed    = "failed"

	providerFailureInvalidRequest   = "invalid_request"
	providerFailureConflict         = "operation_conflict"
	providerFailureAliasBusy        = "alias_busy"
	providerFailureSecretStore      = "secret_store"
	providerFailureDatabase         = "database"
	providerFailureFinalization     = "finalization"
	providerFailureRestartRecovery  = "restart_recovery"
	providerFailureInternal         = "internal"
	providerOperationRetention      = 24 * time.Hour
	providerOperationFinalizerLimit = 5 * time.Second
)

type providerOperationManager struct {
	engine *EngineContext

	mu          sync.Mutex
	flights     map[string]*providerOperationFlight
	starting    bool
	started     bool
	stopped     bool
	startCancel context.CancelFunc
	aliasMu     sync.Mutex
	aliases     map[string]*providerAliasGate

	ctx         context.Context
	cancel      context.CancelFunc
	cleanupWake chan struct{}
	cleanupMu   sync.Mutex
	cleaning    map[string]bool
	stopOnce    sync.Once
	background  sync.WaitGroup
}

type providerOperationFlight struct {
	operationID  string
	done         chan struct{}
	releaseAlias func()
}

type providerAliasGate struct {
	mu   sync.Mutex
	refs int
}

type providerOperationSecretFinalizationError struct {
	cause error
}

func (e *providerOperationSecretFinalizationError) Error() string {
	switch {
	case errors.Is(e.cause, context.Canceled):
		return "provider operation finalization secret read canceled"
	case errors.Is(e.cause, context.DeadlineExceeded):
		return "provider operation finalization secret read timed out"
	case errors.Is(e.cause, secrets.ErrInvalidKey):
		return "provider operation finalization secret key is invalid"
	case errors.Is(e.cause, secrets.ErrUnsupported):
		return "provider operation finalization secret read is unsupported"
	case errors.Is(e.cause, secrets.ErrProviderInit):
		return "provider operation finalization secret provider initialization failed"
	default:
		return "provider operation finalization secret unavailable"
	}
}

func (e *providerOperationSecretFinalizationError) Unwrap() error {
	return e.cause
}

func (e *providerOperationSecretFinalizationError) retryableAtStartup() bool {
	return !errors.Is(e.cause, context.Canceled) &&
		!errors.Is(e.cause, context.DeadlineExceeded) &&
		!errors.Is(e.cause, secrets.ErrInvalidKey) &&
		!errors.Is(e.cause, secrets.ErrUnsupported) &&
		!errors.Is(e.cause, secrets.ErrProviderInit)
}

func newProviderOperationManager(engine *EngineContext) *providerOperationManager {
	return &providerOperationManager{
		engine:      engine,
		flights:     make(map[string]*providerOperationFlight),
		aliases:     make(map[string]*providerAliasGate),
		cleanupWake: make(chan struct{}, 1),
		cleaning:    make(map[string]bool),
	}
}

func (m *providerOperationManager) Start(parent context.Context) error {
	if m == nil || m.engine == nil || m.engine.DB == nil || m.engine.SecretsProvider == nil {
		return fmt.Errorf("provider operation manager is not fully configured")
	}
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return fmt.Errorf("provider operation manager is stopped")
	}
	if m.starting || m.started {
		m.mu.Unlock()
		return fmt.Errorf("provider operation manager is already starting or started")
	}
	startupCtx, startupCancel := context.WithCancel(parent)
	m.starting = true
	m.startCancel = startupCancel
	m.background.Add(1)
	m.mu.Unlock()
	defer m.background.Done()

	if _, err := m.engine.SecretsProvider.List(startupCtx); err != nil {
		m.finishFailedStart()
		startupCancel()
		return fmt.Errorf("list provider secrets: %w", err)
	}
	if err := m.reconcileStartup(startupCtx); err != nil {
		m.finishFailedStart()
		startupCancel()
		return err
	}
	m.mu.Lock()
	m.starting = false
	m.startCancel = nil
	if m.stopped {
		m.mu.Unlock()
		startupCancel()
		return fmt.Errorf("provider operation manager is stopped")
	}
	m.ctx = startupCtx
	m.cancel = startupCancel
	m.started = true
	m.background.Add(1)
	go func() {
		defer m.background.Done()
		m.cleanupLoop()
	}()
	m.mu.Unlock()
	m.WakeCleanup()
	return nil
}

func (m *providerOperationManager) finishFailedStart() {
	m.mu.Lock()
	m.starting = false
	m.startCancel = nil
	m.mu.Unlock()
}

func (m *providerOperationManager) Stop() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() {
		m.mu.Lock()
		m.stopped = true
		if m.startCancel != nil {
			m.startCancel()
		}
		if m.cancel != nil {
			m.cancel()
		}
		m.mu.Unlock()
		m.background.Wait()
	})
}

func (m *providerOperationManager) Commit(ctx context.Context, req *pb.CommitProviderSaveReq) (*pb.ProviderOperation, error) {
	if err := m.operationAdmissionError(); err != nil {
		return nil, err
	}
	provider, normalizedSettings, err := validateProviderSaveRequest(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	request := proto.Clone(provider).(*pb.AddProviderReq)
	request.Settings = normalizedSettings

	m.mu.Lock()
	if err := m.operationAdmissionErrorLocked(); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.background.Add(1)
	defer m.background.Done()
	existing, err := m.get(ctx, req.GetOperationId(), false)
	if err != nil && status.Code(err) != codes.NotFound {
		m.mu.Unlock()
		return nil, err
	}
	if existing != nil {
		if existing.GetAlias() != request.GetAlias() {
			m.mu.Unlock()
			return syntheticProviderOperation(req.GetOperationId(), request.GetAlias(), providerFailureConflict), nil
		}
		flight := m.flights[request.GetAlias()]
		m.mu.Unlock()
		if flight != nil && flight.operationID == req.GetOperationId() {
			return m.waitForFlight(ctx, req.GetOperationId(), flight)
		}
		return m.Get(ctx, req.GetOperationId())
	}
	if flight := m.flights[request.GetAlias()]; flight != nil {
		m.mu.Unlock()
		return syntheticProviderOperation(req.GetOperationId(), request.GetAlias(), providerFailureAliasBusy), nil
	}
	releaseAlias := m.acquireAlias(request.GetAlias())

	secretName := ""
	if request.GetApiKey() != "" {
		secretName = fmt.Sprintf("provider-v2-%d-%s", time.Now().Unix(), uuid.NewString())
	}
	now := time.Now().UTC()
	_, err = m.engine.DB.ExecContext(ctx, `INSERT INTO provider_operations
		(operation_id, alias, state, failure, secret_name, created_at, updated_at, expires_at)
		VALUES (?, ?, ?, '', ?, ?, ?, ?)`,
		req.GetOperationId(), request.GetAlias(), providerOperationPending, secretName,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
		now.Add(providerOperationRetention).Format(time.RFC3339Nano),
	)
	if err != nil {
		releaseAlias()
		m.mu.Unlock()
		if ctx.Err() != nil {
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		if replay, replayErr := m.get(ctx, req.GetOperationId(), false); replayErr == nil {
			if replay.GetAlias() != request.GetAlias() {
				return syntheticProviderOperation(req.GetOperationId(), request.GetAlias(), providerFailureConflict), nil
			}
			return replay, nil
		}
		return nil, status.Error(codes.Internal, "journal provider operation")
	}
	flight := &providerOperationFlight{
		operationID:  req.GetOperationId(),
		done:         make(chan struct{}),
		releaseAlias: releaseAlias,
	}
	m.flights[request.GetAlias()] = flight
	m.background.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.background.Done()
		m.runFlight(flight, request, secretName)
	}()
	return m.waitForFlight(ctx, req.GetOperationId(), flight)
}

func validateProviderSaveRequest(req *pb.CommitProviderSaveReq) (*pb.AddProviderReq, string, error) {
	if req == nil || req.GetProvider() == nil {
		return nil, "", fmt.Errorf("provider request is required")
	}
	if _, err := uuid.Parse(req.GetOperationId()); err != nil {
		return nil, "", fmt.Errorf("operation_id must be a canonical UUID")
	}
	if uuid.MustParse(req.GetOperationId()).String() != req.GetOperationId() {
		return nil, "", fmt.Errorf("operation_id must be a canonical UUID")
	}
	provider := req.GetProvider()
	if !validProviderAliasForSecret(provider.GetAlias()) {
		return nil, "", fmt.Errorf("invalid provider alias %q: use only letters, digits, '_' or '-'", provider.GetAlias())
	}
	if strings.TrimSpace(provider.GetType()) == "" {
		return nil, "", fmt.Errorf("provider type is required")
	}
	settings, err := normalizeProviderSettings(provider.GetSettings())
	if err != nil {
		return nil, "", fmt.Errorf("invalid provider settings: %w", err)
	}
	return provider, settings, nil
}

func (m *providerOperationManager) waitForFlight(ctx context.Context, operationID string, flight *providerOperationFlight) (*pb.ProviderOperation, error) {
	select {
	case <-flight.done:
		queryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), providerOperationFinalizerLimit)
		defer cancel()
		return m.get(queryCtx, operationID, true)
	case <-ctx.Done():
		return nil, status.FromContextError(ctx.Err()).Err()
	}
}

func (m *providerOperationManager) runFlight(flight *providerOperationFlight, request *pb.AddProviderReq, secretName string) {
	defer func() {
		if recover() != nil {
			var state string
			_ = m.engine.DB.QueryRow(`SELECT state FROM provider_operations WHERE operation_id = ?`,
				flight.operationID).Scan(&state)
			if state == providerOperationPending {
				m.failOperation(flight.operationID, secretName, providerFailureInternal)
			}
		}
		m.mu.Lock()
		if current := m.flights[request.GetAlias()]; current == flight {
			delete(m.flights, request.GetAlias())
		}
		close(flight.done)
		m.mu.Unlock()
		flight.releaseAlias()
	}()

	if secretName != "" {
		if err := m.engine.SecretsProvider.Set(m.ctx, secretName, request.GetApiKey()); err != nil {
			m.failOperation(flight.operationID, secretName, providerFailureSecretStore)
			return
		}
	}
	applied, err := m.applyAndFinalize(flight.operationID, request, secretName)
	if err != nil && !applied {
		m.failOperation(flight.operationID, secretName, providerFailureDatabase)
		return
	}
	if err != nil {
		// Applied is intentionally retained for query/startup finalization.
		return
	}
	m.WakeCleanup()
}

func (m *providerOperationManager) applyAndFinalize(operationID string, req *pb.AddProviderReq, secretName string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(m.ctx), providerOperationFinalizerLimit)
	defer cancel()
	m.engine.ProviderRowsMu.Lock()
	defer m.engine.ProviderRowsMu.Unlock()
	if err := m.applyProviderLocked(ctx, operationID, req, secretName); err != nil {
		return false, err
	}
	return true, m.finalizeOperationLocked(ctx, operationID, req.GetApiKey())
}

func (m *providerOperationManager) acquireAlias(alias string) func() {
	m.aliasMu.Lock()
	gate := m.aliases[alias]
	if gate == nil {
		gate = &providerAliasGate{}
		m.aliases[alias] = gate
	}
	gate.refs++
	m.aliasMu.Unlock()

	gate.mu.Lock()
	return func() {
		gate.mu.Unlock()
		m.aliasMu.Lock()
		gate.refs--
		if gate.refs == 0 {
			delete(m.aliases, alias)
		}
		m.aliasMu.Unlock()
	}
}

func (m *providerOperationManager) aliasGateCount() int {
	m.aliasMu.Lock()
	defer m.aliasMu.Unlock()
	return len(m.aliases)
}

func (m *providerOperationManager) applyProviderLocked(ctx context.Context, operationID string, req *pb.AddProviderReq, secretName string) error {
	tx, err := m.engine.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var oldSecret string
	err = tx.QueryRowContext(ctx, `SELECT secret_name FROM llm_providers WHERE alias = ?`, req.GetAlias()).Scan(&oldSecret)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if req.GetIsDefault() {
		if _, err := tx.ExecContext(ctx, `UPDATE llm_providers SET is_default = 0`); err != nil {
			return err
		}
	}
	maxTokens := req.GetMaxTokens()
	if maxTokens == 0 {
		maxTokens = 4096
	}
	isDefault := 0
	if req.GetIsDefault() {
		isDefault = 1
	}
	providerID := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `INSERT INTO llm_providers
		(id, alias, type, model, secret_name, base_url, max_tokens, settings, is_default)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(alias) DO UPDATE SET
		  type = excluded.type,
		  model = excluded.model,
		  secret_name = CASE WHEN excluded.secret_name = '' THEN llm_providers.secret_name ELSE excluded.secret_name END,
		  base_url = CASE WHEN excluded.base_url = '' THEN llm_providers.base_url ELSE excluded.base_url END,
		  settings = CASE WHEN excluded.settings = '{}' THEN llm_providers.settings ELSE excluded.settings END,
		  max_tokens = excluded.max_tokens,
		  is_default = excluded.is_default`,
		providerID, req.GetAlias(), req.GetType(), req.GetModel(), secretName, req.GetBaseUrl(),
		maxTokens, req.GetSettings(), isDefault,
	); err != nil {
		return err
	}
	if oldSecret != "" && secretName != "" && oldSecret != secretName {
		if err := queueProviderSecretCleanupTx(ctx, tx, oldSecret); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE provider_operations SET
		state = ?, failure = '', result_type = ?, result_model = ?, result_is_default = ?, updated_at = CURRENT_TIMESTAMP
		WHERE operation_id = ?`, providerOperationApplied, req.GetType(), req.GetModel(), isDefault, operationID); err != nil {
		return err
	}
	return tx.Commit()
}

func (m *providerOperationManager) failOperation(operationID, secretName, failure string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(m.ctx), providerOperationFinalizerLimit)
	defer cancel()
	m.engine.ProviderRowsMu.Lock()
	defer m.engine.ProviderRowsMu.Unlock()
	tx, err := m.engine.DB.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	if secretName != "" {
		if err := queueProviderSecretCleanupTx(ctx, tx, secretName); err != nil {
			return
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE provider_operations
		SET state = ?, failure = ?, updated_at = CURRENT_TIMESTAMP WHERE operation_id = ?`,
		providerOperationFailed, failure, operationID); err != nil {
		return
	}
	if tx.Commit() == nil {
		m.WakeCleanup()
	}
}

func (m *providerOperationManager) finalizeOperation(parent context.Context, operationID string) error {
	ctx, cancel := context.WithTimeout(parent, providerOperationFinalizerLimit)
	defer cancel()

	var state, secretName string
	if err := m.engine.DB.QueryRowContext(ctx, `SELECT state, secret_name
		FROM provider_operations WHERE operation_id = ?`, operationID).Scan(&state, &secretName); err != nil {
		return err
	}
	if state == providerOperationCommitted || state == providerOperationFailed {
		return nil
	}
	if state != providerOperationApplied {
		return fmt.Errorf("operation is not applied")
	}
	secretValue := ""
	if secretName != "" {
		value, err := m.engine.SecretsProvider.Get(ctx, secretName)
		if err != nil {
			return &providerOperationSecretFinalizationError{cause: err}
		}
		secretValue = value
	}
	m.engine.ProviderRowsMu.Lock()
	defer m.engine.ProviderRowsMu.Unlock()
	return m.finalizeOperationLocked(ctx, operationID, secretValue)
}

func (m *providerOperationManager) finalizeOperationLocked(ctx context.Context, operationID, secretValue string) error {
	var alias, state, secretName string
	if err := m.engine.DB.QueryRowContext(ctx, `SELECT alias, state, secret_name
		FROM provider_operations WHERE operation_id = ?`, operationID).Scan(&alias, &state, &secretName); err != nil {
		return err
	}
	if state == providerOperationCommitted || state == providerOperationFailed {
		return nil
	}
	if state != providerOperationApplied {
		return fmt.Errorf("operation is not applied")
	}
	if secretName != "" && m.engine.SecretsRedactor != nil {
		m.engine.SecretsRedactor.AddValue(secretName, secretValue)
	}
	if m.engine.ProviderRegistry != nil {
		m.engine.ProviderRegistry.InvalidateCacheAlias(alias)
	}
	_, err := m.engine.DB.ExecContext(ctx, `UPDATE provider_operations
		SET state = ?, failure = '', updated_at = CURRENT_TIMESTAMP WHERE operation_id = ?`,
		providerOperationCommitted, operationID)
	return err
}

func (m *providerOperationManager) Get(ctx context.Context, operationID string) (*pb.ProviderOperation, error) {
	if err := m.beginOperation(); err != nil {
		return nil, err
	}
	defer m.background.Done()
	op, err := m.get(ctx, operationID, true)
	if err != nil {
		return nil, err
	}
	return op, nil
}

func (m *providerOperationManager) beginOperation() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.operationAdmissionErrorLocked(); err != nil {
		return err
	}
	m.background.Add(1)
	return nil
}

func (m *providerOperationManager) operationAdmissionError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.operationAdmissionErrorLocked()
}

func (m *providerOperationManager) operationAdmissionErrorLocked() error {
	if m.stopped {
		return status.Error(codes.Unavailable, "provider operations are stopping")
	}
	if !m.started {
		return status.Error(codes.FailedPrecondition, "provider operations are not started")
	}
	return nil
}

func (m *providerOperationManager) get(ctx context.Context, operationID string, finalize bool) (*pb.ProviderOperation, error) {
	op, state, err := queryProviderOperation(ctx, m.engine.DB, operationID)
	if err != nil {
		if ctx.Err() != nil {
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		if errors.Is(err, sql.ErrNoRows) {
			return nil, status.Error(codes.NotFound, "provider operation not found")
		}
		return nil, status.Error(codes.Internal, "query provider operation")
	}
	if finalize && state == providerOperationApplied {
		if err := m.finalizeOperation(context.WithoutCancel(ctx), operationID); err == nil {
			return m.get(ctx, operationID, false)
		}
	}
	return op, nil
}

func queryProviderOperation(ctx context.Context, db *sql.DB, operationID string) (*pb.ProviderOperation, string, error) {
	var alias, state, failure, resultType, resultModel string
	var resultDefault int
	var createdUnix, updatedUnix, expiresUnix int64
	err := db.QueryRowContext(ctx, `SELECT alias, state, failure, result_type, result_model, result_is_default,
		unixepoch(created_at), unixepoch(updated_at), unixepoch(expires_at)
		FROM provider_operations WHERE operation_id = ?`, operationID).Scan(
		&alias, &state, &failure, &resultType, &resultModel, &resultDefault,
		&createdUnix, &updatedUnix, &expiresUnix,
	)
	if err != nil {
		return nil, "", err
	}
	op := &pb.ProviderOperation{
		OperationId: operationID,
		Alias:       alias,
		State:       providerOperationStatePB(state),
		Failure:     providerOperationFailurePB(failure),
		CreatedAt:   timestamppb.New(time.Unix(createdUnix, 0)),
		UpdatedAt:   timestamppb.New(time.Unix(updatedUnix, 0)),
		ExpiresAt:   timestamppb.New(time.Unix(expiresUnix, 0)),
	}
	if state == providerOperationCommitted || state == providerOperationApplied {
		op.Result = &pb.ProviderSaveResult{
			Alias:     alias,
			Type:      resultType,
			Model:     resultModel,
			IsDefault: resultDefault == 1,
		}
	}
	return op, state, nil
}

func syntheticProviderOperation(operationID, alias, failure string) *pb.ProviderOperation {
	return &pb.ProviderOperation{
		OperationId: operationID,
		Alias:       alias,
		State:       pb.ProviderOperationState_PROVIDER_OPERATION_STATE_FAILED,
		Failure:     providerOperationFailurePB(failure),
	}
}

func providerOperationStatePB(state string) pb.ProviderOperationState {
	switch state {
	case providerOperationPending:
		return pb.ProviderOperationState_PROVIDER_OPERATION_STATE_PENDING
	case providerOperationApplied:
		return pb.ProviderOperationState_PROVIDER_OPERATION_STATE_APPLIED
	case providerOperationCommitted:
		return pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED
	case providerOperationFailed:
		return pb.ProviderOperationState_PROVIDER_OPERATION_STATE_FAILED
	default:
		return pb.ProviderOperationState_PROVIDER_OPERATION_STATE_UNSPECIFIED
	}
}

func providerOperationFailurePB(failure string) pb.ProviderOperationFailure {
	switch failure {
	case providerFailureInvalidRequest:
		return pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_INVALID_REQUEST
	case providerFailureConflict:
		return pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_OPERATION_CONFLICT
	case providerFailureAliasBusy:
		return pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_ALIAS_BUSY
	case providerFailureSecretStore:
		return pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_SECRET_STORE
	case providerFailureDatabase:
		return pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_DATABASE
	case providerFailureFinalization:
		return pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_FINALIZATION
	case providerFailureRestartRecovery:
		return pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_RESTART_RECOVERY
	case providerFailureInternal:
		return pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_INTERNAL
	default:
		return pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_UNSPECIFIED
	}
}

func (m *providerOperationManager) reconcileStartup(ctx context.Context) error {
	m.engine.ProviderRowsMu.Lock()
	if _, err := m.engine.DB.ExecContext(ctx, `UPDATE provider_operations
		SET state = ?, failure = ?, updated_at = CURRENT_TIMESTAMP WHERE state = ?`,
		providerOperationFailed, providerFailureRestartRecovery, providerOperationPending); err != nil {
		m.engine.ProviderRowsMu.Unlock()
		return fmt.Errorf("recover pending provider operations: %w", err)
	}
	rows, err := m.engine.DB.QueryContext(ctx, `SELECT operation_id FROM provider_operations WHERE state = ?`, providerOperationApplied)
	if err != nil {
		m.engine.ProviderRowsMu.Unlock()
		return fmt.Errorf("list applied provider operations: %w", err)
	}
	var applied []string
	for rows.Next() {
		var operationID string
		if err := rows.Scan(&operationID); err != nil {
			rows.Close()
			m.engine.ProviderRowsMu.Unlock()
			return err
		}
		applied = append(applied, operationID)
	}
	if err := rows.Close(); err != nil {
		m.engine.ProviderRowsMu.Unlock()
		return err
	}
	m.engine.ProviderRowsMu.Unlock()
	for _, operationID := range applied {
		if err := m.finalizeOperation(ctx, operationID); err != nil {
			var secretErr *providerOperationSecretFinalizationError
			if errors.As(err, &secretErr) && secretErr.retryableAtStartup() {
				continue
			}
			return fmt.Errorf("finalize provider operation: %w", err)
		}
	}

	keys, err := m.engine.SecretsProvider.List(ctx)
	if err != nil {
		return fmt.Errorf("list provider secrets: %w", err)
	}
	for _, key := range keys {
		if !strings.HasPrefix(key, "provider-v2-") {
			continue
		}
		var references int
		if err := m.engine.DB.QueryRowContext(ctx, `SELECT
			(SELECT count(*) FROM llm_providers WHERE secret_name = ?) +
			(SELECT count(*) FROM provider_operations WHERE secret_name = ? AND state IN (?, ?))`,
			key, key, providerOperationPending, providerOperationApplied).Scan(&references); err != nil {
			return err
		}
		if references == 0 {
			if _, err := m.engine.DB.ExecContext(ctx, `INSERT INTO provider_secret_cleanup
				(secret_name, attempt_count, failure, created_at, updated_at, next_attempt_at)
				VALUES (?, 0, '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
				ON CONFLICT(secret_name) DO NOTHING`, key); err != nil {
				return err
			}
		}
	}
	if _, err := m.engine.DB.ExecContext(ctx, `DELETE FROM provider_operations
		WHERE state IN (?, ?) AND unixepoch(expires_at) <= unixepoch()`, providerOperationCommitted, providerOperationFailed); err != nil {
		return err
	}
	return nil
}
