package mesh

import (
	"context"
	"testing"
	"time"

	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

func TestLocalNode_Run(t *testing.T) {
	bb := NewBlackboard()

	// Script: first call writes to blackboard via tool, second call sends
	// a message, third call returns final content (no more tool calls).
	steps := []provider.ScriptedStep{
		{
			// Step 1: agent writes to blackboard
			ToolCalls: []provider.ToolCall{
				{
					ID:   "tc-1",
					Name: "blackboard_write",
					Arguments: map[string]any{
						"section": "results",
						"key":     "answer",
						"value":   "42",
					},
				},
			},
		},
		{
			// Step 2: agent sends a message
			ToolCalls: []provider.ToolCall{
				{
					ID:   "tc-2",
					Name: "send_message",
					Arguments: map[string]any{
						"to":      "reviewer",
						"type":    "result",
						"content": "done computing",
					},
				},
			},
		},
		{
			// Step 3: agent writes "done" to status, triggering ShouldStop
			ToolCalls: []provider.ToolCall{
				{
					ID:   "tc-3",
					Name: "blackboard_write",
					Arguments: map[string]any{
						"section": "status",
						"key":     "", // placeholder — will be set dynamically
						"value":   "done",
					},
				},
			},
		},
		{
			// Step 4: final response (should not be reached if ShouldStop works)
			Content: "finished",
		},
	}

	src := provider.NewScriptedSource(steps, false)
	prov := provider.NewTestProvider(src)

	cfg := NodeConfig{
		Name:          "test-agent",
		Role:          "solver",
		Model:         "mock",
		Provider:      "test",
		Location:      "local",
		SystemPrompt:  "You are a test agent.",
		MaxIterations: 10,
	}

	node := NewLocalNode(cfg, prov, nil)
	nodeID := node.ID()

	// Fix step 3 to use the actual dynamic node ID
	steps[2].ToolCalls[0].Arguments["key"] = nodeID

	outbox := make(chan Message, 64)
	inbox := make(chan Message, 64)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := node.Run(ctx, "compute the answer", bb, inbox, outbox)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// Verify blackboard write
	e, ok := bb.Read("results", "answer")
	if !ok {
		t.Fatal("expected blackboard entry results/answer")
	}
	if e.Value != "42" {
		t.Fatalf("expected value '42', got %v", e.Value)
	}
	if e.Author != nodeID {
		t.Fatalf("expected author %q, got %q", nodeID, e.Author)
	}

	// Verify message was sent through outbox
	select {
	case msg := <-outbox:
		if msg.To != "reviewer" || msg.Content != "done computing" {
			t.Fatalf("unexpected outbox message: %+v", msg)
		}
	default:
		t.Fatal("expected message in outbox")
	}

	// Verify ShouldStop triggered via status
	statusEntry, ok := bb.Read("status", nodeID)
	if !ok {
		t.Fatal("expected status entry for node")
	}
	if statusEntry.Value != "done" {
		t.Fatalf("expected status 'done', got %v", statusEntry.Value)
	}
}

func TestLocalNode_Info(t *testing.T) {
	cfg := NodeConfig{
		Name:     "planner",
		Role:     "planner",
		Model:    "gpt-4",
		Provider: "openai",
		Location: "local",
	}
	src := provider.NewScriptedSource(nil, false)
	prov := provider.NewTestProvider(src)
	node := NewLocalNode(cfg, prov, nil)

	info := node.Info()
	if info.Name != "planner" || info.Role != "planner" || info.Model != "gpt-4" {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestLocalNode_ContextCancellation(t *testing.T) {
	// The agent will keep getting tool calls but context gets cancelled
	steps := []provider.ScriptedStep{
		{Content: "thinking..."},
	}
	src := provider.NewScriptedSource(steps, true) // loop forever
	prov := provider.NewTestProvider(src)

	cfg := NodeConfig{
		Name:          "cancellable",
		Role:          "worker",
		MaxIterations: 100,
	}
	node := NewLocalNode(cfg, prov, nil)

	bb := NewBlackboard()
	outbox := make(chan Message, 64)
	inbox := make(chan Message, 64)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = node.Run(ctx, "infinite task", bb, inbox, outbox)
	// We just verify it doesn't hang — context cancellation should stop it
}
