package daemon

import (
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
)

func TestModelRouting_Classification(t *testing.T) {
	cases := []struct {
		stepID string
		want   stepComplexity
	}{
		// Simple
		{"validate-input", complexitySimple},
		{"log-result", complexitySimple},
		{"set-variable", complexitySimple},
		// Complex
		{"http_call-api", complexityComplex},
		{"db_query-users", complexityComplex},
		{"execute-script", complexityComplex},
		// Review
		{"code-review", complexityReview},
		{"audit-permissions", complexityReview},
		// Unknown defaults to complex
		{"unknown-step", complexityComplex},
	}
	for _, tc := range cases {
		t.Run(tc.stepID, func(t *testing.T) {
			got := ClassifyStep(tc.stepID)
			if got != tc.want {
				t.Errorf("ClassifyStep(%q) = %v, want %v", tc.stepID, got, tc.want)
			}
		})
	}
}

func TestModelRouting_ModelForStep(t *testing.T) {
	routing := config.ModelRouting{
		SimpleTaskModel:  "haiku",
		ComplexTaskModel: "sonnet",
		ReviewModel:      "opus",
	}
	cases := []struct {
		stepID string
		want   string
	}{
		{"log-result", "haiku"},
		{"http_call-external", "sonnet"},
		{"code-review-pr", "opus"},
		{"unknown-step", "sonnet"}, // complex default
	}
	for _, tc := range cases {
		t.Run(tc.stepID, func(t *testing.T) {
			got := ModelForStep(tc.stepID, routing)
			if got != tc.want {
				t.Errorf("ModelForStep(%q) = %q, want %q", tc.stepID, got, tc.want)
			}
		})
	}
}

func TestModelRouting_EmptyConfig(t *testing.T) {
	// When routing is zero-value, ModelForStep returns empty string (use default)
	model := ModelForStep("anything", config.ModelRouting{})
	if model != "" {
		t.Errorf("expected empty string for zero routing config, got %q", model)
	}
}

func TestModelRouting_CostBreakdown(t *testing.T) {
	routing := config.ModelRouting{
		SimpleTaskModel:  "haiku",
		ComplexTaskModel: "sonnet",
		ReviewModel:      "opus",
	}
	steps := []string{"log-result", "http_call-api", "code-review-pr"}
	entries := FleetCostBreakdown(steps, routing)

	if len(entries) != len(steps) {
		t.Fatalf("expected %d entries, got %d", len(steps), len(entries))
	}

	want := []struct {
		model      string
		complexity string
	}{
		{"haiku", "simple"},
		{"sonnet", "complex"},
		{"opus", "review"},
	}
	for i, e := range entries {
		if e.Model != want[i].model {
			t.Errorf("entry[%d] model = %q, want %q", i, e.Model, want[i].model)
		}
		if e.Complexity != want[i].complexity {
			t.Errorf("entry[%d] complexity = %q, want %q", i, e.Complexity, want[i].complexity)
		}
		if e.StepID != steps[i] {
			t.Errorf("entry[%d] StepID = %q, want %q", i, e.StepID, steps[i])
		}
	}
}
