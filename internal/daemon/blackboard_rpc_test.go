package daemon

import (
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/mesh"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestBlackboardRPCReadWriteList(t *testing.T) {
	svc := &Service{meshBB: mesh.NewBlackboard()}
	addr := startTestGRPCServer(t, svc)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	client := pb.NewRatchetDaemonClient(conn)
	ctx := t.Context()

	missing, err := client.BlackboardRead(ctx, &pb.BlackboardReadReq{Section: "plan", Key: "status"})
	if err != nil {
		t.Fatalf("BlackboardRead missing: %v", err)
	}
	if missing.Found {
		t.Fatal("expected missing entry")
	}

	written, err := client.BlackboardWrite(ctx, &pb.BlackboardWriteReq{
		Section: "plan",
		Key:     "status",
		Value:   "ready",
		Author:  "test",
	})
	if err != nil {
		t.Fatalf("BlackboardWrite: %v", err)
	}
	if written.Revision == 0 || written.Value != "ready" || written.Author != "test" {
		t.Fatalf("written entry = %#v", written)
	}

	found, err := client.BlackboardRead(ctx, &pb.BlackboardReadReq{Section: "plan", Key: "status"})
	if err != nil {
		t.Fatalf("BlackboardRead found: %v", err)
	}
	if !found.Found || found.Entry.Value != "ready" {
		t.Fatalf("found entry = %#v", found)
	}

	sections, err := client.BlackboardList(ctx, &pb.BlackboardListReq{})
	if err != nil {
		t.Fatalf("BlackboardList sections: %v", err)
	}
	if len(sections.Sections) != 1 || sections.Sections[0] != "plan" {
		t.Fatalf("sections = %#v", sections.Sections)
	}

	entries, err := client.BlackboardList(ctx, &pb.BlackboardListReq{Section: "plan"})
	if err != nil {
		t.Fatalf("BlackboardList entries: %v", err)
	}
	if len(entries.Entries) != 1 || entries.Entries[0].Key != "status" {
		t.Fatalf("entries = %#v", entries.Entries)
	}
}
