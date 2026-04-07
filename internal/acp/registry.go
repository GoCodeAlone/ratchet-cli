package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// DefaultRegistryURL is the ACP agent registry endpoint.
var DefaultRegistryURL = "https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json"

// RegistryAgent represents an agent entry from the ACP registry.
type RegistryAgent struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Version     string            `json:"version"`
	Homepage    string            `json:"homepage,omitempty"`
	Install     *InstallInfo      `json:"install,omitempty"`
	Auth        []AuthMethodEntry `json:"auth,omitempty"`
}

// InstallInfo describes how to install an ACP agent.
type InstallInfo struct {
	Command string   `json:"command,omitempty"` // e.g. "npm install -g @agent/cli"
	Binary  string   `json:"binary,omitempty"`  // binary name after install
	Args    []string `json:"args,omitempty"`    // args to run as ACP agent
}

// AuthMethodEntry describes an authentication method.
type AuthMethodEntry struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// Registry holds the fetched agent list with a cache.
type Registry struct {
	URL       string // override for testing; defaults to DefaultRegistryURL
	mu        sync.Mutex
	agents    []RegistryAgent
	fetchedAt time.Time
	cacheTTL  time.Duration
}

// NewRegistry creates a registry client with the given cache TTL.
func NewRegistry(cacheTTL time.Duration) *Registry {
	return &Registry{cacheTTL: cacheTTL}
}

// Agents returns all registered ACP agents, fetching from the remote registry if the cache is stale.
func (r *Registry) Agents(ctx context.Context) ([]RegistryAgent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.agents != nil && time.Since(r.fetchedAt) < r.cacheTTL {
		return r.agents, nil
	}

	url := r.URL
	if url == "" {
		url = DefaultRegistryURL
	}
	agents, err := fetchRegistry(ctx, url)
	if err != nil {
		// Return stale cache if available.
		if r.agents != nil {
			return r.agents, nil
		}
		return nil, err
	}

	r.agents = agents
	r.fetchedAt = time.Now()
	return agents, nil
}

// fetchRegistry downloads and parses the ACP registry.
func fetchRegistry(ctx context.Context, url string) ([]RegistryAgent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	// The registry may be an object with an "agents" array or a flat array.
	var wrapper struct {
		Agents []RegistryAgent `json:"agents"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil && wrapper.Agents != nil {
		return wrapper.Agents, nil
	}

	var agents []RegistryAgent
	if err := json.Unmarshal(body, &agents); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	return agents, nil
}
