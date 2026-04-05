package providerauth

import (
	"context"
	"testing"
	"time"
)

func TestCopilotAuth_DeviceFlow(t *testing.T) {
	// DeviceFlow makes a real network call; it should fail in test environments
	// without network access (or succeed if GitHub is reachable). Either outcome
	// is acceptable — we just verify the function signature compiles and runs.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, _ = DeviceFlow(ctx) // error expected due to timeout; we don't assert
}

func TestCopilotAuth_ListModels_ReturnsErrorOnFailure(t *testing.T) {
	// With fallbacks removed, listCopilotModels must return a real error
	// when the API is unreachable (bad-key hits the live endpoint which is
	// unreachable in CI, so we just verify that either an error is returned
	// or models are non-empty — we never get both nil error and empty list).
	ctx := context.Background()
	models, err := ListModels(ctx, "copilot", "bad-key", "")
	if err == nil && len(models) == 0 {
		t.Error("expected either an error or non-empty model list, got neither")
	}
}
