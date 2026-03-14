package daemon

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// startTestServerWithService starts an in-process daemon gRPC server and returns both the
// client and the underlying Service so tests can inject plans directly.
func startTestServerWithService(t *testing.T) (pb.RatchetDaemonClient, *Service) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	EnsureDataDir()

	sock := filepath.Join(tmp, "plan_integration.sock")
	lis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	srv := grpc.NewServer()
	pb.RegisterRatchetDaemonServer(srv, svc)
	go srv.Serve(lis)
	t.Cleanup(func() {
		srv.Stop()
		lis.Close()
	})

	conn, err := grpc.NewClient(
		"unix://"+sock,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	return pb.NewRatchetDaemonClient(conn), svc
}

func TestIntegration_PlanLifecycle(t *testing.T) {
	client, svc := startTestServerWithService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a session.
	session, err := client.CreateSession(ctx, &pb.CreateSessionReq{
		WorkingDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Inject a plan directly via PlanManager.
	steps := []*pb.PlanStep{
		{Id: "step-1", Description: "design the API", Status: "pending"},
		{Id: "step-2", Description: "implement the handler", Status: "pending"},
	}
	plan := svc.plans.Create(session.Id, "build something", steps)
	if plan.Id == "" {
		t.Fatal("expected non-empty plan ID")
	}
	if plan.Status != "proposed" {
		t.Errorf("expected plan status=proposed, got %s", plan.Status)
	}

	// Approve the plan via gRPC and collect the stream event.
	stream, err := client.ApprovePlan(ctx, &pb.ApprovePlanReq{
		SessionId: session.Id,
		PlanId:    plan.Id,
	})
	if err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}

	var gotPlanProposed bool
	var receivedPlan *pb.Plan
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("stream.Recv: %v", err)
		}
		if pp, ok := ev.Event.(*pb.ChatEvent_PlanProposed); ok {
			gotPlanProposed = true
			receivedPlan = pp.PlanProposed
		}
	}

	if !gotPlanProposed {
		t.Fatal("expected ChatEvent_PlanProposed event")
	}
	if receivedPlan.Status != "approved" {
		t.Errorf("expected plan status=approved in event, got %s", receivedPlan.Status)
	}

	// Verify plan status directly in PlanManager.
	p := svc.plans.Get(plan.Id)
	if p == nil {
		t.Fatal("plan not found after approval")
	}
	if p.Status != "approved" {
		t.Errorf("expected plan status=approved, got %s", p.Status)
	}
}

func TestIntegration_PlanReject(t *testing.T) {
	client, svc := startTestServerWithService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a session.
	session, err := client.CreateSession(ctx, &pb.CreateSessionReq{
		WorkingDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Inject a plan directly via PlanManager.
	steps := []*pb.PlanStep{
		{Id: "step-1", Description: "analyze requirements", Status: "pending"},
	}
	plan := svc.plans.Create(session.Id, "write a report", steps)

	// Reject the plan via gRPC.
	_, err = client.RejectPlan(ctx, &pb.RejectPlanReq{
		SessionId: session.Id,
		PlanId:    plan.Id,
		Feedback:  "needs more detail",
	})
	if err != nil {
		t.Fatalf("RejectPlan: %v", err)
	}

	// Verify plan status is rejected via PlanManager.
	p := svc.plans.Get(plan.Id)
	if p == nil {
		t.Fatal("plan not found after rejection")
	}
	if p.Status != "rejected" {
		t.Errorf("expected plan status=rejected, got %s", p.Status)
	}
	if p.Feedback != "needs more detail" {
		t.Errorf("expected feedback='needs more detail', got %s", p.Feedback)
	}
}
