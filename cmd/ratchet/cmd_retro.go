package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	"github.com/GoCodeAlone/ratchet-cli/internal/retro"
)

type retroAnalyzeOutput struct {
	SessionID            string                `json:"session_id,omitempty"`
	Findings             []retroAnalyzeFinding `json:"findings"`
	LocalActions         []string              `json:"local_actions,omitempty"`
	UpstreamInstructions []string              `json:"upstream_instructions,omitempty"`
}

type retroAnalyzeFinding struct {
	Pattern        string `json:"pattern"`
	Evidence       string `json:"evidence"`
	Project        string `json:"project,omitempty"`
	LocalAction    string `json:"local_action,omitempty"`
	UpstreamAction string `json:"upstream_action,omitempty"`
}

type retroAnalyzeOptions struct {
	EvidencePath string
	SessionID    string
	JSON         bool
}

var exitProcess = os.Exit

func handleRetro(args []string) {
	if err := runRetro(context.Background(), args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "retro error: %v\n", err)
		exitProcess(1)
	}
}

func runRetro(ctx context.Context, args []string, w io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: ratchet retro analyze --evidence <evidence.jsonl> [--session ID] [--json]")
	}
	switch args[0] {
	case "analyze":
		opts, err := parseRetroAnalyzeArgs(args[1:])
		if err != nil {
			return err
		}
		return executeRetroAnalyze(ctx, opts, w)
	default:
		return fmt.Errorf("unknown retro command %q", args[0])
	}
}

func parseRetroAnalyzeArgs(args []string) (retroAnalyzeOptions, error) {
	var opts retroAnalyzeOptions
	fs := flag.NewFlagSet("ratchet retro analyze", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.EvidencePath, "evidence", "", "retro evidence JSONL file")
	fs.StringVar(&opts.SessionID, "session", "", "session id to analyze")
	fs.BoolVar(&opts.JSON, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return retroAnalyzeOptions{}, err
	}
	if opts.EvidencePath == "" {
		return retroAnalyzeOptions{}, errors.New("usage: ratchet retro analyze --evidence <evidence.jsonl> [--session ID] [--json]")
	}
	if fs.NArg() != 0 {
		return retroAnalyzeOptions{}, errors.New("usage: ratchet retro analyze --evidence <evidence.jsonl> [--session ID] [--json]")
	}
	return opts, nil
}

func executeRetroAnalyze(ctx context.Context, opts retroAnalyzeOptions, w io.Writer) error {
	if _, err := os.Stat(opts.EvidencePath); err != nil {
		return fmt.Errorf("read retro evidence: %w", err)
	}
	events, err := retro.NewEvidenceStore(opts.EvidencePath, nil).Load()
	if err != nil {
		return fmt.Errorf("read retro evidence: %w", err)
	}
	if opts.SessionID != "" {
		events = filterRetroEventsBySession(events, opts.SessionID)
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	report := retro.NewAnalyzer(nil).Analyze(ctx, retro.Input{
		SessionID: opts.SessionID,
		Events:    events,
	})
	routed := retro.RouteFindings(cfg.Retro, report)
	output := retroAnalyzeOutput{
		SessionID:            report.SessionID,
		Findings:             retroAnalyzeFindings(report.Findings),
		LocalActions:         routed.LocalActions,
		UpstreamInstructions: routed.UpstreamInstructions,
	}
	if opts.JSON {
		return json.NewEncoder(w).Encode(output)
	}
	printRetroAnalyzeText(w, output)
	return nil
}

func retroAnalyzeFindings(findings []retro.Finding) []retroAnalyzeFinding {
	out := make([]retroAnalyzeFinding, 0, len(findings))
	for _, finding := range findings {
		out = append(out, retroAnalyzeFinding{
			Pattern:        finding.Pattern,
			Evidence:       finding.Evidence,
			Project:        finding.Project,
			LocalAction:    finding.LocalAction,
			UpstreamAction: finding.UpstreamAction,
		})
	}
	return out
}

func filterRetroEventsBySession(events []retro.Event, sessionID string) []retro.Event {
	filtered := make([]retro.Event, 0, len(events))
	for _, event := range events {
		if event.SessionID == sessionID {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func printRetroAnalyzeText(w io.Writer, output retroAnalyzeOutput) {
	label := output.SessionID
	if label == "" {
		label = "all sessions"
	}
	fmt.Fprintf(w, "Retro analysis for %s\n\n", label)
	fmt.Fprintln(w, "Findings")
	if len(output.Findings) == 0 {
		fmt.Fprintln(w, "- none")
	} else {
		for _, finding := range output.Findings {
			fmt.Fprintf(w, "- %s: %s\n", finding.Pattern, finding.Evidence)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Local actions")
	if len(output.LocalActions) == 0 {
		fmt.Fprintln(w, "- none")
	} else {
		for _, action := range output.LocalActions {
			fmt.Fprintf(w, "- %s\n", action)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Upstream instructions")
	if len(output.UpstreamInstructions) == 0 {
		fmt.Fprintln(w, "- none")
	} else {
		for _, instruction := range output.UpstreamInstructions {
			fmt.Fprintf(w, "- %s\n", instruction)
		}
	}
}
