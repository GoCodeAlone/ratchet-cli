package retro

import (
	"context"
	"fmt"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	"github.com/GoCodeAlone/workflow/secrets"
)

const (
	EventError            = "error"
	EventTestFailure      = "test_failure"
	EventPermissionDenied = "permission_denied"
	EventCommandOutcome   = "command_outcome"
	EventSessionCreated   = "session_created"
	EventSessionCompleted = "session_completed"
)

const (
	ProjectLocalConfig = "local_config"
	ProjectRatchetCLI  = "ratchet-cli"
)

// Event is one piece of session evidence available to the retro analyzer.
type Event struct {
	Timestamp  time.Time `json:"timestamp"`
	SessionID  string    `json:"session_id,omitempty"`
	Kind       string    `json:"kind"`
	Message    string    `json:"message,omitempty"`
	Command    string    `json:"command,omitempty"`
	Outcome    string    `json:"outcome,omitempty"`
	WorkingDir string    `json:"working_dir,omitempty"`
	Project    string    `json:"project,omitempty"`
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
	Project        string
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
			Project:        firstNonEmpty(event.Project, ProjectLocalConfig),
			LocalAction:    "Review local trust or permission configuration for this command class.",
			UpstreamAction: "If the denial reflects missing ratchet-cli policy ergonomics, submit a PR with the denied command and expected policy behavior.",
		}, true
	case EventTestFailure:
		return Finding{
			Pattern:        "test failure",
			Evidence:       evidence,
			Project:        firstNonEmpty(event.Project, ProjectRatchetCLI),
			LocalAction:    "Record the failing command and rerun the focused test before changing code.",
			UpstreamAction: "If the failure class requires harness support, submit a PR with a regression test and the local failure evidence.",
		}, true
	case EventError:
		return Finding{
			Pattern:        "runtime error",
			Evidence:       evidence,
			Project:        firstNonEmpty(event.Project, ProjectRatchetCLI),
			LocalAction:    "Check whether local configuration or missing credentials caused the error.",
			UpstreamAction: "If the error requires ratchet-cli code changes, submit a PR with the stack or command evidence and a proposed fix path.",
		}, true
	case EventCommandOutcome:
		if event.Outcome == "failed" {
			return Finding{
				Pattern:        "failed command",
				Evidence:       evidence,
				Project:        firstNonEmpty(event.Project, ProjectRatchetCLI),
				LocalAction:    "Prefer a focused retry or config adjustment before broad workflow changes.",
				UpstreamAction: "If retries show the command path is unsupported, submit a PR describing the missing command capability.",
			}, true
		}
	case EventSessionCreated:
		return Finding{}, false
	case EventSessionCompleted:
		return Finding{}, false
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
			if finding.Project == ProjectLocalConfig {
				continue
			}
		}
		if !cfg.UpstreamInstructions {
			continue
		}
		switch {
		case finding.Project == ProjectLocalConfig && finding.LocalAction != "":
			routed.UpstreamInstructions = append(routed.UpstreamInstructions,
				"local changes are disabled; apply manually if appropriate: "+finding.LocalAction)
		case finding.Project == ProjectRatchetCLI && finding.UpstreamAction != "":
			routed.UpstreamInstructions = append(routed.UpstreamInstructions,
				"ratchet-cli PR instruction: "+finding.UpstreamAction)
		case finding.Project != "" && finding.Project != ProjectLocalConfig && finding.UpstreamAction != "":
			routed.UpstreamInstructions = append(routed.UpstreamInstructions,
				fmt.Sprintf("third-party project %s instruction: %s", finding.Project, finding.UpstreamAction))
		case finding.UpstreamAction != "":
			routed.UpstreamInstructions = append(routed.UpstreamInstructions, finding.UpstreamAction)
		}
	}
	return routed
}
