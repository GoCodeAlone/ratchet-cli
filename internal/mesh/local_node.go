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
	// Build tool registry with blackboard and messaging tools
	reg := tools.NewRegistry()
	reg.Register(&BlackboardReadTool{bb: bb})
	reg.Register(&BlackboardWriteTool{bb: bb})
	reg.Register(&BlackboardListTool{bb: bb})
	reg.Register(&SendMessageTool{outbox: outbox, from: n.id})

	// Convert mesh inbox (Message) to provider inbox (provider.Message)
	provInbox := make(chan provider.Message, cap(inbox))
	go func() {
		for msg := range inbox {
			provInbox <- provider.Message{
				Role:    provider.Role("user"),
				Content: "[" + msg.Type + " from " + msg.From + "] " + msg.Content,
			}
		}
		close(provInbox)
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
			e, ok := bb.Read("status", n.id)
			if !ok {
				return ""
			}
			if s, _ := e.Value.(string); s == "done" || s == "approved" {
				return "status: " + s
			}
			return ""
		},
	}

	agentCtx := tools.WithAgentID(ctx, n.id)
	_, err := executor.Execute(agentCtx, cfg, n.config.SystemPrompt, task, n.id)
	return err
}
