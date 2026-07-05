package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEngineReloadPluginsRefreshesRuntimeCapabilities(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pluginDir := filepath.Join(home, ".ratchet", "plugins", "runtime-plugin")
	writePluginFile(t, filepath.Join(pluginDir, ".ratchet-plugin", "plugin.json"), `{"name":"runtime-plugin","version":"1.0.0","description":"test","author":{"name":"test"},"capabilities":{"skills":"skills","hooks":"hooks.yaml"}}`)
	writePluginFile(t, filepath.Join(pluginDir, "skills", "hello", "SKILL.md"), "# Hello\nRuntime skill.")
	writePluginFile(t, filepath.Join(pluginDir, "hooks.yaml"), "hooks:\n  user-prompt-submit:\n    - command: \"true\"\n")

	engine := newTestEngine(t)
	summary, err := engine.ReloadPlugins(context.Background())
	if err != nil {
		t.Fatalf("ReloadPlugins: %v", err)
	}
	if summary.Skills != 1 || summary.Hooks == 0 {
		t.Fatalf("summary = %#v", summary)
	}
	if len(engine.PluginSkills) != 1 {
		t.Fatalf("PluginSkills = %d, want 1", len(engine.PluginSkills))
	}
	got := engine.PluginSkills[0]
	if got.Name != "hello" || got.PluginName != "runtime-plugin" || got.Source != "plugin" {
		t.Fatalf("plugin skill metadata = %#v", got)
	}
	if len(engine.Hooks.Hooks["user-prompt-submit"]) != 1 {
		t.Fatalf("expected plugin user-prompt-submit hook, got %#v", engine.Hooks.Hooks)
	}
}

func writePluginFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
