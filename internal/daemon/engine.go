package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"path/filepath"

	"github.com/GoCodeAlone/ratchet-cli/internal/agent"
	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	"github.com/GoCodeAlone/ratchet-cli/internal/hooks"
	"github.com/GoCodeAlone/ratchet-cli/internal/mcp"
	"github.com/GoCodeAlone/ratchet-cli/internal/plugins"
	"github.com/GoCodeAlone/ratchet-cli/internal/skills"
	ratchetplugin "github.com/GoCodeAlone/workflow-plugin-agent/orchestrator"
	"github.com/GoCodeAlone/workflow/secrets"
	_ "modernc.org/sqlite"
)

// EngineContext holds the daemon's runtime services.
type EngineContext struct {
	DB               *sql.DB
	ProviderRegistry *ratchetplugin.ProviderRegistry
	ToolRegistry     *ratchetplugin.ToolRegistry
	MemoryStore      *ratchetplugin.MemoryStore
	SecretGuard      *ratchetplugin.SecretGuard
	SecretsProvider  secrets.Provider
	MCPDiscoverer    *mcp.Discoverer
	ModelRouting     config.ModelRouting
	Actors           *ActorManager
	Hooks            *hooks.HookConfig
	// Plugin-contributed capabilities
	PluginSkills   []skills.Skill
	PluginAgents   []agent.AgentDefinition
	PluginCommands []plugins.Command
	PluginDaemons  []*plugins.DaemonTool // stopped on Close()
}

func NewEngineContext(ctx context.Context, dbPath string) (*EngineContext, error) { //nolint:unparam

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if err := initDB(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init db: %w", err)
	}

	// Load config for model routing settings (non-fatal on error).
	cfg, _ := config.Load()
	ec := &EngineContext{DB: db}
	if cfg != nil {
		ec.ModelRouting = cfg.ModelRouting
	}

	// Memory store
	ec.MemoryStore = ratchetplugin.NewMemoryStore(db)
	if err := ec.MemoryStore.InitTables(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("memory tables: %w", err)
	}

	// Secret guard using file provider (writable, stored in ~/.ratchet/secrets/)
	secretsDir := filepath.Join(DataDir(), "secrets")
	if err := os.MkdirAll(secretsDir, 0700); err != nil {
		db.Close()
		return nil, fmt.Errorf("create secrets dir: %w", err)
	}
	secretProvider := secrets.NewFileProvider(secretsDir)
	ec.SecretGuard = ratchetplugin.NewSecretGuard(secretProvider, "file")
	ec.SecretsProvider = secretProvider

	// Provider registry
	ec.ProviderRegistry = ratchetplugin.NewProviderRegistry(db, secretProvider)

	// Tool registry
	ec.ToolRegistry = ratchetplugin.NewToolRegistry()

	// MCP CLI discovery (runs in background; errors are non-fatal)
	ec.MCPDiscoverer = mcp.NewDiscoverer(ec.ToolRegistry)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("mcp: discover panic: %v", r)
			}
		}()
		result := ec.MCPDiscoverer.Discover()
		for cli, tools := range result.Registered {
			log.Printf("mcp: discovered %s (%d tools)", cli, len(tools))
		}
	}()

	// Load external plugins from ~/.ratchet/plugins/ and wire capabilities.
	pluginLoader := plugins.NewLoader(filepath.Join(DataDir(), "plugins"))
	pluginResult, pluginErr := pluginLoader.LoadAll(ctx)
	if pluginErr != nil {
		log.Printf("warning: plugin loading: %v", pluginErr)
	} else {
		// Register plugin tools with the tool registry.
		for _, t := range pluginResult.Tools {
			ec.ToolRegistry.Register(t)
		}
		// Store skills, agents, commands for later query.
		ec.PluginSkills = pluginResult.Skills
		ec.PluginAgents = pluginResult.Agents
		ec.PluginCommands = pluginResult.Commands
		log.Printf("plugins: %d skills, %d agents, %d commands, %d tools loaded",
			len(pluginResult.Skills), len(pluginResult.Agents),
			len(pluginResult.Commands), len(pluginResult.Tools))
	}

	// Actor system (non-fatal on error; actors are optional middleware).
	actors, err := NewActorManager(ctx, db)
	if err != nil {
		log.Printf("warning: actor system init: %v", err)
	} else {
		ec.Actors = actors
	}

	// Hooks config (non-fatal; hooks are optional)
	ec.Hooks, _ = hooks.Load("")
	if ec.Hooks == nil {
		ec.Hooks = &hooks.HookConfig{Hooks: make(map[hooks.Event][]hooks.Hook)}
	}
	// Merge plugin-contributed hooks.
	if pluginErr == nil {
		for event, hookList := range pluginResult.Hooks.Hooks {
			ec.Hooks.Hooks[event] = append(ec.Hooks.Hooks[event], hookList...)
		}
	}

	log.Println("engine context initialized")
	return ec, nil
}

func (ec *EngineContext) Close() {
	// Stop plugin daemon tools.
	for _, d := range ec.PluginDaemons {
		if err := d.Stop(); err != nil {
			log.Printf("stop plugin daemon: %v", err)
		}
	}
	if ec.Actors != nil {
		_ = ec.Actors.Close(context.Background())
	}
	if ec.DB != nil {
		_ = ec.DB.Close()
	}
}

func initDB(db *sql.DB) error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			name TEXT,
			status TEXT DEFAULT 'active',
			working_dir TEXT,
			provider TEXT,
			model TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT,
			tool_name TEXT,
			tool_call_id TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		)`,
		`CREATE TABLE IF NOT EXISTS llm_providers (
			id TEXT PRIMARY KEY,
			alias TEXT UNIQUE NOT NULL,
			type TEXT NOT NULL,
			model TEXT,
			secret_name TEXT,
			base_url TEXT,
			max_tokens INTEGER DEFAULT 4096,
			settings TEXT NOT NULL DEFAULT '{}',
			is_default INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS permissions (
			tool_name TEXT NOT NULL,
			scope TEXT NOT NULL,
			allowed INTEGER NOT NULL,
			session_id TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS cron_jobs (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			schedule TEXT NOT NULL,
			command TEXT NOT NULL,
			status TEXT DEFAULT 'active',
			last_run TEXT,
			next_run TEXT,
			run_count INTEGER DEFAULT 0
		)`,
	}
	for _, ddl := range tables {
		if _, err := db.Exec(ddl); err != nil {
			return fmt.Errorf("exec DDL: %w", err)
		}
	}
	return nil
}
