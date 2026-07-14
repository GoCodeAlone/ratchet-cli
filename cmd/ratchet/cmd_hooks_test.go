package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestHooksPolicyReportsManagedAndAbsentPolicy(t *testing.T) {
	policy := testManagedHookPolicy(hooks.ManagedModeAdditive)
	restore := stubManagedHookPolicy(t, policy, "/secure/managed-hooks.yaml")
	defer restore()

	out := captureStdout(t, func() {
		if err := handleHooksPolicy([]string{"--json"}); err != nil {
			t.Fatalf("handleHooksPolicy: %v", err)
		}
	})
	var got hookPolicyRecord
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("Unmarshal: %v\n%s", err, out)
	}
	if got.Mode != string(hooks.ManagedModeAdditive) || got.SourcePath != "/secure/managed-hooks.yaml" || got.ManagedHookCount != 1 {
		t.Fatalf("policy record = %+v", got)
	}

	restore()
	restore = stubManagedHookPolicy(t, nil, "/secure/managed-hooks.yaml")
	defer restore()
	out = captureStdout(t, func() {
		if err := handleHooksPolicy([]string{"--json"}); err != nil {
			t.Fatalf("handleHooksPolicy absent: %v", err)
		}
	})
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("Unmarshal absent: %v\n%s", err, out)
	}
	if got.Mode != "none" || got.SourcePath != "/secure/managed-hooks.yaml" || got.ManagedHookCount != 0 {
		t.Fatalf("absent policy record = %+v", got)
	}
}

func TestHooksInspectionRejectsPositionalArguments(t *testing.T) {
	restore := stubManagedHookPolicy(t, nil, "/secure/managed-hooks.yaml")
	defer restore()
	t.Setenv("HOME", t.TempDir())
	for _, test := range []struct {
		name string
		run  func([]string) error
	}{
		{name: "list", run: handleHooksList},
		{name: "policy", run: handleHooksPolicy},
		{name: "audit", run: handleHooksAudit},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run([]string{"stray", "--json"}); err == nil {
				t.Fatal("inspection command accepted positional arguments")
			}
		})
	}
}

func TestHooksAuditReportsNewestFirstJSONAndAbsentFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	auditPath, err := hooks.DefaultHookAuditPath()
	if err != nil {
		t.Fatalf("DefaultHookAuditPath: %v", err)
	}
	audit := hooks.NewHookAudit(auditPath)
	now := time.Now().UTC()
	for i, result := range []hooks.HookAuditResult{hooks.HookAuditStarted, hooks.HookAuditSuccess} {
		if err := audit.Append(hooks.HookAuditRecord{
			Timestamp:  now.Add(time.Duration(i) * time.Second),
			Event:      hooks.PreCommand,
			Hash:       strings.Repeat(string(rune('a'+i)), 64),
			Source:     hooks.SourceManaged,
			Result:     result,
			DurationMS: int64(i),
		}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	out := captureStdout(t, func() {
		if err := handleHooksAudit([]string{"--json", "--limit", "1"}); err != nil {
			t.Fatalf("handleHooksAudit: %v", err)
		}
	})
	var records []hooks.HookAuditRecord
	if err := json.Unmarshal([]byte(out), &records); err != nil {
		t.Fatalf("Unmarshal: %v\n%s", err, out)
	}
	if len(records) != 1 || records[0].Result != hooks.HookAuditSuccess {
		t.Fatalf("audit records = %+v", records)
	}

	if err := os.Rename(auditPath, auditPath+".1"); err != nil {
		t.Fatalf("archive audit: %v", err)
	}
	if err := audit.Append(hooks.HookAuditRecord{
		Timestamp: now.Add(2 * time.Second), Event: hooks.PreCommand,
		Hash: strings.Repeat("c", 64), Source: hooks.SourceManaged,
		Result: hooks.HookAuditCommandFailed, DurationMS: 2,
	}); err != nil {
		t.Fatalf("Append active generation: %v", err)
	}
	out = captureStdout(t, func() {
		if err := handleHooksAudit([]string{"--json", "--limit", "2"}); err != nil {
			t.Fatalf("handleHooksAudit generations: %v", err)
		}
	})
	if err := json.Unmarshal([]byte(out), &records); err != nil {
		t.Fatalf("Unmarshal generations: %v\n%s", err, out)
	}
	if len(records) != 2 || records[0].Result != hooks.HookAuditCommandFailed || records[1].Result != hooks.HookAuditSuccess {
		t.Fatalf("multi-generation audit records = %+v", records)
	}

	absentHome := t.TempDir()
	t.Setenv("HOME", absentHome)
	t.Setenv("USERPROFILE", absentHome)
	out = captureStdout(t, func() {
		if err := handleHooksAudit([]string{"--json"}); err != nil {
			t.Fatalf("handleHooksAudit absent: %v", err)
		}
	})
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("absent audit output = %q, want []", out)
	}
}

func TestHooksListShowsManagedSourceAndSuppressionFields(t *testing.T) {
	_, workDir := setupHookCLIWorkspace(t)
	restore := stubManagedHookPolicy(t, testManagedHookPolicy(hooks.ManagedModeOnly), "/secure/managed-hooks.yaml")
	defer restore()

	items := decodeHookList(t, captureStdout(t, func() {
		handleHooks([]string{"list", "--json", "--cwd", workDir})
	}))
	managed := assertHookSource(t, items, "managed", "managed")
	if !managed.Managed || managed.Suppressed {
		t.Fatalf("managed fields = %+v", managed)
	}
	for _, source := range []string{"user", "project", "plugin"} {
		item := assertHookSource(t, items, source, "suppressed")
		if item.Managed || !item.Suppressed {
			t.Fatalf("%s suppression fields = %+v", source, item)
		}
	}
}

func TestHooksManagedMutationRejectsOnlyDiscoveredManagedHashes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	policy := testManagedHookPolicy(hooks.ManagedModeAdditive)
	configured := hooks.HookConfig{Hooks: make(map[hooks.Event][]hooks.Hook)}
	configured.ApplyManagedPolicy(policy)
	managedHash := configured.Hooks[hooks.PreCommand][0].Hash
	restore := stubManagedHookPolicy(t, policy, "/secure/managed-hooks.yaml")
	defer restore()

	for _, action := range []string{"trust", "untrust", "disable"} {
		err := mutateHookTrust(action, managedHash)
		if err == nil || !strings.Contains(err.Error(), "managed policy") || !strings.Contains(err.Error(), "immutable") {
			t.Fatalf("%s managed hash error = %v", action, err)
		}
	}
	unknown := strings.Repeat("f", 64)
	if err := mutateHookTrust("disable", unknown); err != nil {
		t.Fatalf("disable unknown hash: %v", err)
	}
	store, err := hooks.LoadTrustStore(hooks.DefaultTrustStorePath())
	if err != nil {
		t.Fatalf("LoadTrustStore: %v", err)
	}
	if !store.IsDisabled(unknown) || store.IsDisabled(managedHash) || store.IsTrusted(managedHash) {
		t.Fatalf("trust store mutated managed hash: %+v", store)
	}
}

type hookCLIItem struct {
	Event      string `json:"event"`
	SourceKind string `json:"source_kind"`
	Status     string `json:"status"`
	Hash       string `json:"hash"`
	Command    string `json:"command"`
	Managed    bool   `json:"managed"`
	Suppressed bool   `json:"suppressed"`
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

func testManagedHookPolicy(mode hooks.ManagedMode) *hooks.ManagedPolicy {
	return &hooks.ManagedPolicy{
		Mode: mode,
		Hooks: hooks.HookConfig{Hooks: map[hooks.Event][]hooks.Hook{
			hooks.PreCommand: {{Command: "echo managed", CommandWindows: "Write-Output managed"}},
		}},
	}
}

func stubManagedHookPolicy(t *testing.T, policy *hooks.ManagedPolicy, path string) func() {
	t.Helper()
	previousLoad := loadManagedHookPolicy
	previousPath := defaultManagedHookPolicyPath
	loadManagedHookPolicy = func(hooks.LoadOptions) (*hooks.ManagedPolicy, error) { return policy, nil }
	defaultManagedHookPolicyPath = func() (string, error) { return path, nil }
	restored := false
	return func() {
		if restored {
			return
		}
		loadManagedHookPolicy = previousLoad
		defaultManagedHookPolicyPath = previousPath
		restored = true
	}
}

func TestHooksPolicyPropagatesSecureLoaderFailure(t *testing.T) {
	previousLoad := loadManagedHookPolicy
	previousPath := defaultManagedHookPolicyPath
	sentinel := errors.New("managed policy unavailable")
	loadManagedHookPolicy = func(hooks.LoadOptions) (*hooks.ManagedPolicy, error) {
		return nil, sentinel
	}
	defaultManagedHookPolicyPath = func() (string, error) { return "/secure/managed-hooks.yaml", nil }
	defer func() {
		loadManagedHookPolicy = previousLoad
		defaultManagedHookPolicyPath = previousPath
	}()
	if err := handleHooksPolicy([]string{"--json"}); !errors.Is(err, sentinel) {
		t.Fatalf("handleHooksPolicy error = %v, want loader sentinel", err)
	}
}

func TestHooksPolicyLoadsTheResolvedSecurePath(t *testing.T) {
	const path = "/secure/managed-hooks.yaml"
	previousLoad := loadManagedHookPolicy
	previousPath := defaultManagedHookPolicyPath
	defaultManagedHookPolicyPath = func() (string, error) { return path, nil }
	loadManagedHookPolicy = func(opts hooks.LoadOptions) (*hooks.ManagedPolicy, error) {
		if opts.ManagedPath != path {
			return nil, errors.New("secure policy path was not passed to loader")
		}
		return nil, nil
	}
	defer func() {
		loadManagedHookPolicy = previousLoad
		defaultManagedHookPolicyPath = previousPath
	}()
	if err := handleHooksPolicy([]string{"--json"}); err != nil {
		t.Fatalf("handleHooksPolicy: %v", err)
	}
}
