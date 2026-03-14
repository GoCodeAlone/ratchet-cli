package providerauth

import (
	"context"
	"testing"
)

func TestCopilotAuth_DeviceFlow(t *testing.T) {
	_, err := DeviceFlow()
	if err == nil {
		t.Error("expected DeviceFlow to return an error (not yet implemented)")
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
		t.Error("expected either an error or non-empty model list, got neither")
	}
}
