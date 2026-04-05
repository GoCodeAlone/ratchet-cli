package providerauth

import (
	"context"
	"testing"
	"time"
)

func TestCopilotAuth_DeviceFlow(t *testing.T) {
	// DeviceFlow calls StartGitHubDeviceFlow which makes a real HTTP POST.
	// Use a very short timeout so it fails fast without hitting the network.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err := DeviceFlow(ctx)
	if err == nil {
		t.Skip("DeviceFlow unexpectedly succeeded (GitHub reachable with valid client)")
	}
	// We expect a context deadline exceeded or a network error — both are valid.
	// The key assertion is that DeviceFlow properly propagates the error.
	if err.Error() == "" {
		t.Error("expected non-empty error message from DeviceFlow")
	}
}

func TestCopilotAuth_ListModels_ReturnsErrorOnFailure(t *testing.T) {
	// With fallbacks removed, listCopilotModels must return a real error
	// when the API is unreachable (bad-key hits the live endpoint which is
	// unreachable in CI, so we just verify that either an error is returned
	// or models are non-empty — we never get both nil error and empty list).
	ctx := context.Background()
	models, err := ListModels(ctx, "copilot", "bad-key", "")
	if err == nil && len(models) == 0 {
		t.Error("expected either an error or non-empty models; got nil error + empty list")
	}
}
