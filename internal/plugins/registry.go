package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RegistryEntry records an installed plugin.
type RegistryEntry struct {
	Source      string    `json:"source"`       // "github:org/repo" or "local:/path"
	Version     string    `json:"version"`
	InstalledAt time.Time `json:"installed_at"`
	Path        string    `json:"path"`
}

// Registry tracks installed plugins in ~/.ratchet/plugins/registry.json.
type Registry struct {
	Plugins  map[string]RegistryEntry `json:"plugins"`
	filePath string
}

// registryPath returns the default registry file path.
func registryPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ratchet", "plugins", "registry.json")
}

// Load reads the registry from disk. Returns an empty registry if the file doesn't exist.
func Load() (*Registry, error) {
	return LoadFrom(registryPath())
}

// LoadFrom reads the registry from a specific path.
func LoadFrom(path string) (*Registry, error) {
	r := &Registry{
		Plugins:  make(map[string]RegistryEntry),
		filePath: path,
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return r, nil
		}
		return nil, fmt.Errorf("read registry: %w", err)
	}
	if err := json.Unmarshal(data, r); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	r.filePath = path
	if r.Plugins == nil {
		r.Plugins = make(map[string]RegistryEntry)
	}
	return r, nil
}

// Save writes the registry to disk.
func (r *Registry) Save() error {
	if err := os.MkdirAll(filepath.Dir(r.filePath), 0o755); err != nil {
		return fmt.Errorf("create registry dir: %w", err)
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	if err := os.WriteFile(r.filePath, data, 0o644); err != nil {
		return fmt.Errorf("write registry: %w", err)
	}
	return nil
}

// Add inserts or replaces a plugin entry and saves.
func (r *Registry) Add(name string, entry RegistryEntry) error {
	r.Plugins[name] = entry
	return r.Save()
}

// Remove deletes a plugin entry and saves. Returns nil if the entry doesn't exist.
func (r *Registry) Remove(name string) error {
	delete(r.Plugins, name)
	return r.Save()
}

// Get retrieves a plugin entry by name.
func (r *Registry) Get(name string) (RegistryEntry, bool) {
	entry, ok := r.Plugins[name]
	return entry, ok
}
