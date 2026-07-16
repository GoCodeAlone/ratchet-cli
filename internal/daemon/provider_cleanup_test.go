package daemon

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GoCodeAlone/workflow/secrets"
)

func TestProviderCleanupDispatcherFairness(t *testing.T) {
	provider := newOperationSecrets()
	for _, key := range []string{"provider-v2-poison", "provider-v2-a", "provider-v2-b", "provider-v2-c"} {
		provider.values[key] = "value"
	}
	var mu sync.Mutex
	active := 0
	maxActive := 0
	deleted := make(map[string]bool)
	poisonAttempts := 0
	provider.deleteHook = func(_ context.Context, key string) error {
		mu.Lock()
		active++
		maxActive = max(maxActive, active)
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		active--
		if key == "provider-v2-poison" {
			poisonAttempts++
		}
		if key != "provider-v2-poison" || poisonAttempts > 1 {
			deleted[key] = true
		}
		mu.Unlock()
		if key == "provider-v2-poison" && poisonAttempts == 1 {
			return errors.New("classified delete failure")
		}
		return nil
	}

	svc, db := newProviderOperationTestService(t, provider)
	svc.providerOps.WakeCleanup()

	deadline := time.Now().Add(3 * time.Second)
	remaining := -1
	for time.Now().Before(deadline) {
		mu.Lock()
		allDeleted := len(deleted) == 4
		mu.Unlock()
		if err := db.QueryRow(`SELECT count(*) FROM provider_secret_cleanup`).Scan(&remaining); err != nil {
			t.Fatal(err)
		}
		if allDeleted && remaining == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	gotDeleted := len(deleted)
	gotMax := maxActive
	mu.Unlock()
	if gotDeleted != 4 {
		t.Fatalf("deleted %d rows, want 4 after poison retry", gotDeleted)
	}
	if gotMax > 2 || gotMax < 1 {
		t.Fatalf("cleanup concurrency = %d, want 1..2", gotMax)
	}

	if poisonAttempts != 2 {
		t.Fatalf("poison delete attempts = %d, want 2", poisonAttempts)
	}
	if remaining != 0 {
		t.Fatalf("cleanup rows remaining = %d, want 0", remaining)
	}
	if _, err := provider.Get(t.Context(), "provider-v2-poison"); err != nil && !errors.Is(err, secrets.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestProviderCleanupCandidateRowsPreservePrimaryAndCloseErrors(t *testing.T) {
	primaryErr := errors.New("candidate row primary failure")
	closeErr := errors.New("candidate row close failure")
	tests := map[string]*providerCleanupRowsStub{
		"scan": {
			next:     true,
			scanErr:  primaryErr,
			closeErr: closeErr,
		},
		"iteration": {
			iterateErr: primaryErr,
			closeErr:   closeErr,
		},
	}
	for name, rows := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := collectProviderCleanupCandidates(rows)
			if !errors.Is(err, primaryErr) {
				t.Fatalf("candidate row error = %v, want primary failure", err)
			}
			if !errors.Is(err, closeErr) {
				t.Fatalf("candidate row error = %v, want close failure", err)
			}
			if !rows.closed {
				t.Fatal("candidate rows were not closed")
			}
		})
	}
}

func TestProviderCleanupDispatchReturnsQueryFailure(t *testing.T) {
	db := openProviderOperationTestDB(t)
	manager := newProviderOperationManager(&EngineContext{DB: db})
	manager.ctx = t.Context()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	err := manager.dispatchCleanup()
	if err == nil {
		t.Fatal("dispatch cleanup succeeded with a closed database")
	}
	if !strings.Contains(err.Error(), "query provider cleanup candidates") {
		t.Fatalf("dispatch cleanup error = %q, want candidate-query classification", err)
	}
}

func TestProviderCleanupErrorReporterSuppressesEquivalentFailures(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	var logs []string
	reporter := providerCleanupErrorReporter{
		now: func() time.Time { return now },
		logf: func(format string, args ...any) {
			logs = append(logs, fmt.Sprintf(format, args...))
		},
	}
	failure := errors.New("candidate query unavailable")

	reporter.report(failure)
	now = now.Add(59 * time.Second)
	reporter.report(errors.New(failure.Error()))
	if len(logs) != 1 {
		t.Fatalf("logs inside suppression window = %v, want one entry", logs)
	}

	now = now.Add(2 * time.Second)
	reporter.report(errors.New(failure.Error()))
	reporter.report(nil)
	reporter.report(errors.New(failure.Error()))
	if len(logs) != 3 {
		t.Fatalf("logs after interval and reset = %v, want three entries", logs)
	}
	for _, got := range logs {
		if want := "provider cleanup: dispatch: candidate query unavailable"; got != want {
			t.Fatalf("cleanup diagnostic = %q, want %q", got, want)
		}
	}
}

func TestProviderOperationStopWaitsForCleanupWorker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const secretName = "provider-v2-stop-cleanup"
		provider := newOperationSecrets()
		provider.values[secretName] = "secret"
		started := make(chan struct{})
		release := make(chan struct{})
		releaseWork := sync.OnceFunc(func() { close(release) })
		defer releaseWork()
		provider.deleteHook = func(context.Context, string) error {
			close(started)
			<-release
			return nil
		}
		svc, _ := newProviderOperationTestService(t, provider)
		svc.providerOps.WakeCleanup()
		waitProviderOperationValue(t, started, "provider cleanup worker")

		stopDone := make(chan struct{})
		go func() {
			svc.providerOps.Stop()
			close(stopDone)
		}()
		assertProviderOperationStopBlocked(t, stopDone)
		releaseWork()
		waitProviderOperationValue(t, stopDone, "provider operation stop")
	})
}

type providerCleanupRowsStub struct {
	next       bool
	scanErr    error
	iterateErr error
	closeErr   error
	closed     bool
}

func (r *providerCleanupRowsStub) Next() bool {
	next := r.next
	r.next = false
	return next
}

func (r *providerCleanupRowsStub) Scan(...any) error {
	return r.scanErr
}

func (r *providerCleanupRowsStub) Err() error {
	return r.iterateErr
}

func (r *providerCleanupRowsStub) Close() error {
	r.closed = true
	return r.closeErr
}
