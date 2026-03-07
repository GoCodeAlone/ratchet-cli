package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/GoCodeAlone/ratchet/provider"
	"github.com/google/uuid"

	"github.com/GoCodeAlone/ratchet-cli/internal/agent"
	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// pendingPermission holds a permission request waiting for a TUI response.
type pendingPermission struct {
	ch chan *pb.PermissionResponse
}

// permissionGate manages in-flight permission requests.
type permissionGate struct {
	mu      sync.Mutex
	pending map[string]*pendingPermission
}

func newPermissionGate() *permissionGate {
	return &permissionGate{
		pending: make(map[string]*pendingPermission),
	}
}

func (g *permissionGate) Wait(requestID string) *pb.PermissionResponse {
	g.mu.Lock()
	pp := &pendingPermission{ch: make(chan *pb.PermissionResponse, 1)}
	g.pending[requestID] = pp
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		delete(g.pending, requestID)
		g.mu.Unlock()
	}()

	select {
	case resp := <-pp.ch:
		return resp
	case <-time.After(5 * time.Minute):
		return &pb.PermissionResponse{RequestId: requestID, Allowed: false, Scope: "once"}
	}
}

func (g *permissionGate) Respond(resp *pb.PermissionResponse) bool {
	g.mu.Lock()
	pp, ok := g.pending[resp.RequestId]
	g.mu.Unlock()
	if !ok {
		return false
	}
	pp.ch <- resp
	return true
}

// handleChat executes a chat turn: loads session, resolves provider, streams tokens, handles tools.
func (s *Service) handleChat(ctx context.Context, sessionID, userMessage string, stream pb.RatchetDaemon_SendMessageServer) error {
	// Load session
	session, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session %s: %w", sessionID, err)
	}

	// Resolve provider
	var prov provider.Provider
	if session.Provider != "" {
		prov, err = s.engine.ProviderRegistry.GetByAlias(ctx, session.Provider)
	} else {
		prov, err = s.engine.ProviderRegistry.GetDefault(ctx)
	}
	if err != nil {
		return sendError(stream, "no provider configured: "+err.Error())
	}

	// Load config for instruction compat
	cfg, _ := config.Load()
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	// Discover instruction files
	instructions := agent.DiscoverInstructions(session.WorkingDir, cfg.InstructionCompat, session.Provider)
	systemPrompt := agent.BuildSystemPrompt(instructions)
	if systemPrompt == "" {
		systemPrompt = "You are a helpful AI coding assistant."
	}

	// Load conversation history
	history, err := s.loadHistory(ctx, sessionID)
	if err != nil {
		log.Printf("load history: %v", err)
	}

	// Prepend system message
	messages := []provider.Message{
		{Role: provider.RoleSystem, Content: systemPrompt},
	}
	messages = append(messages, history...)
	messages = append(messages, provider.Message{
		Role:    provider.RoleUser,
		Content: userMessage,
	})

	// Save user message
	if err := s.saveMessage(ctx, sessionID, "user", userMessage, "", ""); err != nil {
		log.Printf("save user message: %v", err)
	}

	// Stream from provider
	eventCh, err := prov.Stream(ctx, messages, nil) // tools will be added in later task
	if err != nil {
		return sendError(stream, "provider stream: "+err.Error())
	}

	var fullResponse string
	for event := range eventCh {
		switch event.Type {
		case "text":
			fullResponse += event.Text
			if err := stream.Send(&pb.ChatEvent{
				Event: &pb.ChatEvent_Token{
					Token: &pb.TokenDelta{Content: event.Text},
				},
			}); err != nil {
				return err
			}

		case "tool_call":
			if event.Tool == nil {
				continue
			}
			argsJSON, _ := json.Marshal(event.Tool.Arguments)
			callID := event.Tool.ID
			if callID == "" {
				callID = uuid.New().String()
			}

			// Send tool start event
			if err := stream.Send(&pb.ChatEvent{
				Event: &pb.ChatEvent_ToolStart{
					ToolStart: &pb.ToolCallStart{
						ToolName:      event.Tool.Name,
						ArgumentsJson: string(argsJSON),
						CallId:        callID,
					},
				},
			}); err != nil {
				return err
			}

			// Check permission
			if !s.isAutoAllowed(cfg, event.Tool.Name) {
				reqID := uuid.New().String()
				if err := stream.Send(&pb.ChatEvent{
					Event: &pb.ChatEvent_Permission{
						Permission: &pb.PermissionRequest{
							RequestId:     reqID,
							ToolName:      event.Tool.Name,
							ArgumentsJson: string(argsJSON),
							Description:   "Execute tool: " + event.Tool.Name,
						},
					},
				}); err != nil {
					return err
				}
				resp := s.permGate.Wait(reqID)
				if !resp.Allowed {
					stream.Send(&pb.ChatEvent{
						Event: &pb.ChatEvent_ToolResult{
							ToolResult: &pb.ToolCallResult{
								CallId:     callID,
								ResultJson: `{"error": "denied by user"}`,
								Success:    false,
							},
						},
					})
					continue
				}
			}

			// Execute tool
			result, execErr := s.executeTool(ctx, event.Tool)
			resultJSON, _ := json.Marshal(result)
			success := execErr == nil

			stream.Send(&pb.ChatEvent{
				Event: &pb.ChatEvent_ToolResult{
					ToolResult: &pb.ToolCallResult{
						CallId:     callID,
						ResultJson: string(resultJSON),
						Success:    success,
					},
				},
			})

		case "done":
			// Stream complete
		case "error":
			return sendError(stream, event.Error)
		}
	}

	// Save assistant response
	if err := s.saveMessage(ctx, sessionID, "assistant", fullResponse, "", ""); err != nil {
		log.Printf("save assistant message: %v", err)
	}

	// Send completion
	return stream.Send(&pb.ChatEvent{
		Event: &pb.ChatEvent_Complete{
			Complete: &pb.SessionComplete{Summary: "done"},
		},
	})
}

// isAutoAllowed checks if a tool is in the auto-allow list.
func (s *Service) isAutoAllowed(cfg *config.Config, toolName string) bool {
	for _, t := range cfg.Permissions.AutoAllow {
		if t == toolName {
			return true
		}
	}
	return false
}

// executeTool runs a tool via the tool registry.
func (s *Service) executeTool(ctx context.Context, tool *provider.ToolCall) (map[string]any, error) {
	if s.engine.ToolRegistry == nil {
		return map[string]any{"error": "no tool registry"}, fmt.Errorf("no tool registry")
	}
	result, err := s.engine.ToolRegistry.Execute(ctx, tool.Name, tool.Arguments)
	if err != nil {
		return map[string]any{"error": err.Error()}, err
	}
	if m, ok := result.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{"result": result}, nil
}

// loadHistory loads conversation history from DB.
func (s *Service) loadHistory(ctx context.Context, sessionID string) ([]provider.Message, error) {
	rows, err := s.engine.DB.QueryContext(ctx,
		`SELECT role, content, tool_name, tool_call_id FROM messages WHERE session_id = ? ORDER BY created_at`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []provider.Message
	for rows.Next() {
		var m provider.Message
		var toolName, toolCallID string
		if err := rows.Scan(&m.Role, &m.Content, &toolName, &toolCallID); err != nil {
			return nil, err
		}
		if toolCallID != "" {
			m.ToolCallID = toolCallID
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// saveMessage persists a message to the DB.
func (s *Service) saveMessage(ctx context.Context, sessionID, role, content, toolName, toolCallID string) error {
	id := uuid.New().String()
	_, err := s.engine.DB.ExecContext(ctx,
		`INSERT INTO messages (id, session_id, role, content, tool_name, tool_call_id) VALUES (?, ?, ?, ?, ?, ?)`,
		id, sessionID, role, content, toolName, toolCallID,
	)
	return err
}

// sendError sends an error event to the stream.
func sendError(stream pb.RatchetDaemon_SendMessageServer, msg string) error {
	return stream.Send(&pb.ChatEvent{
		Event: &pb.ChatEvent_Error{
			Error: &pb.ErrorEvent{Message: msg},
		},
	})
}
