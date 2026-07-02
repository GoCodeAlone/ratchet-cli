package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/hooks"
)

func TestHelpIncludesHooksCommand(t *testing.T) {
	out := captureStdout(t, printUsage)
	if !strings.Contains(out, "hooks") {
		t.Fatalf("help output missing hooks command:\n%s", out)
	}
}

func TestHandleHooksListJSONShowsUserProjectAndPluginHooks(t *testing.T) {
	home, workDir := setupHookCLIWorkspace(t)

	out := captureStdout(t, func() {
		handleHooks([]string{"list", "--json", "--cwd", workDir})
	})
	items := decodeHookList(t, out)
	assertHookSource(t, items, "user", "trusted")
	project := assertHookSource(t, items, "project", "untrusted")
	assertHookSource(t, items, "plugin", "untrusted")

	store, err := hooks.LoadTrustStore(filepath.Join(home, ".ratchet", "hook-trust.json"))
	if err != nil {
		t.Fatalf("LoadTrustStore: %v", err)
	}
	if store.IsTrusted(project.Hash) {
		t.Fatal("project hook should not start trusted")
	}
}

func TestHandleHooksTrustAndDisableMutateTrustStore(t *testing.T) {
	_, workDir := setupHookCLIWorkspace(t)
	items := decodeHookList(t, captureStdout(t, func() {
		handleHooks([]string{"list", "--json", "--cwd", workDir})
	}))
	project := assertHookSource(t, items, "project", "untrusted")

	out := captureStdout(t, func() {
		handleHooks([]string{"trust", project.Hash})
	})
	if !strings.Contains(out, "Trusted hook") {
		t.Fatalf("trust output = %q", out)
	}
	items = decodeHookList(t, captureStdout(t, func() {
		handleHooks([]string{"list", "--json", "--cwd", workDir})
	}))
	project = assertHookSource(t, items, "project", "trusted")

	out = captureStdout(t, func() {
		handleHooks([]string{"disable", project.Hash})
	})
	if !strings.Contains(out, "Disabled hook") {
		t.Fatalf("disable output = %q", out)
	}
	items = decodeHookList(t, captureStdout(t, func() {
		handleHooks([]string{"list", "--json", "--cwd", workDir})
	}))
	assertHookSource(t, items, "project", "disabled")
}

func TestHandleHooksListTruncatesLongCommands(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ratchet"), 0o700); err != nil {
		t.Fatal(err)
	}
	longCommand := "echo " + strings.Repeat("x", 160)
	if err := os.WriteFile(filepath.Join(home, ".ratchet", "hooks.yaml"), []byte(`
hooks:
  post-command:
    - command: "`+longCommand+`"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		handleHooks([]string{"list", "--json", "--cwd", workDir})
	})
	if strings.Contains(out, longCommand) {
		t.Fatalf("hooks list dumped full command:\n%s", out)
	}
	items := decodeHookList(t, out)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if len(items[0].Command) > 96 || !strings.HasSuffix(items[0].Command, "...") {
		t.Fatalf("command not truncated: %q", items[0].Command)
	}
}

type hookCLIItem struct {
	Event      string `json:"event"`
	SourceKind string `json:"source_kind"`
	Status     string `json:"status"`
	Hash       string `json:"hash"`
	Command    string `json:"command"`
}

func decodeHookList(t *testing.T, out string) []hookCLIItem {
	t.Helper()
	var items []hookCLIItem
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("decode hooks JSON: %v\n%s", err, out)
	}
	return items
}

func assertHookSource(t *testing.T, items []hookCLIItem, sourceKind, status string) hookCLIItem {
	t.Helper()
	for _, item := range items {
		if item.SourceKind == sourceKind {
			if item.Status != status {
				t.Fatalf("%s hook status = %q, want %q: %+v", sourceKind, item.Status, status, item)
			}
			if item.Hash == "" {
				t.Fatalf("%s hook hash is empty: %+v", sourceKind, item)
			}
			return item
		}
	}
	t.Fatalf("missing %s hook in %+v", sourceKind, items)
	return hookCLIItem{}
}

func setupHookCLIWorkspace(t *testing.T) (home, workDir string) {
	t.Helper()
	home = t.TempDir()
	workDir = t.TempDir()
	t.Setenv("HOME", home)

	if err := os.MkdirAll(filepath.Join(home, ".ratchet"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ratchet", "hooks.yaml"), []byte(`
hooks:
  post-command:
    - command: "echo user"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(workDir, ".ratchet"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".ratchet", "hooks.yaml"), []byte(`
hooks:
  post-command:
    - command: "echo project"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	pluginDir := filepath.Join(home, ".ratchet", "plugins", "hook-plugin")
	if err := os.MkdirAll(filepath.Join(pluginDir, ".ratchet-plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"hook-plugin","version":"1.0.0","description":"test","author":{"name":"test"},"capabilities":{"hooks":"hooks.yaml"}}`
	if err := os.WriteFile(filepath.Join(pluginDir, ".ratchet-plugin", "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "hooks.yaml"), []byte(`
hooks:
  post-command:
    - command: "echo plugin"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	return home, workDir
}
