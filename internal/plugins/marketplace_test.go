package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMarketplaceRegistryAddUpdateRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marketplaces.json")

	store, err := LoadMarketplaceRegistry(path)
	if err != nil {
		t.Fatalf("LoadMarketplaceRegistry: %v", err)
	}
	if err := store.Add(MarketplaceSource{Name: "local", Source: "owner/repo", AutoUpdate: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	reloaded, err := LoadMarketplaceRegistry(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, ok := reloaded.Get("local")
	if !ok {
		t.Fatal("marketplace not persisted")
	}
	if got.Source != "owner/repo" || !got.AutoUpdate {
		t.Fatalf("source = %#v, want owner/repo with auto update", got)
	}

	if err := reloaded.Remove("local"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	afterRemove, err := LoadMarketplaceRegistry(path)
	if err != nil {
		t.Fatalf("reload after remove: %v", err)
	}
	if _, ok := afterRemove.Get("local"); ok {
		t.Fatal("marketplace still present after remove")
	}
}

func TestMarketplaceRegistryRejectsInvalidSource(t *testing.T) {
	store, err := LoadMarketplaceRegistry(filepath.Join(t.TempDir(), "marketplaces.json"))
	if err != nil {
		t.Fatalf("LoadMarketplaceRegistry: %v", err)
	}
	if err := store.Add(MarketplaceSource{Name: " ", Source: "owner/repo"}); err == nil {
		t.Fatal("expected blank marketplace name to fail")
	}
	if err := store.Add(MarketplaceSource{Name: "empty", Source: ""}); err == nil {
		t.Fatal("expected blank marketplace source to fail")
	}
}

func TestMarketplaceCatalogLoadsAndValidatesEntries(t *testing.T) {
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "marketplace.json")
	if err := os.WriteFile(catalogPath, []byte(`{
  "plugins": [
    {
      "name": "agent-tools",
      "description": "Agent helpers",
      "version": "1.2.3",
      "source": "local:/plugins/agent-tools",
      "sha256": "abc123",
      "relevance": "agent",
      "autoUpdate": true
    }
  ]
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadMarketplaceCatalog(catalogPath)
	if err != nil {
		t.Fatalf("LoadMarketplaceCatalog: %v", err)
	}
	entry, ok := catalog.Get("agent-tools")
	if !ok {
		t.Fatal("catalog entry not indexed by name")
	}
	if entry.Version != "1.2.3" || entry.Source != "local:/plugins/agent-tools" || !entry.AutoUpdate {
		t.Fatalf("entry = %#v", entry)
	}
}

func TestMarketplaceCatalogRejectsMalformedEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marketplace.json")
	if err := os.WriteFile(path, []byte(`{"plugins":[{"name":"bad","version":"1.0.0"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMarketplaceCatalog(path); err == nil {
		t.Fatal("expected missing source to fail")
	}
}

func TestLoaderSkipsDisabledRegistryEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pluginDir := filepath.Join(home, ".ratchet", "plugins", "disabled-plugin")
	manifest := `{"name":"disabled-plugin","version":"1.0.0","description":"test","author":{"name":"test"},"capabilities":{"skills":"./skills/"}}`
	writeJSON(t, filepath.Join(pluginDir, ".ratchet-plugin", "plugin.json"), manifest)
	if err := os.MkdirAll(filepath.Join(pluginDir, "skills", "hello"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "skills", "hello", "SKILL.md"), []byte("# Hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	reg, err := Load()
	if err != nil {
		t.Fatalf("Load registry: %v", err)
	}
	if err := reg.Add("disabled-plugin", RegistryEntry{
		Source:  "local:/disabled-plugin",
		Version: "1.0.0",
		Path:    pluginDir,
	}); err != nil {
		t.Fatalf("Add registry entry: %v", err)
	}
	if err := reg.SetEnabled("disabled-plugin", false); err != nil {
		t.Fatalf("disable registry entry: %v", err)
	}

	loaded, err := NewLoader(filepath.Join(home, ".ratchet", "plugins")).LoadSkills()
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("skills = %d, want disabled plugin skipped", len(loaded))
	}
}
