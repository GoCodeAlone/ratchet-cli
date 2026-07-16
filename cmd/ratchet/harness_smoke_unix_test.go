//go:build !windows

package main

import (
	"context"
	"database/sql"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
	"github.com/GoCodeAlone/ratchet-cli/internal/harnessredact"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/workflow/secrets"
	"google.golang.org/protobuf/encoding/protojson"
	_ "modernc.org/sqlite"
)

func TestHarnessSmokeDurableProviderSaveRestart(t *testing.T) {
	if raceEnabled {
		t.Skip("production provider durability smoke is disabled under -race")
	}
	if runtime.GOOS == "darwin" {
		t.Skip("macOS system verification ignores SSL_CERT_FILE; Linux CI owns the hermetic TLS fixture proof")
	}
	const sentinel = "HARNESS-PROVIDER-SECRET-SENTINEL"
	fixture := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/chat/completions", "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"id":"chatcmpl-smoke","object":"chat.completion","model":"fixture-model","choices":[{"index":0,"message":{"role":"assistant","content":"provider smoke ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fixture.Close)
	proxy := newHarnessProviderConnectProxy(t, fixture.Listener.Addr().String())
	t.Cleanup(proxy.Close)

	bin := buildRatchetSmokeBinary(t)
	root := t.TempDir()
	home := filepath.Join(root, "home")
	state := filepath.Join(home, ".local", "state")
	work := filepath.Join(root, "work")
	certFile := filepath.Join(root, "provider-fixture-ca.pem")
	for _, dir := range []string{home, state, work} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: fixture.Certificate().Raw})
	if err := os.WriteFile(certFile, certificate, 0o600); err != nil {
		t.Fatalf("write provider fixture certificate: %v", err)
	}
	providerBaseURL := "https://example.com"
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("SSL_CERT_FILE", certFile)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	red := harnessredact.New(home, state, work, root, bin, certFile, fixture.URL, proxy.URL, providerBaseURL, daemon.SocketPath(), daemon.PIDPath(), sentinel).String
	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_STATE_HOME=" + state,
		"SSL_CERT_FILE=" + certFile,
		"HTTPS_PROXY=" + proxy.URL,
		"NO_PROXY=",
	}
	t.Cleanup(bestEffortShutdownHarnessSmokeDaemon)

	s := startRatchetSmokePTY(t, bin, work, env, red,
		"provider", "add", "custom", "durable-smoke", "--model", "fixture-model", "--json")
	s.waitFor("API key", 10*time.Second)
	s.sendLine(sentinel)
	s.waitFor("API compatibility", 10*time.Second)
	s.sendLine("")
	s.waitFor("Base URL", 10*time.Second)
	s.sendLine(providerBaseURL)
	addOutput := s.waitExit(20 * time.Second)
	if strings.Contains(addOutput, sentinel) {
		t.Fatal("provider add output exposed credential sentinel")
	}
	operationID := providerOperationIDFromSmokeOutput(t, addOutput, red)
	waitForRatchetSmokePresent(t, daemon.SocketPath(), 5*time.Second)
	waitForRatchetSmokePresent(t, daemon.PIDPath(), 5*time.Second)

	operationOutput, err := runRatchetSmoke(t, bin, home, "provider", "operation", operationID, "--json")
	if err != nil {
		t.Fatalf("query provider operation: %v\n%s", err, red(operationOutput))
	}
	var persistedOperation pb.ProviderOperation
	if err := protojson.Unmarshal([]byte(operationOutput), &persistedOperation); err != nil {
		t.Fatalf("decode provider operation: %v\n%s", err, red(operationOutput))
	}
	if persistedOperation.GetOperationId() != operationID || persistedOperation.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED {
		t.Fatalf("provider operation = id_match:%t state:%s", persistedOperation.GetOperationId() == operationID, persistedOperation.GetState())
	}
	if strings.Contains(operationOutput, sentinel) {
		t.Fatal("provider operation output exposed credential sentinel")
	}

	db, err := sql.Open("sqlite", daemon.DBPath()+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open production smoke database: %v", err)
	}
	defer db.Close()
	var secretName string
	if err := db.QueryRow(`SELECT secret_name FROM llm_providers WHERE alias = 'durable-smoke'`).Scan(&secretName); err != nil {
		t.Fatalf("query production provider pointer: %v", err)
	}
	if !strings.HasPrefix(secretName, "provider-v2-") {
		t.Fatal("production provider secret is not versioned")
	}
	fileSecrets := secrets.NewFileProvider(filepath.Join(daemon.DataDir(), "secrets"))
	if got, err := fileSecrets.Get(t.Context(), secretName); err != nil || got != sentinel {
		t.Fatalf("production provider credential = present:%t err:%v", got == sentinel, err)
	}

	shutdownHarnessSmokeDaemon(t)
	waitForRatchetSmokeMissing(t, daemon.SocketPath(), 5*time.Second)
	waitForRatchetSmokeMissing(t, daemon.PIDPath(), 5*time.Second)
	startOutput, err := runRatchetSmoke(t, bin, home, "daemon", "start", "--background")
	if err != nil {
		t.Fatalf("restart production daemon: %v\n%s", err, red(startOutput))
	}
	waitForRatchetSmokePresent(t, daemon.SocketPath(), 5*time.Second)
	waitForRatchetSmokePresent(t, daemon.PIDPath(), 5*time.Second)

	replayedOutput, err := runRatchetSmoke(t, bin, home, "provider", "operation", operationID, "--json")
	if err != nil {
		t.Fatalf("replay provider operation after restart: %v\n%s", err, red(replayedOutput))
	}
	var replayedOperation pb.ProviderOperation
	if err := protojson.Unmarshal([]byte(replayedOutput), &replayedOperation); err != nil || replayedOperation.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED {
		t.Fatalf("decode replayed provider operation: %v state:%s\n%s", err, replayedOperation.GetState(), red(replayedOutput))
	}
	listOutput, err := runRatchetSmoke(t, bin, home, "provider", "list")
	if err != nil || !strings.Contains(listOutput, "durable-smoke") {
		t.Fatalf("list provider after restart: %v\n%s", err, red(listOutput))
	}
	testOutput, err := runRatchetSmoke(t, bin, home, "provider", "test", "durable-smoke")
	if err != nil || !strings.Contains(testOutput, "OK") {
		t.Fatalf("test provider after restart: %v\n%s", err, red(testOutput))
	}
	for _, output := range []string{replayedOutput, listOutput, testOutput} {
		if strings.Contains(output, sentinel) {
			t.Fatal("provider restart output exposed credential sentinel")
		}
	}
	shutdownHarnessSmokeDaemon(t)
	waitForRatchetSmokeMissing(t, daemon.SocketPath(), 5*time.Second)
	waitForRatchetSmokeMissing(t, daemon.PIDPath(), 5*time.Second)
}

func TestHarnessSmokeProviderAppliedState(t *testing.T) {
	if raceEnabled {
		t.Skip("production provider state smoke is disabled under -race")
	}
	const (
		operationID = "c603c6bf-7f45-48c8-87e2-f9ef914731d5"
		alias       = "applied-smoke"
		secretName  = "provider-v2-applied-smoke"
		credential  = "APPLIED-SMOKE-CREDENTIAL-SENTINEL"
	)

	bin := buildRatchetSmokeBinary(t)
	root := t.TempDir()
	home := filepath.Join(root, "home")
	state := filepath.Join(home, ".local", "state")
	for _, dir := range []string{home, state} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", state)
	red := harnessredact.New(home, state, root, bin, daemon.SocketPath(), daemon.PIDPath(), secretName, credential).String
	t.Cleanup(bestEffortShutdownHarnessSmokeDaemon)

	startOutput, err := runRatchetSmoke(t, bin, home, "daemon", "start", "--background")
	if err != nil {
		t.Fatalf("start production daemon: %v\n%s", err, red(startOutput))
	}
	waitForRatchetSmokePresent(t, daemon.SocketPath(), 5*time.Second)
	waitForRatchetSmokePresent(t, daemon.PIDPath(), 5*time.Second)

	db, err := sql.Open("sqlite", daemon.DBPath()+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open production smoke database: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO llm_providers
		(id, alias, type, model, secret_name, base_url, max_tokens, settings, is_default)
		VALUES ('applied-smoke-id', ?, 'openai', 'applied-smoke-model', ?, '', 4096, '{}', 1)`,
		alias, secretName); err != nil {
		t.Fatalf("seed production provider: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO provider_operations
		(operation_id, alias, state, failure, secret_name, result_type, result_model,
		 result_is_default, created_at, updated_at, expires_at)
		VALUES (?, ?, 'applied', '', ?, 'openai', 'applied-smoke-model', 1,
		 CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, datetime('now', '+1 day'))`,
		operationID, alias, secretName); err != nil {
		t.Fatalf("seed applied provider operation: %v", err)
	}
	shutdownHarnessSmokeDaemon(t)
	waitForRatchetSmokeMissing(t, daemon.SocketPath(), 5*time.Second)
	waitForRatchetSmokeMissing(t, daemon.PIDPath(), 5*time.Second)
	restartOutput, err := runRatchetSmoke(t, bin, home, "daemon", "start", "--background")
	if err != nil {
		t.Fatalf("restart production daemon with applied operation: %v\n%s", err, red(restartOutput))
	}
	waitForRatchetSmokePresent(t, daemon.SocketPath(), 5*time.Second)
	waitForRatchetSmokePresent(t, daemon.PIDPath(), 5*time.Second)

	appliedOutput, err := runRatchetSmoke(t, bin, home, "provider", "operation", operationID, "--json")
	if err != nil {
		t.Fatalf("query applied provider operation: %v\n%s", err, red(appliedOutput))
	}
	var applied pb.ProviderOperation
	if err := protojson.Unmarshal([]byte(appliedOutput), &applied); err != nil {
		t.Fatalf("decode applied provider operation: %v\n%s", err, red(appliedOutput))
	}
	if applied.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_APPLIED ||
		applied.GetFailure() != pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_UNSPECIFIED {
		t.Fatalf("applied provider operation = %s/%s", applied.GetState(), applied.GetFailure())
	}
	result := applied.GetResult()
	if applied.GetOperationId() != operationID || result == nil || result.GetAlias() != alias ||
		result.GetType() != "openai" || result.GetModel() != "applied-smoke-model" || !result.GetIsDefault() {
		t.Fatalf("applied provider result = %+v", result)
	}
	assertHarnessSmokeOperationState(t, db, operationID, "applied")

	fileSecrets := secrets.NewFileProvider(filepath.Join(daemon.DataDir(), "secrets"))
	if err := fileSecrets.Set(t.Context(), secretName, credential); err != nil {
		t.Fatalf("restore applied provider secret: %v", err)
	}
	committedOutput, err := runRatchetSmoke(t, bin, home, "provider", "operation", operationID, "--json")
	if err != nil {
		t.Fatalf("retry applied provider operation: %v\n%s", err, red(committedOutput))
	}
	var committed pb.ProviderOperation
	if err := protojson.Unmarshal([]byte(committedOutput), &committed); err != nil {
		t.Fatalf("decode committed provider operation: %v\n%s", err, red(committedOutput))
	}
	if committed.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED ||
		committed.GetFailure() != pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_UNSPECIFIED {
		t.Fatalf("retried provider operation = %s/%s, want COMMITTED/UNSPECIFIED", committed.GetState(), committed.GetFailure())
	}
	result = committed.GetResult()
	if committed.GetOperationId() != operationID || result == nil || result.GetAlias() != alias ||
		result.GetType() != "openai" || result.GetModel() != "applied-smoke-model" || !result.GetIsDefault() {
		t.Fatalf("committed provider result = %+v", result)
	}
	assertHarnessSmokeOperationState(t, db, operationID, "committed")
	// StartBackground disconnects daemon logs. Launcher/RPC output is checked here;
	// startup sanitization is covered at the provider-operation manager boundary.
	for _, output := range []string{startOutput, restartOutput, appliedOutput, committedOutput} {
		if strings.Contains(output, credential) || strings.Contains(output, secretName) || strings.Contains(strings.ToLower(output), "secret not found") {
			t.Fatalf("provider operation output exposed secret metadata: %s", red(output))
		}
	}

	shutdownHarnessSmokeDaemon(t)
	waitForRatchetSmokeMissing(t, daemon.SocketPath(), 5*time.Second)
	waitForRatchetSmokeMissing(t, daemon.PIDPath(), 5*time.Second)
}

func assertHarnessSmokeOperationState(t *testing.T, db *sql.DB, operationID, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow(`SELECT state FROM provider_operations WHERE operation_id = ?`, operationID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("provider operation SQL state = %q, want %q", got, want)
	}
}

func providerOperationIDFromSmokeOutput(t *testing.T, output string, red func(string) string) string {
	t.Helper()
	match := regexp.MustCompile(`"operation_id"\s*:\s*"([0-9a-f-]{36})"`).FindStringSubmatch(output)
	if len(match) != 2 {
		t.Fatalf("provider add output has no operation ID:\n%s", red(output))
	}
	return match[1]
}

func shutdownHarnessSmokeDaemon(t *testing.T) {
	t.Helper()
	c, err := client.Connect()
	if err != nil {
		t.Fatalf("connect to production smoke daemon: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	defer c.Close()
	if err := c.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown production smoke daemon: %v", err)
	}
}

func bestEffortShutdownHarnessSmokeDaemon() {
	c, err := client.Connect()
	if err != nil {
		return
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = c.Shutdown(ctx)
}

func waitForRatchetSmokePresent(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for daemon artifact category %q", filepath.Base(path))
}

func newHarnessProviderConnectProxy(t *testing.T, backendAddress string) *httptest.Server {
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
		clientConn, buffered, err := hijacker.Hijack()
		if err != nil {
			_ = backend.Close()
			return
		}
		defer clientConn.Close()
		defer backend.Close()
		_, _ = buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffered.Flush()
		copyDone := make(chan struct{})
		go func() {
			_, _ = io.Copy(backend, clientConn)
			if tcp, ok := backend.(*net.TCPConn); ok {
				_ = tcp.CloseWrite()
			}
			close(copyDone)
		}()
		_, _ = io.Copy(clientConn, backend)
		<-copyDone
	}))
}

func TestHarnessSmokeStartupOnboardingAndRPCShutdown(t *testing.T) {
	if raceEnabled {
		t.Skip("release startup PTY smoke is disabled under -race")
	}
	bin := buildRatchetSmokeBinary(t)
	root := t.TempDir()
	home := filepath.Join(root, "home")
	state := filepath.Join(root, "state")
	work := filepath.Join(root, "work")
	for _, dir := range []string{home, state, work} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", state)
	red := harnessredact.New(home, work, root, daemon.SocketPath(), bin, daemon.PIDPath(), state).String
	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_STATE_HOME=" + state,
	}

	s := startRatchetSmokePTY(t, bin, work, env, red, "--reconfigure")
	if match, _ := s.waitForAny([]string{"press any key", "Welcome to ratchet", "Select your AI provider"}, 10*time.Second); match == "press any key" {
		s.send(" ")
	}
	out := s.waitFor("Select your AI provider", 10*time.Second)
	if strings.Contains(out, "ratchet-tui-smoke") || strings.Contains(out, "tui_smoke") {
		t.Fatalf("release-shaped startup output leaked smoke marker:\n%s", red(out))
	}
	assertSocketAndPIDContained(t, home)

	c, err := client.Connect()
	if err != nil {
		t.Fatalf("connect to startup daemon: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Shutdown(ctx); err != nil {
		_ = c.Close()
		t.Fatalf("shutdown startup daemon: %v", err)
	}
	_ = c.Close()
	waitForRatchetSmokeMissing(t, daemon.SocketPath(), 5*time.Second)
	waitForRatchetSmokeMissing(t, daemon.PIDPath(), 5*time.Second)

	s.sendCtrl('c')
	s.waitExit(5 * time.Second)
}

func assertSocketAndPIDContained(t *testing.T, home string) {
	t.Helper()
	for _, path := range []string{daemon.SocketPath(), daemon.PIDPath()} {
		if !strings.HasPrefix(path, filepath.Join(home, ".ratchet")+string(os.PathSeparator)) {
			t.Fatalf("daemon path %s is outside test home %s", path, home)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected daemon file %s: %v", path, err)
		}
	}
	if info, err := os.Stat(daemon.SocketPath()); err != nil {
		t.Fatalf("stat daemon socket: %v", err)
	} else if info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0600 {
		t.Fatalf("daemon socket mode = %v, want socket 0600", info.Mode())
	}
}

func waitForRatchetSmokeMissing(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to be removed", path)
}
