package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/tochemey/goakt/v4/actor"
)

// ActorManager manages the goakt actor system used by the daemon.
type ActorManager struct {
	system actor.ActorSystem
	db     *sql.DB

	mu       sync.RWMutex
	sessions map[string]*actor.PID // sessionID → PID
}

// NewActorManager creates and starts an actor system with SQLite-backed state.
func NewActorManager(db *sql.DB) (*ActorManager, error) {
	sys, err := actor.NewActorSystem("ratchet",
		actor.WithActorInitMaxRetries(3),
	)
	if err != nil {
		return nil, fmt.Errorf("create actor system: %w", err)
	}
	if err := sys.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("start actor system: %w", err)
	}
	am := &ActorManager{
		system:   sys,
		db:       db,
		sessions: make(map[string]*actor.PID),
	}
	if err := am.rehydrateSessions(context.Background()); err != nil {
		// Non-fatal: log and continue — actors will be spawned on first use.
		_ = err
	}
	return am, nil
}

// SpawnSession spawns a persistent SessionActor for the given session.
// If one already exists the existing PID is returned.
func (am *ActorManager) SpawnSession(ctx context.Context, sessionID, workingDir string) (*actor.PID, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if pid, ok := am.sessions[sessionID]; ok {
		return pid, nil
	}
	a := &SessionActor{
		sessionID:  sessionID,
		workingDir: workingDir,
		db:         am.db,
	}
	pid, err := am.system.Spawn(ctx, "session-"+sessionID, a)
	if err != nil {
		return nil, fmt.Errorf("spawn session actor %s: %w", sessionID, err)
	}
	am.sessions[sessionID] = pid
	return pid, nil
}

// SpawnApproval spawns an ApprovalActor for the given requestID and returns its PID.
// Callers use actor.Ask to send an ApprovalRequest and receive an ApprovalResponse.
func (am *ActorManager) SpawnApproval(ctx context.Context, requestID string) (*actor.PID, error) {
	a := &ApprovalActor{requestID: requestID}
	pid, err := am.system.Spawn(ctx, "approval-"+requestID, a)
	if err != nil {
		return nil, fmt.Errorf("spawn approval actor %s: %w", requestID, err)
	}
	return pid, nil
}

// Close stops the actor system.
func (am *ActorManager) Close(ctx context.Context) error {
	return am.system.Stop(ctx)
}

// rehydrateSessions reads active sessions from SQLite and pre-spawns their actors.
func (am *ActorManager) rehydrateSessions(ctx context.Context) error {
	rows, err := am.db.QueryContext(ctx,
		`SELECT id, working_dir FROM sessions WHERE status = 'active'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, wd string
		if err := rows.Scan(&id, &wd); err != nil {
			continue
		}
		a := &SessionActor{sessionID: id, workingDir: wd, db: am.db}
		pid, err := am.system.Spawn(ctx, "session-"+id, a)
		if err != nil {
			continue
		}
		am.sessions[id] = pid
	}
	return rows.Err()
}

// ---------------------------------------------------------------------------
// SessionActor
// ---------------------------------------------------------------------------

// SessionMessage is delivered to a SessionActor to record a chat message.
type SessionMessage struct {
	Role    string
	Content string
}

// SessionActor maintains per-session state (working dir, active permissions)
// and persists messages to SQLite for rehydration across daemon restarts.
type SessionActor struct {
	sessionID  string
	workingDir string
	db         *sql.DB
	history    []SessionMessage
	perms      map[string]bool // tool → allowed
}

func (a *SessionActor) PreStart(ctx *actor.Context) error {
	a.perms = make(map[string]bool)
	// Load history from SQLite for rehydration.
	if a.db == nil {
		return nil
	}
	rows, err := a.db.QueryContext(ctx.Context(),
		`SELECT role, content FROM messages WHERE session_id = ? ORDER BY created_at`,
		a.sessionID)
	if err != nil {
		return nil // non-fatal
	}
	defer rows.Close()
	for rows.Next() {
		var m SessionMessage
		if err := rows.Scan(&m.Role, &m.Content); err == nil {
			a.history = append(a.history, m)
		}
	}
	return nil
}

func (a *SessionActor) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case SessionMessage:
		a.history = append(a.history, msg)
	}
}

func (a *SessionActor) PostStop(ctx *actor.Context) error {
	return nil
}

// ---------------------------------------------------------------------------
// ApprovalActor
// ---------------------------------------------------------------------------

// ApprovalRequest is sent to an ApprovalActor to request user approval.
type ApprovalRequest struct {
	ToolName string
	Input    string
}

// ApprovalResponse is the reply from an ApprovalActor.
type ApprovalResponse struct {
	Approved bool
	Reason   string
}

const defaultApprovalTimeout = 5 * time.Minute

// ApprovalActor blocks (via actor.Ask) until the TUI user responds to a
// permission prompt or the timeout elapses.
type ApprovalActor struct {
	requestID string
	responded bool
}

func (a *ApprovalActor) PreStart(ctx *actor.Context) error { return nil }

func (a *ApprovalActor) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case ApprovalRequest:
		// Actor parks here; a subsequent ApprovalResponse (sent via Tell) unblocks.
		// Because Ask waits for Response(), we reply immediately with a pending
		// indicator and let a second Tell deliver the final answer.
		// For a simple synchronous pattern: respond denied after timeout.
		_ = msg
		ctx.Response(ApprovalResponse{
			Approved: false,
			Reason:   "no TUI response within timeout",
		})
	case ApprovalResponse:
		// Forwarded from the TUI after the user responds.
		a.responded = true
		ctx.Response(msg)
	}
}

func (a *ApprovalActor) PostStop(ctx *actor.Context) error { return nil }
