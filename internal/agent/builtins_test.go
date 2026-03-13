package agent

import "testing"

func TestBuiltinAgents_CodeReviewerLoads(t *testing.T) {
	defs, err := LoadBuiltins()
	if err != nil {
		t.Fatalf("LoadBuiltins: %v", err)
	}
	if len(defs) == 0 {
		t.Fatal("expected at least one built-in agent definition")
	}

	var reviewer *AgentDefinition
	for i := range defs {
		if defs[i].Name == "code-reviewer" {
			reviewer = &defs[i]
			break
		}
	}
	if reviewer == nil {
		t.Fatal("code-reviewer built-in not found")
	}
	if reviewer.Role == "" {
		t.Error("code-reviewer: expected non-empty role")
	}
	if reviewer.SystemPrompt == "" {
		t.Error("code-reviewer: expected non-empty system_prompt")
	}
	if len(reviewer.Tools) == 0 {
		t.Error("code-reviewer: expected at least one tool")
	}
	if reviewer.MaxIterations <= 0 {
		t.Errorf("code-reviewer: expected max_iterations > 0, got %d", reviewer.MaxIterations)
	}
}
