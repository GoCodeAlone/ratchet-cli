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

type blackboardExportOptions struct {
	jsonl             bool
	section           string
	workflowMessaging bool
}

type blackboardExportRecord struct {
	Section   string                    `json:"section"`
	Key       string                    `json:"key"`
	Value     string                    `json:"value"`
	Author    string                    `json:"author"`
	Revision  int64                     `json:"revision"`
	Timestamp string                    `json:"timestamp"`
	Messaging blackboardMessagingRecord `json:"messaging"`
	Workflow  blackboardWorkflowRecord  `json:"workflow,omitempty"`
}

type blackboardMessagingRecord struct {
	Text string `json:"text"`
}

type blackboardWorkflowRecord struct {
	StepType       string                         `json:"stepType"`
	PluginFamily   string                         `json:"pluginFamily"`
	Input          blackboardWorkflowInput        `json:"input"`
	RequiredConfig []string                       `json:"requiredConfig"`
	Metadata       blackboardWorkflowRecordSource `json:"metadata"`
}

type blackboardWorkflowInput struct {
	Text string `json:"text"`
}

type blackboardWorkflowRecordSource struct {
	Section string `json:"section"`
	Key     string `json:"key"`
}

func handleBlackboard(args []string) {
	c, err := client.EnsureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	if err := runBlackboard(context.Background(), c, args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runBlackboard(ctx context.Context, c blackboardClient, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ratchet blackboard <list|read|write|export> [args...]")
	}

	subcmd := args[0]
	if subcmd == "export" {
		opts, err := parseBlackboardExportOptions(args[1:])
		if err != nil {
			return err
		}
		return runBlackboardExport(ctx, c, opts, stdout)
	}
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
		case "--":
			opts.args = append(opts.args, args[i+1:]...)
			return opts, nil
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

func parseBlackboardExportOptions(args []string) (blackboardExportOptions, error) {
	var opts blackboardExportOptions
	var json bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			json = true
		case "--jsonl":
			opts.jsonl = true
		case "--workflow-messaging":
			opts.workflowMessaging = true
		default:
			if strings.HasPrefix(args[i], "--") {
				return opts, fmt.Errorf("unknown blackboard export flag: %s", args[i])
			}
			if opts.section != "" {
				return opts, fmt.Errorf("usage: ratchet blackboard export [section] [--json|--jsonl]")
			}
			opts.section = args[i]
		}
	}
	if json && opts.jsonl {
		return opts, fmt.Errorf("usage: ratchet blackboard export [section] [--json|--jsonl]")
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

func runBlackboardExport(ctx context.Context, c blackboardClient, opts blackboardExportOptions, stdout io.Writer) error {
	records, err := loadBlackboardExportRecords(ctx, c, opts.section)
	if err != nil {
		return err
	}
	if opts.workflowMessaging {
		for i := range records {
			records[i].Workflow = blackboardWorkflowRecordFromExport(records[i])
		}
	}
	if opts.jsonl {
		enc := json.NewEncoder(stdout)
		for _, record := range records {
			if err := enc.Encode(record); err != nil {
				return err
			}
		}
		return nil
	}
	return writeJSON(stdout, records)
}

func loadBlackboardExportRecords(ctx context.Context, c blackboardClient, section string) ([]blackboardExportRecord, error) {
	if section != "" {
		return loadBlackboardExportSection(ctx, c, section)
	}
	resp, err := c.BlackboardList(ctx, "")
	if err != nil {
		return nil, err
	}
	records := make([]blackboardExportRecord, 0)
	for _, name := range resp.Sections {
		sectionRecords, err := loadBlackboardExportSection(ctx, c, name)
		if err != nil {
			return nil, err
		}
		records = append(records, sectionRecords...)
	}
	return records, nil
}

func loadBlackboardExportSection(ctx context.Context, c blackboardClient, section string) ([]blackboardExportRecord, error) {
	resp, err := c.BlackboardList(ctx, section)
	if err != nil {
		return nil, err
	}
	records := make([]blackboardExportRecord, 0, len(resp.Entries))
	for _, entry := range resp.Entries {
		records = append(records, blackboardExportRecordFromEntry(entry))
	}
	return records, nil
}

func blackboardExportRecordFromEntry(entry *pb.BlackboardEntry) blackboardExportRecord {
	if entry == nil {
		return blackboardExportRecord{}
	}
	section, key, value := entry.GetSection(), entry.GetKey(), entry.GetValue()
	messaging := blackboardMessagingRecord{
		Text: fmt.Sprintf("[%s/%s] %s", section, key, value),
	}
	return blackboardExportRecord{
		Section:   section,
		Key:       key,
		Value:     value,
		Author:    entry.GetAuthor(),
		Revision:  entry.GetRevision(),
		Timestamp: entry.GetTimestamp(),
		Messaging: messaging,
	}
}

func blackboardWorkflowRecordFromExport(record blackboardExportRecord) blackboardWorkflowRecord {
	return blackboardWorkflowRecord{
		StepType:     "step.messaging_send",
		PluginFamily: "workflow-plugin-messaging-core",
		Input: blackboardWorkflowInput{
			Text: record.Messaging.Text,
		},
		RequiredConfig: []string{"channel"},
		Metadata: blackboardWorkflowRecordSource{
			Section: record.Section,
			Key:     record.Key,
		},
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
