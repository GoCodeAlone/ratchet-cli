package acp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRegistryCache(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		agents := []RegistryAgent{{ID: "test-agent", Name: "Test", Description: "A test agent"}}
		json.NewEncoder(w).Encode(agents)
	}))
	defer ts.Close()

	r := &Registry{URL: ts.URL, cacheTTL: 1 * time.Hour}

	// Manually populate cache.
	r.agents = []RegistryAgent{{ID: "cached", Name: "Cached"}}
	r.fetchedAt = time.Now()

	agents, err := r.Agents(context.Background())
	if err != nil {
		t.Fatalf("Agents: %v", err)
	}
	if len(agents) != 1 || agents[0].ID != "cached" {
		t.Error("expected cached result")
	}
}

func TestRegistryStaleCacheOnError(t *testing.T) {
	r := &Registry{
		URL:       "http://127.0.0.1:1", // unreachable port
		cacheTTL:  0,                     // always stale
		agents:    []RegistryAgent{{ID: "stale", Name: "Stale"}},
		fetchedAt: time.Now().Add(-24 * time.Hour),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	agents, err := r.Agents(ctx)
	if err != nil {
		t.Fatalf("expected stale cache, got error: %v", err)
	}
	if len(agents) != 1 || agents[0].ID != "stale" {
		t.Error("expected stale cached result")
	}
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry(24 * time.Hour)
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
	if r.cacheTTL != 24*time.Hour {
		t.Errorf("cacheTTL = %v, want 24h", r.cacheTTL)
	}
}

func TestRegistryAgentJSON(t *testing.T) {
	raw := `[{"id":"goose","name":"Goose","description":"AI agent by Block","version":"1.0.0","homepage":"https://goose.ai"}]`
	var agents []RegistryAgent
	if err := json.Unmarshal([]byte(raw), &agents); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].ID != "goose" {
		t.Errorf("ID = %q, want %q", agents[0].ID, "goose")
	}
}
