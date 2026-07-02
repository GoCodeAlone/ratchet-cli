package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/acpclient"
	"github.com/GoCodeAlone/ratchet-cli/internal/plugins"
)

type acpClientCommandKind string

const (
	acpClientCommandHandled        acpClientCommandKind = "handled"
	acpClientCommandHelp           acpClientCommandKind = "help"
	acpClientCommandExec           acpClientCommandKind = "exec"
	acpClientCommandCompare        acpClientCommandKind = "compare"
	acpClientCommandFlowRun        acpClientCommandKind = "flow-run"
	acpClientCommandSessionsList   acpClientCommandKind = "sessions-list"
	acpClientCommandSessionsShow   acpClientCommandKind = "sessions-show"
	acpClientCommandSessionsExport acpClientCommandKind = "sessions-export"
	acpClientCommandSessionsImport acpClientCommandKind = "sessions-import"
	acpClientCommandSessionsEvents acpClientCommandKind = "sessions-events"
	acpClientCommandStatus         acpClientCommandKind = "status"
	acpClientCommandCancel         acpClientCommandKind = "cancel"
	acpClientCommandQueue          acpClientCommandKind = "queue"
	acpClientCommandDrain          acpClientCommandKind = "drain"
	acpClientCommandWatch          acpClientCommandKind = "watch"
	acpClientCommandProfiles       acpClientCommandKind = "profiles"
)

type acpClientCommand struct {
	kind      acpClientCommandKind
	exec      acpClientExecOptions
	compare   acpClientCompareOptions
	flow      acpClientFlowOptions
	drain     acpClientDrainOptions
	watch     acpClientWatchOptions
	archive   acpClientArchiveOptions
	profiles  acpClientProfilesCommand
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

type acpClientWatchOptions struct {
	Agent         string
	Command       string
	Args          []string
	Cwd           string
	Timeout       time.Duration
	Interval      time.Duration
	MaxPerCycle   int
	MaxCycles     int
	StopWhenEmpty bool
	JSON          bool
}

type acpClientCompareOptions struct {
	Agents   []string
	Commands []string
	Args     []string
	Cwd      string
	Timeout  time.Duration
	JSON     bool
	File     string
	Prompt   string
}

type acpClientFlowOptions struct {
	Path               string
	InputJSON          string
	InputFile          string
	DefaultAgent       string
	Command            string
	Args               []string
	AllowedPermissions []string
	Cwd                string
	RunID              string
	RunRoot            string
	JSON               bool
}

type acpClientArchiveOptions struct {
	Path        string
	Output      string
	HistoryMode string
	SessionID   string
	Cwd         string
	Agent       string
	Command     string
	Args        []string
}

type acpClientProfilesKind string

const (
	acpClientProfilesList    acpClientProfilesKind = "list"
	acpClientProfilesAdd     acpClientProfilesKind = "add"
	acpClientProfilesInstall acpClientProfilesKind = "install"
	acpClientProfilesTrust   acpClientProfilesKind = "trust"
	acpClientProfilesRemove  acpClientProfilesKind = "remove"
)

type acpClientProfilesCommand struct {
	kind    acpClientProfilesKind
	name    string
	source  string
	as      string
	command string
	args    []string
	envKeys []string
	cwd     string
	trust   bool
	json    bool
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
	case acpClientCommandCompare:
		return executeACPClientCompare(context.Background(), cmd.compare, nil, os.Stdout)
	case acpClientCommandFlowRun:
		cmd.flow.RunRoot = filepath.Join(filepath.Dir(store.Path()), "flows")
		return executeACPClientFlowRun(context.Background(), cmd.flow, os.Stdout)
	case acpClientCommandSessionsList:
		return executeACPClientSessionsList(store, cmd.json, os.Stdout)
	case acpClientCommandSessionsShow:
		return executeACPClientSessionShow(store, cmd.sessionID, cmd.json, os.Stdout)
	case acpClientCommandSessionsExport:
		return executeACPClientSessionExport(store, cmd.sessionID, cmd.archive, cmd.json, os.Stdout)
	case acpClientCommandSessionsImport:
		return executeACPClientSessionImport(store, cmd.archive, cmd.json, os.Stdout)
	case acpClientCommandSessionsEvents:
		return executeACPClientSessionEvents(store, cmd.sessionID, cmd.archive, cmd.json, os.Stdout)
	case acpClientCommandStatus:
		return executeACPClientStatus(store, cmd.sessionID, cmd.json, os.Stdout)
	case acpClientCommandCancel:
		return executeACPClientCancel(store, cmd.sessionID, cmd.json, os.Stdout)
	case acpClientCommandQueue:
		return executeACPClientQueue(store, cmd.sessionID, cmd.json, os.Stdout)
	case acpClientCommandDrain:
		return executeACPClientDrain(context.Background(), store, cmd.sessionID, cmd.drain, nil, os.Stdout)
	case acpClientCommandWatch:
		ctx, stop := signal.NotifyContext(context.Background(), acpClientWatchSignals()...)
		defer stop()
		return executeACPClientWatch(ctx, store, cmd.sessionID, cmd.watch, nil, os.Stdout)
	case acpClientCommandProfiles:
		profileStore, err := acpclient.NewDefaultProfileStore()
		if err != nil {
			return err
		}
		return executeACPClientProfiles(profileStore, cmd.profiles, os.Stdout)
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
	reg, err := defaultACPClientRegistry()
	if err != nil {
		return err
	}
	spec, err := reg.Resolve(runOpts)
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
		if len(result.Events) > 0 {
			if err := store.AppendEventLog(rec.ID, result.Events); err != nil {
				return err
			}
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
	case "compare":
		compareOpts, err := parseACPClientCompare(args[1:], output)
		if errors.Is(err, flag.ErrHelp) {
			return acpClientCommand{kind: acpClientCommandHandled}, nil
		}
		if err != nil {
			return acpClientCommand{}, err
		}
		return acpClientCommand{kind: acpClientCommandCompare, compare: compareOpts}, nil
	case "flow":
		return parseACPClientFlow(args[1:], output)
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
	case "watch":
		id, watchOpts, err := parseACPClientWatch(args[1:], output)
		if err != nil {
			return acpClientCommand{}, err
		}
		return acpClientCommand{kind: acpClientCommandWatch, sessionID: id, watch: watchOpts}, nil
	case "profiles":
		profileCmd, err := parseACPClientProfiles(args[1:], output)
		if err != nil {
			return acpClientCommand{}, err
		}
		return acpClientCommand{kind: acpClientCommandProfiles, profiles: profileCmd}, nil
	case "help", "--help", "-h":
		return acpClientCommand{kind: acpClientCommandHelp}, nil
	default:
		return acpClientCommand{}, fmt.Errorf("unknown acp client command: %s", args[0])
	}
}

func parseACPClientProfiles(args []string, output io.Writer) (acpClientProfilesCommand, error) {
	if len(args) == 0 {
		return acpClientProfilesCommand{}, errors.New("usage: ratchet acp client profiles <list|add|install|trust|remove>")
	}
	switch args[0] {
	case "list":
		jsonOut, err := parseJSONOnlyFlags("profiles list", args[1:])
		return acpClientProfilesCommand{kind: acpClientProfilesList, json: jsonOut}, err
	case "add":
		return parseACPClientProfilesAdd(args[1:], output)
	case "install":
		return parseACPClientProfilesInstall(args[1:], output)
	case "trust":
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			return acpClientProfilesCommand{}, errors.New("usage: ratchet acp client profiles trust <name>")
		}
		return acpClientProfilesCommand{kind: acpClientProfilesTrust, name: args[1]}, nil
	case "remove":
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			return acpClientProfilesCommand{}, errors.New("usage: ratchet acp client profiles remove <name>")
		}
		return acpClientProfilesCommand{kind: acpClientProfilesRemove, name: args[1]}, nil
	default:
		return acpClientProfilesCommand{}, fmt.Errorf("unknown acp client profiles command: %s", args[0])
	}
}

func parseACPClientProfilesAdd(args []string, output io.Writer) (acpClientProfilesCommand, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return acpClientProfilesCommand{}, errors.New("usage: ratchet acp client profiles add <name> --command <command> [flags]")
	}
	cmd := acpClientProfilesCommand{kind: acpClientProfilesAdd, name: args[0]}
	fs := flag.NewFlagSet("ratchet acp client profiles add", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.StringVar(&cmd.command, "command", "", "agent command")
	fs.Var((*repeatedStringFlag)(&cmd.args), "arg", "agent command argument")
	fs.Var((*repeatedStringFlag)(&cmd.envKeys), "env-key", "environment variable name required at launch")
	fs.StringVar(&cmd.cwd, "cwd", "", "working directory")
	fs.BoolVar(&cmd.trust, "trust", false, "trust profile after adding")
	if err := fs.Parse(args[1:]); err != nil {
		return acpClientProfilesCommand{}, err
	}
	if len(fs.Args()) > 0 || strings.TrimSpace(cmd.command) == "" {
		return acpClientProfilesCommand{}, errors.New("usage: ratchet acp client profiles add <name> --command <command> [flags]")
	}
	return cmd, nil
}

func parseACPClientProfilesInstall(args []string, output io.Writer) (acpClientProfilesCommand, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return acpClientProfilesCommand{}, errors.New("usage: ratchet acp client profiles install <plugin>/<profile> --as <name> [--trust]")
	}
	cmd := acpClientProfilesCommand{kind: acpClientProfilesInstall, source: args[0]}
	fs := flag.NewFlagSet("ratchet acp client profiles install", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.StringVar(&cmd.as, "as", "", "local profile name")
	fs.BoolVar(&cmd.trust, "trust", false, "trust profile after installing")
	if err := fs.Parse(args[1:]); err != nil {
		return acpClientProfilesCommand{}, err
	}
	if len(fs.Args()) > 0 || strings.TrimSpace(cmd.as) == "" {
		return acpClientProfilesCommand{}, errors.New("usage: ratchet acp client profiles install <plugin>/<profile> --as <name> [--trust]")
	}
	return cmd, nil
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

func parseACPClientCompare(args []string, output io.Writer) (acpClientCompareOptions, error) {
	opts := acpClientCompareOptions{Cwd: ".", Timeout: 30 * time.Second}
	fs := flag.NewFlagSet("ratchet acp client compare", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.Usage = func() {
		fmt.Fprintln(output, "Usage: ratchet acp client compare [flags] <prompt>")
		fmt.Fprintln(output)
		fmt.Fprintln(output, "Flags:")
		fmt.Fprintln(output, "  --agent value")
		fmt.Fprintln(output, "    agent template; repeat for multiple agents")
		fmt.Fprintln(output, "  --command value")
		fmt.Fprintln(output, "    agent command; repeat for multiple commands")
		fmt.Fprintln(output, "  --arg value")
		fmt.Fprintln(output, "    shared agent command argument; repeat for multiple args")
		fmt.Fprintln(output, "  --cwd string")
		fmt.Fprintf(output, "    working directory (default %q)\n", opts.Cwd)
		fmt.Fprintln(output, "  --timeout duration")
		fmt.Fprintf(output, "    per-agent timeout (default %s)\n", opts.Timeout)
		fmt.Fprintln(output, "  --json")
		fmt.Fprintln(output, "    emit JSON")
		fmt.Fprintln(output, "  --file string")
		fmt.Fprintln(output, "    prompt file")
	}
	fs.Var((*repeatedStringFlag)(&opts.Agents), "agent", "agent template")
	fs.Var((*repeatedStringFlag)(&opts.Commands), "command", "agent command")
	fs.Var((*repeatedStringFlag)(&opts.Args), "arg", "shared agent command argument")
	fs.StringVar(&opts.Cwd, "cwd", opts.Cwd, "working directory")
	fs.DurationVar(&opts.Timeout, "timeout", opts.Timeout, "per-agent timeout")
	fs.BoolVar(&opts.JSON, "json", false, "emit JSON")
	fs.StringVar(&opts.File, "file", "", "prompt file")
	if err := fs.Parse(args); err != nil {
		return acpClientCompareOptions{}, err
	}
	targets := len(opts.Agents) + len(opts.Commands)
	if targets < 2 {
		return acpClientCompareOptions{}, errors.New("compare requires at least two --agent or --command values")
	}
	promptArgs := fs.Args()
	switch {
	case opts.File != "" && len(promptArgs) > 0:
		return acpClientCompareOptions{}, errors.New("cannot combine --file with inline prompt text")
	case opts.File == "" && len(promptArgs) == 0:
		return acpClientCompareOptions{}, errors.New("prompt text or --file is required")
	case len(promptArgs) > 0:
		opts.Prompt = strings.Join(promptArgs, " ")
	}
	return opts, nil
}

func parseACPClientFlow(args []string, output io.Writer) (acpClientCommand, error) {
	if len(args) == 0 {
		return acpClientCommand{}, errors.New("usage: ratchet acp client flow run <flow.json> [flags]")
	}
	switch args[0] {
	case "run":
		opts, err := parseACPClientFlowRun(args[1:], output)
		if errors.Is(err, flag.ErrHelp) {
			return acpClientCommand{kind: acpClientCommandHandled}, nil
		}
		if err != nil {
			return acpClientCommand{}, err
		}
		return acpClientCommand{kind: acpClientCommandFlowRun, flow: opts}, nil
	default:
		return acpClientCommand{}, fmt.Errorf("unknown acp client flow command: %s", args[0])
	}
}

func parseACPClientFlowRun(args []string, output io.Writer) (acpClientFlowOptions, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return acpClientFlowOptions{}, errors.New("usage: ratchet acp client flow run <flow.json> [flags]")
	}
	opts := acpClientFlowOptions{Path: args[0], Cwd: "."}
	fs := flag.NewFlagSet("ratchet acp client flow run", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.StringVar(&opts.InputJSON, "input-json", "", "flow input JSON object")
	fs.StringVar(&opts.InputFile, "input-file", "", "flow input JSON file")
	fs.StringVar(&opts.DefaultAgent, "default-agent", "", "default agent template")
	fs.StringVar(&opts.Command, "command", "", "default agent command")
	fs.Var((*repeatedStringFlag)(&opts.Args), "arg", "default agent command argument")
	fs.Var((*repeatedStringFlag)(&opts.AllowedPermissions), "allow", "allow flow permission")
	fs.StringVar(&opts.Cwd, "cwd", opts.Cwd, "working directory")
	fs.StringVar(&opts.RunID, "run-id", "", "flow run id")
	fs.BoolVar(&opts.JSON, "json", false, "emit JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return acpClientFlowOptions{}, err
	}
	if len(fs.Args()) > 0 {
		return acpClientFlowOptions{}, errors.New("usage: ratchet acp client flow run <flow.json> [flags]")
	}
	if opts.InputJSON != "" && opts.InputFile != "" {
		return acpClientFlowOptions{}, errors.New("use only one of --input-json or --input-file")
	}
	for i, permission := range opts.AllowedPermissions {
		permission = strings.TrimSpace(permission)
		if permission == "" {
			return acpClientFlowOptions{}, errors.New("--allow value must not be empty")
		}
		opts.AllowedPermissions[i] = permission
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

func parseACPClientWatch(args []string, output io.Writer) (string, acpClientWatchOptions, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return "", acpClientWatchOptions{}, errors.New("usage: ratchet acp client watch <session-id> [flags]")
	}
	id := args[0]
	opts := acpClientWatchOptions{
		Cwd:         ".",
		Timeout:     30 * time.Second,
		Interval:    5 * time.Second,
		MaxPerCycle: 1,
	}
	fs := flag.NewFlagSet("ratchet acp client watch", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.StringVar(&opts.Agent, "agent", "", "agent template")
	fs.StringVar(&opts.Command, "command", "", "agent command")
	fs.Var((*repeatedStringFlag)(&opts.Args), "arg", "agent command argument")
	fs.StringVar(&opts.Cwd, "cwd", opts.Cwd, "working directory")
	fs.DurationVar(&opts.Timeout, "timeout", opts.Timeout, "prompt timeout")
	fs.DurationVar(&opts.Interval, "interval", opts.Interval, "watch polling interval")
	fs.IntVar(&opts.MaxPerCycle, "max-per-cycle", opts.MaxPerCycle, "maximum queued prompts to drain per cycle")
	fs.IntVar(&opts.MaxCycles, "max-cycles", 0, "maximum watch cycles before exit")
	fs.BoolVar(&opts.StopWhenEmpty, "stop-when-empty", false, "exit after observing an empty queue")
	fs.BoolVar(&opts.JSON, "json", false, "emit newline-delimited JSON cycle summaries")
	if err := fs.Parse(args[1:]); err != nil {
		return "", acpClientWatchOptions{}, err
	}
	if len(fs.Args()) > 0 {
		return "", acpClientWatchOptions{}, errors.New("usage: ratchet acp client watch <session-id> [flags]")
	}
	if opts.Interval <= 0 {
		return "", acpClientWatchOptions{}, errors.New("--interval must be greater than 0")
	}
	if opts.MaxPerCycle <= 0 {
		return "", acpClientWatchOptions{}, errors.New("--max-per-cycle must be greater than 0")
	}
	maxCyclesSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "max-cycles" {
			maxCyclesSet = true
		}
	})
	if maxCyclesSet && opts.MaxCycles <= 0 {
		return "", acpClientWatchOptions{}, errors.New("--max-cycles must be greater than 0")
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
	case "events":
		id, archiveOpts, jsonOut, err := parseACPClientSessionsEvents(args[1:])
		if err != nil {
			return acpClientCommand{}, err
		}
		return acpClientCommand{kind: acpClientCommandSessionsEvents, sessionID: id, archive: archiveOpts, json: jsonOut}, nil
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
	fs.StringVar(&opts.HistoryMode, "history", "summary", "history mode: summary, raw, or both")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return "", acpClientArchiveOptions{}, false, err
	}
	if len(fs.Args()) > 0 || opts.Output == "" {
		return "", acpClientArchiveOptions{}, false, errors.New("usage: ratchet acp client sessions export <session-id> --output <path> [--json]")
	}
	if _, err := parseArchiveHistoryMode(opts.HistoryMode); err != nil {
		return "", acpClientArchiveOptions{}, false, err
	}
	return id, opts, jsonOut, nil
}

func parseACPClientSessionsEvents(args []string) (string, acpClientArchiveOptions, bool, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return "", acpClientArchiveOptions{}, false, errors.New("usage: ratchet acp client sessions events <session-id> [--output <path>] [--json]")
	}
	id := args[0]
	var opts acpClientArchiveOptions
	var jsonOut bool
	fs := flag.NewFlagSet("ratchet acp client sessions events", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.Output, "output", "", "event log output path")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return "", acpClientArchiveOptions{}, false, err
	}
	if len(fs.Args()) > 0 {
		return "", acpClientArchiveOptions{}, false, errors.New("usage: ratchet acp client sessions events <session-id> [--output <path>] [--json]")
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
  compare    Run one prompt serially across multiple ACP agents
  flow       Run JSON ACP client flows
  profiles   Manage local ACP launch profiles
  sessions   List or inspect ACP client sessions
             Subcommands: list, show, history (alias for show), export, import, events
  queue      List queued prompts for an ACP client session
  drain      Drain queued prompts through an external ACP agent
  watch      Explicitly watch and drain queued prompts through an external ACP agent
  status     Show ACP client session status
  cancel     Cancel an ACP client session

Run 'ratchet acp client exec --help' for exec flags.
`)
}

type acpClientProfileListItem struct {
	Name          string `json:"name"`
	Trusted       bool   `json:"trusted"`
	Hash          string `json:"hash"`
	Command       string `json:"command"`
	Template      bool   `json:"template,omitempty"`
	PluginName    string `json:"pluginName,omitempty"`
	PluginVersion string `json:"pluginVersion,omitempty"`
}

var userHomeDir = os.UserHomeDir

func executeACPClientProfiles(store *acpclient.ProfileStore, cmd acpClientProfilesCommand, w io.Writer) error {
	switch cmd.kind {
	case acpClientProfilesList:
		return executeACPClientProfilesList(store, cmd.json, w)
	case acpClientProfilesAdd:
		if _, ok := acpclient.DefaultRegistry().Lookup(cmd.name); ok {
			return fmt.Errorf("%w: %s", acpclient.ErrProfileShadowsBuiltin, cmd.name)
		}
		profile := acpclient.Profile{
			Name:       cmd.name,
			Spec:       acpclient.AgentSpec{Name: cmd.name, Command: cmd.command, Args: cmd.args, EnvKeys: cmd.envKeys},
			Cwd:        cmd.cwd,
			SourceKind: "local",
			SourceID:   "local:" + cmd.name,
			Trusted:    cmd.trust,
		}
		if err := store.Add(profile); err != nil {
			return err
		}
		fmt.Fprintf(w, "Added ACP profile %s\n", cmd.name)
		return nil
	case acpClientProfilesInstall:
		if _, ok := acpclient.DefaultRegistry().Lookup(cmd.as); ok {
			return fmt.Errorf("%w: %s", acpclient.ErrProfileShadowsBuiltin, cmd.as)
		}
		profile, err := findACPProfileTemplate(cmd.source)
		if err != nil {
			return err
		}
		profile.Name = cmd.as
		profile.Spec.Name = cmd.as
		profile.Trusted = cmd.trust
		if err := store.Add(profile); err != nil {
			return err
		}
		fmt.Fprintf(w, "Installed ACP profile %s from %s\n", cmd.as, cmd.source)
		return nil
	case acpClientProfilesTrust:
		if err := store.Trust(cmd.name); err != nil {
			return err
		}
		fmt.Fprintf(w, "Trusted ACP profile %s\n", cmd.name)
		return nil
	case acpClientProfilesRemove:
		if err := store.Remove(cmd.name); err != nil {
			return err
		}
		fmt.Fprintf(w, "Removed ACP profile %s\n", cmd.name)
		return nil
	default:
		return fmt.Errorf("unknown acp client profiles command: %s", cmd.kind)
	}
}

func executeACPClientProfilesList(store *acpclient.ProfileStore, jsonOut bool, w io.Writer) error {
	local, err := store.List()
	if err != nil {
		return err
	}
	templates, err := loadACPProfileTemplates()
	if err != nil {
		return err
	}
	items := make([]acpClientProfileListItem, 0, len(local)+len(templates))
	for _, profile := range local {
		items = append(items, acpClientProfileListItem{
			Name:    profile.Name,
			Trusted: profile.Trusted,
			Hash:    profile.Hash,
			Command: profile.Spec.Command,
		})
	}
	for _, profile := range templates {
		items = append(items, acpClientProfileListItem{
			Name:          profile.Name,
			Hash:          profile.Hash,
			Command:       profile.Spec.Command,
			Template:      true,
			PluginName:    profile.PluginName,
			PluginVersion: profile.PluginVersion,
		})
	}
	if jsonOut {
		return json.NewEncoder(w).Encode(items)
	}
	if len(items) == 0 {
		fmt.Fprintln(w, "No ACP profiles.")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tTRUSTED\tSOURCE\tCOMMAND")
	for _, item := range items {
		source := "local"
		if item.Template {
			source = "plugin:" + item.PluginName
		}
		fmt.Fprintf(tw, "%s\t%v\t%s\t%s\n", item.Name, item.Trusted, source, item.Command)
	}
	return tw.Flush()
}

func findACPProfileTemplate(source string) (acpclient.Profile, error) {
	parts := strings.SplitN(source, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return acpclient.Profile{}, errors.New("profile template must be <plugin>/<profile>")
	}
	templates, err := loadACPProfileTemplates()
	if err != nil {
		return acpclient.Profile{}, err
	}
	for _, profile := range templates {
		if profile.PluginName == parts[0] && profile.Name == parts[1] {
			return profile, nil
		}
	}
	return acpclient.Profile{}, fmt.Errorf("%w: %s", acpclient.ErrProfileNotFound, source)
}

func loadACPProfileTemplates() ([]acpclient.Profile, error) {
	home, err := userHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory for ACP profile templates: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return nil, errors.New("resolve home directory for ACP profile templates: home directory is empty")
	}
	result, err := plugins.NewLoader(filepath.Join(home, ".ratchet", "plugins")).LoadAll(context.Background())
	if err != nil {
		return nil, err
	}
	return result.ACPProfiles, nil
}

func defaultACPClientRegistry() (acpclient.Registry, error) {
	store, err := acpclient.NewDefaultProfileStore()
	if err != nil {
		return acpclient.Registry{}, err
	}
	profiles, err := store.List()
	if err != nil {
		return acpclient.Registry{}, err
	}
	return acpclient.DefaultRegistry().WithProfiles(profiles)
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
	historyMode, err := parseArchiveHistoryMode(opts.HistoryMode)
	if err != nil {
		return err
	}
	if err := acpclient.ExportSession(store, id, opts.Output, acpclient.ExportOptions{HistoryMode: historyMode}); err != nil {
		return err
	}
	if jsonOut {
		return json.NewEncoder(w).Encode(struct {
			SessionID   string `json:"session_id"`
			Path        string `json:"path"`
			Status      string `json:"status"`
			HistoryMode string `json:"history_mode"`
		}{SessionID: id, Path: opts.Output, Status: "exported", HistoryMode: string(historyMode)})
	}
	fmt.Fprintf(w, "exported %s to %s (%s history)\n", id, opts.Output, historyMode)
	return nil
}

func executeACPClientSessionEvents(store *acpclient.Store, id string, opts acpClientArchiveOptions, jsonOut bool, w io.Writer) error {
	events, err := store.ReadEventLog(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", acpclient.ErrRawHistoryUnavailable, id)
		}
		return err
	}
	meta, err := store.EventLogMetadata(id)
	if err != nil {
		return err
	}
	if opts.Output != "" {
		if err := store.CopyEventLog(id, opts.Output); err != nil {
			return err
		}
	}
	if jsonOut {
		return json.NewEncoder(w).Encode(struct {
			SessionID string                   `json:"session_id"`
			Path      string                   `json:"path"`
			Output    string                   `json:"output,omitempty"`
			Status    string                   `json:"status"`
			Events    []acpclient.EventLogLine `json:"events"`
		}{SessionID: id, Path: meta.Path, Output: opts.Output, Status: "ok", Events: events})
	}
	fmt.Fprintf(w, "events for %s: %d (%s)\n", id, len(events), meta.Path)
	if opts.Output != "" {
		fmt.Fprintf(w, "copied events to %s\n", opts.Output)
	}
	return nil
}

func parseArchiveHistoryMode(raw string) (acpclient.ArchiveHistoryMode, error) {
	switch acpclient.ArchiveHistoryMode(strings.TrimSpace(raw)) {
	case "", acpclient.ArchiveHistoryModeSummary:
		return acpclient.ArchiveHistoryModeSummary, nil
	case acpclient.ArchiveHistoryModeRaw:
		return acpclient.ArchiveHistoryModeRaw, nil
	case acpclient.ArchiveHistoryModeBoth:
		return acpclient.ArchiveHistoryModeBoth, nil
	default:
		return "", fmt.Errorf("unsupported archive history mode %q; want summary, raw, or both", raw)
	}
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

func executeACPClientCompare(ctx context.Context, opts acpClientCompareOptions, runner acpclient.CompareRunner, w io.Writer) error {
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
	agents, err := resolveACPClientCompareAgents(opts)
	if err != nil {
		return err
	}
	rows, err := acpclient.Compare(ctx, agents, prompt, acpclient.CompareOptions{
		Cwd:     cwd,
		Timeout: opts.Timeout,
		Runner:  runner,
	})
	if err != nil {
		return err
	}
	if opts.JSON {
		return json.NewEncoder(w).Encode(rows)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tSTATUS\tWALL_MS\tSTOP\tFINAL\tERROR")
	for _, row := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\n", row.Agent, row.Status, row.WallMS, row.StopReason, row.Final, row.Error)
	}
	return tw.Flush()
}

func resolveACPClientCompareAgents(opts acpClientCompareOptions) ([]acpclient.CompareAgent, error) {
	targets := make([]acpclient.CompareAgent, 0, len(opts.Agents)+len(opts.Commands))
	reg, err := defaultACPClientRegistry()
	if err != nil {
		return nil, err
	}
	for _, name := range opts.Agents {
		spec, err := reg.Resolve(acpclient.RunOptions{Agent: name, Args: opts.Args})
		if err != nil {
			return nil, err
		}
		targets = append(targets, acpclient.CompareAgent{Name: name, Spec: spec})
	}
	for _, command := range opts.Commands {
		spec, err := reg.Resolve(acpclient.RunOptions{Command: command, Args: opts.Args})
		if err != nil {
			return nil, err
		}
		targets = append(targets, acpclient.CompareAgent{Name: command, Spec: spec})
	}
	return targets, nil
}

func executeACPClientFlowRun(ctx context.Context, opts acpClientFlowOptions, w io.Writer) error {
	def, err := acpclient.LoadFlowDefinition(opts.Path)
	if err != nil {
		return err
	}
	input, err := readACPClientFlowInput(opts)
	if err != nil {
		return err
	}
	cwd := opts.Cwd
	if cwd == "" {
		cwd = "."
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	reg, err := defaultACPClientRegistry()
	if err != nil {
		return err
	}
	result, err := acpclient.RunFlow(ctx, def, input, acpclient.FlowRunOptions{
		RunID:              opts.RunID,
		RunRoot:            opts.RunRoot,
		Cwd:                cwd,
		DefaultAgent:       opts.DefaultAgent,
		DefaultCommand:     opts.Command,
		DefaultArgs:        opts.Args,
		Registry:           reg,
		AllowedPermissions: opts.AllowedPermissions,
	})
	if err != nil {
		return err
	}
	if opts.JSON {
		return json.NewEncoder(w).Encode(result)
	}
	fmt.Fprintf(w, "flow %s %s\n", result.RunID, result.Status)
	if result.RunDir != "" {
		fmt.Fprintf(w, "run dir: %s\n", result.RunDir)
	}
	return nil
}

func readACPClientFlowInput(opts acpClientFlowOptions) (map[string]any, error) {
	raw := strings.TrimSpace(opts.InputJSON)
	if opts.InputFile != "" {
		b, err := os.ReadFile(opts.InputFile)
		if err != nil {
			return nil, fmt.Errorf("read flow input file: %w", err)
		}
		raw = strings.TrimSpace(string(b))
	}
	if raw == "" {
		return map[string]any{}, nil
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return nil, fmt.Errorf("parse flow input JSON: %w", err)
	}
	return input, nil
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
	reg, err := defaultACPClientRegistry()
	if err != nil {
		return err
	}
	spec, err := reg.Resolve(runOpts)
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

func executeACPClientWatch(ctx context.Context, store *acpclient.Store, id string, opts acpClientWatchOptions, startRunner func(context.Context, acpclient.AgentSpec, acpclient.RunOptions, string) (acpclient.DrainPromptRunner, func() error, error), w io.Writer) error {
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
	reg, err := defaultACPClientRegistry()
	if err != nil {
		return err
	}
	spec, err := reg.Resolve(runOpts)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	_, err = acpclient.WatchQueue(ctx, store, spec, runOpts, id, acpclient.WatchOptions{
		Interval:      opts.Interval,
		MaxPerCycle:   opts.MaxPerCycle,
		MaxCycles:     opts.MaxCycles,
		StopWhenEmpty: opts.StopWhenEmpty,
		StartRunner:   startRunner,
	}, func(cycle acpclient.WatchCycle) {
		if opts.JSON {
			_ = encoder.Encode(acpClientWatchCyclePayload{
				SessionID:     cycle.SessionID,
				ACPSessionID:  cycle.ACPSessionID,
				Cycle:         cycle.Cycle,
				PendingBefore: cycle.PendingBefore,
				Processed:     cycle.Processed,
				Completed:     cycle.Completed,
				Failed:        cycle.Failed,
				Canceled:      cycle.Canceled,
				Remaining:     cycle.Remaining,
				Idle:          cycle.Idle,
			})
			return
		}
		if cycle.Idle {
			fmt.Fprintf(w, "watch idle for %s (cycle %d, remaining: %d)\n", cycle.SessionID, cycle.Cycle, cycle.Remaining)
			return
		}
		fmt.Fprintf(w, "watch cycle %d for %s (processed: %d, completed: %d, failed: %d, canceled: %d, remaining: %d)\n",
			cycle.Cycle, cycle.SessionID, cycle.Processed, cycle.Completed, cycle.Failed, cycle.Canceled, cycle.Remaining)
	})
	return err
}

type acpClientWatchCyclePayload struct {
	SessionID     string `json:"session_id"`
	ACPSessionID  string `json:"acp_session_id,omitempty"`
	Cycle         int    `json:"cycle"`
	PendingBefore int    `json:"pending_before"`
	Processed     int    `json:"processed"`
	Completed     int    `json:"completed"`
	Failed        int    `json:"failed"`
	Canceled      int    `json:"canceled"`
	Remaining     int    `json:"remaining"`
	Idle          bool   `json:"idle"`
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
