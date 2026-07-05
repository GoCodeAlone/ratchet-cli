package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
	if strings.ContainsAny(source.Name, "/|") {
		return fmt.Errorf("marketplace name %q cannot contain '/' or '|'", source.Name)
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
		if removeErr := os.Remove(r.filePath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			_ = os.Remove(tmp)
			return fmt.Errorf("remove old marketplace registry: %w", removeErr)
		}
		if retryErr := os.Rename(tmp, r.filePath); retryErr != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("replace marketplace registry: %w", retryErr)
		}
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
	return validateMarketplaceCatalog(catalog)
}

func LoadMarketplaceCatalogFromSource(ctx context.Context, source string) (*MarketplaceCatalog, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, fmt.Errorf("marketplace source is required")
	}
	if isHTTPSource(source) {
		return loadMarketplaceCatalogURL(ctx, source)
	}
	source = strings.TrimPrefix(source, "file://")
	if isGitHubShorthand(source) {
		catalog, err := loadMarketplaceCatalogURL(ctx, "https://raw.githubusercontent.com/"+source+"/HEAD/.ratchet-plugin/marketplace.json")
		if err == nil {
			return catalog, nil
		}
		return loadMarketplaceCatalogURL(ctx, "https://raw.githubusercontent.com/"+source+"/HEAD/.claude-plugin/marketplace.json")
	}
	path := expandPluginPath(source)
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat marketplace source: %w", err)
	}
	if info.IsDir() {
		for _, rel := range []string{
			filepath.Join(".ratchet-plugin", "marketplace.json"),
			filepath.Join(".claude-plugin", "marketplace.json"),
			"marketplace.json",
		} {
			candidate := filepath.Join(path, rel)
			if _, err := os.Stat(candidate); err == nil {
				return LoadMarketplaceCatalog(candidate)
			}
		}
		return nil, fmt.Errorf("marketplace source %s has no marketplace.json", source)
	}
	return LoadMarketplaceCatalog(path)
}

func InstallFromMarketplace(ctx context.Context, pluginRef string) error {
	return installFromMarketplace(ctx, pluginRef, true)
}

func installFromMarketplace(ctx context.Context, pluginRef string, enabled bool) error {
	pluginName, marketplaceName, ok := strings.Cut(pluginRef, "@")
	if !ok || strings.TrimSpace(pluginName) == "" || strings.TrimSpace(marketplaceName) == "" {
		return fmt.Errorf("invalid marketplace plugin ref %q: expected name@marketplace", pluginRef)
	}
	registry, err := LoadDefaultMarketplaceRegistry()
	if err != nil {
		return err
	}
	marketplace, ok := registry.Get(marketplaceName)
	if !ok {
		return fmt.Errorf("marketplace %q not configured", marketplaceName)
	}
	catalog, err := LoadMarketplaceCatalogFromSource(ctx, marketplace.Source)
	if err != nil {
		return err
	}
	entry, ok := catalog.Get(pluginName)
	if !ok {
		return fmt.Errorf("plugin %q not found in marketplace %q", pluginName, marketplaceName)
	}
	if err := installFromSource(ctx, entry.Source); err != nil {
		return err
	}
	installed, err := Load()
	if err != nil {
		return err
	}
	installedEntry, ok := installed.Get(entry.Name)
	if !ok {
		return fmt.Errorf("installed plugin %q missing from registry", entry.Name)
	}
	installedEntry.Source = marketplaceRegistrySource(marketplaceName, entry)
	installedEntry.Version = entry.Version
	installedEntry.Enabled = enabled
	return installed.Put(entry.Name, installedEntry)
}

func UpdateInstalledPlugin(ctx context.Context, name string) error {
	reg, err := Load()
	if err != nil {
		return err
	}
	entry, ok := reg.Get(name)
	if !ok {
		return fmt.Errorf("plugin %q not installed", name)
	}
	if strings.HasPrefix(entry.Source, "marketplace:") {
		marketplaceName, pluginName, ok := parseMarketplaceRegistrySource(entry.Source)
		if !ok {
			return fmt.Errorf("invalid marketplace source for %q", name)
		}
		return installFromMarketplace(ctx, pluginName+"@"+marketplaceName, entry.Enabled)
	}
	if err := installFromSource(ctx, entry.Source); err != nil {
		return err
	}
	updated, err := Load()
	if err != nil {
		return err
	}
	updatedEntry, ok := updated.Get(name)
	if !ok {
		return fmt.Errorf("updated plugin %q missing from registry", name)
	}
	updatedEntry.Enabled = entry.Enabled
	return updated.Put(name, updatedEntry)
}

func UpdateAllInstalledPlugins(ctx context.Context) error {
	reg, err := Load()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(reg.Plugins))
	for name := range reg.Plugins {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := UpdateInstalledPlugin(ctx, name); err != nil {
			return err
		}
	}
	return nil
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

func installFromSource(ctx context.Context, source string) error {
	source = strings.TrimSpace(source)
	switch {
	case strings.HasPrefix(source, "local:"):
		return InstallFromLocal(expandPluginPath(strings.TrimPrefix(source, "local:")))
	case strings.HasPrefix(source, "github:"):
		return InstallFromGitHub(ctx, strings.TrimPrefix(source, "github:"))
	case isLocalPluginSource(source):
		return InstallFromLocal(expandPluginPath(source))
	default:
		return InstallFromGitHub(ctx, source)
	}
}

func marketplaceRegistrySource(marketplaceName string, entry MarketplaceEntry) string {
	return "marketplace:" + marketplaceName + "/" + entry.Name + "|" + entry.Source
}

func parseMarketplaceRegistrySource(source string) (marketplaceName, pluginName string, ok bool) {
	source = strings.TrimPrefix(source, "marketplace:")
	left, _, _ := strings.Cut(source, "|")
	marketplaceName, pluginName, ok = strings.Cut(left, "/")
	return marketplaceName, pluginName, ok && marketplaceName != "" && pluginName != ""
}

func isHTTPSource(source string) bool {
	return strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://")
}

func isGitHubShorthand(source string) bool {
	parts := strings.Split(source, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" &&
		!strings.ContainsAny(source, `\:.~@`)
}

func isLocalPluginSource(source string) bool {
	if source == "" {
		return false
	}
	if strings.HasPrefix(source, ".") || strings.HasPrefix(source, "~") || filepath.IsAbs(source) {
		return true
	}
	if len(source) >= 2 && source[1] == ':' {
		return true
	}
	_, err := os.Stat(expandPluginPath(source))
	return err == nil
}

func expandPluginPath(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		return filepath.Join(home, path[2:])
	}
	return path
}

func loadMarketplaceCatalogURL(ctx context.Context, source string) (*MarketplaceCatalog, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, fmt.Errorf("build marketplace catalog request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch marketplace catalog: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch marketplace catalog: status %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read marketplace catalog response: %w", err)
	}
	var catalog MarketplaceCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("parse marketplace catalog: %w", err)
	}
	return validateMarketplaceCatalog(catalog)
}

func validateMarketplaceCatalog(catalog MarketplaceCatalog) (*MarketplaceCatalog, error) {
	catalog.byName = make(map[string]MarketplaceEntry, len(catalog.Plugins))
	for i, entry := range catalog.Plugins {
		entry.Name = strings.TrimSpace(entry.Name)
		entry.Source = strings.TrimSpace(entry.Source)
		entry.Version = strings.TrimSpace(entry.Version)
		if entry.Name == "" {
			return nil, fmt.Errorf("marketplace catalog plugin %d missing name", i)
		}
		if strings.ContainsAny(entry.Name, "/|") {
			return nil, fmt.Errorf("marketplace catalog plugin %q cannot contain '/' or '|'", entry.Name)
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
