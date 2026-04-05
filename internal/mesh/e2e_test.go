package mesh

import (
	"context"
	"testing"
	"time"

	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

// TestE2E_ThreeAgentFlow runs a full architect→coder→reviewer flow using
// scripted/mock providers and verifies the blackboard-based coordination.
func TestE2E_ThreeAgentFlow(t *testing.T) {
	// --- Architect ---
	// Writes a plan, then messages the coder, then marks done using its name.
	architectSteps := []provider.ScriptedStep{
		{
			ToolCalls: []provider.ToolCall{
				{
					ID:   "a-1",
					Name: "blackboard_write",
					Arguments: map[string]any{
						"section": "plan",
						"key":     "design",
						"value":   "Create REST API with /tasks endpoint, use handler+service pattern",
					},
				},
			},
		},
		{
			ToolCalls: []provider.ToolCall{
				{
					ID:   "a-2",
					Name: "send_message",
					Arguments: map[string]any{
						"to":      "*",
						"type":    "task",
						"content": "Plan is ready. Please implement the REST API.",
					},
				},
			},
		},
		{
			ToolCalls: []provider.ToolCall{
				{
					ID:   "a-3",
					Name: "blackboard_write",
					Arguments: map[string]any{
						"section": "status",
						"key":     "architect",
						"value":   "done",
					},
				},
			},
		},
		{Content: "architect finished"},
	}

	// --- Coder ---
	// Reads plan, writes code, messages reviewer, marks done.
	coderSteps := []provider.ScriptedStep{
		{
			ToolCalls: []provider.ToolCall{
				{
					ID:   "c-1",
					Name: "blackboard_read",
					Arguments: map[string]any{
						"section": "plan",
						"key":     "design",
					},
				},
			},
		},
		{
			ToolCalls: []provider.ToolCall{
				{
					ID:   "c-2",
					Name: "blackboard_write",
					Arguments: map[string]any{
						"section": "code",
						"key":     "main.go",
						"value":   "package main\n\nimport \"net/http\"\n\nfunc main() {\n  http.HandleFunc(\"/tasks\", handleTasks)\n  http.ListenAndServe(\":8080\", nil)\n}\n\nfunc handleTasks(w http.ResponseWriter, r *http.Request) {\n  w.Write([]byte(`{\"tasks\":[]}`))\n}",
					},
				},
			},
		},
		{
			ToolCalls: []provider.ToolCall{
				{
					ID:   "c-3",
					Name: "send_message",
					Arguments: map[string]any{
						"to":      "*",
						"type":    "task",
						"content": "Code is ready for review.",
					},
				},
			},
		},
		{
			ToolCalls: []provider.ToolCall{
				{
					ID:   "c-4",
					Name: "blackboard_write",
					Arguments: map[string]any{
						"section": "status",
						"key":     "coder",
						"value":   "done",
					},
				},
			},
		},
		{Content: "coder finished"},
	}

	// --- Reviewer ---
	// Reads code, writes review, writes "approved" to status.
	reviewerSteps := []provider.ScriptedStep{
		{
			ToolCalls: []provider.ToolCall{
				{
					ID:   "r-1",
					Name: "blackboard_read",
					Arguments: map[string]any{
						"section": "code",
						"key":     "main.go",
					},
				},
			},
		},
		{
			ToolCalls: []provider.ToolCall{
				{
					ID:   "r-2",
					Name: "blackboard_write",
					Arguments: map[string]any{
						"section": "reviews",
						"key":     "main.go",
						"value":   "LGTM - clean handler pattern, consider adding error handling for production",
					},
				},
			},
		},
		{
			ToolCalls: []provider.ToolCall{
				{
					ID:   "r-3",
					Name: "blackboard_write",
					Arguments: map[string]any{
						"section": "status",
						"key":     "review",
						"value":   "approved",
					},
				},
			},
		},
		{
			ToolCalls: []provider.ToolCall{
				{
					ID:   "r-4",
					Name: "blackboard_write",
					Arguments: map[string]any{
						"section": "status",
						"key":     "reviewer",
						"value":   "done",
					},
				},
			},
		},
		{Content: "reviewer finished"},
	}

	// Build scripted providers.
	architectSrc := provider.NewScriptedSource(architectSteps, false)
	coderSrc := provider.NewScriptedSource(coderSteps, false)
	reviewerSrc := provider.NewScriptedSource(reviewerSteps, false)

	architectProv := provider.NewTestProvider(architectSrc)
	coderProv := provider.NewTestProvider(coderSrc)
	reviewerProv := provider.NewTestProvider(reviewerSrc)

	configs := []NodeConfig{
		{
			Name:          "architect",
			Role:          "architect",
			Model:         "mock",
			Provider:      "test",
			SystemPrompt:  "You are a software architect.",
			MaxIterations: 10,
		},
		{
			Name:          "coder",
			Role:          "coder",
			Model:         "mock",
			Provider:      "test",
			SystemPrompt:  "You are a senior developer.",
			MaxIterations: 10,
		},
		{
			Name:          "reviewer",
			Role:          "reviewer",
			Model:         "mock",
			Provider:      "test",
			SystemPrompt:  "You are a code reviewer.",
			MaxIterations: 10,
		},
	}

	providerIdx := 0
	providers := []provider.Provider{architectProv, coderProv, reviewerProv}

	m := NewAgentMesh()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	handle, err := m.SpawnTeam(ctx, "Build a REST API for task management", configs, func(cfg NodeConfig) provider.Provider {
		p := providers[providerIdx]
		providerIdx++
		return p
	})
	if err != nil {
		t.Fatalf("SpawnTeam: %v", err)
	}

	// Collect events.
	var events []Event
	for ev := range handle.Events {
		events = append(events, ev)
	}

	// Wait for result.
	select {
	case result := <-handle.Done:
		if result.Status != "completed" {
			t.Fatalf("expected 'completed', got %q; errors: %v", result.Status, result.Errors)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for team completion")
	}

	// Verify events include spawns for all three agents.
	spawnCount := 0
	for _, ev := range events {
		if ev.Type == "agent_spawned" {
			spawnCount++
		}
	}
	if spawnCount != 3 {
		t.Errorf("expected 3 agent_spawned events, got %d", spawnCount)
	}
}
