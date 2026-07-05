package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/plugins"
)

func TestExecutePluginMarketplaceAddListRemove(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	catalog := filepath.Join(home, "marketplace.json")
	if err := os.WriteFile(catalog, []byte(`{"plugins":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := executePlugin([]string{"marketplace", "add", "local", catalog, "--auto-update"}, &out); err != nil {
		t.Fatalf("marketplace add: %v", err)
	}
	out.Reset()
	if err := executePlugin([]string{"marketplace", "list"}, &out); err != nil {
		t.Fatalf("marketplace list: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "local") || !strings.Contains(got, catalog) || !strings.Contains(got, "auto") {
		t.Fatalf("marketplace list output = %q", got)
	}
	out.Reset()
	if err := executePlugin([]string{"marketplace", "remove", "local"}, &out); err != nil {
		t.Fatalf("marketplace remove: %v", err)
	}
	out.Reset()
	if err := executePlugin([]string{"marketplace", "list"}, &out); err != nil {
		t.Fatalf("marketplace list empty: %v", err)
	}
	if !strings.Contains(out.String(), "No marketplaces configured.") {
		t.Fatalf("empty list output = %q", out.String())
	}
}

func TestExecutePluginInstallFromMarketplace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	src := setupPluginFixture(t, "fixture-plugin", "1.2.3")
	catalog := filepath.Join(home, "marketplace.json")
	if err := os.WriteFile(catalog, []byte(`{"plugins":[{"name":"fixture-plugin","version":"1.2.3","source":"local:`+filepath.ToSlash(src)+`"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := executePlugin([]string{"marketplace", "add", "local", catalog}, &out); err != nil {
		t.Fatalf("marketplace add: %v", err)
	}
	out.Reset()
	if err := executePlugin([]string{"install", "fixture-plugin@local"}, &out); err != nil {
		t.Fatalf("install marketplace plugin: %v", err)
	}
	if !strings.Contains(out.String(), "Installed plugin: fixture-plugin@local") {
		t.Fatalf("install output = %q", out.String())
	}
	reg, err := plugins.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	entry, ok := reg.Get("fixture-plugin")
	if !ok {
		t.Fatal("fixture-plugin not registered")
	}
	if entry.Version != "1.2.3" || !entry.Enabled || !strings.Contains(entry.Source, "marketplace:local/fixture-plugin") {
		t.Fatalf("entry = %#v", entry)
	}
}

func TestExecutePluginEnableDisableAndUpdateLocal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	src := setupPluginFixture(t, "toggle-plugin", "1.0.0")

	var out bytes.Buffer
	if err := executePlugin([]string{"install", src}, &out); err != nil {
		t.Fatalf("install local: %v", err)
	}
	if err := executePlugin([]string{"disable", "toggle-plugin"}, &out); err != nil {
		t.Fatalf("disable: %v", err)
	}
	reg, err := plugins.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	entry, _ := reg.Get("toggle-plugin")
	if entry.Enabled {
		t.Fatal("plugin still enabled after disable")
	}
	if err := executePlugin([]string{"enable", "toggle-plugin"}, &out); err != nil {
		t.Fatalf("enable: %v", err)
	}
	reg, err = plugins.Load()
	if err != nil {
		t.Fatalf("reload registry: %v", err)
	}
	entry, _ = reg.Get("toggle-plugin")
	if !entry.Enabled {
		t.Fatal("plugin still disabled after enable")
	}

	src2 := setupPluginFixture(t, "toggle-plugin", "1.1.0")
	if err := os.RemoveAll(src); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(src2, src); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := executePlugin([]string{"update", "toggle-plugin"}, &out); err != nil {
		t.Fatalf("update: %v", err)
	}
	reg, err = plugins.Load()
	if err != nil {
		t.Fatalf("reload registry after update: %v", err)
	}
	entry, _ = reg.Get("toggle-plugin")
	if entry.Version != "1.1.0" {
		t.Fatalf("version after update = %q, want 1.1.0", entry.Version)
	}
}

func TestExecutePluginUpdatePreservesDisabledState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	src := setupPluginFixture(t, "disabled-update", "1.0.0")

	var out bytes.Buffer
	if err := executePlugin([]string{"install", src}, &out); err != nil {
		t.Fatalf("install local: %v", err)
	}
	if err := executePlugin([]string{"disable", "disabled-update"}, &out); err != nil {
		t.Fatalf("disable: %v", err)
	}
	src2 := setupPluginFixture(t, "disabled-update", "1.1.0")
	if err := os.RemoveAll(src); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(src2, src); err != nil {
		t.Fatal(err)
	}
	if err := executePlugin([]string{"update", "disabled-update"}, &out); err != nil {
		t.Fatalf("update: %v", err)
	}
	reg, err := plugins.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	entry, _ := reg.Get("disabled-update")
	if entry.Enabled {
		t.Fatal("disabled plugin became enabled after update")
	}
	if entry.Version != "1.1.0" {
		t.Fatalf("version after update = %q, want 1.1.0", entry.Version)
	}
}

func TestExecutePluginMarketplaceUpdatePreservesDisabledState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	src := setupPluginFixture(t, "market-disabled", "1.0.0")
	catalog := filepath.Join(home, "marketplace.json")
	writeMarketplaceCatalog(t, catalog, "market-disabled", "1.0.0", src)

	var out bytes.Buffer
	if err := executePlugin([]string{"marketplace", "add", "local", catalog}, &out); err != nil {
		t.Fatalf("marketplace add: %v", err)
	}
	if err := executePlugin([]string{"install", "market-disabled@local"}, &out); err != nil {
		t.Fatalf("install marketplace plugin: %v", err)
	}
	if err := executePlugin([]string{"disable", "market-disabled"}, &out); err != nil {
		t.Fatalf("disable: %v", err)
	}
	src2 := setupPluginFixture(t, "market-disabled", "1.1.0")
	if err := os.RemoveAll(src); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(src2, src); err != nil {
		t.Fatal(err)
	}
	writeMarketplaceCatalog(t, catalog, "market-disabled", "1.1.0", src)
	if err := executePlugin([]string{"update", "market-disabled"}, &out); err != nil {
		t.Fatalf("update marketplace plugin: %v", err)
	}

	reg, err := plugins.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	entry, _ := reg.Get("market-disabled")
	if entry.Enabled {
		t.Fatal("disabled marketplace plugin became enabled after update")
	}
	if entry.Version != "1.1.0" {
		t.Fatalf("version after update = %q, want 1.1.0", entry.Version)
	}
}

func setupPluginFixture(t *testing.T, name, version string) string {
	t.Helper()
	dir := t.TempDir()
	manifest := `{"name":"` + name + `","version":"` + version + `","description":"test","author":{"name":"test"},"capabilities":{"skills":"./skills/"}}`
	if err := os.MkdirAll(filepath.Join(dir, ".ratchet-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ratchet-plugin", "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "skills", "hello"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "hello", "SKILL.md"), []byte("# Hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeMarketplaceCatalog(t *testing.T, path, name, version, source string) {
	t.Helper()
	body := `{"plugins":[{"name":"` + name + `","version":"` + version + `","source":"local:` + filepath.ToSlash(source) + `"}]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
