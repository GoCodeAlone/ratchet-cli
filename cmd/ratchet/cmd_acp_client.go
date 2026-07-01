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

	"github.com/GoCodeAlone/ratchet-cli/internal/acpclient"
)

type acpClientCommandKind string

const (
	acpClientCommandHandled      acpClientCommandKind = "handled"
	acpClientCommandHelp         acpClientCommandKind = "help"
	acpClientCommandExec         acpClientCommandKind = "exec"
	acpClientCommandSessionsList acpClientCommandKind = "sessions-list"
	acpClientCommandSessionsShow acpClientCommandKind = "sessions-show"
	acpClientCommandStatus       acpClientCommandKind = "status"
	acpClientCommandCancel       acpClientCommandKind = "cancel"
)

type acpClientCommand struct {
	kind      acpClientCommandKind
	exec      acpClientExecOptions
	sessionID string
}

type acpClientExecOptions struct {
	Agent   string
	Command string
	Args    []string
	Cwd     string
	Timeout time.Duration
	JSON    bool
	File    string
	Prompt  string
}

type repeatedStringFlag []string

func (f *repeatedStringFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *repeatedStringFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func handleACPClient(args []string) error {
	cmd, err := parseACPClientCommandWithOutput(args, os.Stdout)
	if err != nil {
		return err
	}
	switch cmd.kind {
	case acpClientCommandHandled:
		return nil
	case acpClientCommandHelp:
		printACPClientUsage(os.Stdout)
		return nil
	case acpClientCommandExec:
		return executeACPClientExec(context.Background(), cmd.exec, defaultACPClientExecRunner{}, os.Stdout)
	case acpClientCommandSessionsList, acpClientCommandSessionsShow, acpClientCommandStatus, acpClientCommandCancel:
		return fmt.Errorf("ratchet acp client %s is planned for a later PR", cmd.kind)
	default:
		return fmt.Errorf("unknown acp client command: %s", cmd.kind)
	}
}

type acpClientExecRunner interface {
	RunPrompt(ctx context.Context, spec acpclient.AgentSpec, opts acpclient.RunOptions, prompt string) (acpclient.Result, error)
}

type defaultACPClientExecRunner struct{}

func (defaultACPClientExecRunner) RunPrompt(ctx context.Context, spec acpclient.AgentSpec, opts acpclient.RunOptions, prompt string) (acpclient.Result, error) {
	client, err := acpclient.Start(ctx, spec, opts)
	if err != nil {
		return acpclient.Result{}, err
	}
	defer client.Close() //nolint:errcheck
	return client.RunPrompt(ctx, prompt)
}

func executeACPClientExec(ctx context.Context, opts acpClientExecOptions, runner acpClientExecRunner, w io.Writer) error {
	prompt := opts.Prompt
	if opts.File != "" {
		b, err := os.ReadFile(opts.File)
		if err != nil {
			return fmt.Errorf("read prompt file: %w", err)
		}
		prompt = string(b)
	}

	cwd := opts.Cwd
	if cwd == "" {
		cwd = "."
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}

	runOpts := acpclient.RunOptions{
		Agent:   opts.Agent,
		Command: opts.Command,
		Args:    opts.Args,
		Cwd:     cwd,
		Timeout: opts.Timeout,
	}
	spec, err := acpclient.DefaultRegistry().Resolve(runOpts)
	if err != nil {
		return err
	}
	result, err := runner.RunPrompt(ctx, spec, runOpts, prompt)
	if err != nil {
		return err
	}
	if opts.JSON {
		return writeACPClientExecJSON(w, spec, result)
	}
	writeACPClientExecHuman(w, result)
	return nil
}

func writeACPClientExecHuman(w io.Writer, result acpclient.Result) {
	if result.Text != "" {
		fmt.Fprint(w, result.Text)
		if !strings.HasSuffix(result.Text, "\n") {
			fmt.Fprintln(w)
		}
	}
	fmt.Fprintf(w, "[stop: %s]\n", result.StopReason)
}

func writeACPClientExecJSON(w io.Writer, spec acpclient.AgentSpec, result acpclient.Result) error {
	fingerprint := spec.Fingerprint()
	if len(fingerprint) > 12 {
		fingerprint = fingerprint[:12]
	}
	payload := struct {
		Command         string `json:"command"`
		SessionID       string `json:"session_id"`
		StopReason      string `json:"stop_reason"`
		Text            string `json:"text"`
		DurationMillis  int64  `json:"duration_ms"`
		CommandFpPrefix string `json:"command_fp_prefix"`
	}{
		Command:         spec.Command,
		SessionID:       string(result.SessionID),
		StopReason:      string(result.StopReason),
		Text:            result.Text,
		DurationMillis:  result.Duration.Milliseconds(),
		CommandFpPrefix: fingerprint,
	}
	return json.NewEncoder(w).Encode(payload)
}

func parseACPClientCommand(args []string) (acpClientCommand, error) {
	return parseACPClientCommandWithOutput(args, io.Discard)
}

func parseACPClientCommandWithOutput(args []string, output io.Writer) (acpClientCommand, error) {
	if len(args) == 0 {
		return acpClientCommand{kind: acpClientCommandHelp}, nil
	}
	switch args[0] {
	case "exec":
		execOpts, err := parseACPClientExec(args[1:], output)
		if errors.Is(err, flag.ErrHelp) {
			return acpClientCommand{kind: acpClientCommandHandled}, nil
		}
		if err != nil {
			return acpClientCommand{}, err
		}
		return acpClientCommand{kind: acpClientCommandExec, exec: execOpts}, nil
	case "sessions":
		return parseACPClientSessions(args[1:])
	case "status":
		id, err := requireSessionID("status", args[1:])
		if err != nil {
			return acpClientCommand{}, err
		}
		return acpClientCommand{kind: acpClientCommandStatus, sessionID: id}, nil
	case "cancel":
		id, err := requireSessionID("cancel", args[1:])
		if err != nil {
			return acpClientCommand{}, err
		}
		return acpClientCommand{kind: acpClientCommandCancel, sessionID: id}, nil
	case "help", "--help", "-h":
		return acpClientCommand{kind: acpClientCommandHelp}, nil
	default:
		return acpClientCommand{}, fmt.Errorf("unknown acp client command: %s", args[0])
	}
}

func parseACPClientExec(args []string, output io.Writer) (acpClientExecOptions, error) {
	var opts acpClientExecOptions
	opts.Cwd = "."
	opts.Timeout = 30 * time.Second

	fs := flag.NewFlagSet("ratchet acp client exec", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.Usage = func() {
		fmt.Fprintln(output, "Usage: ratchet acp client exec [flags] <prompt>")
		fmt.Fprintln(output)
		fmt.Fprintln(output, "Flags:")
		fmt.Fprintln(output, "  --agent string")
		fmt.Fprintln(output, "    agent template")
		fmt.Fprintln(output, "  --command string")
		fmt.Fprintln(output, "    agent command")
		fmt.Fprintln(output, "  --arg value")
		fmt.Fprintln(output, "    agent command argument; repeat for multiple args")
		fmt.Fprintln(output, "  --cwd string")
		fmt.Fprintf(output, "    working directory (default %q)\n", opts.Cwd)
		fmt.Fprintln(output, "  --timeout duration")
		fmt.Fprintf(output, "    prompt timeout (default %s)\n", opts.Timeout)
		fmt.Fprintln(output, "  --json")
		fmt.Fprintln(output, "    emit JSON")
		fmt.Fprintln(output, "  --file string")
		fmt.Fprintln(output, "    prompt file")
	}
	fs.StringVar(&opts.Agent, "agent", "", "agent template")
	fs.StringVar(&opts.Command, "command", "", "agent command")
	fs.Var((*repeatedStringFlag)(&opts.Args), "arg", "agent command argument")
	fs.StringVar(&opts.Cwd, "cwd", opts.Cwd, "working directory")
	fs.DurationVar(&opts.Timeout, "timeout", opts.Timeout, "prompt timeout")
	fs.BoolVar(&opts.JSON, "json", false, "emit JSON")
	fs.StringVar(&opts.File, "file", "", "prompt file")
	if err := fs.Parse(args); err != nil {
		return acpClientExecOptions{}, err
	}
	promptArgs := fs.Args()
	switch {
	case opts.File != "" && len(promptArgs) > 0:
		return acpClientExecOptions{}, errors.New("cannot combine --file with inline prompt text")
	case opts.File == "" && len(promptArgs) == 0:
		return acpClientExecOptions{}, errors.New("prompt text or --file is required")
	case len(promptArgs) > 0:
		opts.Prompt = strings.Join(promptArgs, " ")
	}
	return opts, nil
}

func parseACPClientSessions(args []string) (acpClientCommand, error) {
	if len(args) == 0 || args[0] == "list" {
		return acpClientCommand{kind: acpClientCommandSessionsList}, nil
	}
	switch args[0] {
	case "show", "history":
		id, err := requireSessionID("sessions "+args[0], args[1:])
		if err != nil {
			return acpClientCommand{}, err
		}
		return acpClientCommand{kind: acpClientCommandSessionsShow, sessionID: id}, nil
	default:
		return acpClientCommand{}, fmt.Errorf("unknown acp client sessions command: %s", args[0])
	}
}

func requireSessionID(command string, args []string) (string, error) {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return "", fmt.Errorf("usage: ratchet acp client %s <session-id>", command)
	}
	return args[0], nil
}

func isHelpArg(arg string) bool {
	return arg == "help" || arg == "--help" || arg == "-h"
}

func printACPClientUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: ratchet acp client <command> [flags]

Commands:
  exec       Run one prompt against an external ACP agent
  sessions   List or inspect ACP client sessions
  status     Show ACP client session status
  cancel     Cancel an ACP client session

Run 'ratchet acp client exec --help' for exec flags.
`)
}
