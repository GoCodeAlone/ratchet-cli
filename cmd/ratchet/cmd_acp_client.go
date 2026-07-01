package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type acpClientCommandKind string

const (
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
	cmd, err := parseACPClientCommand(args)
	if err != nil {
		return err
	}
	switch cmd.kind {
	case acpClientCommandHelp:
		printACPClientUsage(os.Stdout)
		return nil
	case acpClientCommandExec:
		return errors.New("ratchet acp client exec is not wired yet")
	case acpClientCommandSessionsList, acpClientCommandSessionsShow, acpClientCommandStatus, acpClientCommandCancel:
		return fmt.Errorf("ratchet acp client %s is planned for a later PR", cmd.kind)
	default:
		return fmt.Errorf("unknown acp client command: %s", cmd.kind)
	}
}

func parseACPClientCommand(args []string) (acpClientCommand, error) {
	if len(args) == 0 || isHelpArg(args[0]) {
		return acpClientCommand{kind: acpClientCommandHelp}, nil
	}
	switch args[0] {
	case "exec":
		execOpts, err := parseACPClientExec(args[1:])
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

func parseACPClientExec(args []string) (acpClientExecOptions, error) {
	var opts acpClientExecOptions
	opts.Cwd = "."
	opts.Timeout = 30 * time.Second

	fs := flag.NewFlagSet("ratchet acp client exec", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
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
