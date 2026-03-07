package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"log"

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
}

func NewEngineContext(ctx context.Context, dbPath string) (*EngineContext, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)

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

	// Secret guard using env provider
	secretProvider := secrets.NewEnvProvider("")
	ec.SecretGuard = ratchetplugin.NewSecretGuard(secretProvider, "env")

	// Provider registry
	ec.ProviderRegistry = ratchetplugin.NewProviderRegistry(db, secretProvider)

	// Tool registry
	ec.ToolRegistry = ratchetplugin.NewToolRegistry()

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
			is_default INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS permissions (
			tool_name TEXT NOT NULL,
			scope TEXT NOT NULL,
			allowed INTEGER NOT NULL,
			session_id TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, ddl := range tables {
		if _, err := db.Exec(ddl); err != nil {
			return fmt.Errorf("exec DDL: %w", err)
		}
	}
	return nil
}
