//go:build !windows

package tui

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/harnessredact"
	"github.com/GoCodeAlone/workflow/secrets"
	_ "modernc.org/sqlite"
)

func TestTUIBinarySmokeProviderSave(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary PTY smoke is disabled under -race")
	}
	const (
		sentinel        = "TUI-PTY-PROVIDER-SECRET-SENTINEL"
		bedrockSentinel = "TUI-PTY-BEDROCK-SECRET-SENTINEL"
	)
	fixture := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"fixture-model","object":"model","owned_by":"smoke"}]}`))
		case "/chat/completions", "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"id":"chatcmpl-smoke","object":"chat.completion","model":"fixture-model","choices":[{"index":0,"message":{"role":"assistant","content":"provider smoke ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fixture.Close)
	proxy := newProviderConnectProxy(t, fixture.Listener.Addr().String())
	t.Cleanup(proxy.Close)

	root := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	home := filepath.Join(tempRoot, "home")
	state := filepath.Join(tempRoot, "state")
	work := filepath.Join(tempRoot, "work")
	daemonRoot := filepath.Join(tempRoot, "daemon")
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke")
	certFile := filepath.Join(tempRoot, "provider-fixture-ca.pem")
	for _, dir := range []string{home, state, work, daemonRoot} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("create provider smoke directory: %v", err)
		}
	}
	if err := os.Chmod(daemonRoot, 0o750); err != nil {
		t.Fatalf("set caller-owned daemon root mode: %v", err)
	}
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: fixture.Certificate().Raw})
	if err := os.WriteFile(certFile, certificate, 0600); err != nil {
		t.Fatalf("write provider fixture certificate: %v", err)
	}
	providerBaseURL := "https://example.com"
	red := harnessredact.New(home, root, tempRoot, daemonRoot, bin, fixture.URL, proxy.URL, providerBaseURL, sentinel, bedrockSentinel).String
	build := exec.CommandContext(context.Background(), "go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build provider smoke binary: %v\n%s", err, red(string(out)))
	}

	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_STATE_HOME=" + state,
		"RATCHET_TUI_SMOKE_ROOT=" + daemonRoot,
		"HTTPS_PROXY=" + proxy.URL,
		"NO_PROXY=",
		"SSL_CERT_FILE=" + certFile,
		"RATCHET_TUI_SMOKE_CA_FILE=" + certFile,
	}
	s := startTUITestPTY(t, bin, work, env, red)
	var transcript strings.Builder
	capture := func() { transcript.WriteString(s.snapshot()) }
	if match, _ := s.waitForAny([]string{"press any key", "Message ratchet"}, 8*time.Second); match == "press any key" {
		s.send(" ")
	}
	s.waitFor("Message ratchet", 8*time.Second)
	s.submitSlash("/provider add")
	s.waitFor("Select your AI provider", 8*time.Second)

	s.send("/")
	s.send("bedrock")
	s.waitFor("Amazon Bedrock", 8*time.Second)
	s.send("\r")
	time.Sleep(250 * time.Millisecond)
	capture()
	s.clear()
	s.send("\r")
	s.waitFor("AWS secret access key", 8*time.Second)
	s.sendLine(bedrockSentinel)
	s.waitFor("AWS access key ID", 8*time.Second)
	s.sendLine("AKIAFIXTURE")
	s.waitFor("region (2/2)", 8*time.Second)
	s.send("\r")
	s.waitFor("Loading models from Amazon Bedrock", 12*time.Second)
	capture()
	s.clear()
	s.send("\x1b")
	s.waitFor("Select your AI provider", 8*time.Second)

	capture()
	s.clear()
	s.send("/")
	s.send("custom")
	s.waitFor("Custom endpoint", 8*time.Second)
	s.send("\r")
	time.Sleep(250 * time.Millisecond)
	capture()
	s.clear()
	s.send("\r")
	s.waitFor("Configure Custom endpoint", 8*time.Second)
	s.sendLine(sentinel)
	s.waitFor("API compatibility", 8*time.Second)
	capture()
	s.clear()
	s.send("\r")
	s.waitFor("URL:", 8*time.Second)
	s.sendLine(providerBaseURL)
	s.waitFor("Select your default model", 12*time.Second)
	s.send("\r")
	s.waitFor("Review provider setup", 8*time.Second)
	if snapshot := s.snapshot(); strings.Contains(snapshot, sentinel) || strings.Contains(snapshot, bedrockSentinel) {
		t.Fatal("provider review exposed credential sentinel")
	}
	s.send("\r")
	s.waitFor("Connection successful", 15*time.Second)
	s.send("\r")
	s.waitFor("Message ratchet", 8*time.Second)
	s.submitSlash("/exit")
	transcript.WriteString(s.waitExit(8 * time.Second))
	if output := transcript.String(); strings.Contains(output, sentinel) || strings.Contains(output, bedrockSentinel) {
		t.Fatal("provider TUI transcript exposed credential sentinel")
	}
	rootInfo, err := os.Stat(daemonRoot)
	if err != nil {
		t.Fatalf("stat caller-owned daemon root: %v", err)
	}
	if got := rootInfo.Mode().Perm(); got != 0o750 {
		t.Fatalf("caller-owned daemon root mode = %04o, want 0750", got)
	}

	db, err := sql.Open("sqlite", filepath.Join(daemonRoot, "ratchet.db"))
	if err != nil {
		t.Fatalf("open provider smoke database: %v", err)
	}
	defer db.Close()
	var secretName, settings string
	if err := db.QueryRow(`SELECT secret_name, settings FROM llm_providers WHERE alias = 'custom'`).Scan(&secretName, &settings); err != nil {
		t.Fatalf("query saved provider: %v", err)
	}
	if !strings.HasPrefix(secretName, "provider-v2-") || strings.Contains(settings, sentinel) || strings.Contains(settings, bedrockSentinel) {
		t.Fatalf("saved provider boundary = secret:%t settings_leak:%t", strings.HasPrefix(secretName, "provider-v2-"), strings.Contains(settings, sentinel) || strings.Contains(settings, bedrockSentinel))
	}
	var operationState string
	if err := db.QueryRow(`SELECT state FROM provider_operations WHERE alias = 'custom' ORDER BY created_at DESC LIMIT 1`).Scan(&operationState); err != nil {
		t.Fatalf("query provider operation state: %v", err)
	}
	if operationState != "committed" {
		t.Fatalf("provider operation state = %q", operationState)
	}
	var bedrockProviders, bedrockOperations int
	if err := db.QueryRow(`SELECT count(*) FROM llm_providers WHERE type = 'bedrock' OR alias = 'bedrock'`).Scan(&bedrockProviders); err != nil {
		t.Fatalf("query abandoned Bedrock providers: %v", err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM provider_operations WHERE alias = 'bedrock'`).Scan(&bedrockOperations); err != nil {
		t.Fatalf("query abandoned Bedrock operations: %v", err)
	}
	if bedrockProviders != 0 || bedrockOperations != 0 {
		t.Fatalf("abandoned Bedrock state = providers:%d operations:%d", bedrockProviders, bedrockOperations)
	}
	fileSecrets := secrets.NewFileProvider(filepath.Join(daemonRoot, "secrets"))
	credential, err := fileSecrets.Get(t.Context(), secretName)
	if err != nil {
		t.Fatalf("resolve provider smoke credential: %v", err)
	}
	if credential != sentinel {
		t.Fatal("provider smoke credential did not round-trip")
	}
	secretNames, err := fileSecrets.List(t.Context())
	if err != nil {
		t.Fatalf("list provider smoke secrets: %v", err)
	}
	for _, name := range secretNames {
		value, getErr := fileSecrets.Get(t.Context(), name)
		if getErr != nil {
			t.Fatalf("inspect provider smoke secret category %q: %v", secretCategoryForSmoke(name), getErr)
		}
		if value == bedrockSentinel {
			t.Fatal("abandoned Bedrock credential remained in secret storage")
		}
	}
}

func secretCategoryForSmoke(name string) string {
	if strings.HasPrefix(name, "provider-v2-") {
		return "provider-v2"
	}
	return "other"
}

func newProviderConnectProxy(t *testing.T, backendAddress string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
			return
		}
		backend, err := net.Dial("tcp", backendAddress)
		if err != nil {
			http.Error(w, "fixture unavailable", http.StatusBadGateway)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			_ = backend.Close()
			http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
			return
		}
		client, buffered, err := hijacker.Hijack()
		if err != nil {
			_ = backend.Close()
			return
		}
		defer client.Close()
		defer backend.Close()
		_, _ = buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffered.Flush()
		copyDone := make(chan struct{})
		go func() {
			_, _ = io.Copy(backend, client)
			if tcp, ok := backend.(*net.TCPConn); ok {
				_ = tcp.CloseWrite()
			}
			close(copyDone)
		}()
		_, _ = io.Copy(client, backend)
		<-copyDone
	}))
}

func TestTUIBinarySmoke(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary PTY smoke is disabled under -race")
	}
	root := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	home := filepath.Join(tempRoot, "home")
	state := filepath.Join(tempRoot, "state")
	work := filepath.Join(tempRoot, "work")
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke")
	spec := loadCommandSurfaceSpec(t, root)
	for _, dir := range []string{home, state, work} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	redValues := append([]string{
		home,
		root,
		tempRoot,
		filepath.Join(tempRoot, "ratchet.sock"),
		bin,
		filepath.Join(tempRoot, "dist"),
		"smoke prompt body",
	}, trustBodiesFromSpec(spec)...)
	redactor := harnessredact.New(redValues...)
	red := redactor.String

	build := exec.CommandContext(context.Background(), "go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke binary: %v\n%s", err, red(string(out)))
	}

	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_STATE_HOME=" + state,
	}
	s := startTUITestPTY(t, bin, work, env, red)
	if match, _ := s.waitForAny([]string{"press any key", "Message ratchet"}, 6*time.Second); match == "press any key" {
		s.send(" ")
	}
	s.waitFor("Message ratchet", 6*time.Second)

	s.sendLine("smoke prompt body")
	s.waitFor("I have completed the task.", 15*time.Second)
	assertNoInstructionOrHookSurface(t, s.snapshot(), red)

	for _, row := range spec.Commands {
		if row.Evidence != "pty-proven" || row.Command == "/exit" || row.Command == "/tree" {
			continue
		}
		s.clear()
		s.submitSlash(row.Command)
		s.waitFor(expectedForSmokeCommand(row.Command), 8*time.Second)
		assertTrustStateAfterCommand(t, s, row.Command)
	}

	s.clear()
	s.submitSlash("/tree")
	s.waitFor(expectedForSmokeCommand("/tree"), 8*time.Second)
	s.send("\x1b")
	s.waitFor("Message ratchet", 8*time.Second)

	for _, row := range spec.Shortcuts {
		if row.Evidence != "pty-proven" {
			continue
		}
		s.clear()
		switch row.Keys {
		case "ctrl+b":
			// Covered in TestTUIBinarySmokeSessionTreeShortcut to keep tree navigation in a fresh PTY session.
			continue
		case "ctrl+s":
			// Covered in TestTUIBinarySmokeSidebarShortcut to keep sidebar navigation in a fresh PTY session.
			continue
		case "ctrl+t":
			// Covered in TestTUIBinarySmokeTeamShortcut to keep team navigation in a fresh PTY session.
			continue
		case "ctrl+j":
			// Covered in TestTUIBinarySmokeJobsShortcut to keep job-panel navigation in a fresh PTY session.
			continue
		default:
			t.Fatalf("unhandled pty-proven shortcut %q", row.Keys)
		}
	}

	assertNoInstructionOrHookSurface(t, s.snapshot(), red)
}

func TestTUIBinarySmokeSlashExit(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary PTY smoke is disabled under -race")
	}
	root := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	home := filepath.Join(tempRoot, "home")
	state := filepath.Join(tempRoot, "state")
	work := filepath.Join(tempRoot, "work")
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke")
	for _, dir := range []string{home, state, work} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	red := harnessredact.New(home, root, tempRoot, filepath.Join(tempRoot, "ratchet.sock"), bin).String
	build := exec.CommandContext(context.Background(), "go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke binary: %v\n%s", err, red(string(out)))
	}

	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_STATE_HOME=" + state,
	}
	s := startTUITestPTY(t, bin, work, env, red)
	if match, _ := s.waitForAny([]string{"press any key", "Message ratchet"}, 6*time.Second); match == "press any key" {
		s.send(" ")
	}
	s.waitFor("Message ratchet", 6*time.Second)
	s.clear()
	s.submitSlash("/exit")
	s.waitExit(5 * time.Second)
}

func TestTUIBinarySmokeJobsShortcut(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary PTY smoke is disabled under -race")
	}
	root := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	home := filepath.Join(tempRoot, "home")
	state := filepath.Join(tempRoot, "state")
	work := filepath.Join(tempRoot, "work")
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke")
	for _, dir := range []string{home, state, work} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	red := harnessredact.New(home, root, tempRoot, filepath.Join(tempRoot, "ratchet.sock"), bin).String
	build := exec.CommandContext(context.Background(), "go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke binary: %v\n%s", err, red(string(out)))
	}

	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_STATE_HOME=" + state,
	}
	s := startTUITestPTY(t, bin, work, env, red)
	if match, _ := s.waitForAny([]string{"press any key", "Message ratchet"}, 6*time.Second); match == "press any key" {
		s.send(" ")
	}
	s.waitFor("Message ratchet", 6*time.Second)
	s.clear()
	s.sendCtrl('j')
	s.waitFor("tui-smoke-daemon", 8*time.Second)
	if strings.Contains(strings.ToLower(s.snapshot()), "rpc error") {
		t.Fatalf("job panel contained RPC error:\n%s", red(s.snapshot()))
	}
	s.sendCtrl('j')
	s.waitFor("Message ratchet", 8*time.Second)
	assertNoInstructionOrHookSurface(t, s.snapshot(), red)
	s.sendCtrl('c')
	s.waitExit(5 * time.Second)
}

func TestTUIBinarySmokeTeamShortcut(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary PTY smoke is disabled under -race")
	}
	root := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	home := filepath.Join(tempRoot, "home")
	state := filepath.Join(tempRoot, "state")
	work := filepath.Join(tempRoot, "work")
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke")
	for _, dir := range []string{home, state, work} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	red := harnessredact.New(home, root, tempRoot, filepath.Join(tempRoot, "ratchet.sock"), bin).String
	build := exec.CommandContext(context.Background(), "go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke binary: %v\n%s", err, red(string(out)))
	}

	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_STATE_HOME=" + state,
	}
	s := startTUITestPTY(t, bin, work, env, red)
	if match, _ := s.waitForAny([]string{"press any key", "Message ratchet"}, 6*time.Second); match == "press any key" {
		s.send(" ")
	}
	s.waitFor("Message ratchet", 6*time.Second)
	s.clear()
	s.sendCtrl('t')
	s.waitFor("Team View", 8*time.Second)
	s.sendCtrl('t')
	s.waitFor("Message ratchet", 8*time.Second)
	assertNoInstructionOrHookSurface(t, s.snapshot(), red)
	s.sendCtrl('c')
	s.waitExit(5 * time.Second)
}

func TestTUIBinarySmokeSidebarShortcut(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary PTY smoke is disabled under -race")
	}
	root := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	home := filepath.Join(tempRoot, "home")
	state := filepath.Join(tempRoot, "state")
	work := filepath.Join(tempRoot, "work")
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke")
	for _, dir := range []string{home, state, work} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	red := harnessredact.New(home, root, tempRoot, filepath.Join(tempRoot, "ratchet.sock"), bin).String
	build := exec.CommandContext(context.Background(), "go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke binary: %v\n%s", err, red(string(out)))
	}

	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_STATE_HOME=" + state,
	}
	s := startTUITestPTY(t, bin, work, env, red)
	if match, _ := s.waitForAny([]string{"press any key", "Message ratchet"}, 6*time.Second); match == "press any key" {
		s.send(" ")
	}
	s.waitFor("Message ratchet", 6*time.Second)
	s.clear()
	s.sendCtrl('s')
	s.waitFor("Sessions", 8*time.Second)
	s.sendCtrl('s')
	s.waitFor("Message ratchet", 8*time.Second)
	assertNoInstructionOrHookSurface(t, s.snapshot(), red)
	s.sendCtrl('c')
	s.waitExit(5 * time.Second)
}

func TestTUIBinarySmokeSessionTreeShortcut(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary PTY smoke is disabled under -race")
	}
	root := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	home := filepath.Join(tempRoot, "home")
	state := filepath.Join(tempRoot, "state")
	work := filepath.Join(tempRoot, "work")
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke")
	for _, dir := range []string{home, state, work} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	red := harnessredact.New(home, root, tempRoot, filepath.Join(tempRoot, "ratchet.sock"), bin).String
	build := exec.CommandContext(context.Background(), "go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke binary: %v\n%s", err, red(string(out)))
	}

	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_STATE_HOME=" + state,
	}
	s := startTUITestPTY(t, bin, work, env, red)
	if match, _ := s.waitForAny([]string{"press any key", "Message ratchet"}, 6*time.Second); match == "press any key" {
		s.send(" ")
	}
	s.waitFor("Message ratchet", 6*time.Second)
	s.clear()
	s.sendCtrl('b')
	s.waitFor("Session Tree", 8*time.Second)
	s.send("\x1b")
	s.waitFor("Message ratchet", 8*time.Second)
	assertNoInstructionOrHookSurface(t, s.snapshot(), red)
	s.sendCtrl('c')
	s.waitExit(5 * time.Second)
}

func TestTUIBinarySmokeEmptyJobs(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary PTY smoke is disabled under -race")
	}
	root := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	home := filepath.Join(tempRoot, "home")
	state := filepath.Join(tempRoot, "state")
	work := filepath.Join(tempRoot, "work")
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke")
	for _, dir := range []string{home, state, work} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	red := harnessredact.New(home, root, tempRoot, filepath.Join(tempRoot, "ratchet.sock"), bin).String
	build := exec.CommandContext(context.Background(), "go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke binary: %v\n%s", err, red(string(out)))
	}

	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_STATE_HOME=" + state,
		"RATCHET_TUI_SMOKE_EMPTY_JOBS=1",
	}
	s := startTUITestPTY(t, bin, work, env, red)
	if match, _ := s.waitForAny([]string{"press any key", "Message ratchet"}, 6*time.Second); match == "press any key" {
		s.send(" ")
	}
	s.waitFor("Message ratchet", 6*time.Second)
	s.clear()
	s.sendCtrl('j')
	s.waitFor("No active jobs", 8*time.Second)
	if strings.Contains(strings.ToLower(s.snapshot()), "rpc error") {
		t.Fatalf("empty job panel contained RPC error:\n%s", red(s.snapshot()))
	}
	s.sendCtrl('c')
	s.waitExit(5 * time.Second)
}

func TestTUIBinarySmokeExitKeys(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary PTY smoke is disabled under -race")
	}
	root := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke")
	redBuild := harnessredact.New(root, tempRoot, bin).String
	build := exec.Command("go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke binary: %v\n%s", err, redBuild(string(out)))
	}
	for _, tc := range []struct {
		name string
		key  byte
	}{
		{name: "ctrl-c", key: 'c'},
		{name: "ctrl-d", key: 'd'},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "home")
			state := filepath.Join(t.TempDir(), "state")
			work := filepath.Join(t.TempDir(), "work")
			for _, dir := range []string{home, state, work} {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatalf("mkdir %s: %v", dir, err)
				}
			}
			red := harnessredact.New(home, root, filepath.Dir(home), filepath.Join(filepath.Dir(home), "ratchet.sock"), bin).String
			env := []string{
				"HOME=" + home,
				"USERPROFILE=" + home,
				"XDG_STATE_HOME=" + state,
			}
			s := startTUITestPTY(t, bin, work, env, red)
			s.waitFor("Message ratchet", 8*time.Second)
			s.sendCtrl(tc.key)
			s.waitExit(5 * time.Second)
		})
	}
}

type commandSurfaceSpec struct {
	Commands  []commandSurfaceRow  `json:"commands"`
	Shortcuts []shortcutSurfaceRow `json:"shortcuts"`
}

type commandSurfaceRow struct {
	Command  string `json:"command"`
	Evidence string `json:"evidence"`
}

type shortcutSurfaceRow struct {
	Keys     string `json:"keys"`
	Evidence string `json:"evidence"`
}

func loadCommandSurfaceSpec(t *testing.T, root string) commandSurfaceSpec {
	t.Helper()
	path := filepath.Join(root, "internal", "tui", "commands", "testdata", "command_surface_spec.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read command surface spec: %v", err)
	}
	var spec commandSurfaceSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse command surface spec: %v", err)
	}
	if len(spec.Commands) == 0 || len(spec.Shortcuts) == 0 {
		t.Fatalf("command surface spec must declare pty-proven commands and shortcuts")
	}
	return spec
}

func trustBodiesFromSpec(spec commandSurfaceSpec) []string {
	var values []string
	for _, row := range spec.Commands {
		if row.Evidence != "pty-proven" || !strings.HasPrefix(row.Command, "/trust ") {
			continue
		}
		fields := commandFields(row.Command)
		if len(fields) < 3 {
			continue
		}
		switch fields[1] {
		case "allow", "deny", "revoke":
			values = append(values, fields[2])
		case "persist":
			if len(fields) >= 4 {
				values = append(values, fields[3])
			}
		}
	}
	return values
}

func (s *tuiPTY) submitSlash(cmd string) {
	s.t.Helper()
	text := cmd + " "
	for _, r := range text {
		s.send(string(r))
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	s.send("\r")
}

func expectedForSmokeCommand(cmd string) string {
	switch {
	case cmd == "/help":
		return "Quit ratchet"
	case cmd == "/provider list":
		return "e2e-mock"
	case cmd == "/tree":
		return "Session Tree"
	case strings.HasPrefix(cmd, "/mode "):
		return "Mode switched"
	case strings.HasPrefix(cmd, "/trust allow"):
		return "Added allow rule"
	case strings.HasPrefix(cmd, "/trust deny"):
		return "Added deny rule"
	case strings.HasPrefix(cmd, "/trust persist allow"):
		return "Persisted allow grant"
	case strings.HasPrefix(cmd, "/trust persist deny"):
		return "Persisted deny grant"
	case strings.HasPrefix(cmd, "/trust revoke"):
		return "Revoked persistent trust grant"
	case cmd == "/trust list":
		return "Mode:"
	case cmd == "/trust grants":
		return "smoke"
	case cmd == "/trust reset":
		return "Mode: conservative"
	default:
		return cmd
	}
}

func assertTrustStateAfterCommand(t *testing.T, s *tuiPTY, cmd string) {
	t.Helper()
	switch {
	case strings.HasPrefix(cmd, "/trust allow"):
		s.clear()
		s.submitSlash("/trust list")
		out := s.waitFor("smoke:allow", 8*time.Second)
		assertTrustRule(t, s, out, cmd, "allow", "smoke", "smoke:allow")
	case strings.HasPrefix(cmd, "/trust deny"):
		s.clear()
		s.submitSlash("/trust list")
		out := s.waitFor("smoke:deny", 8*time.Second)
		assertTrustRule(t, s, out, cmd, "deny", "smoke", "smoke:deny")
	case strings.HasPrefix(cmd, "/trust persist allow"):
		s.clear()
		s.submitSlash("/trust grants")
		out := s.waitFor("smoke:persist-allow", 8*time.Second)
		assertTrustGrant(t, s, out, cmd, "allow", "smoke", "operator", "smoke:persist-allow")
	case strings.HasPrefix(cmd, "/trust persist deny"):
		s.clear()
		s.submitSlash("/trust grants")
		out := s.waitFor("smoke:persist-deny", 8*time.Second)
		assertTrustGrant(t, s, out, cmd, "deny", "smoke", "operator", "smoke:persist-deny")
	case strings.HasPrefix(cmd, "/trust revoke"):
		s.clear()
		s.submitSlash("/trust grants")
		out := s.waitFor("smoke:persist-deny", 8*time.Second)
		assertTrustGrant(t, s, out, cmd, "deny", "smoke", "operator", "smoke:persist-deny")
		if trustGrantPresent(out, "allow", "smoke", "operator", "smoke:persist-allow") {
			t.Fatalf("trust grants after %q still included revoked grant:\n%s", cmd, s.red(out))
		}
	case cmd == "/trust reset":
		s.clear()
		s.submitSlash("/trust list")
		out := s.waitFor("Mode: conservative", 8*time.Second)
		if strings.Contains(out, "smoke:allow") || strings.Contains(out, "smoke:deny") {
			t.Fatalf("trust reset left runtime smoke rules:\n%s", s.red(out))
		}
		s.clear()
		s.submitSlash("/trust grants")
		s.waitFor("smoke:persist-deny", 8*time.Second)
	}
}

func assertTrustRule(t *testing.T, s *tuiPTY, out, cmd, action, scope, pattern string) {
	t.Helper()
	if !trustRulePresent(out, action, scope, pattern) {
		t.Fatalf("trust state after %q missing rule %s %s %q:\n%s", cmd, action, scope, pattern, s.red(out))
	}
}

func assertTrustGrant(t *testing.T, s *tuiPTY, out, cmd, action, scope, grantedBy, pattern string) {
	t.Helper()
	if !trustGrantPresent(out, action, scope, grantedBy, pattern) {
		t.Fatalf("trust state after %q missing grant %s %s %s %q:\n%s", cmd, action, scope, grantedBy, pattern, s.red(out))
	}
}

func trustRulePresent(out, action, scope, pattern string) bool {
	for _, line := range trustOutputLines(out) {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == action && fields[1] == scope && strings.Join(fields[2:], " ") == pattern {
			return true
		}
	}
	return false
}

func trustGrantPresent(out, action, scope, grantedBy, pattern string) bool {
	for _, line := range trustOutputLines(out) {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == action && fields[1] == scope && fields[2] == grantedBy && strings.Join(fields[3:], " ") == pattern {
			return true
		}
	}
	return false
}

func trustOutputLines(out string) []string {
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "⚙") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "⚙"))
		}
		if i := strings.Index(line, "Message ratchet"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func commandFields(s string) []string {
	var fields []string
	var b strings.Builder
	inQuote := false
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\' && inQuote:
			escaped = true
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == '\t' || r == '\n') && !inQuote:
			if b.Len() > 0 {
				fields = append(fields, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		fields = append(fields, b.String())
	}
	return fields
}

func assertNoInstructionOrHookSurface(t *testing.T, out string, red func(string) string) {
	t.Helper()
	for _, token := range []string{".ratchet/hooks.yaml", "AGENTS.md", "CLAUDE.md", ".codex"} {
		if strings.Contains(out, token) {
			t.Fatalf("runtime output leaked instruction/hook surface %q:\n%s", token, red(out))
		}
	}
}
