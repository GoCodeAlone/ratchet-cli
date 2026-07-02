package daemon

import (
	"context"
	"database/sql"
	"log"

	"github.com/GoCodeAlone/ratchet-cli/internal/hooks"
)

// RunHooks runs startup hooks plus workdir-scoped project hooks for the event.
// Project hooks are only loaded when the event data carries a working_dir or a
// session_id that resolves to one.
func (ec *EngineContext) RunHooks(ctx context.Context, event hooks.Event, data map[string]string) error {
	if ec == nil {
		return nil
	}
	store, err := hooks.LoadTrustStore(hooks.DefaultTrustStorePath())
	if err != nil {
		log.Printf("hooks: load trust store: %v", err)
		store = nil
	}

	cfg := &hooks.HookConfig{Hooks: make(map[hooks.Event][]hooks.Hook)}
	if ec.Hooks != nil {
		cfg.Hooks[event] = append(cfg.Hooks[event], ec.Hooks.Hooks[event]...)
	}
	if workDir := ec.hookWorkingDir(ctx, data); workDir != "" {
		projectCfg, err := hooks.LoadWithOptions(hooks.LoadOptions{
			WorkingDir: workDir,
			TrustStore: store,
			SkipUser:   true,
		})
		if err != nil {
			log.Printf("hooks: load project hooks for %s: %v", workDir, err)
		} else {
			cfg.Hooks[event] = append(cfg.Hooks[event], projectCfg.Hooks[event]...)
		}
	}
	cfg.ApplyTrust(store)
	return cfg.Run(event, data)
}

func runHooksAndLog(ctx context.Context, engine *EngineContext, event hooks.Event, data map[string]string, label string) {
	if engine == nil {
		return
	}
	if err := engine.RunHooks(ctx, event, data); err != nil {
		log.Printf("hooks: %s %s failed: %v", label, event, err)
	}
}

func (ec *EngineContext) hookWorkingDir(ctx context.Context, data map[string]string) string {
	if data != nil {
		if workDir := data["working_dir"]; workDir != "" {
			return workDir
		}
		if sessionID := data["session_id"]; sessionID != "" && ec.DB != nil {
			var workDir string
			err := ec.DB.QueryRowContext(ctx, `SELECT working_dir FROM sessions WHERE id = ?`, sessionID).Scan(&workDir)
			if err == nil {
				return workDir
			}
			if err != sql.ErrNoRows {
				log.Printf("hooks: lookup session workdir session=%s: %v", sessionID, err)
			}
		}
	}
	return ""
}
