package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
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

// compactSentinel is a special marker sent by the client's CompactSession call.
// When handleChat detects it, it runs compression immediately without an AI turn.
const compactSentinel = "\x00compact\x00"

// handleChat executes a chat turn: loads session, resolves provider, streams tokens, handles tools.
func (s *Service) handleChat(ctx context.Context, sessionID, userMessage string, stream pb.RatchetDaemon_SendMessageServer) error {
	// Manual compression request: skip the AI turn and compress history directly.
	if userMessage == compactSentinel {
		return s.handleCompact(ctx, sessionID, stream)
	}

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

	// Track token usage (approximate: 1 token ≈ 4 chars).
	// Uses rune count so multi-byte UTF-8 characters don't inflate the estimate.
	inputTokens := (len([]rune(userMessage)) + 3) / 4
	outputTokens := (len([]rune(fullResponse)) + 3) / 4
	s.tokens.AddTokens(sessionID, inputTokens, outputTokens)

	// Auto-compress when context window fills
	contextCfg := cfg.Context
	if contextCfg.CompressionThreshold <= 0 {
		contextCfg.CompressionThreshold = 0.9
	}
	if contextCfg.PreserveMessages <= 0 {
		contextCfg.PreserveMessages = 10
	}
	const defaultModelLimit = 200000 // conservative default (Claude Sonnet)
	if s.tokens.ShouldCompress(sessionID, contextCfg.CompressionThreshold, defaultModelLimit) {
		history, loadErr := s.loadHistory(ctx, sessionID)
		if loadErr == nil && len(history) > contextCfg.PreserveMessages {
			compressed, summary, compErr := Compress(ctx, history, contextCfg.PreserveMessages, prov)
			if compErr == nil {
				removed := len(history) - len(compressed)
				// Persist compressed history by replacing messages in DB
				if dbErr := s.replaceHistory(ctx, sessionID, compressed); dbErr != nil {
					log.Printf("replace history after compression: %v", dbErr)
				} else {
					s.tokens.Reset(sessionID)
					_ = stream.Send(&pb.ChatEvent{
						Event: &pb.ChatEvent_ContextCompressed{
							ContextCompressed: &pb.ContextCompressedEvent{
								SessionId:       sessionID,
								Summary:         summary,
								MessagesRemoved: int32(removed),
								MessagesKept:    int32(len(compressed)),
							},
						},
					})
				}
			}
		}
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

// replaceHistory deletes all messages for a session and re-inserts the compressed set.
func (s *Service) replaceHistory(ctx context.Context, sessionID string, messages []provider.Message) error {
	tx, err := s.engine.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE session_id = ?`, sessionID); err != nil {
		return err
	}
	for _, m := range messages {
		id := uuid.New().String()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO messages (id, session_id, role, content, tool_name, tool_call_id) VALUES (?, ?, ?, ?, ?, ?)`,
			id, sessionID, string(m.Role), m.Content, "", "",
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// handleCompact immediately compresses the session's conversation history and
// sends a ContextCompressed event to the stream. No AI provider call is made.
func (s *Service) handleCompact(ctx context.Context, sessionID string, stream pb.RatchetDaemon_SendMessageServer) error {
	history, err := s.loadHistory(ctx, sessionID)
	if err != nil {
		return sendError(stream, "load history: "+err.Error())
	}

	cfg, _ := config.Load()
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	preserveCount := cfg.Context.PreserveMessages
	if preserveCount <= 0 {
		preserveCount = 10
	}

	var prov provider.Provider
	session, sessErr := s.sessions.Get(ctx, sessionID)
	if sessErr == nil {
		if session.Provider != "" {
			prov, _ = s.engine.ProviderRegistry.GetByAlias(ctx, session.Provider)
		} else {
			prov, _ = s.engine.ProviderRegistry.GetDefault(ctx)
		}
	}

	compressed, summary, compErr := Compress(ctx, history, preserveCount, prov)
	if compErr != nil {
		return sendError(stream, "compress: "+compErr.Error())
	}

	removed := len(history) - len(compressed)
	if removed <= 0 {
		// Nothing to compress; still send a completion event.
		return stream.Send(&pb.ChatEvent{
			Event: &pb.ChatEvent_Complete{
				Complete: &pb.SessionComplete{Summary: "Nothing to compress."},
			},
		})
	}

	if dbErr := s.replaceHistory(ctx, sessionID, compressed); dbErr != nil {
		return sendError(stream, "persist compressed history: "+dbErr.Error())
	}
	s.tokens.Reset(sessionID)

	if err := stream.Send(&pb.ChatEvent{
		Event: &pb.ChatEvent_ContextCompressed{
			ContextCompressed: &pb.ContextCompressedEvent{
				SessionId:       sessionID,
				Summary:         summary,
				MessagesRemoved: int32(removed),
				MessagesKept:    int32(len(compressed)),
			},
		},
	}); err != nil {
		return err
	}
	return stream.Send(&pb.ChatEvent{
		Event: &pb.ChatEvent_Complete{
			Complete: &pb.SessionComplete{Summary: "compressed"},
		},
	})
}

// sendError sends an error event to the stream.
func sendError(stream pb.RatchetDaemon_SendMessageServer, msg string) error {
	return stream.Send(&pb.ChatEvent{
		Event: &pb.ChatEvent_Error{
			Error: &pb.ErrorEvent{Message: msg},
		},
	})
}
