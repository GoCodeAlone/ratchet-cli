package acpclient

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

func TestClientRunPromptAgainstFixtureProcess(t *testing.T) {
	fixture := BuildFixtureAgent(t)

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	client, err := Start(ctx, AgentSpec{Name: "fixture", Command: fixture}, RunOptions{
		Cwd:     t.TempDir(),
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	result, err := client.RunPrompt(ctx, "process hello")
	if err != nil {
		t.Fatalf("RunPrompt: %v", err)
	}
	if result.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("StopReason = %q, want %q", result.StopReason, acpsdk.StopReasonEndTurn)
	}
	if got, want := result.Text, "fixture: process hello"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestDrainQueueAgainstFixtureProcessReusesSession(t *testing.T) {
	fixture := BuildFixtureAgent(t)
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 22, 30, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID:        "fixture-drain",
		Status:    SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		PromptQueue: []QueuedPrompt{
			{ID: "q-1", Prompt: "first", Status: QueuePromptStatusPending, CreatedAt: now},
			{ID: "q-2", Prompt: "second", Status: QueuePromptStatusPending, CreatedAt: now.Add(time.Second)},
		},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	result, err := DrainQueue(ctx, store, AgentSpec{
		Name:    "fixture",
		Command: fixture,
		Args:    []string{"--echo-session", "--load-session"},
	}, RunOptions{
		Cwd:     t.TempDir(),
		Timeout: 5 * time.Second,
	}, "fixture-drain", DrainOptions{Max: 2})
	if err != nil {
		t.Fatalf("DrainQueue: %v", err)
	}
	if result.Completed != 2 || result.ACPSessionID != "fixture-session" {
		t.Fatalf("result = %#v, want two completions on fixture-session", result)
	}
	got, err := store.Get("fixture-drain")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ACPSessionID != "fixture-session" {
		t.Fatalf("ACPSessionID = %q, want fixture-session", got.ACPSessionID)
	}
	if got.PromptQueue[0].Status != QueuePromptStatusCompleted || got.PromptQueue[1].Status != QueuePromptStatusCompleted {
		t.Fatalf("queue statuses = %#v", got.PromptQueue)
	}
	first := got.PromptQueue[0].Response
	second := got.PromptQueue[1].Response
	if !strings.Contains(first, "fixture-session: first") || !strings.Contains(second, "fixture-session: second") {
		t.Fatalf("responses = %q / %q, want session-tagged fixture responses", first, second)
	}
	if len(got.Turns) != 2 || got.Turns[0].Prompt != "first" || got.Turns[1].Prompt != "second" {
		t.Fatalf("turns = %#v, want FIFO turn summaries", got.Turns)
	}
}
