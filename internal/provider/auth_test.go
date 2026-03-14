package providerauth

import (
	"testing"
)

func TestCopilotAuth_DeviceFlow(t *testing.T) {
	_, err := DeviceFlow()
	if err == nil {
		t.Error("expected DeviceFlow to return an error (not yet implemented)")
	}
}

func TestCopilotAuth_FallbackModels(t *testing.T) {
	models := copilotFallbackModels()
	if len(models) == 0 {
		t.Error("expected non-empty fallback models list")
	}
	for _, m := range models {
		if m.ID == "" {
			t.Error("expected non-empty ID in fallback model")
		}
		if m.Name == "" {
			t.Error("expected non-empty Name in fallback model")
		}
	}
}
