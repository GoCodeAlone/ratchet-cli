package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func setupPlugin(t *testing.T, name, version string) string {
	t.Helper()
	dir := t.TempDir()
	manifest := `{"name":"` + name + `","version":"` + version + `","description":"test","author":{"name":"test"},"capabilities":{"skills":"./skills/"}}`
	writeJSON(t, filepath.Join(dir, ".ratchet-plugin", "plugin.json"), manifest)
	// Add a sample skill file so there's something to copy
	if err := os.MkdirAll(filepath.Join(dir, "skills", "hello"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "hello", "SKILL.md"), []byte("# Hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInstallFromLocal(t *testing.T) {
	// Override plugins dir and registry for this test
	src := setupPlugin(t, "my-plugin", "1.0.0")

	pluginsBase := t.TempDir()
	t.Setenv("HOME", pluginsBase) // affects pluginsDir() and registryPath()

	if err := InstallFromLocal(src); err != nil {
		t.Fatalf("InstallFromLocal: %v", err)
	}

	// Verify plugin directory was created
	destDir := filepath.Join(pluginsBase, ".ratchet", "plugins", "my-plugin")
	if _, err := os.Stat(destDir); err != nil {
		t.Errorf("plugin dir not created: %v", err)
	}

	// Verify manifest is present at destination
	if _, err := LoadManifest(destDir); err != nil {
		t.Errorf("manifest not found at dest: %v", err)
	}

	// Verify registry entry
	reg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := reg.Get("my-plugin")
	if !ok {
		t.Fatal("expected registry entry for my-plugin")
	}
	if entry.Version != "1.0.0" {
		t.Errorf("version = %q, want 1.0.0", entry.Version)
	}
	if entry.Path != destDir {
		t.Errorf("path = %q, want %q", entry.Path, destDir)
	}
}

func TestUninstall(t *testing.T) {
	src := setupPlugin(t, "bye-plugin", "0.1.0")

	pluginsBase := t.TempDir()
	t.Setenv("HOME", pluginsBase)

	if err := InstallFromLocal(src); err != nil {
		t.Fatalf("install: %v", err)
	}

	if err := Uninstall("bye-plugin"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	destDir := filepath.Join(pluginsBase, ".ratchet", "plugins", "bye-plugin")
	if _, err := os.Stat(destDir); !os.IsNotExist(err) {
		t.Error("plugin dir should be removed after uninstall")
	}

	reg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Get("bye-plugin"); ok {
		t.Error("registry entry should be removed after uninstall")
	}
}

func TestInstallFromLocal_InvalidManifest(t *testing.T) {
	dir := t.TempDir()
	// No manifest file at all

	pluginsBase := t.TempDir()
	t.Setenv("HOME", pluginsBase)

	err := InstallFromLocal(dir)
	if err == nil {
		t.Error("expected error for directory with no manifest")
	}
}

func TestUninstall_NonExistent(t *testing.T) {
	pluginsBase := t.TempDir()
	t.Setenv("HOME", pluginsBase)

	// Should not error if plugin doesn't exist
	if err := Uninstall("ghost"); err != nil {
		t.Errorf("unexpected error uninstalling non-existent plugin: %v", err)
	}
}
