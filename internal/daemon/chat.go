package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/GoCodeAlone/workflow-plugin-agent/executor"
	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
	"github.com/google/uuid"

	"github.com/GoCodeAlone/ratchet-cli/internal/agent"
	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	"github.com/GoCodeAlone/ratchet-cli/internal/hooks"
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

// reviewSentinel is a prefix sent by the TUI to trigger a code review sub-session.
// The diff content follows immediately after the sentinel.
const reviewSentinel = "\x00review\x00"

// debugLog appends a log entry to ~/.ratchet/debug.log when debug mode is active.
func debugLog(format string, args ...any) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	logPath := filepath.Join(home, ".ratchet", "debug.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	logger := log.New(f, "", log.LstdFlags)
	logger.Printf(format, args...)
}

// broadcastStream wraps a send-message stream and fans out each event to the broadcaster.
type broadcastStream struct {
	pb.RatchetDaemon_SendMessageServer
	sessionID   string
	broadcaster *SessionBroadcaster
}

func (b *broadcastStream) Send(ev *pb.ChatEvent) error {
	if b.broadcaster != nil {
		b.broadcaster.Publish(b.sessionID, ev)
	}
	return b.RatchetDaemon_SendMessageServer.Send(ev)
}

// handleChat executes a chat turn: loads session, resolves provider, streams tokens, handles tools.
func (s *Service) handleChat(ctx context.Context, sessionID, userMessage string, stream pb.RatchetDaemon_SendMessageServer) error {
	stream = &broadcastStream{RatchetDaemon_SendMessageServer: stream, sessionID: sessionID, broadcaster: s.broadcaster}
	// Manual compression request: skip the AI turn and compress history directly.
	if userMessage == compactSentinel {
		return s.handleCompact(ctx, sessionID, stream)
	}

	// Code review request: run a review sub-session against the provided diff.
	if diff, ok := strings.CutPrefix(userMessage, reviewSentinel); ok {
		return s.handleReview(ctx, sessionID, diff, stream)
	}

	// Load session
	session, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session %s: %w", sessionID, err)
	}
	if session.Status == "completed" || session.Status == "killed" {
		return sendError(stream, fmt.Sprintf("session %s is no longer active (status: %s)", sessionID, session.Status))
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

	// Debug: log outgoing messages.
	if s.engine.Debug {
		msgJSON, _ := json.Marshal(messages)
		debugLog("[chat] session=%s sending %d messages: %s", sessionID, len(messages), string(msgJSON))
	}

	// PTY CLI providers (claude_code, copilot_cli, etc.) don't support streaming
	// well — their interactive PTY mode requires complex prompt detection.
	// Use Chat() (non-interactive) for these and fake-stream the response.
	isPTYProvider := strings.HasSuffix(prov.Name(), "_code") || strings.HasSuffix(prov.Name(), "_cli")
	if isPTYProvider {
		resp, chatErr := prov.Chat(ctx, messages, nil)
		if chatErr != nil {
			if isAuthError(chatErr) {
				return sendAuthError(stream, session.Provider, chatErr.Error())
			}
			return sendError(stream, "provider chat: "+chatErr.Error())
		}
		// Save user message after successful provider call.
		if err := s.saveMessage(ctx, sessionID, "user", userMessage, "", ""); err != nil {
			log.Printf("save user message: %v", err)
		}
		// Emit the full response as a single token event.
		if resp != nil && resp.Content != "" {
			if err := stream.Send(&pb.ChatEvent{
				Event: &pb.ChatEvent_Token{Token: &pb.TokenDelta{Content: resp.Content}},
			}); err != nil {
				return err
			}
			if err := s.saveMessage(ctx, sessionID, "assistant", resp.Content, "", ""); err != nil {
				log.Printf("save assistant message: %v", err)
			}
		}
		return stream.Send(&pb.ChatEvent{
			Event: &pb.ChatEvent_Complete{Complete: &pb.SessionComplete{Summary: "done"}},
		})
	}

	// Stream from provider (save user message AFTER successful stream start,
	// so failed requests don't pollute conversation history).
	eventCh, err := prov.Stream(ctx, messages, nil) // tools will be added in later task
	if err != nil {
		if isAuthError(err) {
			return sendAuthError(stream, session.Provider, err.Error())
		}
		return sendError(stream, "provider stream: "+err.Error())
	}

	// Provider accepted the request — now save the user message to history.
	if err := s.saveMessage(ctx, sessionID, "user", userMessage, "", ""); err != nil {
		log.Printf("save user message: %v", err)
	}

	var fullResponse string
	for event := range eventCh {
		switch event.Type {
		case "thinking":
			if err := stream.Send(&pb.ChatEvent{
				Event: &pb.ChatEvent_Thinking{
					Thinking: &pb.ThinkingBlock{Content: event.Thinking},
				},
			}); err != nil {
				return err
			}

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
					if err := stream.Send(&pb.ChatEvent{
						Event: &pb.ChatEvent_ToolResult{
							ToolResult: &pb.ToolCallResult{
								CallId:     callID,
								ResultJson: `{"error": "denied by user"}`,
								Success:    false,
							},
						},
					}); err != nil {
						return err
					}
					continue
				}
			}

			// Execute tool
			result, execErr := s.executeTool(ctx, event.Tool)
			resultJSON, _ := json.Marshal(result)
			success := execErr == nil

			if err := stream.Send(&pb.ChatEvent{
				Event: &pb.ChatEvent_ToolResult{
					ToolResult: &pb.ToolCallResult{
						CallId:     callID,
						ResultJson: string(resultJSON),
						Success:    success,
					},
				},
			}); err != nil {
				return err
			}

		case "done":
			// Stream complete
		case "error":
			if isAuthError(fmt.Errorf("%s", event.Error)) {
				return sendAuthError(stream, session.Provider, event.Error)
			}
			return sendError(stream, event.Error)
		}
	}

	// Debug: log response content.
	if s.engine.Debug {
		debugLog("[chat] session=%s response (%d chars): %s", sessionID, len(fullResponse), fullResponse)
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
	modelLimit := defaultModelLimit
	if session.Model != "" && contextCfg.ModelLimits != nil {
		if limit, ok := contextCfg.ModelLimits[session.Model]; ok {
			modelLimit = limit
		}
	}
	if s.tokens.ShouldCompress(sessionID, contextCfg.CompressionThreshold, modelLimit) {
		if s.engine.Hooks != nil {
			_ = s.engine.Hooks.Run(hooks.OnTokenLimit, map[string]string{
				"tokens_used":  fmt.Sprintf("%d", s.tokens.Total(sessionID)),
				"tokens_limit": fmt.Sprintf("%d", modelLimit),
			})
		}
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
	return slices.Contains(cfg.Permissions.AutoAllow, toolName)
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
		var provErr error
		if session.Provider != "" {
			prov, provErr = s.engine.ProviderRegistry.GetByAlias(ctx, session.Provider)
		} else {
			prov, provErr = s.engine.ProviderRegistry.GetDefault(ctx)
		}
		if provErr != nil {
			log.Printf("compact: resolve provider for summarization: %v (using fallback)", provErr)
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

// handleReview runs the code-reviewer builtin agent against the provided diff.
func (s *Service) handleReview(ctx context.Context, sessionID, diff string, stream pb.RatchetDaemon_SendMessageServer) error {
	// Load code-reviewer builtin definition.
	var reviewerDef agent.AgentDefinition
	if defs, err := agent.LoadBuiltins(); err == nil {
		for _, d := range defs {
			if d.Name == "code-reviewer" {
				reviewerDef = d
				break
			}
		}
	}

	maxIter := reviewerDef.MaxIterations
	if maxIter <= 0 {
		maxIter = 5
	}
	systemPrompt := reviewerDef.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = `You are a code reviewer. Analyze diffs and files for:
- Security vulnerabilities (injection, auth bypass, etc.)
- Logic errors and edge cases
- Code style and naming conventions
- Test coverage gaps
Output structured review: Critical / Important / Minor with file:line refs.`
	}

	// Resolve provider: prefer reviewer's configured provider, then session provider, then default.
	// reviewerDef.Model is a model selection hint, not a provider alias.
	session, sessErr := s.sessions.Get(ctx, sessionID)
	var prov provider.Provider
	var provErr error
	if reviewerDef.Provider != "" {
		prov, provErr = s.engine.ProviderRegistry.GetByAlias(ctx, reviewerDef.Provider)
	}
	if prov == nil {
		if sessErr == nil && session.Provider != "" {
			prov, provErr = s.engine.ProviderRegistry.GetByAlias(ctx, session.Provider)
		} else {
			prov, provErr = s.engine.ProviderRegistry.GetDefault(ctx)
		}
	}
	if provErr != nil {
		return sendError(stream, "review: no provider: "+provErr.Error())
	}

	userMsg := "Please review the following git diff:\n\n```diff\n" + diff + "\n```"

	result, err := executor.Execute(ctx, executor.Config{
		Provider:      prov,
		MaxIterations: maxIter,
	}, systemPrompt, userMsg, "code-reviewer")
	if err != nil {
		return sendError(stream, "review execute: "+err.Error())
	}

	content := ""
	if result != nil {
		content = result.Content
	}

	if err := stream.Send(&pb.ChatEvent{
		Event: &pb.ChatEvent_Token{
			Token: &pb.TokenDelta{Content: content},
		},
	}); err != nil {
		return err
	}

	if sessErr == nil && content != "" {
		if err := s.saveMessage(ctx, sessionID, "assistant", content, "", ""); err != nil {
			log.Printf("save review message: %v", err)
		}
	}

	return stream.Send(&pb.ChatEvent{
		Event: &pb.ChatEvent_Complete{
			Complete: &pb.SessionComplete{Summary: "review complete"},
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

// sendAuthError sends an AuthError event to the stream.
func sendAuthError(stream pb.RatchetDaemon_SendMessageServer, providerAlias, msg string) error {
	return stream.Send(&pb.ChatEvent{
		Event: &pb.ChatEvent_AuthError{
			AuthError: &pb.AuthError{
				Provider: providerAlias,
				Alias:    providerAlias,
				Message:  msg,
			},
		},
	})
}

// isAuthError returns true if the error looks like an authentication failure.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "status 401") ||
		strings.Contains(msg, "status 403") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "authentication_error") ||
		strings.Contains(msg, "invalid api key") ||
		strings.Contains(msg, "invalid x-api-key") ||
		strings.Contains(msg, "permission_error") ||
		strings.Contains(msg, "could not resolve api key")
}
