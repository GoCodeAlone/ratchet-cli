package daemon

import (
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
)

// modelLimitFor is a helper that replicates the per-model limit lookup in handleChat.
func modelLimitFor(model string, limits map[string]int) int {
	const defaultModelLimit = 200000
	if model != "" && limits != nil {
		if limit, ok := limits[model]; ok {
			return limit
		}
	}
	return defaultModelLimit
}

func TestCompression_PerModelLimit(t *testing.T) {
	cfg := config.DefaultConfig()
	limit := modelLimitFor("claude-opus-4-6", cfg.Context.ModelLimits)
	if limit != 1000000 {
		t.Errorf("expected 1000000 for claude-opus-4-6, got %d", limit)
	}
}

func TestCompression_FallbackLimit(t *testing.T) {
	cfg := config.DefaultConfig()
	limit := modelLimitFor("unknown-model-xyz", cfg.Context.ModelLimits)
	if limit != 200000 {
		t.Errorf("expected fallback 200000, got %d", limit)
	}
}

func TestCompression_EmptyModel(t *testing.T) {
	cfg := config.DefaultConfig()
	limit := modelLimitFor("", cfg.Context.ModelLimits)
	if limit != 200000 {
		t.Errorf("expected fallback 200000 for empty model, got %d", limit)
	}
}

func TestCompression_SonnetLimit(t *testing.T) {
	cfg := config.DefaultConfig()
	limit := modelLimitFor("claude-sonnet-4-6", cfg.Context.ModelLimits)
	if limit != 200000 {
		t.Errorf("expected 200000 for claude-sonnet-4-6, got %d", limit)
	}
}
