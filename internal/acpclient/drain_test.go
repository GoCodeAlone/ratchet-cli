package acpclient

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

func TestDrainQueueDrainsPendingFIFOAndPersistsSessionID(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 22, 0, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID:        "drain-fifo",
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
	runner := &fakeDrainRunner{sessionID: "acp-created"}

	result, err := DrainQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "drain-fifo", DrainOptions{
		Now: fixedClock(now.Add(time.Minute)),
		StartRunner: func(_ context.Context, _ AgentSpec, _ RunOptions, existingID string) (DrainPromptRunner, func() error, error) {
			if existingID != "" {
				t.Fatalf("existingID = %q, want empty for first drain", existingID)
			}
			return runner, func() error { runner.closed = true; return nil }, nil
		},
	})
	if err != nil {
		t.Fatalf("DrainQueue: %v", err)
	}
	if result.Completed != 2 || result.Processed != 2 || result.ACPSessionID != "acp-created" {
		t.Fatalf("result = %#v, want two completions on acp-created", result)
	}
	if got, want := strings.Join(runner.prompts, ","), "first,second"; got != want {
		t.Fatalf("prompts = %q, want %q", got, want)
	}
	if !runner.closed {
		t.Fatal("runner close was not called")
	}
	got, err := store.Get("drain-fifo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ACPSessionID != "acp-created" {
		t.Fatalf("ACPSessionID = %q, want acp-created", got.ACPSessionID)
	}
	if got.PromptQueue[0].Status != QueuePromptStatusCompleted || got.PromptQueue[1].Status != QueuePromptStatusCompleted {
		t.Fatalf("queue statuses = %#v", got.PromptQueue)
	}
	if len(got.Turns) != 2 || got.Turns[0].Prompt != "first" || got.Turns[1].Prompt != "second" {
		t.Fatalf("turns = %#v, want FIFO summaries", got.Turns)
	}
	if _, err := store.Owner("drain-fifo"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Owner after drain = %v, want missing", err)
	}
}

func TestDrainQueueReusesPersistedSessionAndHonorsMax(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 22, 5, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID:           "drain-existing",
		ACPSessionID: "acp-existing",
		Status:       SessionStatusQueued,
		CreatedAt:    now,
		UpdatedAt:    now,
		PromptQueue: []QueuedPrompt{
			{ID: "q-1", Prompt: "first", Status: QueuePromptStatusPending, CreatedAt: now},
			{ID: "q-2", Prompt: "second", Status: QueuePromptStatusPending, CreatedAt: now.Add(time.Second)},
		},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	runner := &fakeDrainRunner{sessionID: "acp-existing"}

	result, err := DrainQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "drain-existing", DrainOptions{
		Max: 1,
		Now: fixedClock(now.Add(time.Minute)),
		StartRunner: func(_ context.Context, _ AgentSpec, _ RunOptions, existingID string) (DrainPromptRunner, func() error, error) {
			if existingID != "acp-existing" {
				t.Fatalf("existingID = %q, want acp-existing", existingID)
			}
			return runner, func() error { return nil }, nil
		},
	})
	if err != nil {
		t.Fatalf("DrainQueue: %v", err)
	}
	if result.Completed != 1 || result.Processed != 1 || result.Remaining != 1 {
		t.Fatalf("result = %#v, want one completion and one remaining", result)
	}
	got, err := store.Get("drain-existing")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PromptQueue[0].Status != QueuePromptStatusCompleted || got.PromptQueue[1].Status != QueuePromptStatusPending {
		t.Fatalf("queue statuses = %#v", got.PromptQueue)
	}
}

func TestDrainQueuePersistsPromptEvents(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 22, 7, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID:        "drain-events",
		Status:    SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		PromptQueue: []QueuedPrompt{
			{ID: "q-1", Prompt: "first", Status: QueuePromptStatusPending, CreatedAt: now},
		},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	runner := &fakeDrainRunner{
		sessionID: "acp-events",
		events: []EventLogLine{{
			Seq:       1,
			At:        now,
			Direction: EventDirectionOutbound,
			Message:   json.RawMessage(`{"jsonrpc":"2.0","id":"prompt-1","method":"session/prompt","params":{"sessionId":"acp-events"}}`),
		}},
	}

	result, err := DrainQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "drain-events", DrainOptions{
		Now: fixedClock(now.Add(time.Minute)),
		StartRunner: func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error) {
			return runner, func() error { return nil }, nil
		},
	})
	if err != nil {
		t.Fatalf("DrainQueue: %v", err)
	}
	if result.Completed != 1 {
		t.Fatalf("result = %#v, want one completed prompt", result)
	}
	events, err := store.ReadEventLog("drain-events")
	if err != nil {
		t.Fatalf("ReadEventLog: %v", err)
	}
	if len(events) != 1 || events[0].Direction != EventDirectionOutbound {
		t.Fatalf("events = %#v, want one outbound event", events)
	}
}

func TestDrainQueueEventFailureLeavesCompletionRetryable(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "sessions.json"))
	now := time.Date(2026, 7, 13, 21, 10, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID: "drain-event-retry", Status: SessionStatusQueued, CreatedAt: now, UpdatedAt: now,
		PromptQueue: []QueuedPrompt{{ID: "q-1", Prompt: "retry", Status: QueuePromptStatusPending, CreatedAt: now}},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	eventsPath := filepath.Join(dir, "events")
	if err := os.WriteFile(eventsPath, []byte("blocks event directory\n"), 0o600); err != nil {
		t.Fatalf("write event directory blocker: %v", err)
	}
	runner := &fakeDrainRunner{sessionID: "acp-event-retry", events: []EventLogLine{{
		At: now, Direction: EventDirectionOutbound,
		Message: json.RawMessage(`{"jsonrpc":"2.0","id":1,"result":{}}`),
	}}}
	drain := func() (DrainResult, error) {
		return DrainQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "drain-event-retry", DrainOptions{
			Now: fixedClock(now.Add(time.Minute)),
			StartRunner: func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error) {
				return runner, func() error { return nil }, nil
			},
		})
	}
	if _, err := drain(); err == nil {
		t.Fatal("DrainQueue with blocked event directory succeeded")
	}
	rec, err := store.Get("drain-event-retry")
	if err != nil {
		t.Fatalf("Get after failure: %v", err)
	}
	if rec.PromptQueue[0].Status != QueuePromptStatusRunning || len(rec.Turns) != 0 {
		t.Fatalf("completion committed without events: %#v", rec)
	}
	if err := os.Remove(eventsPath); err != nil {
		t.Fatalf("remove event directory blocker: %v", err)
	}
	result, err := drain()
	if err != nil || result.Completed != 1 {
		t.Fatalf("DrainQueue retry = %#v, %v", result, err)
	}
	events, err := store.ReadEventLog("drain-event-retry")
	if err != nil || len(events) != 1 {
		t.Fatalf("events after retry = %#v, %v", events, err)
	}
	if len(runner.prompts) != 2 {
		t.Fatalf("runner prompts = %#v, want retry after persistence failure", runner.prompts)
	}
}

func TestDrainQueueStopsOnFirstFailureAndClearsOwner(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 22, 10, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID:        "drain-failure",
		Status:    SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		PromptQueue: []QueuedPrompt{
			{ID: "q-1", Prompt: "bad", Status: QueuePromptStatusPending, CreatedAt: now},
			{ID: "q-2", Prompt: "later", Status: QueuePromptStatusPending, CreatedAt: now.Add(time.Second)},
		},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	runner := &fakeDrainRunner{sessionID: "acp-created", failPrompt: "bad"}

	result, err := DrainQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "drain-failure", DrainOptions{
		Now: fixedClock(now.Add(time.Minute)),
		StartRunner: func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error) {
			return runner, func() error { return nil }, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("DrainQueue error = %v, want prompt failure", err)
	}
	if result.Failed != 1 || result.Processed != 1 {
		t.Fatalf("result = %#v, want one failed prompt", result)
	}
	got, err := store.Get("drain-failure")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PromptQueue[0].Status != QueuePromptStatusFailed || got.PromptQueue[1].Status != QueuePromptStatusPending {
		t.Fatalf("queue statuses = %#v", got.PromptQueue)
	}
	if _, err := store.Owner("drain-failure"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Owner after failure = %v, want missing", err)
	}
}

func TestDrainQueueRecoversRunningItemAfterAcquiringOwner(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 22, 12, 0, 0, time.UTC)
	started := now.Add(time.Second)
	if err := store.Upsert(SessionRecord{
		ID:        "drain-recover-owned",
		Status:    SessionStatusRunning,
		CreatedAt: now,
		UpdatedAt: started,
		PromptQueue: []QueuedPrompt{{
			ID:        "q-1",
			Prompt:    "recover me",
			Status:    QueuePromptStatusRunning,
			CreatedAt: now,
			StartedAt: &started,
		}},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	runner := &fakeDrainRunner{sessionID: "acp-created"}

	result, err := DrainQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "drain-recover-owned", DrainOptions{
		Now: fixedClock(now.Add(time.Minute)),
		StartRunner: func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error) {
			return runner, func() error { return nil }, nil
		},
	})
	if err != nil {
		t.Fatalf("DrainQueue: %v", err)
	}
	if result.Completed != 1 || strings.Join(runner.prompts, ",") != "recover me" {
		t.Fatalf("result/prompts = %#v/%#v, want recovered prompt processed", result, runner.prompts)
	}
	got, err := store.Get("drain-recover-owned")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PromptQueue[0].Status != QueuePromptStatusCompleted || got.PromptQueue[0].StartedAt == nil {
		t.Fatalf("recovered prompt = %#v, want completed", got.PromptQueue[0])
	}
}

func TestDrainQueueCancelRequestCancelsPendingBeforePrompt(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 22, 15, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID:        "drain-canceled",
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
	if err := store.RequestCancel("drain-canceled", now.Add(30*time.Second)); err != nil {
		t.Fatalf("RequestCancel: %v", err)
	}

	result, err := DrainQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "drain-canceled", DrainOptions{
		Now: fixedClock(now.Add(time.Minute)),
		StartRunner: func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error) {
			t.Fatal("StartRunner called for a fully canceled queue")
			return nil, nil, nil
		},
	})
	if err != nil {
		t.Fatalf("DrainQueue: %v", err)
	}
	if result.Canceled != 2 || result.Processed != 0 {
		t.Fatalf("result = %#v, want two pending cancellations", result)
	}
	got, err := store.Get("drain-canceled")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PromptQueue[0].Status != QueuePromptStatusCanceled || got.PromptQueue[1].Status != QueuePromptStatusCanceled {
		t.Fatalf("queue statuses = %#v", got.PromptQueue)
	}
}

func TestDrainQueuePassesCancelRequestsToRunner(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 1, 22, 20, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID:        "drain-active-cancel",
		Status:    SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		PromptQueue: []QueuedPrompt{{
			ID: "q-1", Prompt: "first", Status: QueuePromptStatusPending, CreatedAt: now,
		}},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	runner := &fakeDrainRunner{sessionID: "acp-created"}

	_, err := DrainQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "drain-active-cancel", DrainOptions{
		Now: fixedClock(now.Add(time.Minute)),
		StartRunner: func(_ context.Context, _ AgentSpec, opts RunOptions, _ string) (DrainPromptRunner, func() error, error) {
			if err := store.RequestCancel("drain-active-cancel", now.Add(90*time.Second)); err != nil {
				t.Fatalf("RequestCancel: %v", err)
			}
			if opts.CancelRequested == nil || !opts.CancelRequested("acp-created") {
				t.Fatal("CancelRequested did not observe store cancel request")
			}
			return runner, func() error { return nil }, nil
		},
	})
	if err != nil {
		t.Fatalf("DrainQueue: %v", err)
	}
}

type fakeDrainRunner struct {
	sessionID  acpsdk.SessionId
	failPrompt string
	prompts    []string
	events     []EventLogLine
	closed     bool
}

func (r *fakeDrainRunner) SessionID() acpsdk.SessionId {
	return r.sessionID
}

func (r *fakeDrainRunner) Prompt(_ context.Context, prompt string) (Result, error) {
	r.prompts = append(r.prompts, prompt)
	if prompt == r.failPrompt {
		return Result{}, errors.New("fake prompt failed: " + prompt)
	}
	return Result{
		SessionID:  r.sessionID,
		StopReason: acpsdk.StopReasonEndTurn,
		Text:       "response: " + prompt,
		Events:     cloneEvents(r.events),
	}, nil
}

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}
