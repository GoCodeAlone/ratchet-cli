package acp

import (
	"context"
	"fmt"
	"log"
	"sync"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/google/uuid"

	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/version"
)

// RatchetAgent implements the ACP Agent interface by wrapping the ratchet daemon Service.
type RatchetAgent struct {
	svc  *daemon.Service
	conn *acpsdk.AgentSideConnection

	mu       sync.Mutex
	sessions map[string]string // ACP sessionId → ratchet sessionId
	cancels  map[string]context.CancelFunc
}

// NewRatchetAgent creates a new ACP agent wrapping the given ratchet Service.
func NewRatchetAgent(svc *daemon.Service) *RatchetAgent {
	return &RatchetAgent{
		svc:      svc,
		sessions: make(map[string]string),
		cancels:  make(map[string]context.CancelFunc),
	}
}

// SetConnection stores the AgentSideConnection for sending updates back to the client.
func (a *RatchetAgent) SetConnection(conn *acpsdk.AgentSideConnection) {
	a.conn = conn
}

func (a *RatchetAgent) Authenticate(_ context.Context, _ acpsdk.AuthenticateRequest) (acpsdk.AuthenticateResponse, error) {
	return acpsdk.AuthenticateResponse{}, nil
}

func (a *RatchetAgent) Initialize(_ context.Context, _ acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{
		ProtocolVersion: acpsdk.ProtocolVersionNumber,
		AgentCapabilities: acpsdk.AgentCapabilities{
			LoadSession: true,
		},
		AgentInfo: &acpsdk.Implementation{
			Name:    "ratchet",
			Version: version.Version,
		},
	}, nil
}

func (a *RatchetAgent) Cancel(_ context.Context, params acpsdk.CancelNotification) error {
	a.mu.Lock()
	cancel, ok := a.cancels[string(params.SessionId)]
	a.mu.Unlock()
	if ok {
		cancel()
	}
	return nil
}

func (a *RatchetAgent) NewSession(ctx context.Context, params acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	acpID := acpsdk.SessionId(uuid.New().String())

	sess, err := a.svc.CreateSession(ctx, &pb.CreateSessionReq{
		WorkingDir: params.Cwd,
	})
	if err != nil {
		return acpsdk.NewSessionResponse{}, fmt.Errorf("create session: %w", err)
	}

	a.mu.Lock()
	a.sessions[string(acpID)] = sess.Id
	a.mu.Unlock()

	return acpsdk.NewSessionResponse{SessionId: acpID}, nil
}

// LoadSession implements acp.AgentLoader.
func (a *RatchetAgent) LoadSession(ctx context.Context, params acpsdk.LoadSessionRequest) (acpsdk.LoadSessionResponse, error) {
	ratchetID := string(params.SessionId)

	list, err := a.svc.ListSessions(ctx, &pb.Empty{})
	if err != nil {
		return acpsdk.LoadSessionResponse{}, fmt.Errorf("list sessions: %w", err)
	}
	found := false
	for _, s := range list.Sessions {
		if s.Id == ratchetID {
			found = true
			break
		}
	}
	if !found {
		return acpsdk.LoadSessionResponse{}, fmt.Errorf("session %s not found", ratchetID)
	}

	a.mu.Lock()
	a.sessions[ratchetID] = ratchetID
	a.mu.Unlock()

	return acpsdk.LoadSessionResponse{}, nil
}

// Prompt sends a message and streams responses as ACP session updates.
func (a *RatchetAgent) Prompt(ctx context.Context, params acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	a.mu.Lock()
	ratchetID, ok := a.sessions[string(params.SessionId)]
	a.mu.Unlock()
	if !ok {
		return acpsdk.PromptResponse{}, fmt.Errorf("unknown session: %s", params.SessionId)
	}

	var text string
	for _, block := range params.Prompt {
		if block.Text != nil {
			text += block.Text.Text
		}
	}

	promptCtx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.cancels[string(params.SessionId)] = cancel
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.cancels, string(params.SessionId))
		a.mu.Unlock()
		cancel()
	}()

	// Use the channel-based bridge to stream events without implementing gRPC stream interface.
	ch, err := a.svc.SendMessageChan(promptCtx, ratchetID, text)
	if err != nil {
		return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, fmt.Errorf("send message: %w", err)
	}

	if a.conn == nil {
		return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, fmt.Errorf("no ACP connection set")
	}

	for ev := range ch {
		if promptCtx.Err() != nil {
			break
		}

		// Handle permission requests via ACP's RequestPermission RPC.
		if perm, ok := ev.Event.(*pb.ChatEvent_Permission); ok {
			a.handlePermission(promptCtx, params.SessionId, perm.Permission)
			continue
		}

		updates := chatEventToUpdates(ev)
		for _, u := range updates {
			if err := a.conn.SessionUpdate(promptCtx, acpsdk.SessionNotification{
				SessionId: params.SessionId,
				Update:    u,
			}); err != nil {
				log.Printf("acp: session update error: %v", err)
			}
		}
	}

	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

func (a *RatchetAgent) handlePermission(ctx context.Context, sessionID acpsdk.SessionId, perm *pb.PermissionRequest) {
	resp, err := a.conn.RequestPermission(ctx, acpsdk.RequestPermissionRequest{
		SessionId: sessionID,
		ToolCall: acpsdk.RequestPermissionToolCall{
			ToolCallId: acpsdk.ToolCallId(perm.RequestId),
			Title:      acpsdk.Ptr(perm.ToolName),
			Kind:       acpsdk.Ptr(acpsdk.ToolKindEdit),
			RawInput:   perm.ArgumentsJson,
		},
		Options: []acpsdk.PermissionOption{
			{Kind: acpsdk.PermissionOptionKindAllowOnce, Name: "Allow", OptionId: "allow"},
			{Kind: acpsdk.PermissionOptionKindRejectOnce, Name: "Deny", OptionId: "deny"},
		},
	})
	if err != nil {
		log.Printf("acp: request permission error: %v", err)
		return
	}

	allowed := resp.Outcome.Selected != nil && string(resp.Outcome.Selected.OptionId) == "allow"
	if _, err = a.svc.RespondToPermission(ctx, &pb.PermissionResponse{
		RequestId: perm.RequestId,
		Allowed:   allowed,
		Scope:     "once",
	}); err != nil {
		log.Printf("acp: respond to permission error: %v", err)
	}
}

func (a *RatchetAgent) SetSessionMode(_ context.Context, _ acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	return acpsdk.SetSessionModeResponse{}, nil
}

// SetSessionModel implements acp.AgentExperimental.
func (a *RatchetAgent) SetSessionModel(ctx context.Context, params acpsdk.SetSessionModelRequest) (acpsdk.SetSessionModelResponse, error) {
	if params.ModelId != "" {
		_, err := a.svc.UpdateProviderModel(ctx, &pb.UpdateProviderModelReq{
			Model: string(params.ModelId),
		})
		if err != nil {
			return acpsdk.SetSessionModelResponse{}, fmt.Errorf("update model: %w", err)
		}
	}
	return acpsdk.SetSessionModelResponse{}, nil
}

// Compile-time interface checks.
var (
	_ acpsdk.Agent             = (*RatchetAgent)(nil)
	_ acpsdk.AgentLoader       = (*RatchetAgent)(nil)
	_ acpsdk.AgentExperimental = (*RatchetAgent)(nil)
)
