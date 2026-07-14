package acpclient

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWatchQueueDrainsPendingPrompts(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID:        "watch-drain",
		Status:    SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		PromptQueue: []QueuedPrompt{
			{ID: "q-1", Prompt: "first secret prompt", Status: QueuePromptStatusPending, CreatedAt: now},
			{ID: "q-2", Prompt: "second secret prompt", Status: QueuePromptStatusPending, CreatedAt: now.Add(time.Second)},
		},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	runner := &fakeDrainRunner{sessionID: "acp-watch"}
	var cycles []WatchCycle

	result, err := WatchQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "watch-drain", WatchOptions{
		Interval:      time.Millisecond,
		MaxPerCycle:   1,
		StopWhenEmpty: true,
		Now:           fixedClock(now.Add(time.Minute)),
		Sleep:         instantWatchSleep,
		StartRunner: func(_ context.Context, _ AgentSpec, _ RunOptions, existingID string) (DrainPromptRunner, func() error, error) {
			if existingID != "" && existingID != "acp-watch" {
				t.Fatalf("existingID = %q, want empty or acp-watch", existingID)
			}
			return runner, func() error { return nil }, nil
		},
	}, func(cycle WatchCycle) {
		cycles = append(cycles, cycle)
	})
	if err != nil {
		t.Fatalf("WatchQueue: %v", err)
	}
	if result.Completed != 2 || result.Processed != 2 || result.Remaining != 0 {
		t.Fatalf("result = %#v, want two completed and no remaining", result)
	}
	if len(cycles) != 3 {
		t.Fatalf("cycles = %#v, want two drain cycles plus one idle cycle", cycles)
	}
	if !cycles[2].Idle || cycles[2].PendingBefore != 0 {
		t.Fatalf("last cycle = %#v, want idle empty cycle", cycles[2])
	}
	if got := fmt.Sprintf("%#v", cycles); strings.Contains(got, "secret prompt") {
		t.Fatalf("cycle summaries leaked prompt text: %s", got)
	}
	rec, err := store.Get("watch-drain")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.PromptQueue[0].Status != QueuePromptStatusCompleted || rec.PromptQueue[1].Status != QueuePromptStatusCompleted {
		t.Fatalf("queue statuses = %#v", rec.PromptQueue)
	}
}

func TestWatchQueueStopWhenEmptyDoesNotStartAgent(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 2, 12, 5, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID:        "watch-empty",
		Status:    SessionStatusCompleted,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	var cycles []WatchCycle

	result, err := WatchQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "watch-empty", WatchOptions{
		Interval:      time.Millisecond,
		StopWhenEmpty: true,
		Now:           fixedClock(now.Add(time.Minute)),
		Sleep:         instantWatchSleep,
		StartRunner: func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error) {
			t.Fatal("StartRunner called for empty queue")
			return nil, nil, nil
		},
	}, func(cycle WatchCycle) {
		cycles = append(cycles, cycle)
	})
	if err != nil {
		t.Fatalf("WatchQueue: %v", err)
	}
	if result.Processed != 0 || result.Remaining != 0 {
		t.Fatalf("result = %#v, want idle result", result)
	}
	if len(cycles) != 1 || !cycles[0].Idle {
		t.Fatalf("cycles = %#v, want one idle cycle", cycles)
	}
}

func TestWatchQueueRecoversStaleRunningPrompt(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 2, 12, 7, 0, 0, time.UTC)
	started := now.Add(time.Second)
	if err := store.Upsert(SessionRecord{
		ID:        "watch-stale-running",
		Status:    SessionStatusRunning,
		CreatedAt: now,
		UpdatedAt: started,
		PromptQueue: []QueuedPrompt{{
			ID:        "q-1",
			Prompt:    "recover stale prompt",
			Status:    QueuePromptStatusRunning,
			CreatedAt: now,
			StartedAt: &started,
		}},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	runner := &fakeDrainRunner{sessionID: "acp-watch-stale"}

	result, err := WatchQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "watch-stale-running", WatchOptions{
		Interval:      time.Millisecond,
		MaxPerCycle:   1,
		StopWhenEmpty: true,
		Now:           fixedClock(now.Add(time.Minute)),
		Sleep:         instantWatchSleep,
		StartRunner: func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error) {
			return runner, func() error { return nil }, nil
		},
	}, nil)
	if err != nil {
		t.Fatalf("WatchQueue: %v", err)
	}
	if result.Completed != 1 || strings.Join(runner.prompts, ",") != "recover stale prompt" {
		t.Fatalf("result/prompts = %#v/%#v, want recovered prompt processed", result, runner.prompts)
	}
	rec, err := store.Get("watch-stale-running")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.PromptQueue[0].Status != QueuePromptStatusCompleted {
		t.Fatalf("queue status = %q, want completed", rec.PromptQueue[0].Status)
	}
}

func TestWatchQueueStopsAtMaxCycles(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 2, 12, 10, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID:        "watch-max-cycles",
		Status:    SessionStatusCompleted,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	var slept int
	var cycles []WatchCycle

	result, err := WatchQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "watch-max-cycles", WatchOptions{
		Interval:  time.Second,
		MaxCycles: 2,
		Now:       fixedClock(now.Add(time.Minute)),
		Sleep: func(ctx context.Context, d time.Duration) error {
			if d != time.Second {
				t.Fatalf("sleep duration = %s, want 1s", d)
			}
			slept++
			return nil
		},
	}, func(cycle WatchCycle) {
		cycles = append(cycles, cycle)
	})
	if err != nil {
		t.Fatalf("WatchQueue: %v", err)
	}
	if result.Cycles != 2 || len(cycles) != 2 || slept != 1 {
		t.Fatalf("result/cycles/slept = %#v/%#v/%d, want two idle cycles and one sleep", result, cycles, slept)
	}
}

func TestWatchQueueReturnsDrainBusy(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 2, 12, 15, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID:        "watch-busy",
		Status:    SessionStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
		PromptQueue: []QueuedPrompt{
			{ID: "q-1", Prompt: "first", Status: QueuePromptStatusPending, CreatedAt: now},
		},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	lease, err := store.AcquireOwnerLease(OwnerLock{
		SessionID:          "watch-busy",
		PID:                os.Getpid(),
		CommandFingerprint: "other",
		StartedAt:          now,
	})
	if err != nil {
		t.Fatalf("AcquireOwnerLease: %v", err)
	}
	defer func() { _ = lease.Release() }()

	_, err = WatchQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "watch-busy", WatchOptions{
		Interval:    time.Millisecond,
		MaxPerCycle: 1,
		Now:         fixedClock(now.Add(time.Minute)),
		Sleep:       instantWatchSleep,
	}, nil)
	if !errors.Is(err, ErrDrainBusy) {
		t.Fatalf("WatchQueue error = %v, want ErrDrainBusy", err)
	}
}

func TestWatchQueueHoldsOwnerLeaseAcrossIdleSleep(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 13, 20, 5, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{ID: "watch-owned", Status: SessionStatusQueued, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	var claimErr error
	result, err := WatchQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "watch-owned", WatchOptions{
		Interval:  time.Millisecond,
		MaxCycles: 2,
		Now:       fixedClock(now),
		Sleep: func(context.Context, time.Duration) error {
			other, err := store.AcquireOwnerLease(OwnerLock{SessionID: "watch-owned", PID: os.Getpid(), StartedAt: now.Add(time.Second)})
			if other != nil {
				_ = other.Release()
			}
			claimErr = err
			return context.Canceled
		},
	}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WatchQueue error = %v, want context.Canceled", err)
	}
	if !errors.Is(claimErr, ErrOwnerLeaseBusy) {
		t.Fatalf("idle watcher owner claim = %v, want ErrOwnerLeaseBusy", claimErr)
	}
	if result.Cycles != 1 {
		t.Fatalf("cycles = %d, want 1 before canceled sleep", result.Cycles)
	}
	if _, err := store.Owner("watch-owned"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Owner after WatchQueue return = %v, want os.ErrNotExist", err)
	}
}

func TestWatchQueueStopsWhenRunnerCloseFails(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 13, 21, 5, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{
		ID: "watch-close-failure", Status: SessionStatusQueued, CreatedAt: now, UpdatedAt: now,
		PromptQueue: []QueuedPrompt{
			{ID: "q-1", Prompt: "first", Status: QueuePromptStatusPending, CreatedAt: now},
			{ID: "q-2", Prompt: "second", Status: QueuePromptStatusPending, CreatedAt: now.Add(time.Second)},
		},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	closeErr := errors.New("runner close failed")
	starts := 0
	result, err := WatchQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "watch-close-failure", WatchOptions{
		Interval: time.Millisecond, MaxPerCycle: 1, MaxCycles: 2, Now: fixedClock(now.Add(time.Minute)), Sleep: instantWatchSleep,
		StartRunner: func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error) {
			starts++
			return &fakeDrainRunner{sessionID: "acp-close-failure"}, func() error { return closeErr }, nil
		},
	}, nil)
	if !errors.Is(err, closeErr) {
		t.Fatalf("WatchQueue error = %v, want close failure", err)
	}
	if starts != 1 || result.Completed != 1 {
		t.Fatalf("starts/result = %d/%#v, want one completed cycle", starts, result)
	}
	rec, err := store.Get("watch-close-failure")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.PromptQueue[0].Status != QueuePromptStatusCompleted || rec.PromptQueue[1].Status != QueuePromptStatusPending {
		t.Fatalf("queue after close failure = %#v", rec.PromptQueue)
	}
}

func TestWatchQueueReconcilesCancellationProjectionBeforeWork(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
		now := time.Date(2026, 7, 13, 15, 20, 0, 0, time.UTC)
		if err := store.Upsert(SessionRecord{
			ID: "watch-missing-projection", Status: SessionStatusCancelRequested, CreatedAt: now, UpdatedAt: now,
			PromptQueue: []QueuedPrompt{{ID: "q-1", Prompt: "cancel", Status: QueuePromptStatusPending, CreatedAt: now}},
		}); err != nil {
			t.Fatalf("Upsert: %v", err)
		}

		if _, err := WatchQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "watch-missing-projection", WatchOptions{
			StopWhenEmpty: true,
			Sleep:         instantWatchSleep,
			StartRunner: func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error) {
				t.Fatal("StartRunner called for canceled session")
				return nil, nil, nil
			},
		}, nil); err != nil {
			t.Fatalf("WatchQueue: %v", err)
		}
		if _, err := store.CancelRequest("watch-missing-projection"); err != nil {
			t.Fatalf("CancelRequest projection: %v", err)
		}
	})

	t.Run("stale", func(t *testing.T) {
		store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
		now := time.Date(2026, 7, 13, 15, 21, 0, 0, time.UTC)
		if _, err := store.AppendQueuedPrompt(SessionRecord{ID: "watch-stale-projection"}, QueuedPrompt{ID: "q-1", Prompt: "run", CreatedAt: now}); err != nil {
			t.Fatalf("AppendQueuedPrompt: %v", err)
		}
		if err := backgroundWriteJSONAtomic(store.cancelPath("watch-stale-projection"), CancelRequest{SessionID: "watch-stale-projection", RequestedAt: now}); err != nil {
			t.Fatalf("write stale projection: %v", err)
		}

		if _, err := WatchQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "watch-stale-projection", WatchOptions{
			StopWhenEmpty: true,
			Sleep:         instantWatchSleep,
			StartRunner: func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error) {
				if _, err := store.CancelRequest("watch-stale-projection"); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("stale projection still exists at StartRunner: %v", err)
				}
				return &fakeDrainRunner{sessionID: "acp-watch-reconciled"}, func() error { return nil }, nil
			},
		}, nil); err != nil {
			t.Fatalf("WatchQueue: %v", err)
		}
	})
}

func TestWatchQueueProjectionRepairFailurePreventsOwnershipAndLaunch(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "sessions.json"))
	now := time.Date(2026, 7, 13, 15, 25, 0, 0, time.UTC)
	if err := store.Upsert(SessionRecord{ID: "watch-repair-failure", Status: SessionStatusCancelRequested, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	repairErr := errors.New("watch projection repair failed")
	store.cancelWriter = func(string, CancelRequest) error { return repairErr }
	starts := 0

	_, err := WatchQueue(t.Context(), store, AgentSpec{Name: "fixture", Command: "fixture"}, RunOptions{}, "watch-repair-failure", WatchOptions{
		StopWhenEmpty: true,
		StartRunner: func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error) {
			starts++
			return nil, nil, nil
		},
	}, nil)
	if !errors.Is(err, ErrStoreCommitUnconfirmed) || !errors.Is(err, repairErr) {
		t.Fatalf("WatchQueue error = %v, want unconfirmed repair cause", err)
	}
	if starts != 0 {
		t.Fatalf("StartRunner calls = %d, want 0", starts)
	}
	if _, ownerErr := store.Owner("watch-repair-failure"); !errors.Is(ownerErr, os.ErrNotExist) {
		t.Fatalf("Owner after repair failure = %v, want os.ErrNotExist", ownerErr)
	}
}

func instantWatchSleep(context.Context, time.Duration) error {
	return nil
}
