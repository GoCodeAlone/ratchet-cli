//go:build tui_smoke && !windows

package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/GoCodeAlone/ratchet-cli/internal/hooks"
	"github.com/GoCodeAlone/ratchet-cli/internal/mesh"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	ratchetplugin "github.com/GoCodeAlone/workflow-plugin-agent/orchestrator"
	"github.com/GoCodeAlone/workflow-plugin-agent/policy"
	"github.com/GoCodeAlone/workflow/secrets"
	_ "modernc.org/sqlite"
)

// StartTUISmokeDaemon starts the private daemon used by ratchet-tui-smoke.
func StartTUISmokeDaemon(ctx context.Context, tempRoot, socketPath string) (*pb.Session, func(), error) {
	svc, err := newTUISmokeService(ctx, tempRoot)
	if err != nil {
		return nil, func() {}, err
	}

	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		svc.close()
		return nil, func() {}, fmt.Errorf("listen smoke socket: %w", err)
	}
	if err := os.Chmod(socketPath, 0600); err != nil {
		_ = lis.Close()
		svc.close()
		return nil, func() {}, fmt.Errorf("chmod smoke socket: %w", err)
	}

	server := grpc.NewServer()
	pb.RegisterRatchetDaemonServer(server, svc)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.Serve(lis)
	}()

	conn, err := grpc.NewClient("unix://"+socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		server.GracefulStop()
		<-done
		_ = lis.Close()
		svc.close()
		return nil, func() {}, fmt.Errorf("connect smoke setup client: %w", err)
	}
	setupClient := pb.NewRatchetDaemonClient(conn)
	if _, err := setupClient.AddProvider(ctx, &pb.AddProviderReq{
		Alias:     "e2e-mock",
		Type:      "mock",
		IsDefault: true,
	}); err != nil {
		_ = conn.Close()
		server.GracefulStop()
		<-done
		_ = lis.Close()
		svc.close()
		return nil, func() {}, fmt.Errorf("add smoke provider: %w", err)
	}
	session, err := setupClient.CreateSession(ctx, &pb.CreateSessionReq{
		WorkingDir: tempRoot,
		Provider:   "e2e-mock",
	})
	_ = conn.Close()
	if err != nil {
		server.GracefulStop()
		<-done
		_ = lis.Close()
		svc.close()
		return nil, func() {}, fmt.Errorf("create smoke session: %w", err)
	}

	cleanup := func() {
		server.GracefulStop()
		<-done
		_ = lis.Close()
		svc.close()
		_ = os.Remove(socketPath)
	}
	return session, cleanup, nil
}

func newTUISmokeService(ctx context.Context, tempRoot string) (*Service, error) {
	db, err := sql.Open("sqlite", ":memory:?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open smoke db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable smoke foreign keys: %w", err)
	}
	if err := initDB(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init smoke db: %w", err)
	}

	secretProvider := smokeSecretsProvider{}
	redactor := secrets.NewRedactor()
	_ = redactor.LoadFromProvider(ctx, secretProvider)
	engine := &EngineContext{
		DB:               db,
		ProviderRegistry: ratchetplugin.NewProviderRegistry(db, func() secrets.Provider { return secretProvider }),
		ToolRegistry:     ratchetplugin.NewToolRegistry(),
		SecretsProvider:  secretProvider,
		SecretsRedactor:  redactor,
		SecretRedactor:   newEngineSecretRedactor(redactor),
		Hooks:            &hooks.HookConfig{Hooks: map[hooks.Event][]hooks.Hook{}},
	}
	engine.MemoryStore = ratchetplugin.NewMemoryStore(db)
	if err := engine.MemoryStore.InitTables(); err != nil {
		engine.Close()
		return nil, fmt.Errorf("init smoke memory tables: %w", err)
	}
	svc := &Service{
		startedAt:        time.Now(),
		engine:           engine,
		sessions:         NewSessionManager(db),
		permGate:         newPermissionGate(),
		approvalGate:     NewApprovalGate(),
		plans:            NewPlanManager(engine.Hooks),
		tokens:           NewTokenTracker(),
		jobs:             NewJobRegistry(),
		broadcaster:      NewSessionBroadcaster(),
		meshBB:           mesh.NewBlackboard(),
		meshRouter:       mesh.NewRouter(),
		humanGate:        NewHumanGate(),
		projects:         NewProjectRegistry(),
		trustMode:        "conservative",
		trustDefaultMode: "conservative",
		trustEngine:      policy.NewTrustEngine("conservative", nil, nil),
	}
	trustStore, err := policy.NewPermissionStore(engine.DB)
	if err != nil {
		engine.Close()
		return nil, fmt.Errorf("init smoke trust store: %w", err)
	}
	svc.trustStore = trustStore
	svc.trustEngine.SetPermissionStore(trustStore)
	svc.fleet = NewFleetManager(engine.ModelRouting, engine, engine.Hooks)
	svc.teams = NewTeamManager(engine, engine.Hooks)
	if os.Getenv("RATCHET_TUI_SMOKE_EMPTY_JOBS") != "1" {
		svc.jobs.Register("session", NewSessionJobProvider(svc.sessions))
		svc.jobs.Register("fleet_worker", NewFleetJobProvider(svc.fleet))
		svc.jobs.Register("team_agent", NewTeamJobProvider(svc.teams))
		svc.jobs.Register("smoke", smokeJobProvider{root: tempRoot})
	}
	return svc, nil
}

func newTUISmokeServiceForTest(t interface {
	Cleanup(func())
	Fatalf(string, ...any)
}, tempRoot string) *Service {
	svc, err := newTUISmokeService(context.Background(), tempRoot)
	if err != nil {
		t.Fatalf("newTUISmokeService: %v", err)
	}
	t.Cleanup(svc.close)
	return svc
}

func (s *Service) close() {
	if s.engine != nil {
		s.engine.Close()
	}
}

type smokeJobProvider struct {
	root string
}

func (p smokeJobProvider) ActiveJobs() []*pb.Job {
	return []*pb.Job{{
		Id:     "smoke:ready",
		Type:   "smoke",
		Name:   "tui-smoke-daemon",
		Status: "ready",
		Metadata: map[string]string{
			"root": p.root,
		},
	}}
}

func (p smokeJobProvider) PauseJob(string) error {
	return fmt.Errorf("smoke jobs cannot be paused")
}

func (p smokeJobProvider) ResumeJob(string) error {
	return fmt.Errorf("smoke jobs cannot be resumed")
}

func (p smokeJobProvider) KillJob(string) error {
	return fmt.Errorf("smoke jobs cannot be killed")
}

type smokeSecretsProvider struct{}

func (smokeSecretsProvider) Name() string { return "smoke" }

func (smokeSecretsProvider) Get(context.Context, string) (string, error) { return "", nil }

func (smokeSecretsProvider) Set(context.Context, string, string) error { return nil }

func (smokeSecretsProvider) Delete(context.Context, string) error { return nil }

func (smokeSecretsProvider) List(context.Context) ([]string, error) { return nil, nil }

var _ secrets.Provider = smokeSecretsProvider{}
