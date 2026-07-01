package retro

import (
	"context"
	"strings"
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	"github.com/GoCodeAlone/workflow/secrets"
)

func TestAnalyzerRedactsSecretEvidence(t *testing.T) {
	redactor := secrets.NewRedactor()
	redactor.AddValue("api-key", "sk-live-secret")
	analyzer := NewAnalyzer(redactor)

	report := analyzer.Analyze(context.Background(), Input{
		SessionID: "s1",
		Events: []Event{
			{Kind: EventPermissionDenied, Message: "denied bash with sk-live-secret"},
			{Kind: EventTestFailure, Message: "go test failed with token sk-live-secret"},
		},
	})

	if len(report.Findings) == 0 {
		t.Fatal("expected findings")
	}
	for _, finding := range report.Findings {
		joined := finding.Pattern + finding.Evidence + finding.LocalAction + finding.UpstreamAction
		if strings.Contains(joined, "sk-live-secret") {
			t.Fatalf("finding leaked secret: %#v", finding)
		}
		if !strings.Contains(joined, "[REDACTED:api-key]") {
			t.Fatalf("finding missing redacted marker: %#v", finding)
		}
	}
}

func TestRouteFindingsHonorsRetroConfig(t *testing.T) {
	report := Report{Findings: []Finding{{
		Pattern:        "repeated test failure",
		Evidence:       "go test failed twice",
		LocalAction:    "raise local timeout",
		UpstreamAction: "submit PR with focused regression",
	}}}

	disabled := RouteFindings(config.RetroConfig{}, report)
	if len(disabled.LocalActions) != 0 || len(disabled.UpstreamInstructions) != 0 {
		t.Fatalf("disabled routing produced actions: %#v", disabled)
	}

	enabled := RouteFindings(config.RetroConfig{
		Enabled:              true,
		LocalChanges:         true,
		UpstreamInstructions: true,
	}, report)
	if len(enabled.LocalActions) != 1 {
		t.Fatalf("local actions = %#v", enabled.LocalActions)
	}
	if len(enabled.UpstreamInstructions) != 1 {
		t.Fatalf("upstream instructions = %#v", enabled.UpstreamInstructions)
	}
}
