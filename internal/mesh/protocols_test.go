package mesh

import (
	"testing"
)

func TestHandoffProtocol(t *testing.T) {
	bb := NewBlackboard()

	// Design team hands off to dev.
	WriteHandoff(bb, "design", "dev", map[string]string{
		"spec": "REST API with /users endpoint",
	})

	// Dev team reads the handoff.
	handoff, ok := ReadHandoff(bb, "design", "dev")
	if !ok {
		t.Fatal("handoff not found")
	}
	if handoff["spec"] != "REST API with /users endpoint" {
		t.Errorf("got spec %q", handoff["spec"])
	}
}

func TestDirectiveProtocol(t *testing.T) {
	bb := NewBlackboard()

	// Oversight writes directive to dev.
	WriteDirective(bb, "dev", "Focus on error handling tests")

	// Dev reads its directive.
	directive, ok := ReadLatestDirective(bb, "dev")
	if !ok {
		t.Fatal("directive not found")
	}
	if directive != "Focus on error handling tests" {
		t.Errorf("got directive %q", directive)
	}
}
