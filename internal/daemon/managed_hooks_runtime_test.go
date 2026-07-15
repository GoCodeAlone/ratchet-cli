package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"

	"github.com/GoCodeAlone/ratchet-cli/internal/hooks"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestManagedHooksStartupAbsentPolicyIsNormal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	engine, err := newEngineContext(t.Context(), filepath.Join(home, "ratchet.db"), engineManagedHooksRuntime{
		policyPath: filepath.Join(home, "missing-managed-hooks.yaml"),
		audit:      hooks.NewHookAudit(managedHookAuditPath(t)),
	})
	if err != nil {
		t.Fatalf("newEngineContext with absent policy: %v", err)
	}
	t.Cleanup(engine.Close)
	if engine.ManagedHookPolicy != nil {
		t.Fatalf("ManagedHookPolicy = %#v, want nil", engine.ManagedHookPolicy)
	}
}

func TestManagedHooksStartupAbsentPolicyDoesNotResolveAudit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	auditResolved := false
	engine, err := newEngineContext(t.Context(), filepath.Join(home, "ratchet.db"), engineManagedHooksRuntime{
		policyPath: filepath.Join(home, "missing-managed-hooks.yaml"),
		newAudit: func() (hooks.HookAuditWriter, error) {
			auditResolved = true
			return nil, errors.New("audit home unavailable")
		},
	})
	if err != nil {
		t.Fatalf("newEngineContext with absent policy: %v", err)
	}
	t.Cleanup(engine.Close)
	if auditResolved {
		t.Fatal("absent managed policy resolved an unused audit writer")
	}
	if engine.ManagedHookAudit != nil {
		t.Fatalf("ManagedHookAudit = %#v, want nil without managed policy", engine.ManagedHookAudit)
	}
}

func TestManagedHooksStartupMalformedOrInsecurePolicyFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name       string
		document   string
		wantError  string
		loadPolicy func(hooks.LoadOptions) (*hooks.ManagedPolicy, error)
	}{
		{
			name:      "malformed",
			document:  "mode: [\n",
			wantError: "parse",
			loadPolicy: func(options hooks.LoadOptions) (*hooks.ManagedPolicy, error) {
				options.ManagedReadFile = os.ReadFile
				return hooks.LoadManagedPolicy(options)
			},
		},
		{name: "insecure", document: "mode: managed-only\nhooks: {}\n", loadPolicy: hooks.LoadManagedPolicy},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			markers := managedHookMarkers(t)
			writeUserHook(t, home, hooks.SessionStart, managedHookMarker(markers.user))
			managedPath := filepath.Join(home, "managed-hooks.yaml")
			if err := os.WriteFile(managedPath, []byte(test.document), 0o666); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(managedPath, 0o666); err != nil {
				t.Fatal(err)
			}

			engine, err := newEngineContext(t.Context(), filepath.Join(home, "ratchet.db"), engineManagedHooksRuntime{
				policyPath: managedPath,
				audit:      hooks.NewHookAudit(managedHookAuditPath(t)),
				loadPolicy: test.loadPolicy,
			})
			if !errors.Is(err, hooks.ErrManagedPolicy) {
				t.Fatalf("newEngineContext error = %v, want ErrManagedPolicy", err)
			}
			if test.wantError != "" && !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("newEngineContext error = %v, want %q classification", err, test.wantError)
			}
			if engine != nil {
				engine.Close()
				t.Fatal("newEngineContext returned a partially initialized engine")
			}
			assertManagedHookMarker(t, markers.user, false)
		})
	}
}

func TestManagedHooksStartupKeepsManagedPolicyAcrossUnrelatedPluginFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	markers := managedHookMarkers(t)
	event := hooks.SessionStart
	writeUserHook(t, home, event, managedHookMarker(markers.user))
	pluginDir := filepath.Join(home, ".ratchet", "plugins", "broken-plugin")
	writePluginFile(t, filepath.Join(pluginDir, ".ratchet-plugin", "plugin.json"), `{"name":"broken-plugin","version":"1.0.0","description":"test","author":{"name":"test"},"capabilities":{"hooks":"hooks.yaml"}}`)
	writePluginFile(t, filepath.Join(pluginDir, "hooks.yaml"), "hooks: [not-an-event-map")

	managedPath := filepath.Join(t.TempDir(), "managed-hooks.yaml")
	policy := managedHookPolicy(hooks.ManagedModeOnly, event, managedHookMarker(markers.managed), managedPath)
	auditPath := managedHookAuditPath(t)
	engine, err := newEngineContext(t.Context(), filepath.Join(home, "ratchet.db"), engineManagedHooksRuntime{
		policyPath: managedPath,
		audit:      hooks.NewHookAudit(auditPath),
		loadPolicy: func(hooks.LoadOptions) (*hooks.ManagedPolicy, error) {
			return policy, nil
		},
	})
	if err != nil {
		t.Fatalf("newEngineContext optional plugin failure: %v", err)
	}
	t.Cleanup(engine.Close)
	if engine.ManagedHookPolicy != policy || engine.ManagedHookAudit == nil {
		t.Fatalf("fallback managed runtime = policy:%t audit:%t", engine.ManagedHookPolicy == policy, engine.ManagedHookAudit != nil)
	}
	if err := engine.RunHooks(t.Context(), event, map[string]string{"session_id": "plugin-fallback"}); err != nil {
		t.Fatalf("RunHooks after plugin fallback: %v", err)
	}
	assertManagedHookMarker(t, markers.user, false)
	assertManagedHookMarker(t, markers.managed, true)
	assertManagedHookAuditJSONL(t, auditPath, event)
}

func TestManagedHooksRuntimeSessionLifecycleManagedOnly(t *testing.T) {
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
	auditPath := managedHookAuditPath(t)
	engine, err := newEngineContext(t.Context(), filepath.Join(home, "ratchet.db"), engineManagedHooksRuntime{
		policyPath: managedPath,
		audit:      hooks.NewHookAudit(auditPath),
		loadPolicy: func(options hooks.LoadOptions) (*hooks.ManagedPolicy, error) {
			if options.ManagedPath != managedPath {
				return nil, fmt.Errorf("managed path = %q, want %q", options.ManagedPath, managedPath)
			}
			return policy, nil
		},
	})
	if err != nil {
		t.Fatalf("newEngineContext: %v", err)
	}
	t.Cleanup(engine.Close)
	trustReloadedPluginAndProjectHooks(t, engine, workDir, event)

	service := &Service{engine: engine, sessions: NewSessionManager(engine.DB)}
	address := startTestGRPCServer(t, service)
	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := pb.NewRatchetDaemonClient(connection)
	if _, err := client.CreateSession(t.Context(), &pb.CreateSessionReq{WorkingDir: workDir, Provider: "runtime-test"}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	for source, marker := range markers.bySource() {
		assertManagedHookMarker(t, marker, source == hooks.SourceManaged)
	}
	assertManagedHookAuditJSONL(t, auditPath, event)
}

func TestManagedHooksPolicyCommandUsesStandaloneBinary(t *testing.T) {
	if path, err := hooks.DefaultManagedPolicyPath(); err == nil {
		if _, statErr := os.Stat(path); statErr == nil {
			t.Skipf("host has an administrator policy at %s", path)
		}
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	binaryName := "ratchet"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/ratchet")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build production ratchet: %v\n%s", err, output)
	}

	home := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binaryPath, "hooks", "policy", "--json")
	command.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("ratchet hooks policy --json: %v\n%s", err, output)
	}
	if ctx.Err() != nil {
		t.Fatalf("ratchet hooks policy started or waited for daemon: %v", ctx.Err())
	}
	var policyOutput struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(output, &policyOutput); err != nil {
		t.Fatalf("decode policy output %q: %v", output, err)
	}
	if policyOutput.Mode != "none" {
		t.Fatalf("policy mode = %q, want none: %s", policyOutput.Mode, output)
	}
	for _, daemonArtifact := range []string{"ratchet.db", "daemon.pid", "daemon.sock"} {
		if _, err := os.Stat(filepath.Join(home, ".ratchet", daemonArtifact)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("standalone policy inspection created daemon artifact %s: %v", daemonArtifact, err)
		}
	}
}

type managedHookMarkerSet struct {
	user    string
	plugin  string
	project string
	managed string
}

func managedHookMarkers(t *testing.T) managedHookMarkerSet {
	t.Helper()
	dir := t.TempDir()
	return managedHookMarkerSet{
		user:    filepath.Join(dir, "user"),
		plugin:  filepath.Join(dir, "plugin"),
		project: filepath.Join(dir, "project"),
		managed: filepath.Join(dir, "managed"),
	}
}

func (markers managedHookMarkerSet) bySource() map[hooks.SourceKind]string {
	return map[hooks.SourceKind]string{
		hooks.SourceUser:    markers.user,
		hooks.SourcePlugin:  markers.plugin,
		hooks.SourceProject: markers.project,
		hooks.SourceManaged: markers.managed,
	}
}

func managedHookMarker(path string) hooks.Hook {
	return managedHookMarkerWithPrefix(path, "managed-hook")
}

func managedHookMarkerWithPrefix(path, value string) hooks.Hook {
	return hooks.Hook{
		Command:        fmt.Sprintf("printf %s > %s", shellTestQuote(value), shellTestQuote(path)),
		CommandWindows: fmt.Sprintf("Set-Content -NoNewline -Path %s -Value %s", powershellTestQuote(path), powershellTestQuote(value)),
	}
}

func shellTestQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func powershellTestQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func managedHookPolicy(mode hooks.ManagedMode, event hooks.Event, hook hooks.Hook, path string) *hooks.ManagedPolicy {
	policy := &hooks.ManagedPolicy{
		Mode:  mode,
		Hooks: hooks.HookConfig{Hooks: map[hooks.Event][]hooks.Hook{event: {hook}}},
	}
	policy.Hooks.AnnotateSource(hooks.SourceMetadata{
		Kind:           hooks.SourceManaged,
		ID:             "managed:managed-hooks.yaml",
		Path:           path,
		TrustByDefault: true,
	})
	return policy
}

func managedHookAuditPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), ".ratchet", "audit", "hooks.jsonl")
}

func writeUserHook(t *testing.T, home string, event hooks.Event, hook hooks.Hook) {
	t.Helper()
	writeHookConfig(t, filepath.Join(home, ".ratchet", "hooks.yaml"), event, hook)
}

func writeProjectHook(t *testing.T, workDir string, event hooks.Event, hook hooks.Hook) {
	t.Helper()
	writeHookConfig(t, filepath.Join(workDir, ".ratchet", "hooks.yaml"), event, hook)
}

func writeHookConfig(t *testing.T, path string, event hooks.Event, hook hooks.Hook) {
	t.Helper()
	data, err := yaml.Marshal(hooks.HookConfig{Hooks: map[hooks.Event][]hooks.Hook{event: {hook}}})
	if err != nil {
		t.Fatalf("marshal hook config: %v", err)
	}
	writePluginFile(t, path, string(data))
}

func removeManagedHookMarkers(t *testing.T, markers managedHookMarkerSet) {
	t.Helper()
	for _, marker := range markers.bySource() {
		if err := os.Remove(marker); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("remove marker %s: %v", marker, err)
		}
	}
}

func assertManagedHookMarker(t *testing.T, path string, want bool) {
	t.Helper()
	_, err := os.Stat(path)
	if want && err != nil {
		t.Fatalf("expected marker %s: %v", path, err)
	}
	if !want && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected marker %s: %v", path, err)
	}
}

func assertManagedHookAuditJSONL(t *testing.T, path string, event hooks.Event) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open managed audit: %v", err)
	}
	defer file.Close()

	var records []hooks.HookAuditRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record hooks.HookAuditRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("decode audit record: %v", err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan managed audit: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("audit records = %d, want exact start/success pair: %#v", len(records), records)
	}
	if records[0].Result != hooks.HookAuditStarted || records[1].Result != hooks.HookAuditSuccess {
		t.Fatalf("audit results = %q, %q, want started, success", records[0].Result, records[1].Result)
	}
	for _, record := range records {
		if record.Event != event || record.Source != hooks.SourceManaged || record.Hash == "" || record.Timestamp.IsZero() {
			t.Fatalf("invalid managed audit record: %#v", record)
		}
	}
}
