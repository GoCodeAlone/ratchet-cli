package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type SessionInfo struct {
	ID         string
	Name       string
	Status     string // active, background, completed
	WorkingDir string
	Provider   string
	Model      string
	CreatedAt  time.Time
	Agents     int
}

type SessionManager struct {
	db          *sql.DB
	mu          sync.RWMutex
	subscribers map[string][]chan any
	workspaces  map[string][]string
}

func NewSessionManager(db *sql.DB) *SessionManager {
	return &SessionManager{
		db:          db,
		subscribers: make(map[string][]chan any),
		workspaces:  make(map[string][]string),
	}
}

func (sm *SessionManager) Create(ctx context.Context, workingDir, provider, model, initialPrompt string) (*SessionInfo, error) {
	id := uuid.New().String()
	name := generateSessionName(initialPrompt)

	_, err := sm.db.ExecContext(ctx,
		`INSERT INTO sessions (id, name, status, working_dir, provider, model) VALUES (?, ?, 'active', ?, ?, ?)`,
		id, name, workingDir, provider, model,
	)
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}

	sm.mu.Lock()
	sm.workspaces[workingDir] = append(sm.workspaces[workingDir], id)
	sm.mu.Unlock()

	return &SessionInfo{
		ID:         id,
		Name:       name,
		Status:     "active",
		WorkingDir: workingDir,
		Provider:   provider,
		Model:      model,
		CreatedAt:  time.Now(),
	}, nil
}

func (sm *SessionManager) List(ctx context.Context) ([]SessionInfo, error) {
	rows, err := sm.db.QueryContext(ctx,
		`SELECT id, name, status, working_dir, provider, model, created_at FROM sessions ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []SessionInfo
	for rows.Next() {
		var s SessionInfo
		if err := rows.Scan(&s.ID, &s.Name, &s.Status, &s.WorkingDir, &s.Provider, &s.Model, &s.CreatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func (sm *SessionManager) Get(ctx context.Context, id string) (*SessionInfo, error) {
	var s SessionInfo
	err := sm.db.QueryRowContext(ctx,
		`SELECT id, name, status, working_dir, provider, model, created_at FROM sessions WHERE id = ?`, id,
	).Scan(&s.ID, &s.Name, &s.Status, &s.WorkingDir, &s.Provider, &s.Model, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (sm *SessionManager) Kill(ctx context.Context, id string) error {
	_, err := sm.db.ExecContext(ctx, `UPDATE sessions SET status = 'completed' WHERE id = ?`, id)
	if err != nil {
		return err
	}
	sm.mu.Lock()
	delete(sm.subscribers, id)
	for dir, ids := range sm.workspaces {
		filtered := ids[:0]
		for _, sid := range ids {
			if sid != id {
				filtered = append(filtered, sid)
			}
		}
		if len(filtered) == 0 {
			delete(sm.workspaces, dir)
		} else {
			sm.workspaces[dir] = filtered
		}
	}
	sm.mu.Unlock()
	return nil
}

func (sm *SessionManager) SetBackground(ctx context.Context, id string) error {
	_, err := sm.db.ExecContext(ctx, `UPDATE sessions SET status = 'background' WHERE id = ?`, id)
	return err
}

// CleanupStale marks sessions older than maxAge as completed.
// Called on daemon startup to prevent indefinite accumulation.
func (sm *SessionManager) CleanupStale(ctx context.Context, maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge).Format(time.RFC3339)
	result, err := sm.db.ExecContext(ctx,
		`UPDATE sessions SET status = 'completed' WHERE status = 'active' AND created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Subscribe returns a channel for receiving session events.
func (sm *SessionManager) Subscribe(sessionID string) chan any {
	ch := make(chan any, 64)
	sm.mu.Lock()
	sm.subscribers[sessionID] = append(sm.subscribers[sessionID], ch)
	sm.mu.Unlock()
	return ch
}

// Publish sends an event to all subscribers of a session.
func (sm *SessionManager) Publish(sessionID string, event any) {
	sm.mu.RLock()
	subs := sm.subscribers[sessionID]
	sm.mu.RUnlock()
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// drop if subscriber is slow
		}
	}
}

// SessionsInDir returns session IDs operating in the same directory.
func (sm *SessionManager) SessionsInDir(dir string) []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.workspaces[dir]
}

func generateSessionName(prompt string) string {
	if prompt == "" {
		return "new-session"
	}
	name := prompt
	if len(name) > 30 {
		name = name[:30]
	}
	result := make([]byte, 0, len(name))
	for _, c := range name {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' {
			result = append(result, byte(c))
		} else if c >= 'A' && c <= 'Z' {
			result = append(result, byte(c+32))
		} else if c == ' ' {
			result = append(result, '-')
		}
	}
	return string(result)
}
