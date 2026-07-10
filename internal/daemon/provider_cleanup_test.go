package daemon

import (
	"context"
	"errors"
	"sync"
	"testing"
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
	for time.Now().Before(deadline) {
		mu.Lock()
		allDeleted := len(deleted) == 4
		mu.Unlock()
		if allDeleted {
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
	var remaining int
	if err := db.QueryRow(`SELECT count(*) FROM provider_secret_cleanup`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("cleanup rows remaining = %d, want 0", remaining)
	}
	if _, err := provider.Get(t.Context(), "provider-v2-poison"); err != nil && !errors.Is(err, secrets.ErrNotFound) {
		t.Fatal(err)
	}
}
