package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"path/filepath"

	"github.com/GoCodeAlone/ratchet-cli/internal/agent"
	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	"github.com/GoCodeAlone/ratchet-cli/internal/hooks"
	"github.com/GoCodeAlone/ratchet-cli/internal/mcp"
	"github.com/GoCodeAlone/ratchet-cli/internal/plugins"
	"github.com/GoCodeAlone/ratchet-cli/internal/skills"
	"github.com/GoCodeAlone/workflow-plugin-agent/executor"
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
	SecretsProvider  secrets.Provider
	SecretsRedactor  *secrets.Redactor
	SecretRedactor   executor.SecretRedactor
	MCPDiscoverer    *mcp.Discoverer
	ModelRouting     config.ModelRouting
	Actors           *ActorManager
	Hooks            *hooks.HookConfig
	Debug            bool // enable request/response debug logging to ~/.ratchet/debug.log
	ExtensionMu      sync.RWMutex
	ProviderRowsMu   sync.Mutex
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
	ec.SecretsProvider = secretProvider
	ec.SecretsRedactor = secrets.NewRedactor()
	if err := ec.SecretsRedactor.LoadFromProvider(ctx, secretProvider); err != nil {
		db.Close()
		return nil, fmt.Errorf("load secrets redactor: %w", err)
	}
	ec.SecretRedactor = newEngineSecretRedactor(ec.SecretsRedactor)

	// Provider registry
	ec.ProviderRegistry = ratchetplugin.NewProviderRegistry(db, func() secrets.Provider {
		return secretProvider
	})

	// Tool registry and MCP discovery are populated by plugin reload.
	ec.ToolRegistry = ratchetplugin.NewToolRegistry()
	ec.MCPDiscoverer = mcp.NewDiscoverer(ec.ToolRegistry)

	// Actor system (non-fatal on error; actors are optional middleware).
	actors, err := NewActorManager(ctx, db)
	if err != nil {
		log.Printf("warning: actor system init: %v", err)
	} else {
		ec.Actors = actors
	}

	if summary, err := ec.ReloadPlugins(ctx); err != nil {
		log.Printf("warning: plugin loading: %v", err)
	} else {
		log.Printf("plugins: %d skills, %d agents, %d commands, %d tools loaded",
			summary.Skills, summary.Agents, summary.Commands, summary.Tools)
	}

	log.Println("engine context initialized")
	return ec, nil
}

type PluginReloadSummary struct {
	Skills   int
	Agents   int
	Commands int
	Tools    int
	Hooks    int
	Daemons  int
}

func (ec *EngineContext) ReloadPlugins(ctx context.Context) (*PluginReloadSummary, error) {
	pluginLoader := plugins.NewLoader(filepath.Join(DataDir(), "plugins"))
	pluginResult, err := pluginLoader.LoadAll(ctx)
	if err != nil {
		return nil, err
	}

	newRegistry := ratchetplugin.NewToolRegistry()
	for _, t := range pluginResult.Tools {
		newRegistry.Register(t)
	}
	newDiscoverer := mcp.NewDiscoverer(newRegistry)
	discoverMCP(newDiscoverer)

	hookConfig, _ := hooks.LoadWithOptions(hooks.LoadOptions{SkipProject: true})
	if hookConfig == nil {
		hookConfig = &hooks.HookConfig{Hooks: make(map[hooks.Event][]hooks.Hook)}
	}
	if pluginResult.Hooks != nil {
		for event, hookList := range pluginResult.Hooks.Hooks {
			hookConfig.Hooks[event] = append(hookConfig.Hooks[event], hookList...)
		}
	}

	hookCount := 0
	for _, hookList := range hookConfig.Hooks {
		hookCount += len(hookList)
	}

	ec.ExtensionMu.Lock()
	oldDaemons := ec.PluginDaemons
	ec.ToolRegistry = newRegistry
	ec.MCPDiscoverer = newDiscoverer
	ec.PluginSkills = pluginResult.Skills
	ec.PluginAgents = pluginResult.Agents
	ec.PluginCommands = pluginResult.Commands
	ec.PluginDaemons = pluginResult.Daemons
	ec.Hooks = hookConfig
	ec.ExtensionMu.Unlock()

	for _, d := range oldDaemons {
		if err := d.Stop(); err != nil {
			log.Printf("stop old plugin daemon: %v", err)
		}
	}

	return &PluginReloadSummary{
		Skills:   len(pluginResult.Skills),
		Agents:   len(pluginResult.Agents),
		Commands: len(pluginResult.Commands),
		Tools:    len(pluginResult.Tools),
		Hooks:    hookCount,
		Daemons:  len(pluginResult.Daemons),
	}, nil
}

func discoverMCP(discoverer *mcp.Discoverer) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("mcp: discover panic: %v", r)
		}
	}()
	result := discoverer.Discover()
	for cli, tools := range result.Registered {
		log.Printf("mcp: discovered %s (%d tools)", cli, len(tools))
	}
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
			parent_id TEXT,
			root_id TEXT,
			forked_from_message_id TEXT,
			fork_reason TEXT,
			branch_summary TEXT,
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
		`CREATE TABLE IF NOT EXISTS session_compactions (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			summary TEXT NOT NULL,
			reason TEXT NOT NULL,
			messages_removed INTEGER NOT NULL,
			messages_kept INTEGER NOT NULL,
			first_kept_message_id TEXT,
			archive_session_id TEXT,
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
		`CREATE TABLE IF NOT EXISTS provider_operations (
			operation_id TEXT PRIMARY KEY,
			alias TEXT NOT NULL,
			state TEXT NOT NULL,
			failure TEXT NOT NULL DEFAULT '',
			secret_name TEXT NOT NULL DEFAULT '',
			result_type TEXT NOT NULL DEFAULT '',
			result_model TEXT NOT NULL DEFAULT '',
			result_is_default INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS provider_operations_alias_state
			ON provider_operations(alias, state)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS provider_operations_active_alias
			ON provider_operations(alias) WHERE state IN ('pending', 'applied')`,
		`CREATE UNIQUE INDEX IF NOT EXISTS provider_operations_reserved_secret
			ON provider_operations(secret_name) WHERE secret_name != ''`,
		`CREATE TABLE IF NOT EXISTS provider_secret_cleanup (
			secret_name TEXT PRIMARY KEY,
			attempt_count INTEGER NOT NULL DEFAULT 0,
			failure TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			next_attempt_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS provider_secret_cleanup_due
			ON provider_secret_cleanup(next_attempt_at, created_at)`,
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

	if err := ensureColumn(db, "llm_providers", "settings", "TEXT NOT NULL DEFAULT '{}'"); err != nil {
		log.Printf("warning: migration failed: %v", err)
	}
	for _, col := range []string{"parent_id", "root_id", "forked_from_message_id", "fork_reason", "branch_summary"} {
		if err := ensureColumn(db, "sessions", col, "TEXT"); err != nil {
			log.Printf("warning: migration failed: %v", err)
		}
	}
	if err := ensureColumn(db, "session_compactions", "archive_session_id", "TEXT"); err != nil {
		log.Printf("warning: migration failed: %v", err)
	}
	// Migration: clear stale secret_name for providers that don't need API keys.
	// Prior versions always set secret_name="provider_<alias>" even for keyless
	// providers (ollama, llama_cpp), causing "secret not found" errors.
	migrations := []string{
		`UPDATE sessions SET root_id = id WHERE root_id IS NULL OR root_id = ''`,
		`UPDATE llm_providers SET secret_name = '' WHERE secret_name != '' AND type IN ('ollama', 'llama_cpp')`,
	}
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			log.Printf("warning: migration failed: %v", err)
		}
	}

	return nil
}

func ensureColumn(db *sql.DB, table, name, definition string) error {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", sqliteQuoteIdentifier(table)))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var columnName, typ string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &columnName, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if columnName == name {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.Exec(fmt.Sprintf(
		"ALTER TABLE %s ADD COLUMN %s %s",
		sqliteQuoteIdentifier(table),
		sqliteQuoteIdentifier(name),
		definition,
	))
	return err
}

func sqliteQuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
