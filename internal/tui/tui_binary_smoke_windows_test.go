//go:build tui_smoke && windows

package tui

import (
	"bytes"
	"database/sql"
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

	"github.com/ActiveState/termtest"
	"github.com/ActiveState/termtest/expect"

	"github.com/GoCodeAlone/ratchet-cli/internal/harnessredact"
	"github.com/GoCodeAlone/workflow/secrets"
	_ "modernc.org/sqlite"
)

func TestTUIBinaryWindowsConPTYProviderSave(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary ConPTY smoke is disabled under -race")
	}
	const (
		sentinel        = "TUI-CONPTY-PROVIDER-SECRET-SENTINEL"
		bedrockSentinel = "TUI-CONPTY-BEDROCK-SECRET-SENTINEL"
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
	proxy := newWindowsProviderConnectProxy(t, fixture.Listener.Addr().String())
	t.Cleanup(proxy.Close)

	repoRoot := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	home := filepath.Join(tempRoot, "home")
	state := filepath.Join(tempRoot, "state")
	work := filepath.Join(tempRoot, "work")
	daemonRoot := filepath.Join(tempRoot, "daemon")
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke.exe")
	certFile := filepath.Join(tempRoot, "provider-fixture-ca.pem")
	for _, dir := range []string{home, state, work, daemonRoot} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: fixture.Certificate().Raw})
	if err := os.WriteFile(certFile, certificate, 0o600); err != nil {
		t.Fatalf("write provider fixture certificate: %v", err)
	}
	providerBaseURL := "https://example.com"
	red := harnessredact.New(home, repoRoot, tempRoot, daemonRoot, bin, fixture.URL, proxy.URL, providerBaseURL, sentinel, bedrockSentinel).String
	build := exec.Command("go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build provider smoke binary: %v\n%s", err, red(string(out)))
	}

	var transcript bytes.Buffer
	cp, err := termtest.New(termtest.Options{
		CmdName:       bin,
		WorkDirectory: work,
		Environment: append(os.Environ(),
			"HOME="+home,
			"USERPROFILE="+home,
			"XDG_STATE_HOME="+state,
			"RATCHET_TUI_SMOKE_ROOT="+daemonRoot,
			"RATCHET_TUI_SMOKE_CA_FILE="+certFile,
			"HTTPS_PROXY="+proxy.URL,
			"NO_PROXY=",
		),
		DefaultTimeout: 12 * time.Second,
		HideCmdLine:    true,
		ExtraOpts:      []expect.ConsoleOpt{expect.WithStdout(&transcript)},
	})
	if err != nil {
		t.Fatalf("start ConPTY provider smoke binary: %v", err)
	}
	defer cp.Close()

	if err := expectConPTY(cp, "Message ratchet", 8*time.Second); err != nil {
		if splashErr := expectConPTY(cp, "press any key", 2*time.Second); splashErr != nil {
			t.Fatalf("wait for TUI prompt or splash: %v / %v", err, splashErr)
		}
		cp.SendUnterminated(" ")
		if err := expectConPTY(cp, "Message ratchet", 8*time.Second); err != nil {
			t.Fatalf("wait for TUI prompt after splash: %v", err)
		}
	}

	sendConPTYLine(cp, "/provider add")
	mustExpectConPTY(t, cp, "Select your AI provider", 8*time.Second, red)
	cp.SendUnterminated("/bedrock")
	mustExpectConPTY(t, cp, "Amazon Bedrock", 8*time.Second, red)
	cp.SendUnterminated("\r")
	time.Sleep(250 * time.Millisecond)
	cp.SendUnterminated("\r")
	mustExpectConPTY(t, cp, "AWS secret access key", 8*time.Second, red)
	sendConPTYLine(cp, bedrockSentinel)
	mustExpectConPTY(t, cp, "AWS access key ID", 8*time.Second, red)
	sendConPTYLine(cp, "AKIAFIXTURE")
	mustExpectConPTY(t, cp, "region (2/2)", 8*time.Second, red)
	cp.SendUnterminated("\r")
	mustExpectConPTY(t, cp, "Loading models from Amazon Bedrock", 12*time.Second, red)
	cp.SendUnterminated("\x1b")
	mustExpectConPTY(t, cp, "Select your AI provider", 8*time.Second, red)

	cp.SendUnterminated("/custom")
	mustExpectConPTY(t, cp, "Custom endpoint", 8*time.Second, red)
	cp.SendUnterminated("\r")
	time.Sleep(250 * time.Millisecond)
	cp.SendUnterminated("\r")
	mustExpectConPTY(t, cp, "Configure Custom endpoint", 8*time.Second, red)
	sendConPTYLine(cp, sentinel)
	mustExpectConPTY(t, cp, "API compatibility", 8*time.Second, red)
	cp.SendUnterminated("\r")
	mustExpectConPTY(t, cp, "URL:", 8*time.Second, red)
	sendConPTYLine(cp, providerBaseURL)
	mustExpectConPTY(t, cp, "Select your default model", 12*time.Second, red)
	cp.SendUnterminated("\r")
	mustExpectConPTY(t, cp, "Review provider setup", 8*time.Second, red)
	if snapshot := cp.Snapshot(); strings.Contains(snapshot, sentinel) || strings.Contains(snapshot, bedrockSentinel) {
		t.Fatal("provider review exposed credential sentinel")
	}
	cp.SendUnterminated("\r")
	mustExpectConPTY(t, cp, "Connection successful", 15*time.Second, red)
	cp.SendUnterminated("\r")
	mustExpectConPTY(t, cp, "Message ratchet", 8*time.Second, red)
	sendConPTYLine(cp, "/exit ")
	if err := expectConPTYExit(cp, 8*time.Second); err != nil {
		t.Fatalf("wait for provider smoke exit: %v\n%s", err, red(cp.Snapshot()))
	}
	if output := transcript.String(); strings.Contains(output, sentinel) || strings.Contains(output, bedrockSentinel) {
		t.Fatal("provider ConPTY transcript exposed credential sentinel")
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
			t.Fatalf("inspect provider smoke secret: %v", getErr)
		}
		if value == bedrockSentinel {
			t.Fatal("abandoned Bedrock credential remained in secret storage")
		}
	}
}

func mustExpectConPTY(t *testing.T, cp *termtest.ConsoleProcess, value string, timeout time.Duration, red func(string) string) {
	t.Helper()
	if err := expectConPTY(cp, value, timeout); err != nil {
		t.Fatalf("wait for %q: %v\n%s", value, err, red(cp.Snapshot()))
	}
}

func newWindowsProviderConnectProxy(t *testing.T, backendAddress string) *httptest.Server {
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

func TestTUIBinaryWindowsConPTYSmoke(t *testing.T) {
	if raceEnabled {
		t.Skip("TUI binary ConPTY smoke is disabled under -race")
	}
	root := tuiRepoRoot(t)
	tempRoot := t.TempDir()
	home := filepath.Join(tempRoot, "home")
	state := filepath.Join(tempRoot, "state")
	work := filepath.Join(tempRoot, "work")
	bin := filepath.Join(tempRoot, "ratchet-tui-smoke.exe")
	for _, dir := range []string{home, state, work} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	red := harnessredact.New(home, root, tempRoot, work, bin, "smoke prompt body").String

	build := exec.Command("go", "build", "-tags", "tui_smoke", "-o", bin, "./cmd/ratchet-tui-smoke")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke binary: %v\n%s", err, red(string(out)))
	}

	cp, err := termtest.New(termtest.Options{
		CmdName:        bin,
		WorkDirectory:  work,
		Environment:    append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "XDG_STATE_HOME="+state),
		DefaultTimeout: 8 * time.Second,
		HideCmdLine:    true,
	})
	if err != nil {
		t.Fatalf("start ConPTY smoke binary: %v", err)
	}
	defer cp.Close()

	if err := expectConPTY(cp, "Message ratchet", 8*time.Second); err != nil {
		if splashErr := expectConPTY(cp, "press any key", 2*time.Second); splashErr != nil {
			t.Fatalf("wait for TUI prompt or splash: %v / %v", err, splashErr)
		}
		cp.SendUnterminated(" ")
		if err := expectConPTY(cp, "Message ratchet", 8*time.Second); err != nil {
			t.Fatalf("wait for TUI prompt after splash: %v", err)
		}
	}

	sendConPTYLine(cp, "smoke prompt body")
	if err := expectConPTY(cp, "I have completed the task.", 15*time.Second); err != nil {
		t.Fatalf("wait for smoke response: %v", err)
	}
	sendConPTYLine(cp, "/help ")
	if err := expectConPTY(cp, "Quit ratchet", 8*time.Second); err != nil {
		t.Fatalf("wait for help command output: %v", err)
	}
	sendConPTYLine(cp, "/exit ")
	if err := expectConPTYExit(cp, 8*time.Second); err != nil {
		t.Fatalf("wait for clean exit: %v", err)
	}
}

func sendConPTYLine(cp *termtest.ConsoleProcess, value string) {
	cp.SendUnterminated(value + "\r")
}

func expectConPTY(cp *termtest.ConsoleProcess, value string, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		_, err := cp.Expect(value, timeout)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout + time.Second):
		_ = cp.Close()
		return termtest.ErrWaitTimeout
	}
}

func expectConPTYExit(cp *termtest.ConsoleProcess, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		_, err := cp.ExpectExitCode(0, timeout)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout + time.Second):
		_ = cp.Close()
		return termtest.ErrWaitTimeout
	}
}
