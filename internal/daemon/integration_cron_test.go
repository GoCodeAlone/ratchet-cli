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
		Schedule:  "5m",
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

	// After stop, job should have status=stopped or be absent from active list.
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

func TestIntegration_CronMultipleJobs(t *testing.T) {
	client, _ := startTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schedules := []struct{ schedule, cmd string }{
		{"5m", "/digest"},
		{"1h", "/report"},
		{"*/30 * * * *", "/backup"},
	}

	var ids []string
	for _, s := range schedules {
		job, err := client.CreateCron(ctx, &pb.CreateCronReq{
			SessionId: "sess-multi",
			Schedule:  s.schedule,
			Command:   s.cmd,
		})
		if err != nil {
			t.Fatalf("CreateCron(%s): %v", s.schedule, err)
		}
		ids = append(ids, job.Id)
	}

	list, err := client.ListCrons(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListCrons: %v", err)
	}
	if len(list.Jobs) < len(schedules) {
		t.Errorf("expected at least %d jobs, got %d", len(schedules), len(list.Jobs))
	}

	// Stop all.
	for _, id := range ids {
		_, _ = client.StopCron(ctx, &pb.CronJobReq{JobId: id})
	}
}
