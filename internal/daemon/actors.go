package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/GoCodeAlone/workflow-plugin-agent/executor"
	"github.com/tochemey/goakt/v4/actor"
)

// ActorManager manages the goakt actor system used by the daemon.
type ActorManager struct {
	system actor.ActorSystem
	db     *sql.DB
	ctx    context.Context

	mu       sync.RWMutex
	sessions map[string]*actor.PID // sessionID → PID
}

// NewActorManager creates and starts an actor system with SQLite-backed state.
// The provided context is stored and propagated to the actor system and rehydration.
func NewActorManager(ctx context.Context, db *sql.DB) (*ActorManager, error) {
	sys, err := actor.NewActorSystem("ratchet",
		actor.WithActorInitMaxRetries(3),
	)
	if err != nil {
		return nil, fmt.Errorf("create actor system: %w", err)
	}
	if err := sys.Start(ctx); err != nil {
		return nil, fmt.Errorf("start actor system: %w", err)
	}
	am := &ActorManager{
		system:   sys,
		db:       db,
		ctx:      ctx,
		sessions: make(map[string]*actor.PID),
	}
	// Rehydrate sessions asynchronously — don't block daemon startup.
	// Actors will be spawned lazily on first use if rehydration is slow.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("actor: rehydrate panic: %v", r)
			}
		}()
		rehydrateCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := am.rehydrateSessions(rehydrateCtx); err != nil {
			log.Printf("actor: rehydrate sessions: %v", err)
		}
	}()
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
// gate and timeout are wired through to the actor; pass nil gate to auto-deny.
func (am *ActorManager) SpawnApproval(ctx context.Context, requestID string, gate *ApprovalGate, timeout time.Duration) (*actor.PID, error) {
	a := &ApprovalActor{requestID: requestID, gate: gate, timeout: timeout}
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
		spawnCtx, spawnCancel := context.WithTimeout(ctx, 5*time.Second)
		pid, err := am.system.Spawn(spawnCtx, "session-"+id, a)
		spawnCancel()
		if err != nil {
			log.Printf("actor: rehydrate session %s: %v (will spawn lazily)", id, err)
			continue
		}
		am.mu.Lock()
		am.sessions[id] = pid
		am.mu.Unlock()
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

// ApprovalActor blocks (via actor.Ask) until the TUI user responds to a
// permission prompt or the timeout elapses.
type ApprovalActor struct {
	requestID string
	gate      *ApprovalGate
	timeout   time.Duration
	responded bool
}

func (a *ApprovalActor) PreStart(ctx *actor.Context) error { return nil }

func (a *ApprovalActor) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case ApprovalRequest:
		_ = msg
		if a.gate == nil {
			ctx.Response(ApprovalResponse{
				Approved: false,
				Reason:   "no approval gate configured",
			})
			return
		}
		timeout := a.timeout
		if timeout == 0 {
			timeout = 30 * time.Minute
		}
		record, err := a.gate.WaitForResolution(ctx.Context(), a.requestID, timeout)
		if err != nil {
			ctx.Response(ApprovalResponse{
				Approved: false,
				Reason:   err.Error(),
			})
			return
		}
		approved := record != nil && record.Status == executor.ApprovalApproved
		reason := ""
		if record != nil {
			reason = record.ReviewerComment
			if record.Status == executor.ApprovalTimeout {
				reason = "approval timed out"
			}
		}
		a.responded = true
		ctx.Response(ApprovalResponse{Approved: approved, Reason: reason})
	case ApprovalResponse:
		// Forwarded from the TUI after the user responds.
		a.responded = true
		ctx.Response(msg)
	}
}

func (a *ApprovalActor) PostStop(ctx *actor.Context) error { return nil }
