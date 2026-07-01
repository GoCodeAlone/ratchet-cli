package retro

import (
	"context"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	"github.com/GoCodeAlone/workflow/secrets"
)

const (
	EventError            = "error"
	EventTestFailure      = "test_failure"
	EventPermissionDenied = "permission_denied"
	EventCommandOutcome   = "command_outcome"
)

// Event is one piece of session evidence available to the retro analyzer.
type Event struct {
	Kind    string
	Message string
	Command string
	Outcome string
}

// Input is the bounded session context analyzed by the optional retro loop.
type Input struct {
	SessionID  string
	WorkingDir string
	Events     []Event
}

// Finding is a compact improvement observation.
type Finding struct {
	Pattern        string
	Evidence       string
	LocalAction    string
	UpstreamAction string
}

// Report is the analyzer output.
type Report struct {
	SessionID string
	Findings  []Finding
}

type Analyzer struct {
	redactor *secrets.Redactor
}

func NewAnalyzer(redactor *secrets.Redactor) *Analyzer {
	return &Analyzer{redactor: redactor}
}

func (a *Analyzer) Analyze(_ context.Context, input Input) Report {
	report := Report{SessionID: input.SessionID}
	for _, event := range input.Events {
		finding, ok := a.findingForEvent(event)
		if !ok {
			continue
		}
		report.Findings = append(report.Findings, finding)
	}
	return report
}

func (a *Analyzer) findingForEvent(event Event) (Finding, bool) {
	evidence := a.redact(firstNonEmpty(event.Message, event.Command, event.Outcome))
	switch event.Kind {
	case EventPermissionDenied:
		return Finding{
			Pattern:        "permission denial",
			Evidence:       evidence,
			LocalAction:    "Review local trust or permission configuration for this command class.",
			UpstreamAction: "If the denial reflects missing ratchet-cli policy ergonomics, submit a PR with the denied command and expected policy behavior.",
		}, true
	case EventTestFailure:
		return Finding{
			Pattern:        "test failure",
			Evidence:       evidence,
			LocalAction:    "Record the failing command and rerun the focused test before changing code.",
			UpstreamAction: "If the failure class requires harness support, submit a PR with a regression test and the local failure evidence.",
		}, true
	case EventError:
		return Finding{
			Pattern:        "runtime error",
			Evidence:       evidence,
			LocalAction:    "Check whether local configuration or missing credentials caused the error.",
			UpstreamAction: "If the error requires ratchet-cli code changes, submit a PR with the stack or command evidence and a proposed fix path.",
		}, true
	case EventCommandOutcome:
		if event.Outcome == "failed" {
			return Finding{
				Pattern:        "failed command",
				Evidence:       evidence,
				LocalAction:    "Prefer a focused retry or config adjustment before broad workflow changes.",
				UpstreamAction: "If retries show the command path is unsupported, submit a PR describing the missing command capability.",
			}, true
		}
	}
	return Finding{}, false
}

func (a *Analyzer) redact(text string) string {
	if a == nil || a.redactor == nil {
		return text
	}
	return a.redactor.Redact(text)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// RoutedActions separates locally actionable items from upstream PR instructions.
type RoutedActions struct {
	LocalActions         []string
	UpstreamInstructions []string
}

func RouteFindings(cfg config.RetroConfig, report Report) RoutedActions {
	if !cfg.Enabled {
		return RoutedActions{}
	}
	var routed RoutedActions
	for _, finding := range report.Findings {
		if cfg.LocalChanges && finding.LocalAction != "" {
			routed.LocalActions = append(routed.LocalActions, finding.LocalAction)
		}
		if cfg.UpstreamInstructions && finding.UpstreamAction != "" {
			routed.UpstreamInstructions = append(routed.UpstreamInstructions, finding.UpstreamAction)
		}
	}
	return routed
}
