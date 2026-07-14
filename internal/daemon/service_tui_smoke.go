//go:build tui_smoke

package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/hooks"
	"github.com/GoCodeAlone/ratchet-cli/internal/mesh"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	ratchetplugin "github.com/GoCodeAlone/workflow-plugin-agent/orchestrator"
	"github.com/GoCodeAlone/workflow-plugin-agent/policy"
	"github.com/GoCodeAlone/workflow/secrets"
	_ "modernc.org/sqlite"
)

func newTUISmokeService(ctx context.Context, tempRoot string) (*Service, error) {
	if err := os.MkdirAll(tempRoot, 0700); err != nil {
		return nil, fmt.Errorf("create smoke root: %w", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(tempRoot, "ratchet.db")+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
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

	secretsDir := filepath.Join(tempRoot, "secrets")
	if err := os.MkdirAll(secretsDir, 0700); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create smoke secrets directory: %w", err)
	}
	secretProvider := secrets.NewFileProvider(secretsDir)
	redactor := secrets.NewRedactor()
	if err := redactor.LoadFromProvider(ctx, secretProvider); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("load smoke secrets redactor: %w", err)
	}
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
	providerOps := newProviderOperationManager(engine)
	if err := providerOps.Start(ctx); err != nil {
		engine.Close()
		return nil, fmt.Errorf("start smoke provider operations: %w", err)
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
		providerOps:      providerOps,
		acpBackground:    disabledACPBackgroundDrainManager{},
	}
	trustStore, err := policy.NewPermissionStore(engine.DB)
	if err != nil {
		providerOps.Stop()
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
	s.Close()
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
