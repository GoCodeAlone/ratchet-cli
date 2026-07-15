package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/hooks"
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

func TestPluginReloadManagedPolicyCannotBeBypassedByOrdering(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workDir := t.TempDir()
	markers := managedHookMarkers(t)
	event := hooks.SessionStart
	writeUserHook(t, home, event, managedHookMarker(markers.user))
	writeProjectHook(t, workDir, event, managedHookMarker(markers.project))
	writeRuntimePluginHook(t, home, event, managedHookMarker(markers.plugin))

	managedPath := filepath.Join(t.TempDir(), "managed-hooks.yaml")
	writePluginFile(t, managedPath, "mode: managed-only\n")
	policy := managedHookPolicy(hooks.ManagedModeOnly, event, managedHookMarker(markers.managed), managedPath)
	engine := newTestEngine(t)
	engine.managedHooks = engineManagedHooksRuntime{
		policyPath: managedPath,
		audit:      hooks.NewHookAudit(managedHookAuditPath(t)),
		loadPolicy: func(options hooks.LoadOptions) (*hooks.ManagedPolicy, error) {
			if options.ManagedPath != managedPath {
				return nil, fmt.Errorf("managed path = %q, want %q", options.ManagedPath, managedPath)
			}
			return policy, nil
		},
	}

	for attempt := range 2 {
		summary, err := engine.ReloadPlugins(t.Context())
		if err != nil {
			t.Fatalf("ReloadPlugins attempt %d: %v", attempt+1, err)
		}
		if summary.Hooks != 3 {
			t.Fatalf("ReloadPlugins attempt %d hooks = %d, want user, plugin, and managed", attempt+1, summary.Hooks)
		}
		trustReloadedPluginAndProjectHooks(t, engine, workDir, event)
		removeManagedHookMarkers(t, markers)
		if err := engine.RunHooks(t.Context(), event, map[string]string{"working_dir": workDir}); err != nil {
			t.Fatalf("RunHooks attempt %d: %v", attempt+1, err)
		}
		for source, marker := range markers.bySource() {
			assertManagedHookMarker(t, marker, source == hooks.SourceManaged)
		}
		for _, hook := range engine.Hooks.Hooks[event] {
			if hook.SourceKind != hooks.SourceManaged && !hook.Suppressed {
				t.Fatalf("reload attempt %d published unsuppressed %s hook", attempt+1, hook.SourceKind)
			}
		}
	}
}

func TestPluginReloadMalformedManagedPolicyKeepsPublishedHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	markers := managedHookMarkers(t)
	event := hooks.SessionStart
	writeUserHook(t, home, event, managedHookMarker(markers.user))
	writeRuntimePluginHook(t, home, event, managedHookMarker(markers.plugin))

	managedPath := filepath.Join(t.TempDir(), "managed-hooks.yaml")
	writePluginFile(t, managedPath, "mode: managed-only\n")
	policy := managedHookPolicy(hooks.ManagedModeOnly, event, managedHookMarker(markers.managed), managedPath)
	malformed := false
	engine := newTestEngine(t)
	engine.managedHooks = engineManagedHooksRuntime{
		policyPath: managedPath,
		audit:      hooks.NewHookAudit(managedHookAuditPath(t)),
		loadPolicy: func(options hooks.LoadOptions) (*hooks.ManagedPolicy, error) {
			if malformed {
				options.ManagedReadFile = os.ReadFile
				return hooks.LoadManagedPolicy(options)
			}
			return policy, nil
		},
	}
	if _, err := engine.ReloadPlugins(t.Context()); err != nil {
		t.Fatalf("initial ReloadPlugins: %v", err)
	}
	publishedHooks := engine.Hooks
	publishedPolicy := engine.ManagedHookPolicy

	malformed = true
	writePluginFile(t, managedPath, "mode: [\n")
	writeUserHook(t, home, event, managedHookMarker(filepath.Join(t.TempDir(), "replacement-user")))
	_, err := engine.ReloadPlugins(t.Context())
	if !errors.Is(err, hooks.ErrManagedPolicy) {
		t.Fatalf("ReloadPlugins error = %v, want ErrManagedPolicy", err)
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Fatalf("ReloadPlugins error = %v, want real parser classification", err)
	}
	if engine.Hooks != publishedHooks || engine.ManagedHookPolicy != publishedPolicy {
		t.Fatal("failed managed reload published a partial hook set")
	}
	if len(engine.Hooks.Hooks[event]) != 3 {
		t.Fatalf("published hook count changed after failed reload: %#v", engine.Hooks.Hooks[event])
	}
}

func trustReloadedPluginAndProjectHooks(t *testing.T, engine *EngineContext, workDir string, event hooks.Event) {
	t.Helper()
	store, err := hooks.LoadTrustStore(hooks.DefaultTrustStorePath())
	if err != nil {
		t.Fatalf("LoadTrustStore: %v", err)
	}
	for _, hook := range engine.Hooks.Hooks[event] {
		if hook.SourceKind == hooks.SourcePlugin {
			if err := store.Trust(hook.Hash); err != nil {
				t.Fatalf("trust plugin hook: %v", err)
			}
		}
	}
	project, err := hooks.LoadWithOptions(hooks.LoadOptions{WorkingDir: workDir, TrustStore: store, SkipUser: true})
	if err != nil {
		t.Fatalf("load project hook: %v", err)
	}
	if err := store.Trust(project.Hooks[event][0].Hash); err != nil {
		t.Fatalf("trust project hook: %v", err)
	}
}

func writeRuntimePluginHook(t *testing.T, home string, event hooks.Event, hook hooks.Hook) {
	t.Helper()
	pluginDir := filepath.Join(home, ".ratchet", "plugins", "runtime-plugin")
	writePluginFile(t, filepath.Join(pluginDir, ".ratchet-plugin", "plugin.json"), `{"name":"runtime-plugin","version":"1.0.0","description":"test","author":{"name":"test"},"capabilities":{"hooks":"hooks.yaml"}}`)
	writeHookConfig(t, filepath.Join(pluginDir, "hooks.yaml"), event, hook)
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
