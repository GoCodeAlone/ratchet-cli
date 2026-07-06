package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestHandleSessionsHistoryCloneForkTree(t *testing.T) {
	fake := &fakeSessionsClient{
		history: &pb.SessionHistory{Messages: []*pb.HistoryMessage{
			{Id: "msg-1", Role: "user", Content: "hello", Timestamp: timestamppb.Now()},
			{Id: "msg-2", Role: "assistant", Content: "world", Timestamp: timestamppb.Now()},
		}},
		clone: &pb.Session{Id: "clone-1", ParentId: "sess-1", RootId: "sess-1"},
		fork:  &pb.Session{Id: "fork-1", ParentId: "sess-1", RootId: "sess-1", ForkedFromMessageId: "msg-1"},
		tree: &pb.SessionList{Sessions: []*pb.Session{
			{Id: "sess-1", RootId: "sess-1", Status: "active", Provider: "mock", WorkingDir: "/tmp/project", BranchSummary: "root summary with spaces and enough detail to truncate"},
			{Id: "fork-1", ParentId: "sess-1", RootId: "sess-1", ForkedFromMessageId: "msg-1", Status: "active", BranchSummary: "fork\nsummary\tok\x01"},
		}},
		compactions: &pb.SessionCompactionList{Records: []*pb.CompactionRecord{
			{Id: "comp-1", Reason: "manual", Summary: "short summary", MessagesRemoved: 4, MessagesKept: 3, FirstKeptMessageId: "msg-5", ArchiveSessionId: "archive-1", CreatedAt: timestamppb.Now()},
		}},
	}
	withFakeSessionsClient(t, fake)

	listOut := captureStdout(t, func() {
		handleSessions([]string{"list"})
	})
	for _, want := range []string{"SUMMARY", formatSummary(fake.tree.Sessions[0].BranchSummary)} {
		if !strings.Contains(listOut, want) {
			t.Fatalf("list output missing %q:\n%s", want, listOut)
		}
	}

	historyOut := captureStdout(t, func() {
		handleSessions([]string{"history", "sess-1"})
	})
	for _, want := range []string{"MESSAGE_ID", "msg-1", "user", "hello"} {
		if !strings.Contains(historyOut, want) {
			t.Fatalf("history output missing %q:\n%s", want, historyOut)
		}
	}

	cloneOut := captureStdout(t, func() {
		handleSessions([]string{"clone", "sess-1"})
	})
	if !strings.Contains(cloneOut, "clone-1") || fake.cloneReason != "manual clone" {
		t.Fatalf("clone output/reason = %q / %q", cloneOut, fake.cloneReason)
	}

	forkOut := captureStdout(t, func() {
		handleSessions([]string{"fork", "sess-1", "--at", "msg-1"})
	})
	if !strings.Contains(forkOut, "fork-1") || fake.forkMessageID != "msg-1" {
		t.Fatalf("fork output/message = %q / %q", forkOut, fake.forkMessageID)
	}

	treeOut := captureStdout(t, func() {
		handleSessions([]string{"tree", "sess-1"})
	})
	for _, want := range []string{"SESSION_ID", "SUMMARY", "sess-1", "fork-1", "msg-1", "fork summary ok"} {
		if !strings.Contains(treeOut, want) {
			t.Fatalf("tree output missing %q:\n%s", want, treeOut)
		}
	}

	rawSummary := "new\nbranch\tsummary\x01"
	summaryOut := captureStdout(t, func() {
		handleSessions([]string{"summary", "fork-1", rawSummary})
	})
	if !strings.Contains(summaryOut, "new branch summary") || fake.summaryText != rawSummary {
		t.Fatalf("summary output/text = %q / %q", summaryOut, fake.summaryText)
	}

	compactionsOut := captureStdout(t, func() {
		handleSessions([]string{"compactions", "sess-1"})
	})
	for _, want := range []string{"COMPACTION_ID", "ARCHIVE_SESSION", "comp-1", "manual", "msg-5", "archive-1", "short summary"} {
		if !strings.Contains(compactionsOut, want) {
			t.Fatalf("compactions output missing %q:\n%s", want, compactionsOut)
		}
	}
}

func TestHandleSessionsBrowseRunsInjectedBrowser(t *testing.T) {
	fake := &fakeSessionsClient{tree: &pb.SessionList{}}
	withFakeSessionsClient(t, fake)
	oldRun := runSessionBrowser
	var gotID string
	runSessionBrowser = func(_ context.Context, _ sessionsClient, rootID string) (string, error) {
		gotID = rootID
		return "fork-1", nil
	}
	t.Cleanup(func() { runSessionBrowser = oldRun })

	out := captureStdout(t, func() {
		handleSessions([]string{"browse", "sess-1"})
	})

	if gotID != "sess-1" {
		t.Fatalf("browser root ID = %q, want sess-1", gotID)
	}
	if !strings.Contains(out, "Selected session: fork-1") {
		t.Fatalf("browse output missing selected session:\n%s", out)
	}
}

func TestHandleSessionsBrowseValidatesID(t *testing.T) {
	oldEnsure := ensureSessionsClient
	ensureSessionsClient = func() (sessionsClient, error) {
		t.Fatal("ensureSessionsClient should not run for browse without id")
		return nil, nil
	}
	t.Cleanup(func() { ensureSessionsClient = oldEnsure })
	oldRun := runSessionBrowser
	runSessionBrowser = func(context.Context, sessionsClient, string) (string, error) {
		return "", fmt.Errorf("browser should not run without id")
	}
	t.Cleanup(func() { runSessionBrowser = oldRun })

	out := captureStdout(t, func() {
		handleSessions([]string{"browse"})
	})

	if !strings.Contains(out, "Usage: ratchet sessions browse <id>") {
		t.Fatalf("missing browse usage:\n%s", out)
	}
}

func TestHandleSessionsExportWritesSensitiveBundle(t *testing.T) {
	fake := &fakeSessionsClient{
		history: &pb.SessionHistory{Messages: []*pb.HistoryMessage{
			{Id: "msg-1", Role: "user", Content: "secret prompt", Timestamp: timestamppb.Now()},
			{Id: "msg-2", Role: "assistant", Content: "secret response", Timestamp: timestamppb.Now()},
		}},
		tree: &pb.SessionList{Sessions: []*pb.Session{
			{Id: "sess-1", RootId: "sess-1", Status: "active", Provider: "mock", WorkingDir: "/tmp/project", BranchSummary: "root"},
			{Id: "fork-1", ParentId: "sess-1", RootId: "sess-1", Status: "done", Provider: "mock", WorkingDir: "/tmp/project", BranchSummary: "fork"},
		}},
		compactions: &pb.SessionCompactionList{Records: []*pb.CompactionRecord{
			{Id: "comp-1", Reason: "manual", Summary: "compact summary", MessagesRemoved: 2, MessagesKept: 1, CreatedAt: timestamppb.Now()},
		}},
	}
	withFakeSessionsClient(t, fake)
	outPath := filepath.Join(t.TempDir(), "session.json")

	stdout := captureStdout(t, func() {
		handleSessions([]string{"export", "sess-1", "--output", outPath, "--json"})
	})
	if strings.Contains(stdout, "secret prompt") || strings.Contains(stdout, "secret response") {
		t.Fatalf("stdout leaked message content:\n%s", stdout)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat export: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %v, want 0600", got)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	var bundle daemonSessionExportBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatalf("unmarshal export: %v\n%s", err, data)
	}
	if bundle.Schema != "ratchet.session-export.v1" || bundle.Session.Id != "sess-1" {
		t.Fatalf("bundle identity = %q/%q", bundle.Schema, bundle.Session.Id)
	}
	if len(bundle.Tree) != 2 || len(bundle.Messages) != 2 || len(bundle.Compactions) != 1 {
		t.Fatalf("bundle counts tree=%d messages=%d compactions=%d", len(bundle.Tree), len(bundle.Messages), len(bundle.Compactions))
	}
	if !strings.Contains(stdout, `"messages": 2`) {
		t.Fatalf("json summary missing message count:\n%s", stdout)
	}
}

func TestHandleSessionsExportRequiresOutput(t *testing.T) {
	fake := &fakeSessionsClient{tree: &pb.SessionList{}}
	withFakeSessionsClient(t, fake)
	out := captureStdout(t, func() {
		handleSessions([]string{"export", "sess-1"})
	})
	if !strings.Contains(out, "Usage: ratchet sessions export") {
		t.Fatalf("missing usage:\n%s", out)
	}
}

func withFakeSessionsClient(t *testing.T, fake *fakeSessionsClient) {
	t.Helper()
	old := ensureSessionsClient
	ensureSessionsClient = func() (sessionsClient, error) {
		return fake, nil
	}
	t.Cleanup(func() { ensureSessionsClient = old })
}

type fakeSessionsClient struct {
	history       *pb.SessionHistory
	clone         *pb.Session
	fork          *pb.Session
	tree          *pb.SessionList
	compactions   *pb.SessionCompactionList
	cloneReason   string
	forkReason    string
	forkMessageID string
	summaryText   string
}

func (f *fakeSessionsClient) Close() error {
	return nil
}

func (f *fakeSessionsClient) ListSessions(context.Context) (*pb.SessionList, error) {
	return f.tree, nil
}

func (f *fakeSessionsClient) KillSession(context.Context, string) error {
	return nil
}

func (f *fakeSessionsClient) ListSessionMessages(_ context.Context, _ string) (*pb.SessionHistory, error) {
	return f.history, nil
}

func (f *fakeSessionsClient) CloneSession(_ context.Context, _ string, reason string) (*pb.Session, error) {
	f.cloneReason = reason
	return f.clone, nil
}

func (f *fakeSessionsClient) ForkSession(_ context.Context, _ string, messageID, reason string) (*pb.Session, error) {
	f.forkMessageID = messageID
	f.forkReason = reason
	return f.fork, nil
}

func (f *fakeSessionsClient) GetSessionTree(context.Context, string) (*pb.SessionList, error) {
	return f.tree, nil
}

func (f *fakeSessionsClient) ListSessionCompactions(context.Context, string) (*pb.SessionCompactionList, error) {
	return f.compactions, nil
}

func (f *fakeSessionsClient) UpdateSessionSummary(_ context.Context, sessionID, summary string) (*pb.Session, error) {
	f.summaryText = summary
	return &pb.Session{Id: sessionID, BranchSummary: summary}, nil
}
