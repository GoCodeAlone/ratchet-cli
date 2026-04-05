package mesh

import (
	"context"

	"github.com/GoCodeAlone/workflow-plugin-agent/executor"
	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
	"github.com/GoCodeAlone/workflow-plugin-agent/tools"
	"github.com/google/uuid"
)

// LocalNode is a mesh node backed by a local LLM provider. It delegates work
// to the workflow-plugin-agent executor and wires mesh blackboard/messaging
// as agent tools.
type LocalNode struct {
	id       string
	config   NodeConfig
	provider provider.Provider
	onEvent  func(executor.Event)
}

// NewLocalNode creates a LocalNode that uses the given provider.
// If onEvent is non-nil it will receive executor events during Run.
func NewLocalNode(cfg NodeConfig, prov provider.Provider, onEvent func(executor.Event)) *LocalNode {
	return &LocalNode{
		id:       cfg.Name + "-" + uuid.NewString()[:8],
		config:   cfg,
		provider: prov,
		onEvent:  onEvent,
	}
}

// ID returns the node's unique identifier.
func (n *LocalNode) ID() string { return n.id }

// Info returns the node's static metadata.
func (n *LocalNode) Info() NodeInfo {
	return NodeInfo{
		Name:     n.config.Name,
		Role:     n.config.Role,
		Model:    n.config.Model,
		Provider: n.config.Provider,
		Location: n.config.Location,
	}
}

// Run executes the agent loop. It registers blackboard and messaging tools,
// then delegates to executor.Execute. The loop terminates when:
//   - The blackboard section "status" has this node's ID set to "done" or "approved"
//   - MaxIterations is reached
//   - The context is cancelled
func (n *LocalNode) Run(ctx context.Context, task string, bb *Blackboard, inbox <-chan Message, outbox chan<- Message) error {
	// Build tool registry respecting the per-agent allowlist from the config.
	// If Tools is empty we fall back to registering all mesh tools so that
	// agents with no explicit restriction still work (backward compatible).
	allTools := []tools.Tool{
		&BlackboardReadTool{bb: bb},
		&BlackboardWriteTool{bb: bb},
		&BlackboardListTool{bb: bb},
		&SendMessageTool{outbox: outbox, from: n.id},
	}
	reg := tools.NewRegistry()
	if len(n.config.Tools) == 0 {
		for _, t := range allTools {
			reg.Register(t)
		}
	} else {
		allowed := make(map[string]bool, len(n.config.Tools))
		for _, name := range n.config.Tools {
			allowed[name] = true
		}
		for _, t := range allTools {
			if allowed[t.Name()] {
				reg.Register(t)
			}
		}
	}

	// Convert mesh inbox (Message) to provider inbox (provider.Message).
	// Use at least capacity 1 as a safeguard: single-source inboxes (directly
	// from router.Register) are already buffered, but defensive sizing here
	// ensures the adapter never blocks even in edge cases (e.g., future code
	// paths that pass an unbuffered channel).
	provInboxSize := cap(inbox)
	if provInboxSize < 1 {
		provInboxSize = 1
	}
	provInbox := make(chan provider.Message, provInboxSize)
	go func() {
		defer close(provInbox)
		for {
			var msg Message
			var ok bool
			select {
			case <-ctx.Done():
				return
			case msg, ok = <-inbox:
				if !ok {
					return
				}
			}
			provMsg := provider.Message{
				Role:    provider.Role("user"),
				Content: "[" + msg.Type + " from " + msg.From + "] " + msg.Content,
			}
			select {
			case <-ctx.Done():
				return
			case provInbox <- provMsg:
			}
		}
	}()

	maxIter := n.config.MaxIterations
	if maxIter <= 0 {
		maxIter = 10
	}

	cfg := executor.Config{
		Provider:      n.provider,
		ToolRegistry:  reg,
		MaxIterations: maxIter,
		Inbox:         provInbox,
		OnEvent:       n.onEvent,
		ShouldStop: func() (reason string) {
			// Check by node ID
			if e, ok := bb.Read("status", n.id); ok {
				if s, _ := e.Value.(string); s == "done" || s == "approved" {
					return "status: " + s
				}
			}
			// Also check by node name for convenience
			if e, ok := bb.Read("status", n.config.Name); ok {
				if s, _ := e.Value.(string); s == "done" || s == "approved" {
					return "status: " + s
				}
			}
			return ""
		},
	}

	agentCtx := tools.WithAgentID(ctx, n.id)
	_, err := executor.Execute(agentCtx, cfg, n.config.SystemPrompt, task, n.id)
	return err
}
