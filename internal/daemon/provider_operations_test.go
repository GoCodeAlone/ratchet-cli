package daemon

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	ratchetplugin "github.com/GoCodeAlone/workflow-plugin-agent/orchestrator"
	"github.com/GoCodeAlone/workflow/secrets"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	_ "modernc.org/sqlite"
)

func TestProviderOperationSchemaUpgradesLegacyDatabase(t *testing.T) {
	db := openProviderOperationTestDB(t)
	_, err := db.Exec(`CREATE TABLE llm_providers (
		id TEXT PRIMARY KEY,
		alias TEXT UNIQUE NOT NULL,
		type TEXT NOT NULL,
		model TEXT,
		secret_name TEXT,
		base_url TEXT,
		max_tokens INTEGER DEFAULT 4096,
		is_default INTEGER DEFAULT 0
	)`)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := initDB(db); err != nil {
			t.Fatalf("initDB: %v", err)
		}
	}
	for _, table := range []string{"provider_operations", "provider_secret_cleanup"} {
		var name string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name); err != nil {
			t.Fatalf("table %s: %v", table, err)
		}
	}
	_, err = db.Exec(`INSERT INTO provider_operations
		(operation_id, alias, state, expires_at) VALUES
		('first-active', 'same-alias', 'pending', datetime('now', '+1 day')),
		('second-active', 'same-alias', 'applied', datetime('now', '+1 day'))`)
	if err == nil {
		t.Fatal("schema accepted two active operations for one alias")
	}
}

func TestProviderOperationSchemaFailureStopsStartup(t *testing.T) {
	db := openProviderOperationTestDB(t)
	if _, err := db.Exec(`CREATE VIEW provider_operations AS SELECT 1 AS operation_id`); err != nil {
		t.Fatal(err)
	}
	if err := initDB(db); err == nil {
		t.Fatal("initDB succeeded with conflicting provider_operations view")
	}
}

func TestProviderOperationStatePBMapsApplied(t *testing.T) {
	if got := providerOperationStatePB(providerOperationApplied); got != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_APPLIED {
		t.Fatalf("providerOperationStatePB(applied) = %s, want APPLIED", got)
	}
}

func TestGetProviderOperationFinalizationFailureRemainsAppliedAndRetries(t *testing.T) {
	const (
		operationID = "b4aff148-ef2b-4b18-9971-e4702ab45586"
		alias       = "applied-retry"
		secretName  = "provider-v2-applied-retry"
		credential  = "APPLIED-RETRY-CREDENTIAL-SENTINEL"
		rawError    = "APPLIED-RETRY-RAW-ERROR-SENTINEL"
	)

	provider := newOperationSecrets()
	svc, db := newProviderOperationTestService(t, provider)
	if err := provider.Set(t.Context(), secretName, credential); err != nil {
		t.Fatal(err)
	}
	insertProviderRow(t, db, alias, secretName, "retry-model")
	if _, err := db.Exec(`INSERT INTO provider_operations
		(operation_id, alias, state, failure, secret_name, result_type, result_model,
		 result_is_default, created_at, updated_at, expires_at)
		VALUES (?, ?, 'applied', '', ?, 'openai', 'retry-model', 1,
		 CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, datetime('now', '+1 day'))`,
		operationID, alias, secretName); err != nil {
		t.Fatal(err)
	}

	failGet := true
	getAttempts := 0
	provider.getHook = func(_ context.Context, key string) error {
		if key != secretName {
			return nil
		}
		getAttempts++
		if failGet {
			return errors.New(rawError)
		}
		return nil
	}

	applied, err := svc.GetProviderOperation(t.Context(), &pb.GetProviderOperationReq{OperationId: operationID})
	if err != nil {
		t.Fatal(err)
	}
	if applied.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_APPLIED {
		t.Fatalf("first query state = %s, want APPLIED", applied.GetState())
	}
	if applied.GetFailure() != pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_UNSPECIFIED {
		t.Fatalf("first query failure = %s, want UNSPECIFIED", applied.GetFailure())
	}
	result := applied.GetResult()
	if result == nil || result.GetAlias() != alias || result.GetType() != "openai" ||
		result.GetModel() != "retry-model" || !result.GetIsDefault() {
		t.Fatalf("first query result = %+v", result)
	}
	wire, err := protojson.Marshal(applied)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), credential) || strings.Contains(string(wire), rawError) {
		t.Fatalf("applied operation exposed secret or raw error: %s", wire)
	}
	assertOperationFailure(t, db, operationID, providerOperationApplied, "")

	failGet = false
	committed, err := svc.GetProviderOperation(t.Context(), &pb.GetProviderOperationReq{OperationId: operationID})
	if err != nil {
		t.Fatal(err)
	}
	if committed.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED {
		t.Fatalf("retry query state = %s, want COMMITTED", committed.GetState())
	}
	if getAttempts != 2 {
		t.Fatalf("secret get attempts = %d, want 2", getAttempts)
	}
	assertOperationFailure(t, db, operationID, providerOperationCommitted, "")
}

func TestProviderOperationStartupKeepsUnfinalizedAppliedRetryable(t *testing.T) {
	const (
		operationID = "3833c939-829e-4749-ac13-195081eb181d"
		alias       = "startup-applied-retry"
		secretName  = "provider-v2-startup-applied-retry"
	)

	provider := newOperationSecrets()
	db := openProviderOperationTestDB(t)
	if err := initDB(db); err != nil {
		t.Fatal(err)
	}
	if err := provider.Set(t.Context(), secretName, "startup-retry-credential"); err != nil {
		t.Fatal(err)
	}
	insertProviderRow(t, db, alias, secretName, "startup-retry-model")
	if _, err := db.Exec(`INSERT INTO provider_operations
		(operation_id, alias, state, failure, secret_name, result_type, result_model,
		 result_is_default, created_at, updated_at, expires_at)
		VALUES (?, ?, 'applied', '', ?, 'openai', 'startup-retry-model', 1,
		 CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, datetime('now', '+1 day'))`,
		operationID, alias, secretName); err != nil {
		t.Fatal(err)
	}

	failGet := true
	getAttempts := 0
	provider.getHook = func(_ context.Context, key string) error {
		if key == secretName {
			getAttempts++
			if failGet {
				return errors.New("startup finalization unavailable")
			}
		}
		return nil
	}
	engine := &EngineContext{
		DB:              db,
		SecretsProvider: provider,
		SecretsRedactor: secrets.NewRedactor(),
	}
	manager := newProviderOperationManager(engine)
	if err := manager.Start(t.Context()); err != nil {
		t.Fatalf("provider operation startup with retryable applied row: %v", err)
	}
	t.Cleanup(manager.Stop)
	if getAttempts != 1 {
		t.Fatalf("startup secret get attempts = %d, want 1", getAttempts)
	}
	svc := &Service{engine: engine, providerOps: manager}

	applied, err := svc.GetProviderOperation(t.Context(), &pb.GetProviderOperationReq{OperationId: operationID})
	if err != nil {
		t.Fatal(err)
	}
	if applied.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_APPLIED {
		t.Fatalf("unavailable finalization state = %s, want APPLIED", applied.GetState())
	}
	if getAttempts != 2 {
		t.Fatalf("failed query secret get attempts = %d, want 2", getAttempts)
	}
	assertOperationFailure(t, db, operationID, providerOperationApplied, "")

	failGet = false
	committed, err := svc.GetProviderOperation(t.Context(), &pb.GetProviderOperationReq{OperationId: operationID})
	if err != nil {
		t.Fatal(err)
	}
	if committed.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED {
		t.Fatalf("recovered finalization state = %s, want COMMITTED", committed.GetState())
	}
	if getAttempts != 3 {
		t.Fatalf("recovered secret get attempts = %d, want 3", getAttempts)
	}
	assertOperationFailure(t, db, operationID, providerOperationCommitted, "")
}

func TestProviderOperationStartupFinalizesAppliedWithAvailableSecret(t *testing.T) {
	const (
		operationID = "db396feb-4d9f-4fea-aef8-0f3ada67ae44"
		alias       = "startup-applied-commit"
		secretName  = "provider-v2-startup-applied-commit"
	)

	provider := newOperationSecrets()
	db := openProviderOperationTestDB(t)
	if err := initDB(db); err != nil {
		t.Fatal(err)
	}
	if err := provider.Set(t.Context(), secretName, "startup-commit-credential"); err != nil {
		t.Fatal(err)
	}
	insertProviderRow(t, db, alias, secretName, "startup-commit-model")
	if _, err := db.Exec(`INSERT INTO provider_operations
		(operation_id, alias, state, failure, secret_name, result_type, result_model,
		 result_is_default, created_at, updated_at, expires_at)
		VALUES (?, ?, 'applied', '', ?, 'openai', 'startup-commit-model', 1,
		 CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, datetime('now', '+1 day'))`,
		operationID, alias, secretName); err != nil {
		t.Fatal(err)
	}
	engine := &EngineContext{
		DB:              db,
		SecretsProvider: provider,
		SecretsRedactor: secrets.NewRedactor(),
	}
	manager := newProviderOperationManager(engine)
	if err := manager.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Stop)
	assertOperationFailure(t, db, operationID, providerOperationCommitted, "")
}

func TestProviderOperationStartupFinalizationDatabaseFailureStopsStartup(t *testing.T) {
	const (
		operationID = "674e5764-ecc9-4d94-b790-53b9b59ce1f0"
		alias       = "startup-applied-db-failure"
		secretName  = "provider-v2-startup-applied-db-failure"
	)

	provider := newOperationSecrets()
	db := openProviderOperationTestDB(t)
	if err := initDB(db); err != nil {
		t.Fatal(err)
	}
	if err := provider.Set(t.Context(), secretName, "startup-db-failure-credential"); err != nil {
		t.Fatal(err)
	}
	insertProviderRow(t, db, alias, secretName, "startup-db-failure-model")
	if _, err := db.Exec(`INSERT INTO provider_operations
		(operation_id, alias, state, failure, secret_name, result_type, result_model,
		 result_is_default, created_at, updated_at, expires_at)
		VALUES (?, ?, 'applied', '', ?, 'openai', 'startup-db-failure-model', 1,
		 CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, datetime('now', '+1 day'))`,
		operationID, alias, secretName); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_provider_operation_finalization
		BEFORE UPDATE OF state ON provider_operations
		WHEN OLD.state = 'applied' AND NEW.state = 'committed'
		BEGIN SELECT RAISE(ABORT, 'forced finalization failure'); END`); err != nil {
		t.Fatal(err)
	}
	engine := &EngineContext{
		DB:              db,
		SecretsProvider: provider,
		SecretsRedactor: secrets.NewRedactor(),
	}
	manager := newProviderOperationManager(engine)
	err := manager.Start(t.Context())
	manager.Stop()
	if err == nil {
		t.Fatal("provider operation startup ignored finalization database failure")
	}
	if !strings.Contains(err.Error(), "finalize provider operation") {
		t.Fatalf("provider operation startup error = %q", err)
	}
	assertOperationFailure(t, db, operationID, providerOperationApplied, "")
}

func TestProviderOperationStartupFinalizationContextFailuresStopStartup(t *testing.T) {
	const (
		operationID = "d08ad01e-199a-42a4-89e0-e7b09c7016cf"
		alias       = "startup-applied-context-failure"
		secretName  = "provider-v2-startup-applied-context-failure"
	)

	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "canceled", err: context.Canceled, want: context.Canceled},
		{name: "wrapped deadline", err: fmt.Errorf("secret read: %w", context.DeadlineExceeded), want: context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := newOperationSecrets()
			db := openProviderOperationTestDB(t)
			if err := initDB(db); err != nil {
				t.Fatal(err)
			}
			if err := provider.Set(t.Context(), secretName, "startup-context-failure-credential"); err != nil {
				t.Fatal(err)
			}
			insertProviderRow(t, db, alias, secretName, "startup-context-failure-model")
			if _, err := db.Exec(`INSERT INTO provider_operations
				(operation_id, alias, state, failure, secret_name, result_type, result_model,
				 result_is_default, created_at, updated_at, expires_at)
				VALUES (?, ?, 'applied', '', ?, 'openai', 'startup-context-failure-model', 1,
				 CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, datetime('now', '+1 day'))`,
				operationID, alias, secretName); err != nil {
				t.Fatal(err)
			}
			provider.getHook = func(context.Context, string) error {
				return tt.err
			}
			engine := &EngineContext{
				DB:              db,
				SecretsProvider: provider,
				SecretsRedactor: secrets.NewRedactor(),
			}
			manager := newProviderOperationManager(engine)
			err := manager.Start(t.Context())
			manager.Stop()
			if !errors.Is(err, tt.want) {
				t.Fatalf("provider operation startup error = %v, want %v", err, tt.want)
			}
			assertOperationFailure(t, db, operationID, providerOperationApplied, "")
		})
	}
}

func TestCommitProviderSaveRollbackPreservesActiveSecret(t *testing.T) {
	provider := newOperationSecrets()
	provider.values["provider_old"] = "old-secret"
	svc, db := newProviderOperationTestService(t, provider)
	insertProviderRow(t, db, "work", "provider_old", "old-model")
	if _, err := db.Exec(`CREATE TRIGGER fail_provider_update BEFORE UPDATE ON llm_providers
		WHEN NEW.alias = 'work' BEGIN SELECT RAISE(ABORT, 'forced apply failure'); END`); err != nil {
		t.Fatal(err)
	}

	op, err := svc.CommitProviderSave(t.Context(), providerSaveRequest(
		"f43f9294-9df8-4645-94d1-d46c72afe055", "work", "new-secret",
	))
	if err != nil {
		t.Fatal(err)
	}
	if op.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_FAILED ||
		op.GetFailure() != pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_DATABASE {
		t.Fatalf("operation = %+v, want failed/database", op)
	}

	var secretName, model string
	if err := db.QueryRow(`SELECT secret_name, model FROM llm_providers WHERE alias = 'work'`).Scan(&secretName, &model); err != nil {
		t.Fatal(err)
	}
	if secretName != "provider_old" || model != "old-model" {
		t.Fatalf("active provider = secret %q model %q, want provider_old/old-model", secretName, model)
	}
	if got, err := provider.Get(t.Context(), "provider_old"); err != nil || got != "old-secret" {
		t.Fatalf("old secret = %q, %v", got, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		keys, listErr := provider.List(t.Context())
		if listErr == nil && len(keys) == 1 && keys[0] == "provider_old" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("inactive rollback secret was not retired; keys = %v", mustListOperationSecrets(t, provider))
}

func TestCommitProviderSaveReplayAliasBusyAndConflict(t *testing.T) {
	provider := newOperationSecrets()
	started := make(chan struct{})
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	defer releaseOnce()
	provider.setHook = func(_ context.Context, _, value string) error {
		if value == "first-secret" {
			close(started)
			<-release
		}
		return nil
	}
	svc, db := newProviderOperationTestService(t, provider)
	opID := "b100c1cd-6d90-44b9-920c-5704b10e456f"

	firstDone := make(chan *pb.ProviderOperation, 1)
	go func() {
		op, _ := svc.CommitProviderSave(context.Background(), providerSaveRequest(opID, "work", "first-secret"))
		firstDone <- op
	}()
	<-started

	attachCtx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := svc.CommitProviderSave(attachCtx, providerSaveRequest(opID, "work", "ignored-secret")); status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("same-ID attachment code = %v, want DeadlineExceeded (err=%v)", status.Code(err), err)
	}

	busyID := "da159b56-b5fa-4b34-8906-27d498493f37"
	busy, err := svc.CommitProviderSave(t.Context(), providerSaveRequest(busyID, "work", "must-not-retain"))
	if err != nil {
		t.Fatal(err)
	}
	if busy.GetFailure() != pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_ALIAS_BUSY {
		t.Fatalf("busy failure = %s, want alias busy", busy.GetFailure())
	}
	assertNoOperationRow(t, db, busyID)
	if slices.Contains(provider.setValues(), "must-not-retain") {
		t.Fatal("busy credential reached the secrets provider")
	}

	conflict, err := svc.CommitProviderSave(t.Context(), providerSaveRequest(opID, "other", "must-not-retain"))
	if err != nil {
		t.Fatal(err)
	}
	if conflict.GetFailure() != pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_OPERATION_CONFLICT {
		t.Fatalf("conflict failure = %s, want operation conflict", conflict.GetFailure())
	}

	releaseOnce()
	if op := <-firstDone; op.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED {
		t.Fatalf("first operation = %+v, want committed", op)
	}
	replayed, err := svc.CommitProviderSave(t.Context(), providerSaveRequest(opID, "work", "different-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if replayed.GetResult().GetModel() != "test-model" || slices.Contains(provider.setValues(), "different-secret") {
		t.Fatalf("replay was not first-write-wins: %+v", replayed)
	}

	canceled, cancelCanceled := context.WithCancel(t.Context())
	cancelCanceled()
	canceledID := "6083902b-0007-40cc-a36f-b8a8fed948fe"
	if _, err := svc.CommitProviderSave(canceled, providerSaveRequest(canceledID, "canceled", "secret")); status.Code(err) != codes.Canceled {
		t.Fatalf("canceled admission code = %v, want Canceled (err=%v)", status.Code(err), err)
	}
	assertNoOperationRow(t, db, canceledID)
	if got := svc.providerOps.aliasGateCount(); got != 0 {
		t.Fatalf("retained alias gates = %d, want 0", got)
	}
}

func TestProviderOperationBlockingSecretAdmissionAndRestart(t *testing.T) {
	provider := newOperationSecrets()
	started := make(chan struct{})
	release := make(chan struct{})
	provider.setHook = func(_ context.Context, _, value string) error {
		if value == "blocked-secret" {
			close(started)
			<-release
		}
		return nil
	}
	svc, db := newProviderOperationTestService(t, provider)
	blockedID := "70b4ec9e-5144-4587-bb43-7124143d56ad"

	ctx, expire := newDeadlineSignalContext(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := svc.CommitProviderSave(ctx, providerSaveRequest(blockedID, "blocked", "blocked-secret"))
		done <- err
	}()
	if err := waitProviderSecretAdmission(started, done, time.After(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	expire()
	if err := waitProviderSaveResult(done, time.After(5*time.Second)); status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("blocked save code = %v, want DeadlineExceeded (err=%v)", status.Code(err), err)
	}
	assertOperationState(t, db, blockedID, "pending")

	other, err := svc.CommitProviderSave(t.Context(), providerSaveRequest(
		"98e50518-e328-4a76-8034-dd3232beaa29", "other", "other-secret",
	))
	if err != nil || other.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED {
		t.Fatalf("unrelated save = %+v, %v", other, err)
	}
	busy, err := svc.CommitProviderSave(t.Context(), providerSaveRequest(
		"de51c54a-a0a5-4566-bcb4-269a2a3bf84c", "blocked", "replacement",
	))
	if err != nil || busy.GetFailure() != pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_ALIAS_BUSY {
		t.Fatalf("replacement = %+v, %v", busy, err)
	}
	close(release)
	waitProviderOperation(t, svc, blockedID, pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED)

	svc.providerOps.Stop()
	pendingID := "29636aa2-0189-4014-acbb-f489415a7de4"
	if _, err := db.Exec(`INSERT INTO provider_operations
		(operation_id, alias, state, failure, secret_name, created_at, updated_at, expires_at)
		VALUES (?, 'restart', 'pending', '', 'provider-v2-restart', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, datetime('now', '+1 day'))`, pendingID); err != nil {
		t.Fatal(err)
	}
	if err := provider.Set(t.Context(), "provider-v2-restart", "restart-secret"); err != nil {
		t.Fatal(err)
	}
	expiredID := "f91fe749-45d8-48b4-89a1-4f2406c10a89"
	if _, err := db.Exec(`INSERT INTO provider_operations
		(operation_id, alias, state, failure, secret_name, created_at, updated_at, expires_at)
		VALUES (?, 'expired', 'committed', '', '', ?, ?, ?)`, expiredID,
		time.Now().Add(-48*time.Hour).UTC().Format(time.RFC3339Nano),
		time.Now().Add(-48*time.Hour).UTC().Format(time.RFC3339Nano),
		time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	restarted := newProviderOperationManager(svc.engine)
	if err := restarted.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restarted.Stop)
	assertOperationFailure(t, db, pendingID, "failed", "restart_recovery")
	assertNoOperationRow(t, db, expiredID)
	restarted.Stop()
	provider.listErr = errors.New("list unavailable")
	if err := newProviderOperationManager(svc.engine).Start(t.Context()); err == nil {
		t.Fatal("provider operation startup succeeded when secret List failed")
	}
}

func TestWaitProviderSecretAdmissionOutcomes(t *testing.T) {
	workerErr := errors.New("worker failed")
	tests := []struct {
		name    string
		started bool
		done    bool
		doneErr error
		timeout bool
		wantErr error
		wantMsg string
	}{
		{name: "admitted", started: true},
		{name: "worker error", done: true, doneErr: workerErr, wantErr: workerErr},
		{name: "worker success", done: true, wantMsg: "returned before secret admission"},
		{name: "timeout", timeout: true, wantMsg: "timed out waiting for blocked secret admission"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			started := make(chan struct{})
			done := make(chan error, 1)
			timeout := make(chan time.Time)
			if tt.started {
				close(started)
			}
			if tt.done {
				done <- tt.doneErr
			}
			if tt.timeout {
				close(timeout)
			}

			err := waitProviderSecretAdmission(started, done, timeout)
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("wait error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && tt.wantMsg == "" && err != nil {
				t.Fatalf("wait error = %v, want nil", err)
			}
			if tt.wantMsg != "" {
				if err == nil {
					t.Fatalf("wait error = nil, want substring %q", tt.wantMsg)
				}
				if !strings.Contains(err.Error(), tt.wantMsg) {
					t.Fatalf("wait error = %q, want substring %q", err, tt.wantMsg)
				}
			}
		})
	}
}

func TestDeadlineSignalContextPreservesCancellationCause(t *testing.T) {
	t.Run("manual deadline", func(t *testing.T) {
		ctx, expire := newDeadlineSignalContext(t.Context())
		expire()
		expire()
		<-ctx.Done()
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatalf("context error = %v, want DeadlineExceeded", ctx.Err())
		}
	})
	t.Run("parent cancellation", func(t *testing.T) {
		parent, cancel := context.WithCancel(t.Context())
		ctx, expire := newDeadlineSignalContext(parent)
		cancel()
		<-ctx.Done()
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("context error = %v, want Canceled", ctx.Err())
		}
		expire()
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("context error after expiry = %v, want stable Canceled", ctx.Err())
		}
	})
}

func TestWaitProviderSaveResultTimesOut(t *testing.T) {
	done := make(chan error)
	timeout := make(chan time.Time)
	close(timeout)

	err := waitProviderSaveResult(done, timeout)
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for blocked provider save") {
		t.Fatalf("wait error = %v, want timeout", err)
	}
}

func waitProviderSecretAdmission(started <-chan struct{}, done <-chan error, timeout <-chan time.Time) error {
	select {
	case <-started:
		return nil
	case err := <-done:
		if err == nil {
			return errors.New("blocked save returned before secret admission")
		}
		return fmt.Errorf("blocked save returned before secret admission: %w", err)
	case <-timeout:
		return errors.New("timed out waiting for blocked secret admission")
	}
}

func waitProviderSaveResult(done <-chan error, timeout <-chan time.Time) error {
	select {
	case err := <-done:
		return err
	case <-timeout:
		return errors.New("timed out waiting for blocked provider save")
	}
}

type deadlineSignalContext struct {
	context.Context
}

func newDeadlineSignalContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	return deadlineSignalContext{Context: ctx}, func() {
		cancel(context.DeadlineExceeded)
	}
}

func (c deadlineSignalContext) Err() error {
	return context.Cause(c.Context)
}

func TestAddProviderMapsAliasBusyToAborted(t *testing.T) {
	provider := newOperationSecrets()
	started := make(chan struct{})
	release := make(chan struct{})
	provider.setHook = func(_ context.Context, _, value string) error {
		if value == "blocked-secret" {
			close(started)
			<-release
		}
		return nil
	}
	svc, _ := newProviderOperationTestService(t, provider)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = svc.CommitProviderSave(t.Context(), providerSaveRequest(
			"70b4ec9e-5144-4587-bb43-7124143d56ad", "blocked", "blocked-secret",
		))
	}()
	<-started

	_, err := svc.AddProvider(t.Context(), &pb.AddProviderReq{
		Alias: "blocked", Type: "openai", Model: "replacement", ApiKey: "replacement-secret",
	})
	if got := status.Code(err); got != codes.Aborted {
		t.Fatalf("AddProvider busy code = %v, want Aborted (err=%v)", got, err)
	}
	close(release)
	<-done
}

func TestProviderSaveFailureErrorCodes(t *testing.T) {
	tests := []struct {
		failure pb.ProviderOperationFailure
		code    codes.Code
	}{
		{pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_INVALID_REQUEST, codes.InvalidArgument},
		{pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_OPERATION_CONFLICT, codes.AlreadyExists},
		{pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_ALIAS_BUSY, codes.Aborted},
		{pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_RESTART_RECOVERY, codes.Aborted},
		{pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_SECRET_STORE, codes.Unavailable},
		{pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_DATABASE, codes.Internal},
		{pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_FINALIZATION, codes.Internal},
		{pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_INTERNAL, codes.Internal},
	}
	for _, tt := range tests {
		if got := status.Code(providerSaveFailureError(tt.failure)); got != tt.code {
			t.Errorf("providerSaveFailureError(%s) code = %v, want %v", tt.failure, got, tt.code)
		}
	}
}

func TestValidateProviderSaveRequestExplainsInvalidSettings(t *testing.T) {
	const malformed = `{"region":}`
	_, _, err := validateProviderSaveRequest(&pb.CommitProviderSaveReq{
		OperationId: "70b4ec9e-5144-4587-bb43-7124143d56ad",
		Provider: &pb.AddProviderReq{
			Alias: "custom", Type: "custom", Settings: malformed,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid provider settings: invalid character") {
		t.Fatalf("invalid settings error = %v", err)
	}
	if strings.Contains(err.Error(), malformed) {
		t.Fatal("invalid settings error echoed the settings body")
	}
}

func TestProviderOperationWorkerPanicReleasesOwnership(t *testing.T) {
	provider := newOperationSecrets()
	var logs bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(oldOutput) })
	provider.setHook = func(_ context.Context, _, value string) error {
		if value == "panic-secret" {
			panic("credential-bearing panic-secret")
		}
		return nil
	}
	svc, _ := newProviderOperationTestService(t, provider)
	panicked, err := svc.CommitProviderSave(t.Context(), providerSaveRequest(
		"80dcfb61-73ce-4128-a62b-72aee7797950", "work", "panic-secret",
	))
	if err != nil {
		t.Fatal(err)
	}
	if panicked.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_FAILED ||
		panicked.GetFailure() != pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_INTERNAL {
		t.Fatalf("panicked operation = %+v", panicked)
	}
	if strings.Contains(logs.String(), "credential-bearing panic-secret") {
		t.Fatal("worker panic value leaked to logs")
	}

	retry, err := svc.CommitProviderSave(t.Context(), providerSaveRequest(
		"a5de80d2-1500-4d4e-a60e-231e7e4ffad0", "work", "safe-secret",
	))
	if err != nil || retry.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED {
		t.Fatalf("retry = %+v, %v", retry, err)
	}
}

func TestProviderMutationOrdering(t *testing.T) {
	provider := newOperationSecrets()
	provider.values["provider_old"] = "old-secret"
	started := make(chan struct{})
	release := make(chan struct{})
	provider.setHook = func(_ context.Context, _, value string) error {
		if value == "new-secret" {
			close(started)
			<-release
		}
		return nil
	}
	provider.getHook = func(_ context.Context, key string) error {
		if strings.HasPrefix(key, "provider-v2-") {
			return errors.New("live finalization must not re-read the secret")
		}
		return nil
	}
	svc, db := newProviderOperationTestService(t, provider)
	insertProviderRow(t, db, "work", "provider_old", "old-model")

	saveDone := make(chan *pb.ProviderOperation, 1)
	go func() {
		op, _ := svc.CommitProviderSave(context.Background(), providerSaveRequest(
			"096959a4-e984-4c7c-95dc-09d183394a55", "work", "new-secret",
		))
		saveDone <- op
	}()
	<-started
	if _, err := svc.UpdateProviderModel(t.Context(), &pb.UpdateProviderModelReq{Alias: "work", Model: "intermediate"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetDefaultProvider(t.Context(), &pb.SetDefaultProviderReq{Alias: "work"}); err != nil {
		t.Fatal(err)
	}
	removeDone := make(chan error, 1)
	go func() {
		_, err := svc.RemoveProvider(context.Background(), &pb.RemoveProviderReq{Alias: "work"})
		removeDone <- err
	}()
	select {
	case err := <-removeDone:
		t.Fatalf("remove completed before admitted save: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if op := <-saveDone; op.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED {
		t.Fatalf("save state = %s, want committed", op.GetState())
	}
	if err := <-removeDone; err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM llm_providers WHERE alias = 'work'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("provider count = %d, want remove to linearize after save", count)
	}
}

func TestProviderOperationPayloadContainsNoSensitiveFields(t *testing.T) {
	provider := newOperationSecrets()
	svc, db := newProviderOperationTestService(t, provider)
	credential := "credential-sentinel-42"
	baseURL := "https://private-endpoint.invalid/sentinel"
	settings := `{"region":"settings-sentinel"}`
	req := providerSaveRequest("44d28da9-52a0-45f0-a380-2ea31a2192bb", "work", credential)
	req.Provider.BaseUrl = baseURL
	req.Provider.Settings = settings

	var logs bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(oldOutput) })
	op, err := svc.CommitProviderSave(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := protojson.Marshal(op)
	if err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query(`SELECT operation_id, alias, state, failure, secret_name,
		result_type, result_model, result_is_default, created_at, updated_at, expires_at
		FROM provider_operations`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var persisted strings.Builder
	columns, _ := rows.Columns()
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		fmt.Fprint(&persisted, values)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for name, timestamp := range map[string]time.Time{
		"created_at": op.GetCreatedAt().AsTime(),
		"updated_at": op.GetUpdatedAt().AsTime(),
		"expires_at": op.GetExpiresAt().AsTime(),
	} {
		if timestamp.IsZero() {
			t.Fatalf("operation %s is zero", name)
		}
	}

	columnRows, err := db.Query(`PRAGMA table_info(provider_operations)`)
	if err != nil {
		t.Fatal(err)
	}
	defer columnRows.Close()
	var operationColumns []string
	for columnRows.Next() {
		var cid, notNull, primaryKey int
		var name, typ string
		var defaultValue any
		if err := columnRows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		operationColumns = append(operationColumns, name)
	}
	slices.Sort(operationColumns)
	wantColumns := []string{
		"alias", "created_at", "expires_at", "failure", "operation_id",
		"result_is_default", "result_model", "result_type", "secret_name",
		"state", "updated_at",
	}
	slices.Sort(wantColumns)
	if !slices.Equal(operationColumns, wantColumns) {
		t.Fatalf("provider_operations columns = %v, want %v", operationColumns, wantColumns)
	}
	combined := string(wire) + persisted.String() + logs.String()
	for _, sentinel := range []string{credential, baseURL, "settings-sentinel"} {
		if strings.Contains(combined, sentinel) {
			t.Fatalf("operation payload leaked sensitive sentinel %q", sentinel)
		}
	}
}

func newProviderOperationTestService(t *testing.T, provider secrets.Provider) (*Service, *sql.DB) {
	t.Helper()
	db := openProviderOperationTestDB(t)
	if err := initDB(db); err != nil {
		t.Fatal(err)
	}
	redactor := secrets.NewRedactor()
	engine := &EngineContext{
		DB:              db,
		SecretsProvider: provider,
		SecretsRedactor: redactor,
	}
	engine.ProviderRegistry = ratchetplugin.NewProviderRegistry(db, func() secrets.Provider { return provider })
	manager := newProviderOperationManager(engine)
	if err := manager.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Stop)
	return &Service{engine: engine, providerOps: manager}, db
}

func openProviderOperationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+strings.ReplaceAll(t.Name(), "/", "-")+"-"+uuid.NewString()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func providerSaveRequest(operationID, alias, credential string) *pb.CommitProviderSaveReq {
	return &pb.CommitProviderSaveReq{
		OperationId: operationID,
		Provider: &pb.AddProviderReq{
			Alias:     alias,
			Type:      "openai",
			Model:     "test-model",
			ApiKey:    credential,
			IsDefault: true,
		},
	}
}

func insertProviderRow(t *testing.T, db *sql.DB, alias, secretName, model string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO llm_providers
		(id, alias, type, model, secret_name, base_url, max_tokens, settings, is_default)
		VALUES (?, ?, 'openai', ?, ?, '', 4096, '{}', 1)`, "id-"+alias, alias, model, secretName)
	if err != nil {
		t.Fatal(err)
	}
}

func waitProviderOperation(t *testing.T, svc *Service, operationID string, want pb.ProviderOperationState) *pb.ProviderOperation {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		op, err := svc.GetProviderOperation(t.Context(), &pb.GetProviderOperationReq{OperationId: operationID})
		if err == nil && op.GetState() == want {
			return op
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("operation %s did not reach %s", operationID, want)
	return nil
}

func assertNoOperationRow(t *testing.T, db *sql.DB, operationID string) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM provider_operations WHERE operation_id = ?`, operationID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("operation %s was journaled", operationID)
	}
}

func assertOperationState(t *testing.T, db *sql.DB, operationID, state string) {
	t.Helper()
	var got string
	if err := db.QueryRow(`SELECT state FROM provider_operations WHERE operation_id = ?`, operationID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != state {
		t.Fatalf("operation %s state = %q, want %q", operationID, got, state)
	}
}

func assertOperationFailure(t *testing.T, db *sql.DB, operationID, state, failure string) {
	t.Helper()
	var gotState, gotFailure string
	if err := db.QueryRow(`SELECT state, failure FROM provider_operations WHERE operation_id = ?`, operationID).Scan(&gotState, &gotFailure); err != nil {
		t.Fatal(err)
	}
	if gotState != state || gotFailure != failure {
		t.Fatalf("operation %s = %s/%s, want %s/%s", operationID, gotState, gotFailure, state, failure)
	}
}

type operationSecrets struct {
	mu         sync.Mutex
	values     map[string]string
	sets       []string
	setHook    func(context.Context, string, string) error
	getHook    func(context.Context, string) error
	deleteHook func(context.Context, string) error
	listErr    error
}

func newOperationSecrets() *operationSecrets {
	return &operationSecrets{values: make(map[string]string)}
}

func (p *operationSecrets) Name() string { return "operation-test" }

func (p *operationSecrets) Get(ctx context.Context, key string) (string, error) {
	if p.getHook != nil {
		if err := p.getHook(ctx, key); err != nil {
			return "", err
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	value, ok := p.values[key]
	if !ok {
		return "", secrets.ErrNotFound
	}
	return value, nil
}

func (p *operationSecrets) Set(ctx context.Context, key, value string) error {
	if p.setHook != nil {
		if err := p.setHook(ctx, key, value); err != nil {
			return err
		}
	}
	p.mu.Lock()
	p.values[key] = value
	p.sets = append(p.sets, value)
	p.mu.Unlock()
	return nil
}

func (p *operationSecrets) Delete(ctx context.Context, key string) error {
	if p.deleteHook != nil {
		if err := p.deleteHook(ctx, key); err != nil {
			return err
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.values[key]; !ok {
		return secrets.ErrNotFound
	}
	delete(p.values, key)
	return nil
}

func (p *operationSecrets) List(context.Context) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.listErr != nil {
		return nil, p.listErr
	}
	return slices.Sorted(maps.Keys(p.values)), nil
}

func (p *operationSecrets) setValues() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.sets)
}

func mustListOperationSecrets(t *testing.T, provider secrets.Provider) []string {
	t.Helper()
	keys, err := provider.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

var _ secrets.Provider = (*operationSecrets)(nil)
