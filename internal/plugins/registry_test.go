package plugins

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRegistry_LoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	r, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom empty: %v", err)
	}
	if len(r.Plugins) != 0 {
		t.Fatalf("expected empty registry, got %d plugins", len(r.Plugins))
	}

	entry := RegistryEntry{
		Source:      "github:GoCodeAlone/my-plugin",
		Version:     "1.0.0",
		InstalledAt: time.Now().Truncate(time.Second),
		Path:        "/home/user/.ratchet/plugins/my-plugin",
	}
	if err := r.Add("my-plugin", entry); err != nil {
		t.Fatalf("Add: %v", err)
	}

	r2, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom after save: %v", err)
	}
	got, ok := r2.Get("my-plugin")
	if !ok {
		t.Fatal("expected my-plugin in reloaded registry")
	}
	if got.Source != entry.Source {
		t.Errorf("source = %q, want %q", got.Source, entry.Source)
	}
	if got.Version != entry.Version {
		t.Errorf("version = %q, want %q", got.Version, entry.Version)
	}
}

func TestRegistry_AddRemove(t *testing.T) {
	dir := t.TempDir()
	r, err := LoadFrom(filepath.Join(dir, "registry.json"))
	if err != nil {
		t.Fatal(err)
	}

	if err := r.Add("plugin-a", RegistryEntry{Source: "local:/tmp/a", Version: "0.1.0"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Add("plugin-b", RegistryEntry{Source: "local:/tmp/b", Version: "0.2.0"}); err != nil {
		t.Fatal(err)
	}
	if len(r.Plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(r.Plugins))
	}

	if err := r.Remove("plugin-a"); err != nil {
		t.Fatal(err)
	}
	if len(r.Plugins) != 1 {
		t.Fatalf("expected 1 plugin after remove, got %d", len(r.Plugins))
	}
	if _, ok := r.Get("plugin-a"); ok {
		t.Error("plugin-a should not exist after remove")
	}
}

func TestRegistry_GetMissing(t *testing.T) {
	dir := t.TempDir()
	r, err := LoadFrom(filepath.Join(dir, "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("expected false for missing plugin")
	}
}

func TestRegistry_RemoveMissing(t *testing.T) {
	dir := t.TempDir()
	r, err := LoadFrom(filepath.Join(dir, "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Remove of non-existent entry should not error
	if err := r.Remove("ghost"); err != nil {
		t.Errorf("unexpected error removing missing plugin: %v", err)
	}
}
