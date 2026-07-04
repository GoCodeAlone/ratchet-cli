package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

type blackboardClient interface {
	BlackboardRead(ctx context.Context, section, key string) (*pb.BlackboardReadResp, error)
	BlackboardWrite(ctx context.Context, section, key, value, author string) (*pb.BlackboardEntry, error)
	BlackboardList(ctx context.Context, section string) (*pb.BlackboardListResp, error)
}

type blackboardOptions struct {
	json   bool
	author string
	args   []string
}

func handleBlackboard(args []string) {
	c, err := client.EnsureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	if err := runBlackboard(context.Background(), c, args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runBlackboard(ctx context.Context, c blackboardClient, args []string, stdout, stderr io.Writer) error {
	_ = stderr
	if len(args) == 0 {
		return fmt.Errorf("usage: ratchet blackboard <list|read|write> [args...]")
	}

	subcmd := args[0]
	opts, err := parseBlackboardOptions(args[1:])
	if err != nil {
		return err
	}

	switch subcmd {
	case "list":
		return runBlackboardList(ctx, c, opts, stdout)
	case "read":
		return runBlackboardRead(ctx, c, opts, stdout)
	case "write":
		return runBlackboardWrite(ctx, c, opts, stdout)
	default:
		return fmt.Errorf("unknown blackboard command: %s", subcmd)
	}
}

func parseBlackboardOptions(args []string) (blackboardOptions, error) {
	opts := blackboardOptions{author: defaultBlackboardAuthor()}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			opts.json = true
		case "--author":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--author requires a value")
			}
			opts.author = args[i+1]
			i++
		default:
			opts.args = append(opts.args, args[i])
		}
	}
	return opts, nil
}

func defaultBlackboardAuthor() string {
	for _, key := range []string{"USER", "USERNAME"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return "ratchet-cli"
}

func runBlackboardList(ctx context.Context, c blackboardClient, opts blackboardOptions, stdout io.Writer) error {
	if len(opts.args) > 1 {
		return fmt.Errorf("usage: ratchet blackboard list [section] [--json]")
	}
	section := ""
	if len(opts.args) == 1 {
		section = opts.args[0]
	}
	resp, err := c.BlackboardList(ctx, section)
	if err != nil {
		return err
	}
	if opts.json {
		return writeJSON(stdout, resp)
	}
	if section == "" {
		for _, name := range resp.Sections {
			fmt.Fprintln(stdout, name)
		}
		return nil
	}
	for _, entry := range resp.Entries {
		fmt.Fprintf(stdout, "%s/%s\t%s\tauthor=%s\trev=%d\n",
			entry.GetSection(), entry.GetKey(), entry.GetValue(), entry.GetAuthor(), entry.GetRevision())
	}
	return nil
}

func runBlackboardRead(ctx context.Context, c blackboardClient, opts blackboardOptions, stdout io.Writer) error {
	if len(opts.args) != 2 {
		return fmt.Errorf("usage: ratchet blackboard read <section> <key> [--json]")
	}
	resp, err := c.BlackboardRead(ctx, opts.args[0], opts.args[1])
	if err != nil {
		return err
	}
	if opts.json {
		return writeJSON(stdout, resp)
	}
	if !resp.Found {
		return fmt.Errorf("blackboard entry not found: %s/%s", opts.args[0], opts.args[1])
	}
	fmt.Fprintln(stdout, resp.Entry.GetValue())
	return nil
}

func runBlackboardWrite(ctx context.Context, c blackboardClient, opts blackboardOptions, stdout io.Writer) error {
	if len(opts.args) < 3 {
		return fmt.Errorf("usage: ratchet blackboard write <section> <key> <value...> [--author name] [--json]")
	}
	section, key := opts.args[0], opts.args[1]
	value := strings.Join(opts.args[2:], " ")
	entry, err := c.BlackboardWrite(ctx, section, key, value, opts.author)
	if err != nil {
		return err
	}
	if opts.json {
		return writeJSON(stdout, entry)
	}
	fmt.Fprintf(stdout, "wrote %s/%s rev=%d author=%s\n",
		entry.GetSection(), entry.GetKey(), entry.GetRevision(), entry.GetAuthor())
	return nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
