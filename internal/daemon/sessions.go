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
	ID                  string
	Name                string
	Status              string // active, background, completed
	WorkingDir          string
	Provider            string
	Model               string
	ParentID            string
	RootID              string
	ForkedFromMessageID string
	ForkReason          string
	BranchSummary       string
	CreatedAt           time.Time
	Agents              int
}

type SessionHistoryMessage struct {
	ID         string
	Role       string
	Content    string
	ToolName   string
	ToolCallID string
	CreatedAt  time.Time
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
		`INSERT INTO sessions (id, name, status, working_dir, provider, model, root_id, branch_summary) VALUES (?, ?, 'active', ?, ?, ?, ?, ?)`,
		id, name, workingDir, provider, model, id, initialPrompt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}

	sm.mu.Lock()
	sm.workspaces[workingDir] = append(sm.workspaces[workingDir], id)
	sm.mu.Unlock()

	return &SessionInfo{
		ID:            id,
		Name:          name,
		Status:        "active",
		WorkingDir:    workingDir,
		Provider:      provider,
		Model:         model,
		RootID:        id,
		BranchSummary: initialPrompt,
		CreatedAt:     time.Now(),
	}, nil
}

func (sm *SessionManager) List(ctx context.Context) ([]SessionInfo, error) {
	rows, err := sm.db.QueryContext(ctx,
		`SELECT id, name, status, working_dir, provider, model,
		 COALESCE(parent_id, ''), COALESCE(NULLIF(root_id, ''), id), COALESCE(forked_from_message_id, ''), COALESCE(fork_reason, ''), COALESCE(branch_summary, ''),
		 created_at
		 FROM sessions ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []SessionInfo
	for rows.Next() {
		var s SessionInfo
		if err := rows.Scan(
			&s.ID, &s.Name, &s.Status, &s.WorkingDir, &s.Provider, &s.Model,
			&s.ParentID, &s.RootID, &s.ForkedFromMessageID, &s.ForkReason, &s.BranchSummary,
			&s.CreatedAt,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func (sm *SessionManager) Get(ctx context.Context, id string) (*SessionInfo, error) {
	var s SessionInfo
	err := sm.db.QueryRowContext(ctx,
		`SELECT id, name, status, working_dir, provider, model,
		 COALESCE(parent_id, ''), COALESCE(NULLIF(root_id, ''), id), COALESCE(forked_from_message_id, ''), COALESCE(fork_reason, ''), COALESCE(branch_summary, ''),
		 created_at
		 FROM sessions WHERE id = ?`, id,
	).Scan(
		&s.ID, &s.Name, &s.Status, &s.WorkingDir, &s.Provider, &s.Model,
		&s.ParentID, &s.RootID, &s.ForkedFromMessageID, &s.ForkReason, &s.BranchSummary,
		&s.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (sm *SessionManager) ListMessages(ctx context.Context, sessionID string) ([]SessionHistoryMessage, error) {
	if _, err := sm.Get(ctx, sessionID); err != nil {
		return nil, err
	}
	rows, err := sm.db.QueryContext(ctx,
		`SELECT id, role, content, tool_name, tool_call_id, created_at
		 FROM messages
		 WHERE session_id = ?
		 ORDER BY rowid`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []SessionHistoryMessage
	for rows.Next() {
		var m SessionHistoryMessage
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.ToolName, &m.ToolCallID, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// Clone creates a new child session with all visible messages copied from the source.
func (sm *SessionManager) Clone(ctx context.Context, sourceID, reason string) (*SessionInfo, error) {
	source, err := sm.Get(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	return sm.createChild(ctx, source, "", reason, 0)
}

// Fork creates a new child session with messages copied through messageID.
func (sm *SessionManager) Fork(ctx context.Context, sourceID, messageID, reason string) (*SessionInfo, error) {
	source, err := sm.Get(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	var cutoffRowID int64
	if err := sm.db.QueryRowContext(ctx,
		`SELECT rowid FROM messages WHERE session_id = ? AND id = ?`,
		sourceID, messageID,
	).Scan(&cutoffRowID); err != nil {
		return nil, err
	}
	return sm.createChild(ctx, source, messageID, reason, cutoffRowID)
}

// ListTree returns the root session and all descendants sharing the same root.
func (sm *SessionManager) ListTree(ctx context.Context, rootOrSessionID string) ([]SessionInfo, error) {
	session, err := sm.Get(ctx, rootOrSessionID)
	if err != nil {
		return nil, err
	}
	rootID := session.RootID
	if rootID == "" {
		rootID = session.ID
	}
	rows, err := sm.db.QueryContext(ctx,
		`SELECT id, name, status, working_dir, provider, model,
		 COALESCE(parent_id, ''), COALESCE(NULLIF(root_id, ''), id), COALESCE(forked_from_message_id, ''), COALESCE(fork_reason, ''), COALESCE(branch_summary, ''),
		 created_at
		 FROM sessions
		 WHERE id = ? OR root_id = ?
		 ORDER BY created_at, id`,
		rootID, rootID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []SessionInfo
	for rows.Next() {
		var s SessionInfo
		if err := rows.Scan(
			&s.ID, &s.Name, &s.Status, &s.WorkingDir, &s.Provider, &s.Model,
			&s.ParentID, &s.RootID, &s.ForkedFromMessageID, &s.ForkReason, &s.BranchSummary,
			&s.CreatedAt,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func (sm *SessionManager) createChild(ctx context.Context, source *SessionInfo, forkedFromMessageID, reason string, cutoffRowID int64) (*SessionInfo, error) {
	id := uuid.New().String()
	name := source.Name
	if name == "" {
		name = "new-session"
	}
	rootID := source.RootID
	if rootID == "" {
		rootID = source.ID
	}

	tx, err := sm.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO sessions (id, name, status, working_dir, provider, model, parent_id, root_id, forked_from_message_id, fork_reason)
		 VALUES (?, ?, 'active', ?, ?, ?, ?, ?, ?, ?)`,
		id, name, source.WorkingDir, source.Provider, source.Model, source.ID, rootID, forkedFromMessageID, reason,
	); err != nil {
		return nil, fmt.Errorf("insert child session: %w", err)
	}

	query := `SELECT role, content, tool_name, tool_call_id FROM messages WHERE session_id = ? ORDER BY rowid`
	args := []any{source.ID}
	if cutoffRowID > 0 {
		query = `SELECT role, content, tool_name, tool_call_id FROM messages WHERE session_id = ? AND rowid <= ? ORDER BY rowid`
		args = append(args, cutoffRowID)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select source messages: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var role, content, toolName, toolCallID string
		if err := rows.Scan(&role, &content, &toolName, &toolCallID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO messages (id, session_id, role, content, tool_name, tool_call_id) VALUES (?, ?, ?, ?, ?, ?)`,
			uuid.New().String(), id, role, content, toolName, toolCallID,
		); err != nil {
			return nil, fmt.Errorf("copy message: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	sm.mu.Lock()
	sm.workspaces[source.WorkingDir] = append(sm.workspaces[source.WorkingDir], id)
	sm.mu.Unlock()

	return &SessionInfo{
		ID:                  id,
		Name:                name,
		Status:              "active",
		WorkingDir:          source.WorkingDir,
		Provider:            source.Provider,
		Model:               source.Model,
		ParentID:            source.ID,
		RootID:              rootID,
		ForkedFromMessageID: forkedFromMessageID,
		ForkReason:          reason,
		CreatedAt:           time.Now(),
	}, nil
}

func (sm *SessionManager) UpdateModel(ctx context.Context, id, model string) error {
	result, err := sm.db.ExecContext(ctx, `UPDATE sessions SET model = ? WHERE id = ?`, model, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
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
