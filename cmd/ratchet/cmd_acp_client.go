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
	"text/tabwriter"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/acpclient"
)

type acpClientCommandKind string

const (
	acpClientCommandHandled        acpClientCommandKind = "handled"
	acpClientCommandHelp           acpClientCommandKind = "help"
	acpClientCommandExec           acpClientCommandKind = "exec"
	acpClientCommandSessionsList   acpClientCommandKind = "sessions-list"
	acpClientCommandSessionsShow   acpClientCommandKind = "sessions-show"
	acpClientCommandSessionsExport acpClientCommandKind = "sessions-export"
	acpClientCommandSessionsImport acpClientCommandKind = "sessions-import"
	acpClientCommandStatus         acpClientCommandKind = "status"
	acpClientCommandCancel         acpClientCommandKind = "cancel"
	acpClientCommandQueue          acpClientCommandKind = "queue"
	acpClientCommandDrain          acpClientCommandKind = "drain"
)

type acpClientCommand struct {
	kind      acpClientCommandKind
	exec      acpClientExecOptions
	drain     acpClientDrainOptions
	archive   acpClientArchiveOptions
	sessionID string
	json      bool
}

type acpClientExecOptions struct {
	Agent     string
	Command   string
	Args      []string
	Cwd       string
	Timeout   time.Duration
	JSON      bool
	File      string
	Prompt    string
	SessionID string
	NoWait    bool
}

type acpClientDrainOptions struct {
	Agent   string
	Command string
	Args    []string
	Cwd     string
	Timeout time.Duration
	Max     int
}

type acpClientArchiveOptions struct {
	Path      string
	Output    string
	SessionID string
	Cwd       string
	Agent     string
	Command   string
	Args      []string
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
	store, err := acpclient.NewDefaultStore()
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
		return executeACPClientExecWithStore(context.Background(), cmd.exec, defaultACPClientExecRunner{}, store, os.Stdout)
	case acpClientCommandSessionsList:
		return executeACPClientSessionsList(store, cmd.json, os.Stdout)
	case acpClientCommandSessionsShow:
		return executeACPClientSessionShow(store, cmd.sessionID, cmd.json, os.Stdout)
	case acpClientCommandSessionsExport:
		return executeACPClientSessionExport(store, cmd.sessionID, cmd.archive, cmd.json, os.Stdout)
	case acpClientCommandSessionsImport:
		return executeACPClientSessionImport(store, cmd.archive, cmd.json, os.Stdout)
	case acpClientCommandStatus:
		return executeACPClientStatus(store, cmd.sessionID, cmd.json, os.Stdout)
	case acpClientCommandCancel:
		return executeACPClientCancel(store, cmd.sessionID, cmd.json, os.Stdout)
	case acpClientCommandQueue:
		return executeACPClientQueue(store, cmd.sessionID, cmd.json, os.Stdout)
	case acpClientCommandDrain:
		return executeACPClientDrain(context.Background(), store, cmd.sessionID, cmd.drain, nil, os.Stdout)
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
	return executeACPClientExecWithStore(ctx, opts, runner, nil, w)
}

func executeACPClientExecWithStore(ctx context.Context, opts acpClientExecOptions, runner acpClientExecRunner, store *acpclient.Store, w io.Writer) error {
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
	if opts.NoWait {
		sessionID := strings.TrimSpace(opts.SessionID)
		if sessionID == "" {
			sessionID = newLocalACPClientID("local")
		}
		if store == nil {
			return errors.New("acp client store is required for --no-wait")
		}
		queueID := ""
		queueDepth := 0
		now := time.Now().UTC()
		rec := acpclient.SessionRecord{
			ID:                 sessionID,
			Agent:              spec.Name,
			CommandFingerprint: spec.Fingerprint(),
			Cwd:                cwd,
			Status:             acpclient.SessionStatusQueued,
			Summary:            summarizeACPClientText(prompt),
		}
		queued, err := store.AppendQueuedPrompt(rec, acpclient.QueuedPrompt{
			ID:        newLocalACPClientID("queue"),
			Prompt:    prompt,
			Status:    acpclient.QueuePromptStatusPending,
			CreatedAt: now,
		})
		if err != nil {
			return err
		}
		queueDepth = len(queued.PromptQueue)
		if queueDepth > 0 {
			queueID = queued.PromptQueue[queueDepth-1].ID
		}
		if opts.JSON {
			return json.NewEncoder(w).Encode(struct {
				SessionID  string `json:"session_id"`
				QueueID    string `json:"queue_id,omitempty"`
				QueueDepth int    `json:"queue_depth"`
				Status     string `json:"status"`
			}{SessionID: sessionID, QueueID: queueID, QueueDepth: queueDepth, Status: acpclient.SessionStatusQueued})
		}
		fmt.Fprintf(w, "queued prompt %s for %s (queue depth: %d)\n", queueID, sessionID, queueDepth)
		return nil
	}
	var activeSessionID string
	if store != nil {
		runOpts.SessionStarted = func(sessionID string) error {
			activeSessionID = sessionID
			now := time.Now().UTC()
			rec := acpclient.SessionRecord{
				ID:                 sessionID,
				Agent:              spec.Name,
				CommandFingerprint: spec.Fingerprint(),
				Cwd:                cwd,
				Status:             acpclient.SessionStatusRunning,
				CreatedAt:          now,
				UpdatedAt:          now,
			}
			if existing, err := store.Get(sessionID); err == nil {
				rec.CreatedAt = existing.CreatedAt
				rec.Turns = existing.Turns
			}
			if err := store.Upsert(rec); err != nil {
				return err
			}
			return store.WriteOwner(acpclient.OwnerLock{
				SessionID:          sessionID,
				PID:                os.Getpid(),
				CommandFingerprint: spec.Fingerprint(),
				StartedAt:          now,
			})
		}
		runOpts.CancelRequested = func(sessionID string) bool {
			_, err := store.CancelRequest(sessionID)
			return err == nil
		}
		defer func() {
			if activeSessionID != "" {
				_ = store.ClearOwner(activeSessionID)
			}
		}()
	}
	result, err := runner.RunPrompt(ctx, spec, runOpts, prompt)
	if err != nil {
		return err
	}
	if store != nil {
		now := time.Now().UTC()
		rec := acpclient.SessionRecord{
			ID:                 string(result.SessionID),
			Agent:              spec.Name,
			CommandFingerprint: spec.Fingerprint(),
			Cwd:                cwd,
			Status:             acpclient.SessionStatusCompleted,
			CreatedAt:          now,
			UpdatedAt:          now,
			LastStopReason:     string(result.StopReason),
			Summary:            summarizeACPClientText(result.Text),
			Turns: []acpclient.TurnSummary{{
				Prompt:     summarizeACPClientText(prompt),
				Response:   summarizeACPClientText(result.Text),
				StopReason: string(result.StopReason),
				CreatedAt:  now,
			}},
		}
		if existing, err := store.Get(rec.ID); err == nil {
			rec.CreatedAt = existing.CreatedAt
			rec.Turns = append(existing.Turns, rec.Turns...)
		}
		if err := store.Upsert(rec); err != nil {
			return err
		}
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
		id, jsonOut, err := parseACPClientIDCommand("status", args[1:])
		if err != nil {
			return acpClientCommand{}, err
		}
		return acpClientCommand{kind: acpClientCommandStatus, sessionID: id, json: jsonOut}, nil
	case "cancel":
		id, jsonOut, err := parseACPClientIDCommand("cancel", args[1:])
		if err != nil {
			return acpClientCommand{}, err
		}
		return acpClientCommand{kind: acpClientCommandCancel, sessionID: id, json: jsonOut}, nil
	case "queue":
		id, jsonOut, err := parseACPClientQueue(args[1:])
		if err != nil {
			return acpClientCommand{}, err
		}
		return acpClientCommand{kind: acpClientCommandQueue, sessionID: id, json: jsonOut}, nil
	case "drain":
		id, drainOpts, err := parseACPClientDrain(args[1:], output)
		if err != nil {
			return acpClientCommand{}, err
		}
		return acpClientCommand{kind: acpClientCommandDrain, sessionID: id, drain: drainOpts}, nil
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
		fmt.Fprintln(output, "  --session string")
		fmt.Fprintln(output, "    existing ACP client session id for queued prompts")
		fmt.Fprintln(output, "  --no-wait")
		fmt.Fprintln(output, "    queue prompt locally without launching the agent")
	}
	fs.StringVar(&opts.Agent, "agent", "", "agent template")
	fs.StringVar(&opts.Command, "command", "", "agent command")
	fs.Var((*repeatedStringFlag)(&opts.Args), "arg", "agent command argument")
	fs.StringVar(&opts.Cwd, "cwd", opts.Cwd, "working directory")
	fs.DurationVar(&opts.Timeout, "timeout", opts.Timeout, "prompt timeout")
	fs.BoolVar(&opts.JSON, "json", false, "emit JSON")
	fs.StringVar(&opts.File, "file", "", "prompt file")
	fs.StringVar(&opts.SessionID, "session", "", "existing ACP client session id for queued prompts")
	fs.BoolVar(&opts.NoWait, "no-wait", false, "queue prompt locally without launching the agent")
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

func parseACPClientQueue(args []string) (string, bool, error) {
	var id string
	var jsonOut bool
	for _, arg := range args {
		switch {
		case arg == "--json":
			jsonOut = true
		case strings.TrimSpace(arg) == "":
			return "", false, errors.New("usage: ratchet acp client queue <session-id> [--json]")
		case id == "":
			id = arg
		default:
			return "", false, errors.New("usage: ratchet acp client queue <session-id> [--json]")
		}
	}
	if id == "" {
		return "", false, errors.New("usage: ratchet acp client queue <session-id> [--json]")
	}
	return id, jsonOut, nil
}

func parseACPClientDrain(args []string, output io.Writer) (string, acpClientDrainOptions, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return "", acpClientDrainOptions{}, errors.New("usage: ratchet acp client drain <session-id> [flags]")
	}
	id := args[0]
	opts := acpClientDrainOptions{Cwd: ".", Timeout: 30 * time.Second}
	fs := flag.NewFlagSet("ratchet acp client drain", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.StringVar(&opts.Agent, "agent", "", "agent template")
	fs.StringVar(&opts.Command, "command", "", "agent command")
	fs.Var((*repeatedStringFlag)(&opts.Args), "arg", "agent command argument")
	fs.StringVar(&opts.Cwd, "cwd", opts.Cwd, "working directory")
	fs.DurationVar(&opts.Timeout, "timeout", opts.Timeout, "prompt timeout")
	fs.IntVar(&opts.Max, "max", 0, "maximum queued prompts to drain")
	if err := fs.Parse(args[1:]); err != nil {
		return "", acpClientDrainOptions{}, err
	}
	if len(fs.Args()) > 0 {
		return "", acpClientDrainOptions{}, errors.New("usage: ratchet acp client drain <session-id> [flags]")
	}
	maxSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "max" {
			maxSet = true
		}
	})
	if maxSet && opts.Max <= 0 {
		return "", acpClientDrainOptions{}, errors.New("--max must be greater than 0")
	}
	return id, opts, nil
}

func parseACPClientSessions(args []string) (acpClientCommand, error) {
	if len(args) == 0 || args[0] == "list" || strings.HasPrefix(args[0], "-") {
		jsonOut, err := parseJSONOnlyFlags("sessions list", trimCommand(args, "list"))
		if err != nil {
			return acpClientCommand{}, err
		}
		return acpClientCommand{kind: acpClientCommandSessionsList, json: jsonOut}, nil
	}
	switch args[0] {
	case "show", "history":
		jsonOut, rest, err := parseJSONAndRestFlags("sessions "+args[0], args[1:])
		if err != nil {
			return acpClientCommand{}, err
		}
		id, err := requireSessionID("sessions "+args[0], rest)
		if err != nil {
			return acpClientCommand{}, err
		}
		return acpClientCommand{kind: acpClientCommandSessionsShow, sessionID: id, json: jsonOut}, nil
	case "export":
		id, archiveOpts, jsonOut, err := parseACPClientSessionsExport(args[1:])
		if err != nil {
			return acpClientCommand{}, err
		}
		return acpClientCommand{kind: acpClientCommandSessionsExport, sessionID: id, archive: archiveOpts, json: jsonOut}, nil
	case "import":
		archiveOpts, jsonOut, err := parseACPClientSessionsImport(args[1:])
		if err != nil {
			return acpClientCommand{}, err
		}
		return acpClientCommand{kind: acpClientCommandSessionsImport, archive: archiveOpts, json: jsonOut}, nil
	default:
		return acpClientCommand{}, fmt.Errorf("unknown acp client sessions command: %s", args[0])
	}
}

func parseACPClientSessionsExport(args []string) (string, acpClientArchiveOptions, bool, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return "", acpClientArchiveOptions{}, false, errors.New("usage: ratchet acp client sessions export <session-id> --output <path> [--json]")
	}
	id := args[0]
	var opts acpClientArchiveOptions
	var jsonOut bool
	fs := flag.NewFlagSet("ratchet acp client sessions export", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.Output, "output", "", "archive output path")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return "", acpClientArchiveOptions{}, false, err
	}
	if len(fs.Args()) > 0 || opts.Output == "" {
		return "", acpClientArchiveOptions{}, false, errors.New("usage: ratchet acp client sessions export <session-id> --output <path> [--json]")
	}
	return id, opts, jsonOut, nil
}

func parseACPClientSessionsImport(args []string) (acpClientArchiveOptions, bool, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return acpClientArchiveOptions{}, false, errors.New("usage: ratchet acp client sessions import <archive> [flags]")
	}
	opts := acpClientArchiveOptions{Path: args[0]}
	var jsonOut bool
	fs := flag.NewFlagSet("ratchet acp client sessions import", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.SessionID, "session", "", "imported local session id")
	fs.StringVar(&opts.Cwd, "cwd", "", "imported working directory")
	fs.StringVar(&opts.Agent, "agent", "", "agent template")
	fs.StringVar(&opts.Command, "command", "", "agent command")
	fs.Var((*repeatedStringFlag)(&opts.Args), "arg", "agent command argument")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return acpClientArchiveOptions{}, false, err
	}
	if len(fs.Args()) > 0 {
		return acpClientArchiveOptions{}, false, errors.New("usage: ratchet acp client sessions import <archive> [flags]")
	}
	return opts, jsonOut, nil
}

func parseACPClientIDCommand(command string, args []string) (string, bool, error) {
	jsonOut, rest, err := parseJSONAndRestFlags(command, args)
	if err != nil {
		return "", false, err
	}
	id, err := requireSessionID(command, rest)
	if err != nil {
		return "", false, err
	}
	return id, jsonOut, nil
}

func parseJSONOnlyFlags(command string, args []string) (bool, error) {
	jsonOut, rest, err := parseJSONAndRestFlags(command, args)
	if err != nil {
		return false, err
	}
	if len(rest) > 0 {
		return false, fmt.Errorf("usage: ratchet acp client %s [--json]", command)
	}
	return jsonOut, nil
}

func parseJSONAndRestFlags(command string, args []string) (bool, []string, error) {
	var jsonOut bool
	fs := flag.NewFlagSet("ratchet acp client "+command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return false, nil, err
	}
	return jsonOut, fs.Args(), nil
}

func trimCommand(args []string, command string) []string {
	if len(args) > 0 && args[0] == command {
		return args[1:]
	}
	return args
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
             Subcommands: list, show, history (alias for show), export, import
  queue      List queued prompts for an ACP client session
  drain      Drain queued prompts through an external ACP agent
  status     Show ACP client session status
  cancel     Cancel an ACP client session

Run 'ratchet acp client exec --help' for exec flags.
`)
}

func executeACPClientSessionsList(store *acpclient.Store, jsonOut bool, w io.Writer) error {
	records, err := store.List()
	if err != nil {
		return err
	}
	if jsonOut {
		return json.NewEncoder(w).Encode(records)
	}
	if len(records) == 0 {
		fmt.Fprintln(w, "No ACP client sessions.")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tAGENT\tUPDATED\tSUMMARY")
	for _, rec := range records {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", rec.ID, rec.Status, rec.Agent, rec.UpdatedAt.Format(time.RFC3339), rec.Summary)
	}
	return tw.Flush()
}

func executeACPClientSessionShow(store *acpclient.Store, id string, jsonOut bool, w io.Writer) error {
	rec, err := store.Get(id)
	if err != nil {
		return err
	}
	if jsonOut {
		return json.NewEncoder(w).Encode(rec)
	}
	fmt.Fprintf(w, "id: %s\nstatus: %s\nagent: %s\ncwd: %s\nupdated: %s\n", rec.ID, rec.Status, rec.Agent, rec.Cwd, rec.UpdatedAt.Format(time.RFC3339))
	if rec.LastStopReason != "" {
		fmt.Fprintf(w, "stop: %s\n", rec.LastStopReason)
	}
	if rec.Summary != "" {
		fmt.Fprintf(w, "summary: %s\n", rec.Summary)
	}
	if rec.PendingPrompt != nil {
		fmt.Fprintf(w, "pending prompt: %s\n", rec.PendingPrompt.Status)
	}
	return nil
}

func executeACPClientSessionExport(store *acpclient.Store, id string, opts acpClientArchiveOptions, jsonOut bool, w io.Writer) error {
	if err := acpclient.ExportSession(store, id, opts.Output, acpclient.ExportOptions{}); err != nil {
		return err
	}
	if jsonOut {
		return json.NewEncoder(w).Encode(struct {
			SessionID string `json:"session_id"`
			Path      string `json:"path"`
			Status    string `json:"status"`
		}{SessionID: id, Path: opts.Output, Status: "exported"})
	}
	fmt.Fprintf(w, "exported %s to %s\n", id, opts.Output)
	return nil
}

func executeACPClientSessionImport(store *acpclient.Store, opts acpClientArchiveOptions, jsonOut bool, w io.Writer) error {
	importOpts := acpclient.ImportOptions{
		SessionID: opts.SessionID,
		Cwd:       opts.Cwd,
	}
	if opts.Agent != "" || opts.Command != "" || len(opts.Args) > 0 {
		spec, err := acpclient.DefaultRegistry().Resolve(acpclient.RunOptions{
			Agent:   opts.Agent,
			Command: opts.Command,
			Args:    opts.Args,
		})
		if err != nil {
			return err
		}
		importOpts.Agent = spec.Name
		importOpts.CommandFingerprint = spec.Fingerprint()
	}
	rec, err := acpclient.ImportSession(store, opts.Path, importOpts)
	if err != nil {
		return err
	}
	if jsonOut {
		return json.NewEncoder(w).Encode(struct {
			SessionID string `json:"session_id"`
			Path      string `json:"path"`
			Status    string `json:"status"`
		}{SessionID: rec.ID, Path: opts.Path, Status: rec.Status})
	}
	fmt.Fprintf(w, "imported %s from %s\n", rec.ID, opts.Path)
	return nil
}

func executeACPClientQueue(store *acpclient.Store, id string, jsonOut bool, w io.Writer) error {
	rec, err := store.Get(id)
	if err != nil {
		return err
	}
	items := rec.PromptQueue
	if jsonOut {
		return json.NewEncoder(w).Encode(struct {
			SessionID string                   `json:"session_id"`
			Items     []acpclient.QueuedPrompt `json:"items"`
		}{SessionID: id, Items: items})
	}
	if len(items) == 0 {
		fmt.Fprintf(w, "No queued prompts for %s.\n", id)
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tCREATED\tPROMPT")
	for _, item := range items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", item.ID, item.Status, item.CreatedAt.Format(time.RFC3339), summarizeACPClientText(item.Prompt))
	}
	return tw.Flush()
}

func executeACPClientDrain(ctx context.Context, store *acpclient.Store, id string, opts acpClientDrainOptions, startRunner func(context.Context, acpclient.AgentSpec, acpclient.RunOptions, string) (acpclient.DrainPromptRunner, func() error, error), w io.Writer) error {
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
	result, err := acpclient.DrainQueue(ctx, store, spec, runOpts, id, acpclient.DrainOptions{
		Max:         opts.Max,
		StartRunner: startRunner,
	})
	if err != nil {
		return err
	}
	noun := "prompts"
	if result.Completed == 1 {
		noun = "prompt"
	}
	fmt.Fprintf(w, "drained %d %s for %s (failed: %d, canceled: %d, remaining: %d)\n", result.Completed, noun, id, result.Failed, result.Canceled, result.Remaining)
	return nil
}

func executeACPClientStatus(store *acpclient.Store, id string, jsonOut bool, w io.Writer) error {
	rec, err := store.Get(id)
	if err != nil {
		return err
	}
	owner, ownerErr := store.Owner(id)
	payload := struct {
		acpclient.SessionRecord
		Owner *acpclient.OwnerLock `json:"owner,omitempty"`
	}{SessionRecord: rec}
	if ownerErr == nil {
		payload.Owner = &owner
	}
	if jsonOut {
		return json.NewEncoder(w).Encode(payload)
	}
	fmt.Fprintf(w, "%s: %s\n", rec.ID, rec.Status)
	if rec.PendingPrompt != nil {
		fmt.Fprintf(w, "pending prompt: %s\n", rec.PendingPrompt.Status)
	}
	pending, running, completed, canceled, failed := countACPClientQueue(rec.PromptQueue)
	if len(rec.PromptQueue) > 0 {
		fmt.Fprintf(w, "queue: %d pending, %d running, %d completed, %d canceled, %d failed\n", pending, running, completed, canceled, failed)
	}
	if ownerErr == nil {
		fmt.Fprintf(w, "owner pid: %d started: %s\n", owner.PID, owner.StartedAt.Format(time.RFC3339))
	}
	if rec.Summary != "" {
		fmt.Fprintf(w, "summary: %s\n", rec.Summary)
	}
	return nil
}

func executeACPClientCancel(store *acpclient.Store, id string, jsonOut bool, w io.Writer) error {
	_, ownerErr := store.Owner(id)
	if ownerErr == nil {
		if err := store.RequestCancel(id, time.Now().UTC()); err != nil {
			return err
		}
		if jsonOut {
			return json.NewEncoder(w).Encode(struct {
				SessionID string `json:"session_id"`
				Status    string `json:"status"`
			}{SessionID: id, Status: acpclient.SessionStatusCancelRequested})
		}
		fmt.Fprintf(w, "requested cancel for active session %s\n", id)
		return nil
	} else if !errors.Is(ownerErr, os.ErrNotExist) {
		return ownerErr
	}
	rec, err := store.Get(id)
	if err != nil {
		return err
	}
	pending, _, _, _, _ := countACPClientQueue(rec.PromptQueue)
	if pending > 0 {
		if rec.PendingPrompt != nil && rec.PendingPrompt.Status == acpclient.PendingPromptStatusPending {
			if err := store.MarkPendingCanceled(id, time.Now().UTC()); err != nil {
				return err
			}
			if jsonOut {
				return json.NewEncoder(w).Encode(struct {
					SessionID string `json:"session_id"`
					Status    string `json:"status"`
				}{SessionID: id, Status: acpclient.SessionStatusCanceled})
			}
			fmt.Fprintf(w, "canceled pending prompt for %s\n", id)
			return nil
		}
		count, err := store.CancelPendingQueue(id, time.Now().UTC())
		if err != nil {
			return err
		}
		if jsonOut {
			return json.NewEncoder(w).Encode(struct {
				SessionID string `json:"session_id"`
				Status    string `json:"status"`
				Canceled  int    `json:"canceled"`
			}{SessionID: id, Status: acpclient.SessionStatusCanceled, Canceled: count})
		}
		fmt.Fprintf(w, "canceled %d pending prompts for %s\n", count, id)
		return nil
	}
	if rec.PendingPrompt == nil || rec.PendingPrompt.Status != acpclient.PendingPromptStatusPending {
		return fmt.Errorf("session %s has no active owner or pending prompt", id)
	}
	if err := store.MarkPendingCanceled(id, time.Now().UTC()); err != nil {
		return err
	}
	if jsonOut {
		return json.NewEncoder(w).Encode(struct {
			SessionID string `json:"session_id"`
			Status    string `json:"status"`
		}{SessionID: id, Status: acpclient.SessionStatusCanceled})
	}
	fmt.Fprintf(w, "canceled pending prompt for %s\n", id)
	return nil
}

func countACPClientQueue(items []acpclient.QueuedPrompt) (pending, running, completed, canceled, failed int) {
	for _, item := range items {
		switch item.Status {
		case acpclient.QueuePromptStatusPending:
			pending++
		case acpclient.QueuePromptStatusRunning:
			running++
		case acpclient.QueuePromptStatusCompleted:
			completed++
		case acpclient.QueuePromptStatusCanceled:
			canceled++
		case acpclient.QueuePromptStatusFailed:
			failed++
		}
	}
	return pending, running, completed, canceled, failed
}

func summarizeACPClientText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 240 {
		return text[:240]
	}
	return text
}

func newLocalACPClientID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}
