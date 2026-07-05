package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MarketplaceSource records a configured plugin marketplace catalog source.
type MarketplaceSource struct {
	Name       string `json:"name"`
	Source     string `json:"source"`
	AutoUpdate bool   `json:"autoUpdate,omitempty"`
}

// MarketplaceRegistry stores configured marketplace sources.
type MarketplaceRegistry struct {
	Marketplaces map[string]MarketplaceSource `json:"marketplaces"`
	filePath     string
}

// MarketplaceEntry describes a plugin entry published by a marketplace catalog.
type MarketplaceEntry struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
	Source      string `json:"source"`
	SHA256      string `json:"sha256,omitempty"`
	Relevance   string `json:"relevance,omitempty"`
	AutoUpdate  bool   `json:"autoUpdate,omitempty"`
}

// MarketplaceCatalog is the on-disk marketplace.json shape.
type MarketplaceCatalog struct {
	Plugins []MarketplaceEntry `json:"plugins"`
	byName  map[string]MarketplaceEntry
}

func marketplaceRegistryPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ratchet", "plugins", "marketplaces.json")
}

// LoadDefaultMarketplaceRegistry reads the user's marketplace registry.
func LoadDefaultMarketplaceRegistry() (*MarketplaceRegistry, error) {
	return LoadMarketplaceRegistry(marketplaceRegistryPath())
}

// LoadMarketplaceRegistry reads a marketplace registry from path.
func LoadMarketplaceRegistry(path string) (*MarketplaceRegistry, error) {
	r := &MarketplaceRegistry{
		Marketplaces: make(map[string]MarketplaceSource),
		filePath:     path,
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return r, nil
		}
		return nil, fmt.Errorf("read marketplace registry: %w", err)
	}
	if err := json.Unmarshal(data, r); err != nil {
		return nil, fmt.Errorf("parse marketplace registry: %w", err)
	}
	r.filePath = path
	if r.Marketplaces == nil {
		r.Marketplaces = make(map[string]MarketplaceSource)
	}
	return r, nil
}

func (r *MarketplaceRegistry) Add(source MarketplaceSource) error {
	source.Name = strings.TrimSpace(source.Name)
	source.Source = strings.TrimSpace(source.Source)
	if source.Name == "" {
		return fmt.Errorf("marketplace name is required")
	}
	if source.Source == "" {
		return fmt.Errorf("marketplace source is required")
	}
	r.Marketplaces[source.Name] = source
	return r.Save()
}

func (r *MarketplaceRegistry) Remove(name string) error {
	delete(r.Marketplaces, strings.TrimSpace(name))
	return r.Save()
}

func (r *MarketplaceRegistry) Get(name string) (MarketplaceSource, bool) {
	source, ok := r.Marketplaces[strings.TrimSpace(name)]
	return source, ok
}

func (r *MarketplaceRegistry) List() []MarketplaceSource {
	out := make([]MarketplaceSource, 0, len(r.Marketplaces))
	for _, source := range r.Marketplaces {
		out = append(out, source)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *MarketplaceRegistry) Save() error {
	if err := os.MkdirAll(filepath.Dir(r.filePath), 0o755); err != nil {
		return fmt.Errorf("create marketplace registry dir: %w", err)
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal marketplace registry: %w", err)
	}
	tmp := r.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write marketplace registry temp file: %w", err)
	}
	if err := os.Rename(tmp, r.filePath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace marketplace registry: %w", err)
	}
	return nil
}

// LoadMarketplaceCatalog loads a local marketplace catalog JSON file.
func LoadMarketplaceCatalog(path string) (*MarketplaceCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read marketplace catalog: %w", err)
	}
	var catalog MarketplaceCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("parse marketplace catalog: %w", err)
	}
	catalog.byName = make(map[string]MarketplaceEntry, len(catalog.Plugins))
	for i, entry := range catalog.Plugins {
		entry.Name = strings.TrimSpace(entry.Name)
		entry.Source = strings.TrimSpace(entry.Source)
		entry.Version = strings.TrimSpace(entry.Version)
		if entry.Name == "" {
			return nil, fmt.Errorf("marketplace catalog plugin %d missing name", i)
		}
		if entry.Source == "" {
			return nil, fmt.Errorf("marketplace catalog plugin %q missing source", entry.Name)
		}
		if entry.Version == "" {
			return nil, fmt.Errorf("marketplace catalog plugin %q missing version", entry.Name)
		}
		catalog.Plugins[i] = entry
		catalog.byName[entry.Name] = entry
	}
	return &catalog, nil
}

func (c *MarketplaceCatalog) Get(name string) (MarketplaceEntry, bool) {
	if c.byName == nil {
		c.byName = make(map[string]MarketplaceEntry, len(c.Plugins))
		for _, entry := range c.Plugins {
			c.byName[entry.Name] = entry
		}
	}
	entry, ok := c.byName[strings.TrimSpace(name)]
	return entry, ok
}
