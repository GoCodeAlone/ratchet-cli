package daemon

import (
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
)

// stepComplexity classifies a step as "simple", "complex", or "review".
type stepComplexity int

const (
	complexitySimple  stepComplexity = iota
	complexityComplex stepComplexity = iota
	complexityReview  stepComplexity = iota
)

// simpleStepTypes contains step type name fragments that indicate lightweight work.
var simpleStepTypes = []string{
	"set", "log", "validate", "assert", "check", "echo", "noop", "sleep",
}

// complexStepTypes contains step type name fragments that indicate heavy work.
var complexStepTypes = []string{
	"http_call", "http", "db_query", "database", "sql", "code", "exec", "execute",
	"generate", "build", "compile", "deploy",
}

// reviewStepTypes contains step type name fragments that indicate review tasks.
var reviewStepTypes = []string{
	"review", "audit", "inspect", "check_code", "lint",
}

// ClassifyStep returns the complexity tier for a step based on its type/name.
func ClassifyStep(stepID string) stepComplexity {
	lower := strings.ToLower(stepID)
	for _, kw := range reviewStepTypes {
		if strings.Contains(lower, kw) {
			return complexityReview
		}
	}
	for _, kw := range complexStepTypes {
		if strings.Contains(lower, kw) {
			return complexityComplex
		}
	}
	for _, kw := range simpleStepTypes {
		if strings.Contains(lower, kw) {
			return complexitySimple
		}
	}
	// Default: complex (safer choice for unknown steps)
	return complexityComplex
}

// ModelForStep returns the model to use for a step based on routing config.
// Falls back to an empty string (caller should use default) when not configured.
func ModelForStep(stepID string, routing config.ModelRouting) string {
	switch ClassifyStep(stepID) {
	case complexitySimple:
		return routing.SimpleTaskModel
	case complexityReview:
		return routing.ReviewModel
	default:
		return routing.ComplexTaskModel
	}
}

// WorkerCostEntry holds the model assignment for a single fleet worker step.
type WorkerCostEntry struct {
	WorkerName string
	StepID     string
	Model      string
	Complexity string
}

// FleetCostBreakdown returns per-worker model assignments for a set of step IDs.
// This is the basis for estimating per-worker cost when routing to different models.
func FleetCostBreakdown(steps []string, routing config.ModelRouting) []WorkerCostEntry {
	entries := make([]WorkerCostEntry, len(steps))
	for i, stepID := range steps {
		c := ClassifyStep(stepID)
		var complexity string
		switch c {
		case complexitySimple:
			complexity = "simple"
		case complexityReview:
			complexity = "review"
		default:
			complexity = "complex"
		}
		entries[i] = WorkerCostEntry{
			WorkerName: strings.ToLower(strings.ReplaceAll(stepID, " ", "-")),
			StepID:     stepID,
			Model:      ModelForStep(stepID, routing),
			Complexity: complexity,
		}
	}
	return entries
}
