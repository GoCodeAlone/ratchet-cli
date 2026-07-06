package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const daemonSessionExportSchema = "ratchet.session-export.v1"

type sessionsClient interface {
	Close() error
	ListSessions(context.Context) (*pb.SessionList, error)
	KillSession(context.Context, string) error
	ListSessionMessages(context.Context, string) (*pb.SessionHistory, error)
	CloneSession(context.Context, string, string) (*pb.Session, error)
	ForkSession(context.Context, string, string, string) (*pb.Session, error)
	GetSessionTree(context.Context, string) (*pb.SessionList, error)
	ListSessionCompactions(context.Context, string) (*pb.SessionCompactionList, error)
	UpdateSessionSummary(context.Context, string, string) (*pb.Session, error)
}

type daemonSessionExportBundle struct {
	Schema      string                 `json:"schema"`
	ExportedBy  string                 `json:"exported_by"`
	ExportedAt  string                 `json:"exported_at"`
	Session     *pb.Session            `json:"session"`
	Tree        []*pb.Session          `json:"tree"`
	Messages    []*pb.HistoryMessage   `json:"messages"`
	Compactions []*pb.CompactionRecord `json:"compactions"`
}

type daemonSessionExportSummary struct {
	Output      string `json:"output"`
	SessionID   string `json:"session_id"`
	Tree        int    `json:"tree"`
	Messages    int    `json:"messages"`
	Compactions int    `json:"compactions"`
}

var ensureSessionsClient = func() (sessionsClient, error) {
	return client.EnsureDaemon()
}

var runSessionBrowser = func(ctx context.Context, c sessionsClient, rootID string) (string, error) {
	return tui.RunSessionBrowser(ctx, c, rootID)
}

func handleSessions(args []string) {
	if len(args) == 0 {
		printSessionsUsage()
		return
	}
	if args[0] == "browse" && len(args) < 2 {
		fmt.Println("Usage: ratchet sessions browse <id>")
		return
	}

	c, err := ensureSessionsClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	switch args[0] {
	case "list":
		resp, err := c.ListSessions(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(resp.Sessions) == 0 {
			fmt.Println("No sessions.")
			return
		}
		fmt.Printf("%-36s %-10s %-20s %-32s %s\n", "ID", "STATUS", "PROVIDER", "SUMMARY", "WORKING_DIR")
		for _, s := range resp.Sessions {
			fmt.Printf("%-36s %-10s %-20s %-32s %s\n", s.Id, s.Status, s.Provider, formatSummary(s.BranchSummary), s.WorkingDir)
		}
	case "kill":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet sessions kill <id>")
			return
		}
		if err := c.KillSession(context.Background(), args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Killed session: %s\n", args[1])
	case "history":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet sessions history <id>")
			return
		}
		resp, err := c.ListSessionMessages(context.Background(), args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(resp.Messages) == 0 {
			fmt.Println("No messages.")
			return
		}
		fmt.Printf("%-36s %-10s %-25s %s\n", "MESSAGE_ID", "ROLE", "TIMESTAMP", "CONTENT")
		for _, msg := range resp.Messages {
			fmt.Printf("%-36s %-10s %-25s %s\n", msg.Id, msg.Role, formatTimestamp(msg.Timestamp), msg.Content)
		}
	case "clone":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet sessions clone <id>")
			return
		}
		session, err := c.CloneSession(context.Background(), args[1], "manual clone")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Cloned session: %s\n", session.Id)
		fmt.Printf("Parent: %s\nRoot: %s\n", session.ParentId, session.RootId)
	case "fork":
		sessionID, messageID, ok := parseForkArgs(args[1:])
		if !ok {
			fmt.Println("Usage: ratchet sessions fork <id> --at <message-id>")
			return
		}
		session, err := c.ForkSession(context.Background(), sessionID, messageID, "manual fork")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Forked session: %s\n", session.Id)
		fmt.Printf("Parent: %s\nRoot: %s\nForkedFrom: %s\n", session.ParentId, session.RootId, session.ForkedFromMessageId)
	case "tree":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet sessions tree <id>")
			return
		}
		resp, err := c.GetSessionTree(context.Background(), args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(resp.Sessions) == 0 {
			fmt.Println("No sessions.")
			return
		}
		fmt.Printf("%-36s %-10s %-36s %-36s %-36s %s\n", "SESSION_ID", "STATUS", "PARENT_ID", "ROOT_ID", "FORKED_FROM", "SUMMARY")
		for _, s := range resp.Sessions {
			fmt.Printf("%-36s %-10s %-36s %-36s %-36s %s\n", s.Id, s.Status, s.ParentId, s.RootId, s.ForkedFromMessageId, formatSummary(s.BranchSummary))
		}
	case "browse":
		selectedID, err := runSessionBrowser(context.Background(), c, args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if selectedID != "" {
			fmt.Printf("Selected session: %s\n", selectedID)
		}
	case "summary":
		if len(args) < 3 {
			fmt.Println("Usage: ratchet sessions summary <id> <text>")
			return
		}
		summary := strings.Join(args[2:], " ")
		session, err := c.UpdateSessionSummary(context.Background(), args[1], summary)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Updated session summary: %s\n", session.Id)
		fmt.Printf("Summary: %s\n", sanitizeSummary(session.BranchSummary))
	case "compactions":
		if len(args) < 2 {
			fmt.Println("Usage: ratchet sessions compactions <id>")
			return
		}
		resp, err := c.ListSessionCompactions(context.Background(), args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(resp.Records) == 0 {
			fmt.Println("No compactions.")
			return
		}
		fmt.Printf("%-36s %-8s %-25s %-7s %-7s %-36s %-36s %s\n", "COMPACTION_ID", "REASON", "CREATED_AT", "REMOVED", "KEPT", "FIRST_KEPT", "ARCHIVE_SESSION", "SUMMARY")
		for _, record := range resp.Records {
			fmt.Printf("%-36s %-8s %-25s %-7d %-7d %-36s %-36s %s\n",
				record.Id,
				record.Reason,
				formatTimestamp(record.CreatedAt),
				record.MessagesRemoved,
				record.MessagesKept,
				record.FirstKeptMessageId,
				record.ArchiveSessionId,
				record.Summary,
			)
		}
	case "export":
		if err := executeDaemonSessionExport(context.Background(), c, args[1:], os.Stdout); err != nil {
			if strings.HasPrefix(err.Error(), "Usage:") {
				fmt.Println(err)
				return
			}
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("unknown sessions command: %s\n", args[0])
	}
}

func printSessionsUsage() {
	fmt.Println("Usage: ratchet sessions <list|kill|history|clone|fork|tree|browse|summary|compactions|export>")
}

func executeDaemonSessionExport(ctx context.Context, c sessionsClient, args []string, w interface{ Write([]byte) (int, error) }) error {
	sessionID, output, jsonOut, ok := parseDaemonSessionExportArgs(args)
	if !ok {
		return errors.New("Usage: ratchet sessions export <id> --output <path> [--json]")
	}
	tree, err := c.GetSessionTree(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("export session tree: %w", err)
	}
	history, err := c.ListSessionMessages(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("export session history: %w", err)
	}
	compactions, err := c.ListSessionCompactions(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("export session compactions: %w", err)
	}
	session := findSessionInTree(sessionID, tree.Sessions)
	if session == nil {
		return fmt.Errorf("session %q not found in tree", sessionID)
	}
	bundle := daemonSessionExportBundle{
		Schema:      daemonSessionExportSchema,
		ExportedBy:  "ratchet-cli",
		ExportedAt:  time.Now().UTC().Format(time.RFC3339),
		Session:     session,
		Tree:        tree.Sessions,
		Messages:    history.Messages,
		Compactions: compactions.Records,
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session export: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return fmt.Errorf("create export dir: %w", err)
	}
	if err := os.WriteFile(output, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write session export: %w", err)
	}
	summary := daemonSessionExportSummary{
		Output:      output,
		SessionID:   sessionID,
		Tree:        len(bundle.Tree),
		Messages:    len(bundle.Messages),
		Compactions: len(bundle.Compactions),
	}
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	}
	fmt.Fprintf(w, "Exported session %s to %s (%d tree sessions, %d messages, %d compactions)\n",
		sessionID, output, summary.Tree, summary.Messages, summary.Compactions)
	return nil
}

func parseDaemonSessionExportArgs(args []string) (sessionID, output string, jsonOut bool, ok bool) {
	if len(args) < 3 {
		return "", "", false, false
	}
	sessionID = args[0]
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--output", "-o":
			if i+1 >= len(args) {
				return "", "", false, false
			}
			output = args[i+1]
			i++
		case "--json":
			jsonOut = true
		default:
			return "", "", false, false
		}
	}
	return sessionID, output, jsonOut, sessionID != "" && output != ""
}

func findSessionInTree(sessionID string, sessions []*pb.Session) *pb.Session {
	for _, session := range sessions {
		if session.GetId() == sessionID {
			return session
		}
	}
	return nil
}

func parseForkArgs(args []string) (sessionID, messageID string, ok bool) {
	if len(args) != 3 || args[1] != "--at" {
		return "", "", false
	}
	return args[0], args[2], args[0] != "" && args[2] != ""
}

func formatTimestamp(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().Format(time.RFC3339)
}

func formatSummary(summary string) string {
	const maxSummaryRunes = 32
	summary = sanitizeSummary(summary)
	runes := []rune(summary)
	if len(runes) <= maxSummaryRunes {
		return summary
	}
	return string(runes[:maxSummaryRunes-3]) + "..."
}

func sanitizeSummary(summary string) string {
	var b strings.Builder
	lastSpace := false
	for _, r := range summary {
		if unicode.IsSpace(r) {
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}
	return strings.TrimSpace(b.String())
}
