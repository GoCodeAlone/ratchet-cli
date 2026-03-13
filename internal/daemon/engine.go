package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"path/filepath"

	"github.com/GoCodeAlone/ratchet-cli/internal/plugins"
	"github.com/GoCodeAlone/ratchet/ratchetplugin"
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
}

func NewEngineContext(ctx context.Context, dbPath string) (*EngineContext, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if err := initDB(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("init db: %w", err)
	}

	ec := &EngineContext{DB: db}

	// Memory store
	ec.MemoryStore = ratchetplugin.NewMemoryStore(db)
	if err := ec.MemoryStore.InitTables(); err != nil {
		db.Close()
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

	// Load external plugins from ~/.ratchet/plugins/
	pluginLoader := plugins.NewLoader(filepath.Join(DataDir(), "plugins"))
	loaded, err := pluginLoader.LoadAll()
	if err != nil {
		log.Printf("warning: plugin loading: %v", err)
	}
	for _, p := range loaded {
		log.Printf("loaded plugin: %s (%s)", p.Name, p.Path)
	}

	log.Println("engine context initialized")
	return ec, nil
}

func (ec *EngineContext) Close() {
	if ec.DB != nil {
		ec.DB.Close()
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
