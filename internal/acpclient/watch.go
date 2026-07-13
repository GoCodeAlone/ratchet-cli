package acpclient

import (
	"context"
	"errors"
	"strings"
	"time"
)

const defaultWatchInterval = 5 * time.Second

type WatchOptions struct {
	Interval      time.Duration
	MaxPerCycle   int
	MaxCycles     int
	StopWhenEmpty bool
	Now           func() time.Time
	Sleep         func(context.Context, time.Duration) error
	StartRunner   func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error)
}

type WatchCycle struct {
	SessionID     string
	ACPSessionID  string
	Cycle         int
	PendingBefore int
	Processed     int
	Completed     int
	Failed        int
	Canceled      int
	Remaining     int
	Idle          bool
	StartedAt     time.Time
	CompletedAt   time.Time
}

type WatchResult struct {
	SessionID    string
	ACPSessionID string
	Cycles       int
	Processed    int
	Completed    int
	Failed       int
	Canceled     int
	Remaining    int
}

func WatchQueue(ctx context.Context, store *Store, spec AgentSpec, opts RunOptions, sessionID string, watchOpts WatchOptions, onCycle func(WatchCycle)) (result WatchResult, err error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return WatchResult{}, errors.New("acp client session id is required")
	}
	if store == nil {
		return WatchResult{}, errors.New("acp client store is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	lease, err := acquireDrainOwnerLease(store, spec, sessionID, watchOpts.now())
	if err != nil {
		return WatchResult{}, err
	}
	defer func() { err = errors.Join(err, lease.Release()) }()

	interval := watchOpts.Interval
	if interval <= 0 {
		interval = defaultWatchInterval
	}
	maxPerCycle := watchOpts.MaxPerCycle
	if maxPerCycle <= 0 {
		maxPerCycle = 1
	}
	sleep := watchOpts.Sleep
	if sleep == nil {
		sleep = defaultWatchSleep
	}

	result = WatchResult{SessionID: sessionID}
	for {
		if err := ctx.Err(); err != nil {
			result.Remaining = countPendingQueue(store, sessionID)
			return result, err
		}

		cycleNumber := result.Cycles + 1
		startedAt := watchOpts.now()
		queueWork, err := watchQueueWork(store, sessionID)
		if err != nil {
			result.Remaining = countPendingQueue(store, sessionID)
			return result, err
		}

		cycle := WatchCycle{
			SessionID:     sessionID,
			Cycle:         cycleNumber,
			PendingBefore: queueWork,
			StartedAt:     startedAt,
		}
		if queueWork == 0 {
			cycle.Idle = true
			cycle.Remaining = 0
			cycle.CompletedAt = watchOpts.now()
			result.Cycles = cycleNumber
			result.Remaining = 0
			if onCycle != nil {
				onCycle(cycle)
			}
			if watchOpts.StopWhenEmpty || reachedMaxWatchCycles(result.Cycles, watchOpts.MaxCycles) {
				return result, nil
			}
			if err := sleep(ctx, interval); err != nil {
				return result, err
			}
			continue
		}

		drainResult, err := drainQueueOwned(ctx, store, spec, opts, sessionID, DrainOptions{
			Max:         maxPerCycle,
			Now:         watchOpts.Now,
			StartRunner: watchOpts.StartRunner,
		})
		cycle.ACPSessionID = drainResult.ACPSessionID
		cycle.Processed = drainResult.Processed
		cycle.Completed = drainResult.Completed
		cycle.Failed = drainResult.Failed
		cycle.Canceled = drainResult.Canceled
		cycle.Remaining = drainResult.Remaining
		cycle.CompletedAt = watchOpts.now()

		result.Cycles = cycleNumber
		if drainResult.ACPSessionID != "" {
			result.ACPSessionID = drainResult.ACPSessionID
		}
		result.Processed += drainResult.Processed
		result.Completed += drainResult.Completed
		result.Failed += drainResult.Failed
		result.Canceled += drainResult.Canceled
		result.Remaining = drainResult.Remaining
		if onCycle != nil {
			onCycle(cycle)
		}
		if err != nil {
			return result, err
		}
		if reachedMaxWatchCycles(result.Cycles, watchOpts.MaxCycles) {
			return result, nil
		}
		if err := sleep(ctx, interval); err != nil {
			return result, err
		}
	}
}

func watchQueueWork(store *Store, sessionID string) (int, error) {
	rec, err := store.Get(sessionID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, prompt := range rec.PromptQueue {
		if prompt.Status == QueuePromptStatusPending || prompt.Status == QueuePromptStatusRunning {
			count++
		}
	}
	return count, nil
}

func reachedMaxWatchCycles(cycles, maxCycles int) bool {
	return maxCycles > 0 && cycles >= maxCycles
}

func (opts WatchOptions) now() time.Time {
	if opts.Now != nil {
		return opts.Now().UTC()
	}
	return time.Now().UTC()
}

func defaultWatchSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
