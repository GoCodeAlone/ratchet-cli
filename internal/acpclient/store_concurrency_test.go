package acpclient

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

func TestSessionStoreSerializesTwoHandlePromptAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	seed := NewStore(path)
	now := time.Date(2026, 7, 13, 16, 0, 0, 0, time.UTC)
	if err := seed.Upsert(SessionRecord{ID: "session-1", Status: SessionStatusQueued, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed Upsert: %v", err)
	}

	first := NewStore(path)
	second := NewStore(path)
	firstLoaded := make(chan struct{})
	secondLoaded := make(chan struct{})
	releaseFirst := make(chan struct{})
	first.transactionLoaded = func() {
		close(firstLoaded)
		<-releaseFirst
	}
	second.transactionLoaded = func() { close(secondLoaded) }

	firstDone := make(chan error, 1)
	go func() {
		_, err := first.AppendQueuedPrompt(SessionRecord{ID: "session-1"}, QueuedPrompt{ID: "q-1", Prompt: "first", CreatedAt: now.Add(time.Second)})
		firstDone <- err
	}()
	select {
	case <-firstLoaded:
	case <-time.After(time.Second):
		t.Fatal("first transaction did not load")
	}

	secondDone := make(chan error, 1)
	go func() {
		_, err := second.AppendQueuedPrompt(SessionRecord{ID: "session-1"}, QueuedPrompt{ID: "q-2", Prompt: "second", CreatedAt: now.Add(2 * time.Second)})
		secondDone <- err
	}()
	select {
	case <-secondLoaded:
		close(releaseFirst)
		<-firstDone
		<-secondDone
		t.Fatal("second handle loaded sessions.json before the first transaction committed")
	case <-time.After(150 * time.Millisecond):
	}

	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first AppendQueuedPrompt: %v", err)
	}
	select {
	case <-secondLoaded:
	case <-time.After(time.Second):
		t.Fatal("second transaction did not proceed after first commit")
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second AppendQueuedPrompt: %v", err)
	}

	got, err := seed.Get("session-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.PromptQueue) != 2 || got.PromptQueue[0].ID != "q-1" || got.PromptQueue[1].ID != "q-2" {
		t.Fatalf("prompt queue = %#v, want q-1 then q-2", got.PromptQueue)
	}
}

func TestSessionLifecycleUpdatesPreserveQueueAndTurns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	store := NewStore(path)
	now := time.Date(2026, 7, 13, 17, 0, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID:        "session-lifecycle",
		Status:    SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		Turns: []TurnSummary{{
			Prompt:    "prior",
			Response:  "prior response",
			CreatedAt: now,
		}},
		PromptQueue: []QueuedPrompt{{
			ID:        "q-1",
			Prompt:    "queued",
			Status:    QueuePromptStatusPending,
			CreatedAt: now,
		}},
	}); err != nil {
		t.Fatalf("seed Upsert: %v", err)
	}

	startedAt := now.Add(time.Minute)
	if err := store.MarkSessionStarted(SessionRecord{
		ID:                 "session-lifecycle",
		Agent:              "fixture",
		CommandFingerprint: "fingerprint",
		Cwd:                "/tmp/project",
		Status:             SessionStatusRunning,
		CreatedAt:          startedAt,
		UpdatedAt:          startedAt,
	}); err != nil {
		t.Fatalf("MarkSessionStarted: %v", err)
	}
	completedAt := startedAt.Add(time.Minute)
	if err := store.MarkSessionCompleted(SessionRecord{
		ID:                 "session-lifecycle",
		Agent:              "fixture",
		CommandFingerprint: "fingerprint",
		Cwd:                "/tmp/project",
		Status:             SessionStatusCompleted,
		UpdatedAt:          completedAt,
		LastStopReason:     "end_turn",
		Summary:            "latest response",
	}, TurnSummary{
		Prompt:     "latest",
		Response:   "latest response",
		StopReason: "end_turn",
		CreatedAt:  completedAt,
	}); err != nil {
		t.Fatalf("MarkSessionCompleted: %v", err)
	}

	got, err := store.Get("session-lifecycle")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %s, want original %s", got.CreatedAt, now)
	}
	if len(got.PromptQueue) != 1 || got.PromptQueue[0].ID != "q-1" {
		t.Fatalf("PromptQueue = %#v, want preserved q-1", got.PromptQueue)
	}
	if len(got.Turns) != 2 || got.Turns[0].Prompt != "prior" || got.Turns[1].Prompt != "latest" {
		t.Fatalf("Turns = %#v, want prior then latest", got.Turns)
	}
}

func TestWatchQueueConcurrentSessionsPreserveCrossHandleEnqueuesExactlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	workerStore := NewStore(path)
	enqueueStore := NewStore(path)
	now := time.Date(2026, 7, 13, 16, 30, 0, 0, time.UTC)
	sessions := []string{"session-1", "session-2"}
	for _, sessionID := range sessions {
		if err := workerStore.Upsert(SessionRecord{
			ID:        sessionID,
			Agent:     "fixture",
			Status:    SessionStatusQueued,
			CreatedAt: now,
			UpdatedAt: now,
			PromptQueue: []QueuedPrompt{{
				ID:        sessionID + "-q1",
				Prompt:    sessionID + "-first",
				Status:    QueuePromptStatusPending,
				CreatedAt: now,
			}},
		}); err != nil {
			t.Fatalf("seed %s: %v", sessionID, err)
		}
	}

	entered := map[string]chan struct{}{
		"session-1": make(chan struct{}),
		"session-2": make(chan struct{}),
	}
	release := make(chan struct{})
	var startsMu sync.Mutex
	started := make(map[string]bool)
	var callsMu sync.Mutex
	calls := make(map[string]int)

	results := make(chan error, len(sessions))
	for _, sessionID := range sessions {
		go func() {
			startRunner := func(_ context.Context, _ AgentSpec, _ RunOptions, _ string) (DrainPromptRunner, func() error, error) {
				startsMu.Lock()
				firstStart := !started[sessionID]
				started[sessionID] = true
				startsMu.Unlock()
				if firstStart {
					close(entered[sessionID])
					<-release
				}
				return &concurrentStoreRunner{sessionID: acpsdk.SessionId("acp-" + sessionID), callsMu: &callsMu, calls: calls}, func() error { return nil }, nil
			}
			_, err := WatchQueue(t.Context(), workerStore, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, sessionID, WatchOptions{
				Interval:      time.Millisecond,
				MaxPerCycle:   1,
				StopWhenEmpty: true,
				Now:           func() time.Time { return now },
				Sleep:         func(context.Context, time.Duration) error { return nil },
				StartRunner:   startRunner,
			}, nil)
			results <- err
		}()
	}
	for _, sessionID := range sessions {
		select {
		case <-entered[sessionID]:
		case <-time.After(time.Second):
			t.Fatalf("%s watcher did not reach runner startup", sessionID)
		}
	}
	for _, sessionID := range sessions {
		if _, err := enqueueStore.AppendQueuedPrompt(SessionRecord{ID: sessionID}, QueuedPrompt{
			ID:        sessionID + "-q2",
			Prompt:    sessionID + "-second",
			CreatedAt: now.Add(time.Second),
		}); err != nil {
			t.Fatalf("enqueue %s: %v", sessionID, err)
		}
	}
	close(release)
	for range sessions {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("WatchQueue: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("WatchQueue did not finish")
		}
	}

	for _, sessionID := range sessions {
		rec, err := enqueueStore.Get(sessionID)
		if err != nil {
			t.Fatalf("Get %s: %v", sessionID, err)
		}
		if rec.Agent != "fixture" || rec.ACPSessionID != "acp-"+sessionID || rec.Status != SessionStatusCompleted {
			t.Fatalf("session %s metadata reverted: %#v", sessionID, rec)
		}
		if len(rec.PromptQueue) != 2 {
			t.Fatalf("session %s queue = %#v, want two prompts", sessionID, rec.PromptQueue)
		}
		for _, prompt := range rec.PromptQueue {
			if prompt.Status != QueuePromptStatusCompleted {
				t.Fatalf("session %s prompt = %#v, want completed", sessionID, prompt)
			}
		}
	}
	callsMu.Lock()
	defer callsMu.Unlock()
	for _, sessionID := range sessions {
		for _, suffix := range []string{"first", "second"} {
			prompt := fmt.Sprintf("%s-%s", sessionID, suffix)
			if calls[prompt] != 1 {
				t.Fatalf("prompt %q calls = %d, want exactly 1; all calls: %#v", prompt, calls[prompt], calls)
			}
		}
	}
}

type concurrentStoreRunner struct {
	sessionID acpsdk.SessionId
	callsMu   *sync.Mutex
	calls     map[string]int
}

func (r *concurrentStoreRunner) SessionID() acpsdk.SessionId {
	return r.sessionID
}

func (r *concurrentStoreRunner) Prompt(_ context.Context, prompt string) (Result, error) {
	r.callsMu.Lock()
	r.calls[prompt]++
	r.callsMu.Unlock()
	return Result{SessionID: r.sessionID, StopReason: acpsdk.StopReasonEndTurn, Text: "response: " + prompt}, nil
}
