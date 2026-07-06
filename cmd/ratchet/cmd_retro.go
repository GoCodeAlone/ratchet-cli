package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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

type retroInstructionsOptions struct {
	EvidencePath string
	SessionID    string
	OutputPath   string
}

type retroBundleOptions struct {
	EvidencePath string
	SessionID    string
	OutputDir    string
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
		return errors.New("usage: ratchet retro <analyze|instructions|bundle> --evidence <evidence.jsonl> [--session ID]")
	}
	switch args[0] {
	case "analyze":
		opts, err := parseRetroAnalyzeArgs(args[1:])
		if err != nil {
			return err
		}
		return executeRetroAnalyze(ctx, opts, w)
	case "instructions":
		opts, err := parseRetroInstructionsArgs(args[1:])
		if err != nil {
			return err
		}
		return executeRetroInstructions(ctx, opts, w)
	case "bundle":
		opts, err := parseRetroBundleArgs(args[1:])
		if err != nil {
			return err
		}
		return executeRetroBundle(ctx, opts, w)
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
	output, err := buildRetroAnalyzeOutput(ctx, opts.EvidencePath, opts.SessionID)
	if err != nil {
		return err
	}
	if opts.JSON {
		return json.NewEncoder(w).Encode(output)
	}
	printRetroAnalyzeText(w, output)
	return nil
}

func parseRetroInstructionsArgs(args []string) (retroInstructionsOptions, error) {
	var opts retroInstructionsOptions
	fs := flag.NewFlagSet("ratchet retro instructions", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.EvidencePath, "evidence", "", "retro evidence JSONL file")
	fs.StringVar(&opts.SessionID, "session", "", "session id to analyze")
	fs.StringVar(&opts.OutputPath, "output", "", "write Markdown instructions to this path")
	if err := fs.Parse(args); err != nil {
		return retroInstructionsOptions{}, err
	}
	if opts.EvidencePath == "" {
		return retroInstructionsOptions{}, errors.New("usage: ratchet retro instructions --evidence <evidence.jsonl> [--session ID] [--output instructions.md]")
	}
	if fs.NArg() != 0 {
		return retroInstructionsOptions{}, errors.New("usage: ratchet retro instructions --evidence <evidence.jsonl> [--session ID] [--output instructions.md]")
	}
	return opts, nil
}

func executeRetroInstructions(ctx context.Context, opts retroInstructionsOptions, w io.Writer) error {
	output, err := buildRetroAnalyzeOutput(ctx, opts.EvidencePath, opts.SessionID)
	if err != nil {
		return err
	}
	markdown := renderRetroInstructionsMarkdown(output)
	if opts.OutputPath == "" {
		_, err := io.WriteString(w, markdown)
		return err
	}
	if err := os.WriteFile(opts.OutputPath, []byte(markdown), 0600); err != nil {
		return fmt.Errorf("write retro instructions: %w", err)
	}
	_, err = fmt.Fprintf(w, "wrote retro instructions: %s\n", opts.OutputPath)
	return err
}

func parseRetroBundleArgs(args []string) (retroBundleOptions, error) {
	var opts retroBundleOptions
	fs := flag.NewFlagSet("ratchet retro bundle", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.EvidencePath, "evidence", "", "retro evidence JSONL file")
	fs.StringVar(&opts.SessionID, "session", "", "session id to analyze")
	fs.StringVar(&opts.OutputDir, "output", "", "write bundle to this directory")
	if err := fs.Parse(args); err != nil {
		return retroBundleOptions{}, err
	}
	if opts.EvidencePath == "" || opts.OutputDir == "" {
		return retroBundleOptions{}, errors.New("usage: ratchet retro bundle --evidence <evidence.jsonl> [--session ID] --output <dir>")
	}
	if fs.NArg() != 0 {
		return retroBundleOptions{}, errors.New("usage: ratchet retro bundle --evidence <evidence.jsonl> [--session ID] --output <dir>")
	}
	return opts, nil
}

func executeRetroBundle(ctx context.Context, opts retroBundleOptions, w io.Writer) error {
	output, err := buildRetroAnalyzeOutput(ctx, opts.EvidencePath, opts.SessionID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(opts.OutputDir, 0700); err != nil {
		return fmt.Errorf("create retro bundle: %w", err)
	}
	analysis, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("encode retro analysis: %w", err)
	}
	analysis = append(analysis, '\n')
	if err := os.WriteFile(filepath.Join(opts.OutputDir, "analysis.json"), analysis, 0600); err != nil {
		return fmt.Errorf("write retro bundle analysis: %w", err)
	}
	instructions := renderRetroInstructionsMarkdown(output)
	if err := os.WriteFile(filepath.Join(opts.OutputDir, "instructions.md"), []byte(instructions), 0600); err != nil {
		return fmt.Errorf("write retro bundle instructions: %w", err)
	}
	manifest := struct {
		SessionID            string    `json:"session_id,omitempty"`
		CreatedAt            time.Time `json:"created_at"`
		Files                []string  `json:"files"`
		Findings             int       `json:"findings"`
		LocalActions         int       `json:"local_actions"`
		UpstreamInstructions int       `json:"upstream_instructions"`
		RawEvidenceCopied    bool      `json:"raw_evidence_copied"`
	}{
		SessionID:            output.SessionID,
		CreatedAt:            time.Now().UTC(),
		Files:                []string{"analysis.json", "instructions.md", "manifest.json"},
		Findings:             len(output.Findings),
		LocalActions:         len(output.LocalActions),
		UpstreamInstructions: len(output.UpstreamInstructions),
		RawEvidenceCopied:    false,
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode retro bundle manifest: %w", err)
	}
	manifestData = append(manifestData, '\n')
	if err := os.WriteFile(filepath.Join(opts.OutputDir, "manifest.json"), manifestData, 0600); err != nil {
		return fmt.Errorf("write retro bundle manifest: %w", err)
	}
	_, err = fmt.Fprintf(w, "wrote retro bundle: %s\n", opts.OutputDir)
	return err
}

func buildRetroAnalyzeOutput(ctx context.Context, evidencePath, sessionID string) (retroAnalyzeOutput, error) {
	if _, err := os.Stat(evidencePath); err != nil {
		return retroAnalyzeOutput{}, fmt.Errorf("read retro evidence: %w", err)
	}
	events, err := retro.NewEvidenceStore(evidencePath, nil).Load()
	if err != nil {
		return retroAnalyzeOutput{}, fmt.Errorf("read retro evidence: %w", err)
	}
	if sessionID != "" {
		events = filterRetroEventsBySession(events, sessionID)
	}
	cfg, err := config.Load()
	if err != nil {
		return retroAnalyzeOutput{}, fmt.Errorf("load config: %w", err)
	}
	report := retro.NewAnalyzer(nil).Analyze(ctx, retro.Input{
		SessionID: sessionID,
		Events:    events,
	})
	routed := retro.RouteFindings(cfg.Retro, report)
	return retroAnalyzeOutput{
		SessionID:            report.SessionID,
		Findings:             retroAnalyzeFindings(report.Findings),
		LocalActions:         routed.LocalActions,
		UpstreamInstructions: routed.UpstreamInstructions,
	}, nil
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

func renderRetroInstructionsMarkdown(output retroAnalyzeOutput) string {
	label := output.SessionID
	if label == "" {
		label = "all sessions"
	}
	var b strings.Builder
	fmt.Fprintln(&b, "# Ratchet Retro PR Instructions")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- Session: %s\n", label)
	fmt.Fprintln(&b, "- Scope: reporting-only handoff; review before opening any PR.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Findings")
	if len(output.Findings) == 0 {
		fmt.Fprintln(&b, "- none")
	} else {
		for _, finding := range output.Findings {
			fmt.Fprintf(&b, "- %s: %s\n", markdownListText(finding.Pattern), markdownListText(finding.Evidence))
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Upstream Instructions")
	if len(output.UpstreamInstructions) == 0 {
		fmt.Fprintln(&b, "- none")
	} else {
		for _, instruction := range output.UpstreamInstructions {
			fmt.Fprintf(&b, "- %s\n", markdownListText(instruction))
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Local Actions")
	if len(output.LocalActions) == 0 {
		fmt.Fprintln(&b, "- none")
	} else {
		for _, action := range output.LocalActions {
			fmt.Fprintf(&b, "- %s\n", markdownListText(action))
		}
	}
	return b.String()
}

func markdownListText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
