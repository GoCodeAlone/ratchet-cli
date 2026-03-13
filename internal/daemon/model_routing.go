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
