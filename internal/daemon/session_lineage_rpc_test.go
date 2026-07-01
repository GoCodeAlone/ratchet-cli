package daemon

import (
	"slices"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSessionLineageHistoryCloneForkTreeRPC(t *testing.T) {
	h := newE2EHarness(t)
	ctx := t.Context()

	source := h.createSession(t, "e2e-mock")
	firstID := insertMessage(t, h.DB, source.Id, "user", "first")
	secondID := insertMessage(t, h.DB, source.Id, "assistant", "second")
	insertMessage(t, h.DB, source.Id, "user", "third")

	history, err := h.Client.ListSessionMessages(ctx, &pb.SessionMessagesReq{SessionId: source.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Messages) != 3 {
		t.Fatalf("history length = %d, want 3", len(history.Messages))
	}
	if history.Messages[0].Id != firstID {
		t.Fatalf("first history ID = %q, want %q", history.Messages[0].Id, firstID)
	}

	clone, err := h.Client.CloneSession(ctx, &pb.CloneSessionReq{
		SessionId: source.Id,
		Reason:    "parallel attempt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if clone.ParentId != source.Id || clone.RootId != source.Id {
		t.Fatalf("clone lineage parent=%q root=%q, want source %q", clone.ParentId, clone.RootId, source.Id)
	}
	cloneHistory, err := h.Client.ListSessionMessages(ctx, &pb.SessionMessagesReq{SessionId: clone.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(cloneHistory.Messages) != 3 {
		t.Fatalf("clone history length = %d, want 3", len(cloneHistory.Messages))
	}

	fork, err := h.Client.ForkSession(ctx, &pb.ForkSessionReq{
		SessionId: source.Id,
		MessageId: secondID,
		Reason:    "branch from second",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fork.ParentId != source.Id || fork.RootId != source.Id || fork.ForkedFromMessageId != secondID {
		t.Fatalf("fork lineage parent=%q root=%q from=%q, want source/source/%s", fork.ParentId, fork.RootId, fork.ForkedFromMessageId, secondID)
	}
	forkHistory, err := h.Client.ListSessionMessages(ctx, &pb.SessionMessagesReq{SessionId: fork.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(forkHistory.Messages) != 2 {
		t.Fatalf("fork history length = %d, want 2", len(forkHistory.Messages))
	}

	tree, err := h.Client.GetSessionTree(ctx, &pb.SessionTreeReq{SessionId: source.Id})
	if err != nil {
		t.Fatal(err)
	}
	gotIDs := make([]string, 0, len(tree.Sessions))
	for _, session := range tree.Sessions {
		gotIDs = append(gotIDs, session.Id)
	}
	for _, want := range []string{source.Id, clone.Id, fork.Id} {
		if !slices.Contains(gotIDs, want) {
			t.Fatalf("tree IDs %v missing %s", gotIDs, want)
		}
	}

	_, err = h.Client.ForkSession(ctx, &pb.ForkSessionReq{
		SessionId: source.Id,
		MessageId: "missing-message",
		Reason:    "bad branch",
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("missing message fork code = %v, want NotFound (err=%v)", status.Code(err), err)
	}
}

func TestSessionLineageRPCValidatesRequests(t *testing.T) {
	h := newE2EHarness(t)
	ctx := t.Context()

	for name, call := range map[string]func() error{
		"list nil": func() error {
			_, err := h.Svc.ListSessionMessages(ctx, nil)
			return err
		},
		"list empty session": func() error {
			_, err := h.Client.ListSessionMessages(ctx, &pb.SessionMessagesReq{})
			return err
		},
		"clone nil": func() error {
			_, err := h.Svc.CloneSession(ctx, nil)
			return err
		},
		"clone empty session": func() error {
			_, err := h.Client.CloneSession(ctx, &pb.CloneSessionReq{})
			return err
		},
		"fork nil": func() error {
			_, err := h.Svc.ForkSession(ctx, nil)
			return err
		},
		"fork empty session": func() error {
			_, err := h.Client.ForkSession(ctx, &pb.ForkSessionReq{MessageId: "msg-1"})
			return err
		},
		"fork empty message": func() error {
			_, err := h.Client.ForkSession(ctx, &pb.ForkSessionReq{SessionId: "sess-1"})
			return err
		},
		"tree nil": func() error {
			_, err := h.Svc.GetSessionTree(ctx, nil)
			return err
		},
		"tree empty session": func() error {
			_, err := h.Client.GetSessionTree(ctx, &pb.SessionTreeReq{})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if code := status.Code(call()); code != codes.InvalidArgument {
				t.Fatalf("code = %v, want InvalidArgument", code)
			}
		})
	}
}
