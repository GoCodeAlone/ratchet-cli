package mesh

import (
	"context"
	"testing"
	"time"

	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

func TestSpawnTeam_TwoNodes(t *testing.T) {
	// Planner: writes a plan to the blackboard, sends a message to the worker,
	// then marks itself done.
	plannerSteps := []provider.ScriptedStep{
		{
			// Write plan to blackboard.
			ToolCalls: []provider.ToolCall{
				{
					ID:   "p-1",
					Name: "blackboard_write",
					Arguments: map[string]any{
						"section": "plan",
						"key":     "task",
						"value":   "implement feature X",
					},
				},
			},
		},
		{
			// Send task to worker (use broadcast since we don't know the worker's dynamic ID).
			ToolCalls: []provider.ToolCall{
				{
					ID:   "p-2",
					Name: "send_message",
					Arguments: map[string]any{
						"to":      "*",
						"type":    "task",
						"content": "please implement feature X",
					},
				},
			},
		},
		{
			// Mark planner done — placeholder key, set below.
			ToolCalls: []provider.ToolCall{
				{
					ID:   "p-3",
					Name: "blackboard_write",
					Arguments: map[string]any{
						"section": "status",
						"key":     "", // set after node creation
						"value":   "done",
					},
				},
			},
		},
		{Content: "planner finished"},
	}

	// Worker: reads the plan, writes code, writes an artifact, marks done.
	workerSteps := []provider.ScriptedStep{
		{
			// Read the plan.
			ToolCalls: []provider.ToolCall{
				{
					ID:   "w-1",
					Name: "blackboard_read",
					Arguments: map[string]any{
						"section": "plan",
						"key":     "task",
					},
				},
			},
		},
		{
			// Write code to blackboard.
			ToolCalls: []provider.ToolCall{
				{
					ID:   "w-2",
					Name: "blackboard_write",
					Arguments: map[string]any{
						"section": "code",
						"key":     "main.go",
						"value":   "package main\nfunc main() {}",
					},
				},
			},
		},
		{
			// Write artifact.
			ToolCalls: []provider.ToolCall{
				{
					ID:   "w-3",
					Name: "blackboard_write",
					Arguments: map[string]any{
						"section": "artifacts",
						"key":     "output",
						"value":   "feature X implemented",
					},
				},
			},
		},
		{
			// Mark worker done — placeholder key, set below.
			ToolCalls: []provider.ToolCall{
				{
					ID:   "w-4",
					Name: "blackboard_write",
					Arguments: map[string]any{
						"section": "status",
						"key":     "", // set after node creation
						"value":   "done",
					},
				},
			},
		},
		{Content: "worker finished"},
	}

	// Build providers. We need the scripted sources so we can fix up the
	// status keys after node IDs are assigned.
	plannerSrc := provider.NewScriptedSource(plannerSteps, false)
	workerSrc := provider.NewScriptedSource(workerSteps, false)

	plannerProv := provider.NewTestProvider(plannerSrc)
	workerProv := provider.NewTestProvider(workerSrc)

	configs := []NodeConfig{
		{
			Name:          "planner",
			Role:          "planner",
			Model:         "mock",
			Provider:      "test",
			SystemPrompt:  "You are a planner.",
			MaxIterations: 10,
		},
		{
			Name:          "worker",
			Role:          "worker",
			Model:         "mock",
			Provider:      "test",
			SystemPrompt:  "You are a worker.",
			MaxIterations: 10,
		},
	}

	providerIdx := 0
	providers := []provider.Provider{plannerProv, workerProv}

	mesh := NewAgentMesh()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	handle, err := mesh.SpawnTeam(ctx, "build feature X", configs, func(cfg NodeConfig) provider.Provider {
		p := providers[providerIdx]
		providerIdx++
		return p
	})
	if err != nil {
		t.Fatalf("SpawnTeam: %v", err)
	}

	// Fix up the status keys with the real dynamic node IDs.
	// We need to find the actual node IDs that were generated.
	mesh.mu.RLock()
	var plannerID, workerID string
	for id, node := range mesh.nodes {
		info := node.Info()
		if info.Name == "planner" {
			plannerID = id
		}
		if info.Name == "worker" {
			workerID = id
		}
	}
	mesh.mu.RUnlock()

	if plannerID == "" || workerID == "" {
		t.Fatal("expected both planner and worker nodes to be registered")
	}

	// Patch the scripted steps to use real node IDs for the status writes.
	plannerSteps[2].ToolCalls[0].Arguments["key"] = plannerID
	workerSteps[3].ToolCalls[0].Arguments["key"] = workerID

	// Wait for completion.
	select {
	case result := <-handle.Done:
		if result.Status != "completed" {
			t.Fatalf("expected status 'completed', got %q; errors: %v", result.Status, result.Errors)
		}

		// Check artifacts.
		art, ok := result.Artifacts["output"]
		if !ok {
			t.Fatal("expected artifact 'output'")
		}
		if art.Value != "feature X implemented" {
			t.Fatalf("unexpected artifact value: %v", art.Value)
		}

	case <-ctx.Done():
		t.Fatal("timed out waiting for team completion")
	}
}

func TestSpawnTeam_EmptyConfigs(t *testing.T) {
	mesh := NewAgentMesh()
	_, err := mesh.SpawnTeam(context.Background(), "task", nil, nil)
	if err == nil {
		t.Fatal("expected error for empty configs")
	}
}

func TestSpawnTeam_CancelledContext(t *testing.T) {
	steps := []provider.ScriptedStep{
		{Content: "thinking..."},
	}
	src := provider.NewScriptedSource(steps, true)
	prov := provider.NewTestProvider(src)

	configs := []NodeConfig{
		{
			Name:          "agent",
			Role:          "worker",
			MaxIterations: 100,
		},
	}

	mesh := NewAgentMesh()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	handle, err := mesh.SpawnTeam(ctx, "infinite task", configs, func(_ NodeConfig) provider.Provider {
		return prov
	})
	if err != nil {
		t.Fatalf("SpawnTeam: %v", err)
	}

	// The team should complete (with or without errors) once context times out.
	select {
	case <-handle.Done:
		// ok
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for cancelled team")
	}
}

func TestSpawnTeam_Events(t *testing.T) {
	steps := []provider.ScriptedStep{
		{
			ToolCalls: []provider.ToolCall{
				{
					ID:   "tc-1",
					Name: "blackboard_write",
					Arguments: map[string]any{
						"section": "status",
						"key":     "", // set below
						"value":   "done",
					},
				},
			},
		},
		{Content: "done"},
	}

	src := provider.NewScriptedSource(steps, false)
	prov := provider.NewTestProvider(src)

	configs := []NodeConfig{
		{
			Name:          "eventer",
			Role:          "worker",
			MaxIterations: 10,
		},
	}

	mesh := NewAgentMesh()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	handle, err := mesh.SpawnTeam(ctx, "event task", configs, func(_ NodeConfig) provider.Provider {
		return prov
	})
	if err != nil {
		t.Fatalf("SpawnTeam: %v", err)
	}

	// Fix up status key.
	mesh.mu.RLock()
	var nodeID string
	for id := range mesh.nodes {
		nodeID = id
	}
	mesh.mu.RUnlock()
	steps[0].ToolCalls[0].Arguments["key"] = nodeID

	// Collect events until done.
	var events []Event
	for {
		select {
		case ev, ok := <-handle.Events:
			if !ok {
				goto collected
			}
			events = append(events, ev)
		case <-ctx.Done():
			t.Fatal("timed out collecting events")
		}
	}
collected:

	// Should have at least agent_spawned.
	hasSpawn := false
	for _, ev := range events {
		if ev.Type == "agent_spawned" {
			hasSpawn = true
			break
		}
	}
	if !hasSpawn {
		t.Fatal("expected at least one agent_spawned event")
	}
}
