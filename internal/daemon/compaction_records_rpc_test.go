package daemon

import (
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCompactionRecordRPC(t *testing.T) {
	h := newE2EHarness(t)
	ctx := t.Context()
	session := h.createSession(t, "e2e-mock")
	if _, err := appendCompactionRecord(ctx, h.DB, CompactionRecord{
		SessionID:          session.Id,
		Summary:            "rpc summary",
		Reason:             "manual",
		MessagesRemoved:    4,
		MessagesKept:       3,
		FirstKeptMessageID: "msg-5",
	}); err != nil {
		t.Fatal(err)
	}

	got, err := h.Client.ListSessionCompactions(ctx, &pb.SessionCompactionsReq{SessionId: session.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Records) != 1 {
		t.Fatalf("record count = %d, want 1", len(got.Records))
	}
	if got.Records[0].Reason != "manual" || got.Records[0].Summary != "rpc summary" {
		t.Fatalf("record = %+v", got.Records[0])
	}

	_, err = h.Client.ListSessionCompactions(ctx, &pb.SessionCompactionsReq{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty session code = %v, want InvalidArgument", status.Code(err))
	}
	_, err = h.Svc.ListSessionCompactions(ctx, nil)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("nil request code = %v, want InvalidArgument", status.Code(err))
	}
}
