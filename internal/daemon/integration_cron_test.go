package daemon

import (
	"context"
	"testing"
	"time"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestIntegration_CronLifecycle(t *testing.T) {
	client, _ := startTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a cron job via gRPC.
	job, err := client.CreateCron(ctx, &pb.CreateCronReq{
		SessionId: "sess-cron-1",
		Schedule:  "100ms",
		Command:   "/digest",
	})
	if err != nil {
		t.Fatalf("CreateCron: %v", err)
	}
	if job.Id == "" {
		t.Fatal("expected non-empty job ID")
	}
	if job.Status != "active" {
		t.Errorf("expected status=active, got %s", job.Status)
	}

	// List crons — verify it appears.
	list, err := client.ListCrons(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListCrons: %v", err)
	}
	found := false
	for _, j := range list.Jobs {
		if j.Id == job.Id {
			found = true
		}
	}
	if !found {
		t.Error("created cron not found in list")
	}

	// Pause the job.
	_, err = client.PauseCron(ctx, &pb.CronJobReq{JobId: job.Id})
	if err != nil {
		t.Fatalf("PauseCron: %v", err)
	}

	// Verify paused status.
	list2, err := client.ListCrons(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListCrons after pause: %v", err)
	}
	for _, j := range list2.Jobs {
		if j.Id == job.Id && j.Status != "paused" {
			t.Errorf("expected status=paused, got %s", j.Status)
		}
	}

	// Resume the job.
	_, err = client.ResumeCron(ctx, &pb.CronJobReq{JobId: job.Id})
	if err != nil {
		t.Fatalf("ResumeCron: %v", err)
	}

	// Stop the job.
	_, err = client.StopCron(ctx, &pb.CronJobReq{JobId: job.Id})
	if err != nil {
		t.Fatalf("StopCron: %v", err)
	}

	// After stop, job should have status=stopped or be absent from list.
	list3, err := client.ListCrons(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListCrons after stop: %v", err)
	}
	for _, j := range list3.Jobs {
		if j.Id == job.Id && j.Status == "active" {
			t.Errorf("expected cron to be stopped, got status=%s", j.Status)
		}
	}
}

func TestIntegration_CronTick(t *testing.T) {
	client, _ := startTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	job, err := client.CreateCron(ctx, &pb.CreateCronReq{
		SessionId: "sess-cron-tick",
		Schedule:  "100ms",
		Command:   "/check",
	})
	if err != nil {
		t.Fatalf("CreateCron: %v", err)
	}

	// Wait and verify run_count increased.
	time.Sleep(350 * time.Millisecond)

	list, err := client.ListCrons(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListCrons: %v", err)
	}
	for _, j := range list.Jobs {
		if j.Id == job.Id {
			if j.RunCount == 0 {
				t.Error("expected run_count > 0 after waiting")
			}
			break
		}
	}

	_, _ = client.StopCron(ctx, &pb.CronJobReq{JobId: job.Id})
}
