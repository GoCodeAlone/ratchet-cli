//go:build tui_smoke && !windows

package daemon

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/GoCodeAlone/ratchet-cli/internal/hooks"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/workflow/secrets"
	"github.com/google/uuid"
)

func TestTUISmokeServiceBackgroundDrainIsDisabled(t *testing.T) {
	svc := newTUISmokeServiceForTest(t, t.TempDir())
	_, err := svc.StartACPBackgroundDrain(t.Context(), &pb.StartACPBackgroundDrainReq{
		SessionId: "session-1", Profile: "codex", Acknowledged: true,
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %s, want disabled %s: %v", status.Code(err), codes.FailedPrecondition, err)
	}
}

func TestTUISmokeServiceNeverLoadsHostManagedPolicy(t *testing.T) {
	svc := newTUISmokeServiceForTest(t, t.TempDir())
	svc.engine.managedHooks.loadPolicy = func(hooks.LoadOptions) (*hooks.ManagedPolicy, error) {
		panic("TUI smoke read host managed policy")
	}
	policy, err := svc.engine.loadManagedHookPolicy()
	if err != nil || policy != nil {
		t.Fatalf("disabled managed policy = %#v, %v", policy, err)
	}
}

func TestTUISmokeProviderSavePersistsSecretBoundary(t *testing.T) {
	const (
		sentinel         = "TUI-SMOKE-PROVIDER-SECRET-SENTINEL"
		secondSentinel   = "TUI-SMOKE-PROVIDER-SECOND-SENTINEL"
		recoverySentinel = "TUI-SMOKE-PROVIDER-RECOVERY-SENTINEL"
	)
	fixture := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secondSentinel {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-smoke","object":"chat.completion","model":"fixture-model-2","choices":[{"index":0,"message":{"role":"assistant","content":"provider smoke ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(fixture.Close)
	defaultTransport := http.DefaultTransport
	fixtureTransport := fixture.Client().Transport.(*http.Transport).Clone()
	fixtureTransport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, fixture.Listener.Addr().String())
	}
	http.DefaultTransport = fixtureTransport
	t.Cleanup(func() { http.DefaultTransport = defaultTransport })
	ctx := t.Context()
	root := t.TempDir()
	socketPath := filepath.Join(root, "ratchet.sock")
	client, stop := startTUISmokeClient(t, ctx, root, socketPath)
	operationID := uuid.NewString()
	operation, err := client.CommitProviderSave(ctx, &pb.CommitProviderSaveReq{
		OperationId: operationID,
		Provider: &pb.AddProviderReq{
			Alias:     "custom-smoke",
			Type:      "mock",
			Model:     "fixture-model",
			ApiKey:    sentinel,
			BaseUrl:   "http://127.0.0.1:1/v1",
			Settings:  `{"api_compat":"openai"}`,
			IsDefault: true,
		},
	})
	if err != nil {
		t.Fatalf("commit smoke provider: %v", err)
	}
	if operation.GetOperationId() != operationID || operation.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED {
		t.Fatalf("smoke provider operation = id:%q state:%s", operation.GetOperationId(), operation.GetState())
	}
	stop()

	db, err := sql.Open("sqlite", filepath.Join(root, "ratchet.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open persisted smoke db: %v", err)
	}
	defer db.Close()
	var journalMode string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("query smoke journal mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("smoke journal mode = %q, want wal", journalMode)
	}
	var secretName, settings, baseURL string
	if err := db.QueryRowContext(ctx, `SELECT secret_name, settings, base_url FROM llm_providers WHERE alias = ?`, "custom-smoke").Scan(&secretName, &settings, &baseURL); err != nil {
		t.Fatalf("query persisted smoke provider: %v", err)
	}
	if !strings.HasPrefix(secretName, "provider-v2-") {
		t.Fatalf("persisted provider secret category = %q", secretCategory(secretName))
	}
	for label, value := range map[string]string{"settings": settings, "base URL": baseURL, "operation ID": operation.GetOperationId()} {
		if strings.Contains(value, sentinel) {
			t.Fatalf("%s contains provider credential sentinel", label)
		}
	}

	fileSecrets := secrets.NewFileProvider(filepath.Join(root, "secrets"))
	credential, err := fileSecrets.Get(ctx, secretName)
	if err != nil {
		t.Fatalf("resolve persisted provider credential: %v", err)
	}
	if credential != sentinel {
		t.Fatal("persisted provider credential did not round-trip")
	}
	redactor := secrets.NewRedactor()
	if err := redactor.LoadFromProvider(ctx, fileSecrets); err != nil {
		t.Fatalf("load persisted smoke redactor: %v", err)
	}
	if redacted := redactor.Redact("credential=" + sentinel); strings.Contains(redacted, sentinel) {
		t.Fatal("persisted provider credential was not redacted")
	}

	pendingID := uuid.NewString()
	pendingSecret := "provider-v2-pending-smoke"
	orphanSecret := "provider-v2-orphan-smoke"
	if err := fileSecrets.Set(ctx, pendingSecret, recoverySentinel); err != nil {
		t.Fatalf("seed pending secret: %v", err)
	}
	if err := fileSecrets.Set(ctx, orphanSecret, "orphan-secret"); err != nil {
		t.Fatalf("seed orphan secret: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO provider_operations
		(operation_id, alias, state, failure, secret_name, created_at, updated_at, expires_at)
		VALUES (?, 'pending-smoke', 'pending', '', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, datetime('now', '+1 day'))`, pendingID, pendingSecret); err != nil {
		t.Fatalf("seed pending operation: %v", err)
	}

	client, stop = startTUISmokeClient(t, ctx, root, socketPath)
	t.Cleanup(stop)
	replayed, err := client.GetProviderOperation(ctx, &pb.GetProviderOperationReq{OperationId: operationID})
	if err != nil {
		t.Fatalf("replay committed operation: %v", err)
	}
	if replayed.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED {
		t.Fatalf("replayed operation state = %s", replayed.GetState())
	}
	recovered, err := client.GetProviderOperation(ctx, &pb.GetProviderOperationReq{OperationId: pendingID})
	if err != nil {
		t.Fatalf("query recovered operation: %v", err)
	}
	if recovered.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_FAILED || recovered.GetFailure() != pb.ProviderOperationFailure_PROVIDER_OPERATION_FAILURE_RESTART_RECOVERY {
		t.Fatalf("recovered operation = %s/%s", recovered.GetState(), recovered.GetFailure())
	}
	if wire, err := protojson.Marshal(recovered); err != nil {
		t.Fatalf("marshal recovered operation: %v", err)
	} else if containsAnySmokeSentinel(string(wire), sentinel, secondSentinel, recoverySentinel) {
		t.Fatal("operation status exposed provider credential sentinel")
	}
	if wire, err := protojson.Marshal(replayed); err != nil {
		t.Fatalf("marshal replayed operation: %v", err)
	} else if containsAnySmokeSentinel(string(wire), sentinel, secondSentinel, recoverySentinel) {
		t.Fatal("replayed operation exposed provider credential sentinel")
	}
	var recoveredState, recoveredFailure, recoveredSecretName string
	if err := db.QueryRowContext(ctx, `SELECT state, failure, secret_name FROM provider_operations WHERE operation_id = ?`, pendingID).Scan(&recoveredState, &recoveredFailure, &recoveredSecretName); err != nil {
		t.Fatalf("query recovered operation status: %v", err)
	}
	if containsAnySmokeSentinel(strings.Join([]string{recoveredState, recoveredFailure, recoveredSecretName}, "\n"), sentinel, secondSentinel, recoverySentinel) {
		t.Fatal("persisted recovery status exposed provider credential sentinel")
	}
	if got, err := fileSecrets.Get(ctx, secretName); err != nil || got != sentinel {
		t.Fatalf("referenced secret after restart = present:%t err:%v", got == sentinel, err)
	}
	waitTUISmokeCondition(t, "startup orphan cleanup", func() bool {
		_, orphanErr := fileSecrets.Get(ctx, orphanSecret)
		_, pendingErr := fileSecrets.Get(ctx, pendingSecret)
		return errors.Is(orphanErr, secrets.ErrNotFound) && errors.Is(pendingErr, secrets.ErrNotFound)
	})

	secondID := uuid.NewString()
	second, err := client.CommitProviderSave(ctx, &pb.CommitProviderSaveReq{
		OperationId: secondID,
		Provider: &pb.AddProviderReq{
			Alias:     "custom-smoke",
			Type:      "custom",
			Model:     "fixture-model-2",
			ApiKey:    secondSentinel,
			BaseUrl:   "https://example.com",
			Settings:  `{"api_compat":"openai","region":"smoke"}`,
			IsDefault: true,
		},
	})
	if err != nil || second.GetState() != pb.ProviderOperationState_PROVIDER_OPERATION_STATE_COMMITTED {
		t.Fatalf("upsert smoke provider = %+v, %v", second, err)
	}
	result, err := client.TestProvider(ctx, &pb.TestProviderReq{Alias: "custom-smoke"})
	if err != nil || !result.GetSuccess() {
		t.Fatalf("resolve upserted provider through registry = %+v, %v", result, err)
	}
	var secondSecret, secondSettings string
	if err := db.QueryRowContext(ctx, `SELECT secret_name, settings FROM llm_providers WHERE alias = ?`, "custom-smoke").Scan(&secondSecret, &secondSettings); err != nil {
		t.Fatalf("query upserted provider: %v", err)
	}
	if !strings.HasPrefix(secondSecret, "provider-v2-") || secondSecret == secretName || strings.Contains(secondSettings, secondSentinel) {
		t.Fatalf("upserted provider boundary = versioned:%t rotated:%t settings_leak:%t", strings.HasPrefix(secondSecret, "provider-v2-"), secondSecret != secretName, strings.Contains(secondSettings, secondSentinel))
	}
	if got, err := fileSecrets.Get(ctx, secondSecret); err != nil || got != secondSentinel {
		t.Fatalf("upserted provider credential = present:%t err:%v", got == secondSentinel, err)
	}
	waitTUISmokeCondition(t, "retired provider version cleanup", func() bool {
		_, getErr := fileSecrets.Get(ctx, secretName)
		return errors.Is(getErr, secrets.ErrNotFound)
	})

	retrySecret := "provider-v2-retry-smoke"
	if err := fileSecrets.Set(ctx, retrySecret, "retry-secret"); err != nil {
		t.Fatalf("seed retry secret: %v", err)
	}
	secretsDir := filepath.Join(root, "secrets")
	if err := os.Chmod(secretsDir, 0o500); err != nil {
		t.Fatalf("make secrets directory read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(secretsDir, 0o700) })
	if _, err := db.ExecContext(ctx, `INSERT INTO provider_secret_cleanup
		(secret_name, attempt_count, failure, created_at, updated_at, next_attempt_at)
		VALUES (?, 0, '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, retrySecret); err != nil {
		t.Fatalf("queue retry cleanup: %v", err)
	}
	waitTUISmokeCondition(t, "cleanup delete failure", func() bool {
		var failure string
		return db.QueryRowContext(ctx, `SELECT failure FROM provider_secret_cleanup WHERE secret_name = ?`, retrySecret).Scan(&failure) == nil && failure == "delete"
	})
	if err := os.Chmod(secretsDir, 0o700); err != nil {
		t.Fatalf("restore secrets directory permissions: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE provider_secret_cleanup SET next_attempt_at = CURRENT_TIMESTAMP WHERE secret_name = ?`, retrySecret); err != nil {
		t.Fatalf("schedule cleanup retry: %v", err)
	}
	waitTUISmokeCondition(t, "cleanup retry convergence", func() bool {
		_, getErr := fileSecrets.Get(ctx, retrySecret)
		var rows int
		queryErr := db.QueryRowContext(ctx, `SELECT count(*) FROM provider_secret_cleanup WHERE secret_name = ?`, retrySecret).Scan(&rows)
		return errors.Is(getErr, secrets.ErrNotFound) && queryErr == nil && rows == 0
	})

	if _, err := client.RemoveProvider(ctx, &pb.RemoveProviderReq{Alias: "custom-smoke"}); err != nil {
		t.Fatalf("remove smoke provider: %v", err)
	}
	waitTUISmokeCondition(t, "removed provider cleanup", func() bool {
		_, getErr := fileSecrets.Get(ctx, secondSecret)
		return errors.Is(getErr, secrets.ErrNotFound)
	})
}

func containsAnySmokeSentinel(text string, sentinels ...string) bool {
	for _, sentinel := range sentinels {
		if strings.Contains(text, sentinel) {
			return true
		}
	}
	return false
}

func startTUISmokeClient(t *testing.T, ctx context.Context, root, socketPath string) (pb.RatchetDaemonClient, func()) {
	t.Helper()
	_, cleanup, err := StartTUISmokeDaemon(ctx, root, socketPath)
	if err != nil {
		t.Fatalf("start smoke daemon: %v", err)
	}
	conn, err := grpc.NewClient("unix://"+socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		cleanup()
		t.Fatalf("connect smoke daemon: %v", err)
	}
	return pb.NewRatchetDaemonClient(conn), func() {
		_ = conn.Close()
		cleanup()
	}
}

func waitTUISmokeCondition(t *testing.T, label string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("%s did not converge", label)
}

func secretCategory(name string) string {
	if strings.HasPrefix(name, "provider-v2-") {
		return "provider-v2"
	}
	if name == "" {
		return "empty"
	}
	return "other"
}

func TestSmokeServiceInitializesSafeJobProviders(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	socketPath := filepath.Join(root, "ratchet.sock")

	session, cleanup, err := StartTUISmokeDaemon(ctx, root, socketPath)
	if err != nil {
		t.Fatalf("StartTUISmokeDaemon: %v", err)
	}
	defer cleanup()
	if session == nil {
		t.Fatal("expected initial smoke session")
	}
	if session.GetWorkingDir() != root {
		t.Fatalf("expected smoke session working dir %q, got %q", root, session.GetWorkingDir())
	}

	svc := newTUISmokeServiceForTest(t, root)
	if svc.engine == nil {
		t.Fatal("expected smoke engine")
	}
	if svc.engine.MCPDiscoverer != nil {
		t.Fatal("smoke service must disable MCP discovery")
	}
	if len(svc.engine.PluginSkills) != 0 || len(svc.engine.PluginAgents) != 0 ||
		len(svc.engine.PluginCommands) != 0 || len(svc.engine.PluginDaemons) != 0 {
		t.Fatal("smoke service must not load plugin capabilities")
	}
	if svc.autorespond != nil {
		t.Fatal("smoke service must not load autoresponder config from host workdir")
	}
	if svc.cron != nil {
		t.Fatal("smoke service must not start cron/background scheduler")
	}
	if svc.jobs == nil {
		t.Fatal("expected smoke job registry")
	}
}

func TestSmokeServiceListJobsHasMarkerOrEmptyState(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	socketPath := filepath.Join(root, "ratchet.sock")
	_, cleanup, err := StartTUISmokeDaemon(ctx, root, socketPath)
	if err != nil {
		t.Fatalf("StartTUISmokeDaemon: %v", err)
	}
	defer cleanup()

	conn, err := grpc.NewClient("unix://"+socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("connect smoke socket: %v", err)
	}
	defer conn.Close()

	client := pb.NewRatchetDaemonClient(conn)
	list, err := client.ListJobs(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if list == nil {
		t.Fatal("expected job list")
	}
	for _, job := range list.Jobs {
		if job.GetType() == "" || job.GetName() == "" {
			t.Fatalf("smoke job should expose test-observable type/name: %#v", job)
		}
	}
}
