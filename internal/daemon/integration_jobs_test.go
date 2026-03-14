package daemon

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestIntegration_JobsAggregate(t *testing.T) {
	client, _ := startTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Create a session job.
	session, err := client.CreateSession(ctx, &pb.CreateSessionReq{WorkingDir: "/tmp"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Create a cron job.
	cronJob, err := client.CreateCron(ctx, &pb.CreateCronReq{
		SessionId: session.Id,
		Schedule:  "5m",
		Command:   "/digest",
	})
	if err != nil {
		t.Fatalf("CreateCron: %v", err)
	}

	// Start a fleet (fire-and-forget; drain asynchronously).
	fleetStream, err := client.StartFleet(ctx, &pb.StartFleetReq{
		SessionId:  session.Id,
		PlanId:     "plan-jobs-test",
		MaxWorkers: 1,
	})
	if err != nil {
		t.Fatalf("StartFleet: %v", err)
	}
	go func() {
		for {
			_, err := fleetStream.Recv()
			if err == io.EOF || err != nil {
				return
			}
		}
	}()

	// ListJobs should surface the session and cron.
	list, err := client.ListJobs(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}

	typesSeen := map[string]bool{}
	for _, j := range list.Jobs {
		typesSeen[j.Type] = true
	}
	if !typesSeen["session"] {
		t.Error("expected session job in ListJobs")
	}
	if !typesSeen["cron"] {
		t.Error("expected cron job in ListJobs")
	}

	// PauseJob the cron.
	cronJobID := "cron:" + cronJob.Id
	_, err = client.PauseJob(ctx, &pb.JobReq{JobId: cronJobID})
	if err != nil {
		t.Fatalf("PauseJob cron: %v", err)
	}

	// KillJob the session.
	sessionJobID := "session:" + session.Id
	_, err = client.KillJob(ctx, &pb.JobReq{JobId: sessionJobID})
	if err != nil {
		t.Fatalf("KillJob session: %v", err)
	}

	// Verify killed session no longer active.
	list2, err := client.ListJobs(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListJobs after kill: %v", err)
	}
	for _, j := range list2.Jobs {
		if j.Id == sessionJobID && j.Status == "active" {
			t.Error("session job should no longer be active after kill")
		}
	}

	// Cleanup cron.
	_, _ = client.StopCron(ctx, &pb.CronJobReq{JobId: cronJob.Id})
}

func TestIntegration_JobsUnknownType(t *testing.T) {
	client, _ := startTestServer(t)
	ctx := context.Background()

	_, err := client.KillJob(ctx, &pb.JobReq{JobId: "unknown:xyz"})
	if err == nil {
		t.Error("expected error for unknown job type")
	}
	if !strings.Contains(err.Error(), "no provider") {
		t.Errorf("unexpected error message: %v", err)
	}
}
