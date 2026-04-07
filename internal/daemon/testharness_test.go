package daemon

import (
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	"github.com/GoCodeAlone/ratchet-cli/internal/hooks"
	"github.com/GoCodeAlone/ratchet-cli/internal/mesh"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	ratchetplugin "github.com/GoCodeAlone/workflow-plugin-agent/orchestrator"
	"github.com/GoCodeAlone/workflow/secrets"
)

// E2EHarness is a fully wired test harness for E2E daemon tests.
//
// It creates a real Service backed by in-memory SQLite and a real file-based
// SecretsProvider in a temp directory, starts a gRPC server on a random TCP
// port, and returns a connected gRPC client. The only mock is the LLM provider
// itself (type "mock"), which returns scripted responses without any network I/O.
//
// This exercises the exact same code path as the production daemon:
//   - Real initDB (including the migration that clears stale secret_name for
//     keyless providers like ollama/llama_cpp)
//   - Real ProviderRegistry with all built-in factories
//   - Real AddProvider RPC logic (only sets secret_name when ApiKey is non-empty)
//   - Real gRPC server/client round-trip
type E2EHarness struct {
	Client  pb.RatchetDaemonClient
	Conn    *grpc.ClientConn
	Svc     *Service
	DB      *sql.DB
	Secrets secrets.Provider
}

// newE2EHarness assembles a complete E2E harness and registers cleanup with t.
//
// The harness inserts one default "e2e-mock" provider (type "mock", no API key)
// so that sessions without a pinned provider resolve immediately. Tests can
// register additional providers via the Client RPC to cover more scenarios.
func newE2EHarness(t *testing.T) *E2EHarness {
	t.Helper()

	// In-memory SQLite with WAL and busy-timeout to match production settings.
	db, err := sql.Open("sqlite", ":memory:?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	// Enable foreign keys to match production behavior.
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	// initDB creates all tables AND runs the migration that clears stale
	// secret_name for keyless provider types (ollama, llama_cpp).
	if err := initDB(db); err != nil {
		t.Fatalf("initDB: %v", err)
	}

	// File-based SecretsProvider in an isolated temp directory.
	// This matches the production SecretsProvider type (not the in-memory stub
	// used by newTestEngine) so secret resolution errors surface in tests.
	secretsDir := filepath.Join(t.TempDir(), "secrets")
	if err := os.MkdirAll(secretsDir, 0700); err != nil {
		t.Fatalf("create secrets dir: %v", err)
	}
	secretProvider := secrets.NewFileProvider(secretsDir)

	// ProviderRegistry with all built-in factories including "mock", "ollama",
	// "llama_cpp", "anthropic", etc. Uses the real secrets provider so that
	// secret resolution failures are caught in tests.
	reg := ratchetplugin.NewProviderRegistry(db, secretProvider)

	// HookConfig: empty but non-nil so callers can register hooks in tests.
	hks := &hooks.HookConfig{Hooks: make(map[hooks.Event][]hooks.Hook)}

	// EngineContext wired to the in-memory DB and real secrets provider.
	engine := &EngineContext{
		DB:              db,
		ProviderRegistry: reg,
		ToolRegistry:    ratchetplugin.NewToolRegistry(),
		SecretsProvider: secretProvider,
		Hooks:           hks,
	}

	// MemoryStore for agent memory (needed by teams/fleet internals).
	engine.MemoryStore = ratchetplugin.NewMemoryStore(db)
	if err := engine.MemoryStore.InitTables(); err != nil {
		t.Fatalf("memory tables: %v", err)
	}

	// Assemble Service with all required fields — mirrors NewService but uses
	// the in-memory engine instead of loading from disk.
	svc := &Service{
		startedAt:    time.Now(),
		engine:       engine,
		sessions:     NewSessionManager(db),
		permGate:     newPermissionGate(),
		approvalGate: NewApprovalGate(),
		plans:        NewPlanManager(hks),
		tokens:       NewTokenTracker(),
		jobs:         NewJobRegistry(),
		broadcaster:  NewSessionBroadcaster(),
		meshBB:       mesh.NewBlackboard(),
		meshRouter:   mesh.NewRouter(),
	}
	svc.fleet = NewFleetManager(config.ModelRouting{}, engine, hks)
	svc.teams = NewTeamManager(engine, hks)

	// CronScheduler: starts background goroutines; cancel via context cleanup.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	svc.cron = NewCronScheduler(db, func(sessionID, command string) {
		go func() {
			tickCtx, tickCancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer tickCancel()
			ns := &noopSendServer{ctx: tickCtx}
			_ = svc.handleChat(tickCtx, sessionID, command, ns)
		}()
	})
	if err := svc.cron.Start(ctx); err != nil {
		t.Fatalf("start cron: %v", err)
	}

	// Job providers map job types to their managers (same as NewService).
	svc.jobs.Register("session", NewSessionJobProvider(svc.sessions))
	svc.jobs.Register("fleet_worker", NewFleetJobProvider(svc.fleet))
	svc.jobs.Register("team_agent", NewTeamJobProvider(svc.teams))
	svc.jobs.Register("cron", NewCronJobProvider(svc.cron))

	// Insert a default mock provider directly into the DB so sessions without
	// a pinned provider resolve without an extra RPC call.
	_, err = db.Exec(`INSERT INTO llm_providers
		(id, alias, type, model, secret_name, base_url, max_tokens, settings, is_default)
		VALUES ('e2e-mock-id', 'e2e-mock', 'mock', '', '', '', 4096, '{}', 1)`)
	if err != nil {
		t.Fatalf("insert default mock provider: %v", err)
	}

	// Start a real gRPC server on a random TCP port (reuses the helper from
	// mesh_stream_test.go which is in the same package).
	addr := startTestGRPCServer(t, svc)

	// Connect a real gRPC client to the server.
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial gRPC: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return &E2EHarness{
		Client:  pb.NewRatchetDaemonClient(conn),
		Conn:    conn,
		Svc:     svc,
		DB:      db,
		Secrets: secretProvider,
	}
}

// addProvider calls AddProvider via gRPC and fatally fails on error.
// Pass apiKey="" for keyless providers (e.g. ollama, llama_cpp simulated via mock).
func (h *E2EHarness) addProvider(t *testing.T, alias, provType, apiKey string, isDefault bool) *pb.Provider {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p, err := h.Client.AddProvider(ctx, &pb.AddProviderReq{
		Alias:     alias,
		Type:      provType,
		ApiKey:    apiKey,
		IsDefault: isDefault,
	})
	if err != nil {
		t.Fatalf("AddProvider alias=%q type=%q: %v", alias, provType, err)
	}
	return p
}

// createSession creates a session pinned to providerAlias and fatally fails on error.
func (h *E2EHarness) createSession(t *testing.T, providerAlias string) *pb.Session {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := h.Client.CreateSession(ctx, &pb.CreateSessionReq{
		WorkingDir: t.TempDir(),
		Provider:   providerAlias,
	})
	if err != nil {
		t.Fatalf("CreateSession provider=%q: %v", providerAlias, err)
	}
	return session
}

// sendMessage sends a user message and collects all streamed token content.
// Returns (concatenated token text, gotComplete).
func (h *E2EHarness) sendMessage(t *testing.T, sessionID, content string) (string, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	stream, err := h.Client.SendMessage(ctx, &pb.SendMessageReq{
		SessionId: sessionID,
		Content:   content,
	})
	if err != nil {
		t.Fatalf("SendMessage session=%q: %v", sessionID, err)
	}

	var buf strings.Builder
	var gotComplete bool
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("stream.Recv: %v", err)
		}
		switch e := ev.Event.(type) {
		case *pb.ChatEvent_Token:
			buf.WriteString(e.Token.Content)
		case *pb.ChatEvent_Complete:
			gotComplete = true
		case *pb.ChatEvent_Error:
			t.Errorf("error event: %s", e.Error.Message)
		}
	}
	return buf.String(), gotComplete
}

// secretExists reports whether the named secret is present in the file provider.
func (h *E2EHarness) secretExists(t *testing.T, secretName string) bool {
	t.Helper()
	names, err := h.Secrets.List(context.Background())
	if err != nil {
		t.Fatalf("list secrets: %v", err)
	}
	return slices.Contains(names, secretName)
}

// providerSecretName returns the secret_name column for a provider alias in the DB.
// Returns "" if the provider is not found.
func (h *E2EHarness) providerSecretName(t *testing.T, alias string) string {
	t.Helper()
	var secretName string
	err := h.DB.QueryRowContext(context.Background(),
		`SELECT secret_name FROM llm_providers WHERE alias = ?`, alias,
	).Scan(&secretName)
	if err == sql.ErrNoRows {
		return ""
	}
	if err != nil {
		t.Fatalf("query secret_name for %q: %v", alias, err)
	}
	return secretName
}
