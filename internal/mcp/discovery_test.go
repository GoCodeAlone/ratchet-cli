package mcp

import (
	"errors"
	"testing"

	ratchetplugin "github.com/GoCodeAlone/workflow-plugin-agent/orchestrator"
)

func newTestDiscoverer(lookPath func(string) (string, error)) *Discoverer {
	reg := ratchetplugin.NewToolRegistry()
	d := NewDiscoverer(reg)
	d.lookPath = lookPath
	return d
}

func TestMCPDiscovery_GHFound(t *testing.T) {
	d := newTestDiscoverer(func(name string) (string, error) {
		if name == "gh" {
			return "/usr/bin/gh", nil
		}
		return "", errors.New("not found")
	})

	result := d.Discover()

	if _, ok := result.Registered["gh"]; !ok {
		t.Error("expected gh to be registered")
	}
	names := result.Registered["gh"]
	if len(names) != 3 {
		t.Errorf("expected 3 gh tools, got %d", len(names))
	}
	// docker and kubectl not in path → not registered
	if _, ok := result.Registered["docker"]; ok {
		t.Error("docker should not be registered")
	}
}

func TestMCPDiscovery_NoCLIs(t *testing.T) {
	d := newTestDiscoverer(func(name string) (string, error) {
		return "", errors.New("not found")
	})

	result := d.Discover()
	if len(result.Registered) != 0 {
		t.Errorf("expected no CLIs registered, got %v", result.Registered)
	}
}

func TestMCPDiscovery_CacheResults(t *testing.T) {
	calls := 0
	d := newTestDiscoverer(func(name string) (string, error) {
		calls++
		if name == "docker" {
			return "/usr/bin/docker", nil
		}
		return "", errors.New("not found")
	})

	r1 := d.Discover()
	r2 := d.Discover()

	// Second call should return the cached result without re-running LookPath.
	if r1 != r2 {
		t.Error("expected same pointer on second call (cache hit)")
	}
	// LookPath should have been called only during the first Discover().
	if calls != len(knownCLIs) {
		t.Errorf("expected %d lookPath calls, got %d", len(knownCLIs), calls)
	}
}

func TestMCPDiscovery_Enable(t *testing.T) {
	d := newTestDiscoverer(func(name string) (string, error) {
		if name == "gh" {
			return "/usr/bin/gh", nil
		}
		return "", errors.New("not found")
	})

	if err := d.Enable("gh"); err != nil {
		t.Fatalf("Enable gh: %v", err)
	}
	if err := d.Enable("unknown"); err == nil {
		t.Error("expected error for unknown CLI")
	}
}

func TestMCPDiscovery_Disable(t *testing.T) {
	d := newTestDiscoverer(func(name string) (string, error) {
		if name == "docker" {
			return "/usr/bin/docker", nil
		}
		return "", errors.New("not found")
	})

	result := d.Discover()
	if _, ok := result.Registered["docker"]; !ok {
		t.Fatal("expected docker registered before disable")
	}

	d.Disable("docker")
	if _, ok := result.Registered["docker"]; ok {
		t.Error("docker should be absent after disable")
	}
}
