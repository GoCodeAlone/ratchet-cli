package main

import (
	"context"
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
			{Id: "sess-1", RootId: "sess-1", Status: "active", BranchSummary: "root summary"},
			{Id: "fork-1", ParentId: "sess-1", RootId: "sess-1", ForkedFromMessageId: "msg-1", Status: "active", BranchSummary: "fork summary"},
		}},
		compactions: &pb.SessionCompactionList{Records: []*pb.CompactionRecord{
			{Id: "comp-1", Reason: "manual", Summary: "short summary", MessagesRemoved: 4, MessagesKept: 3, FirstKeptMessageId: "msg-5", ArchiveSessionId: "archive-1", CreatedAt: timestamppb.Now()},
		}},
	}
	withFakeSessionsClient(t, fake)

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
	for _, want := range []string{"SESSION_ID", "SUMMARY", "sess-1", "fork-1", "msg-1", "fork summary"} {
		if !strings.Contains(treeOut, want) {
			t.Fatalf("tree output missing %q:\n%s", want, treeOut)
		}
	}

	summaryOut := captureStdout(t, func() {
		handleSessions([]string{"summary", "fork-1", "new branch summary"})
	})
	if !strings.Contains(summaryOut, "new branch summary") || fake.summaryText != "new branch summary" {
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
