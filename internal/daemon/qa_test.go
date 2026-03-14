package daemon

// QA validation tests for Phase 14b: job control + cron + compression + review.
// These tests exercise the full gRPC stack (startTestServer) to simulate the
// same scenarios that would be validated interactively in the TUI.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GoCodeAlone/ratchet-cli/internal/agent"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

// ---------------------------------------------------------------------------
// Task 29: Job control panel — verify jobs from multiple managers are visible
// ---------------------------------------------------------------------------

func TestQA_JobControlPanel_AggregatesAllTypes(t *testing.T) {
	client, _ := startTestServer(t)
	ctx := context.Background()

	// Create a session (session job).
	sess, err := client.CreateSession(ctx, &pb.CreateSessionReq{WorkingDir: t.TempDir()})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Create a cron job (simulates /loop 10s /sessions).
	cronJob, err := client.CreateCron(ctx, &pb.CreateCronReq{
		SessionId: sess.Id,
		Schedule:  "10s",
		Command:   "/sessions",
	})
	if err != nil {
		t.Fatalf("CreateCron: %v", err)
	}

	// List jobs — both should appear.
	jobs, err := client.ListJobs(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}

	var foundSession, foundCron bool
	for _, j := range jobs.Jobs {
		if j.Type == "session" && j.SessionId == sess.Id {
			foundSession = true
		}
		if j.Type == "cron" && strings.HasSuffix(j.Id, cronJob.Id) {
			foundCron = true
		}
	}
	if !foundSession {
		t.Error("QA29: session job not visible in ListJobs")
	}
	if !foundCron {
		t.Error("QA29: cron job not visible in ListJobs")
	}
}

func TestQA_JobControlPanel_PauseCron(t *testing.T) {
	client, _ := startTestServer(t)
	ctx := context.Background()

	cronJob, err := client.CreateCron(ctx, &pb.CreateCronReq{
		SessionId: "qa-sess",
		Schedule:  "10s",
		Command:   "/sessions",
	})
	if err != nil {
		t.Fatalf("CreateCron: %v", err)
	}

	// Simulate 'p' key on job panel: pause the cron job.
	_, err = client.PauseJob(ctx, &pb.JobReq{JobId: "cron:" + cronJob.Id})
	if err != nil {
		t.Fatalf("PauseJob: %v", err)
	}

	// Verify status changed to paused.
	list, err := client.ListCrons(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListCrons: %v", err)
	}
	for _, j := range list.Jobs {
		if j.Id == cronJob.Id && j.Status != "paused" {
			t.Errorf("QA29: expected cron status=paused, got %q", j.Status)
		}
	}
}

func TestQA_JobControlPanel_KillSession(t *testing.T) {
	client, _ := startTestServer(t)
	ctx := context.Background()

	sess, err := client.CreateSession(ctx, &pb.CreateSessionReq{WorkingDir: t.TempDir()})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Simulate 'k' key: kill the session job.
	_, err = client.KillJob(ctx, &pb.JobReq{JobId: "session:" + sess.Id})
	if err != nil {
		t.Fatalf("KillJob: %v", err)
	}

	// Verify session is no longer active in ListJobs.
	jobs, err := client.ListJobs(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListJobs after kill: %v", err)
	}
	for _, j := range jobs.Jobs {
		if j.Type == "session" && j.SessionId == sess.Id && j.Status == "active" {
			t.Error("QA29: killed session should not appear as active in job list")
		}
	}
}

// ---------------------------------------------------------------------------
// Task 30: Cron/loop scheduling end-to-end
// ---------------------------------------------------------------------------

func TestQA_CronScheduling_LoopAndVerifyTicks(t *testing.T) {
	client, _ := startTestServer(t)
	ctx := context.Background()

	// /loop 5s /sessions equivalent — use 100ms for speed.
	job, err := client.CreateCron(ctx, &pb.CreateCronReq{
		SessionId: "qa-loop-sess",
		Schedule:  "100ms",
		Command:   "/sessions",
	})
	if err != nil {
		t.Fatalf("CreateCron (loop): %v", err)
	}

	// Wait enough for 2-3 ticks.
	time.Sleep(400 * time.Millisecond)

	list, err := client.ListCrons(ctx, &pb.Empty{})
	if err != nil {
		t.Fatalf("ListCrons: %v", err)
	}
	var found *pb.CronJob
	for _, j := range list.Jobs {
		if j.Id == job.Id {
			jCopy := j
			found = jCopy
		}
	}
	if found == nil {
		t.Fatal("QA30: job not found after ticks")
	}
	if found.RunCount < 2 {
		t.Errorf("QA30: expected >= 2 ticks, got run_count=%d", found.RunCount)
	}
	if found.LastRun == "" {
		t.Error("QA30: last_run should be set after ticks")
	}

	// /cron pause → no ticks for 200ms.
	_, err = client.PauseCron(ctx, &pb.CronJobReq{JobId: job.Id})
	if err != nil {
		t.Fatalf("PauseCron: %v", err)
	}

	countAtPause := found.RunCount
	time.Sleep(200 * time.Millisecond)

	list2, _ := client.ListCrons(ctx, &pb.Empty{})
	for _, j := range list2.Jobs {
		if j.Id == job.Id && j.RunCount > countAtPause {
			t.Errorf("QA30: paused job should not tick (was %d, now %d)", countAtPause, j.RunCount)
		}
	}

	// /cron resume → ticks resume.
	_, err = client.ResumeCron(ctx, &pb.CronJobReq{JobId: job.Id})
	if err != nil {
		t.Fatalf("ResumeCron: %v", err)
	}
	time.Sleep(250 * time.Millisecond)

	list3, _ := client.ListCrons(ctx, &pb.Empty{})
	for _, j := range list3.Jobs {
		if j.Id == job.Id && j.RunCount <= countAtPause {
			t.Errorf("QA30: resumed job should have ticked (still at %d)", j.RunCount)
		}
	}

	// /cron stop → removed/stopped.
	_, err = client.StopCron(ctx, &pb.CronJobReq{JobId: job.Id})
	if err != nil {
		t.Fatalf("StopCron: %v", err)
	}
	list4, _ := client.ListCrons(ctx, &pb.Empty{})
	for _, j := range list4.Jobs {
		if j.Id == job.Id && j.Status == "active" {
			t.Error("QA30: stopped job should not be active")
		}
	}
}

func TestQA_CronScheduling_InvalidExpression(t *testing.T) {
	client, _ := startTestServer(t)
	ctx := context.Background()

	_, err := client.CreateCron(ctx, &pb.CreateCronReq{
		SessionId: "qa",
		Schedule:  "not-valid",
		Command:   "/help",
	})
	if err == nil {
		t.Error("QA30: expected error for invalid schedule expression")
	}
}

// ---------------------------------------------------------------------------
// Task 31: Context compression
// ---------------------------------------------------------------------------

func TestQA_ContextCompression_TokenTrackerThreshold(t *testing.T) {
	// Validate the TokenTracker correctly triggers compression threshold.
	tracker := NewTokenTracker()
	sessionID := "qa-compress-sess"

	// Add tokens approaching threshold.
	tracker.AddTokens(sessionID, 8000, 1000) // 9000 of 10000 = 90%
	if !tracker.ShouldCompress(sessionID, 0.9, 10000) {
		t.Error("QA31: expected ShouldCompress=true at 90% threshold")
	}

	// Below threshold should not trigger.
	tracker2 := NewTokenTracker()
	tracker2.AddTokens(sessionID, 5000, 0)
	if tracker2.ShouldCompress(sessionID, 0.9, 10000) {
		t.Error("QA31: expected ShouldCompress=false at 50%")
	}

	// After reset, should not compress.
	tracker.Reset(sessionID)
	if tracker.ShouldCompress(sessionID, 0.9, 10000) {
		t.Error("QA31: expected ShouldCompress=false after reset")
	}
}

func TestQA_ContextCompression_SummarizePreservesRecent(t *testing.T) {
	ctx := context.Background()

	// Build a message history > preserve window.
	messages := make([]provider.Message, 20)
	for i := range messages {
		role := provider.RoleUser
		if i%2 == 1 {
			role = provider.RoleAssistant
		}
		messages[i] = provider.Message{Role: role, Content: fmt.Sprintf("message %d", i)}
	}

	compressed, summary, err := Compress(ctx, messages, 5, nil)
	if err != nil {
		t.Fatalf("QA31: Compress: %v", err)
	}
	if summary == "" {
		t.Error("QA31: expected non-empty summary")
	}
	// First message should be the summary.
	if len(compressed) == 0 || compressed[0].Role != provider.RoleSystem {
		t.Error("QA31: compressed[0] should be system summary message")
	}
	// Last 5 messages should be preserved.
	if len(compressed) < 5 {
		t.Errorf("QA31: expected at least 5 messages preserved, got %d", len(compressed))
	}
}

// ---------------------------------------------------------------------------
// Task 32: Code review agent loads
// ---------------------------------------------------------------------------

func TestQA_CodeReviewAgent_BuiltinLoads(t *testing.T) {
	// Verify the code-reviewer builtin is properly embedded and parseable.
	defs, err := agent.LoadBuiltins()
	if err != nil {
		t.Fatalf("QA32: LoadBuiltins: %v", err)
	}
	var found bool
	for _, d := range defs {
		if d.Name == "code-reviewer" {
			found = true
			if d.SystemPrompt == "" {
				t.Error("QA32: code-reviewer missing system_prompt")
			}
			if len(d.Tools) == 0 {
				t.Error("QA32: code-reviewer should have tools defined")
			}
		}
	}
	if !found {
		t.Error("QA32: code-reviewer builtin agent definition not found")
	}
}
