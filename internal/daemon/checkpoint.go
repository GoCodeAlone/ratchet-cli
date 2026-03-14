package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Checkpoint captures daemon state for graceful reload.
// The actual message history lives in SQLite; the checkpoint records only
// what needs active resumption so the new daemon knows what to restart.
type Checkpoint struct {
	Version   string               `json:"version"`
	Sessions  []SessionCheckpoint  `json:"sessions"`
	CronJobs  []CronCheckpoint     `json:"cron_jobs"`
	Providers []ProviderCheckpoint `json:"providers"`
}

// SessionCheckpoint records an active session to resume after reload.
type SessionCheckpoint struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	WorkingDir string `json:"working_dir"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Status     string `json:"status"`
}

// CronCheckpoint records a cron job that should be resumed after reload.
type CronCheckpoint struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Schedule  string `json:"schedule"`
	Command   string `json:"command"`
	Status    string `json:"status"`
}

// ProviderCheckpoint records a configured provider alias.
type ProviderCheckpoint struct {
	Alias string `json:"alias"`
	Type  string `json:"type"`
	Model string `json:"model"`
}

// CheckpointPath returns the path to the checkpoint file.
// It is a variable so tests can override it.
var CheckpointPath = func() string {
	return filepath.Join(DataDir(), "checkpoint.json")
}

// ExportCheckpoint reads active state from svc and returns a Checkpoint.
func ExportCheckpoint(svc *Service) (*Checkpoint, error) {
	cp := &Checkpoint{
		Version: currentVersion(),
	}

	// Collect active sessions
	ctx := context.Background()
	sessions, err := svc.sessions.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	for _, s := range sessions {
		if s.Status == "active" || s.Status == "background" {
			cp.Sessions = append(cp.Sessions, SessionCheckpoint{
				ID:         s.ID,
				Name:       s.Name,
				WorkingDir: s.WorkingDir,
				Provider:   s.Provider,
				Model:      s.Model,
				Status:     s.Status,
			})
		}
	}

	// Collect active cron jobs
	cronJobs, err := svc.cron.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cron jobs: %w", err)
	}
	for _, j := range cronJobs {
		if j.Status == "active" {
			cp.CronJobs = append(cp.CronJobs, CronCheckpoint{
				ID:        j.ID,
				SessionID: j.SessionID,
				Schedule:  j.Schedule,
				Command:   j.Command,
				Status:    j.Status,
			})
		}
	}

	// Collect providers from DB
	rows, err := svc.engine.DB.QueryContext(ctx,
		`SELECT alias, type, model FROM llm_providers ORDER BY alias`)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p ProviderCheckpoint
		if err := rows.Scan(&p.Alias, &p.Type, &p.Model); err != nil {
			return nil, fmt.Errorf("scan provider: %w", err)
		}
		cp.Providers = append(cp.Providers, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate providers: %w", err)
	}

	return cp, nil
}

// SaveCheckpoint writes cp to the checkpoint file (~/.ratchet/checkpoint.json).
func SaveCheckpoint(cp *Checkpoint) error {
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}
	if err := os.WriteFile(CheckpointPath(), data, 0600); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}
	return nil
}

// LoadCheckpoint reads the checkpoint file. Returns an error if the file does
// not exist or cannot be parsed.
func LoadCheckpoint() (*Checkpoint, error) {
	data, err := os.ReadFile(CheckpointPath())
	if err != nil {
		return nil, err
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("parse checkpoint: %w", err)
	}
	return &cp, nil
}

// currentVersion returns the running binary version string.
// It defers to the version package via a package-level var so tests can
// substitute a value without importing the version package here.
var currentVersion = func() string {
	return daemonVersion
}

// daemonVersion is set during daemon startup from the version package.
var daemonVersion = "dev"
