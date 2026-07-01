package acpclient

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

var ErrDrainBusy = errors.New("acp client session is already being drained")

type DrainPromptRunner interface {
	SessionID() acpsdk.SessionId
	Prompt(context.Context, string) (Result, error)
}

type DrainOptions struct {
	Max         int
	Now         func() time.Time
	StartRunner func(context.Context, AgentSpec, RunOptions, string) (DrainPromptRunner, func() error, error)
}

type DrainResult struct {
	SessionID    string
	ACPSessionID string
	Processed    int
	Completed    int
	Failed       int
	Canceled     int
	Remaining    int
}

func DrainQueue(ctx context.Context, store *Store, spec AgentSpec, opts RunOptions, sessionID string, drainOpts DrainOptions) (DrainResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return DrainResult{}, errors.New("acp client session id is required")
	}
	if store == nil {
		return DrainResult{}, errors.New("acp client store is required")
	}
	now := drainOpts.now()
	if err := store.AcquireOwner(OwnerLock{
		SessionID:          sessionID,
		PID:                os.Getpid(),
		CommandFingerprint: spec.Fingerprint(),
		StartedAt:          now,
	}); err != nil {
		if errors.Is(err, os.ErrExist) {
			if _, ownerErr := store.Owner(sessionID); ownerErr != nil {
				return DrainResult{}, ownerErr
			}
			return DrainResult{}, fmt.Errorf("%w: %s", ErrDrainBusy, sessionID)
		}
		return DrainResult{}, err
	}
	if _, err := store.recoverRunningQueueItems(sessionID, now); err != nil {
		_ = store.ClearOwner(sessionID)
		return DrainResult{}, err
	}

	result := DrainResult{SessionID: sessionID}
	var runner DrainPromptRunner
	var closeRunner func() error
	defer func() {
		if closeRunner != nil {
			_ = closeRunner()
		}
		_ = store.ClearOwner(sessionID)
	}()

	max := drainOpts.Max
	for max <= 0 || result.Processed < max {
		if drainCancelRequested(store, sessionID, opts.CancelRequested, result.ACPSessionID) {
			canceled, err := store.CancelPendingQueue(sessionID, drainOpts.now())
			if err != nil {
				return result, err
			}
			result.Canceled += canceled
			break
		}
		next, ok, err := store.NextQueuedPrompt(sessionID)
		if err != nil {
			return result, err
		}
		if !ok {
			break
		}
		if runner == nil {
			rec, err := store.Get(sessionID)
			if err != nil {
				return result, err
			}
			startOpts := opts
			priorCancel := startOpts.CancelRequested
			startOpts.CancelRequested = func(acpSessionID string) bool {
				return drainCancelRequested(store, sessionID, priorCancel, acpSessionID)
			}
			start := drainOpts.StartRunner
			if start == nil {
				start = defaultDrainStartRunner
			}
			runner, closeRunner, err = start(ctx, spec, startOpts, rec.ACPSessionID)
			if err != nil {
				return result, err
			}
			result.ACPSessionID = string(runner.SessionID())
			if rec.ACPSessionID == "" && result.ACPSessionID != "" {
				rec.ACPSessionID = result.ACPSessionID
				if err := store.Upsert(rec); err != nil {
					return result, err
				}
			}
		}
		if drainCancelRequested(store, sessionID, opts.CancelRequested, result.ACPSessionID) {
			canceled, err := store.CancelPendingQueue(sessionID, drainOpts.now())
			if err != nil {
				return result, err
			}
			result.Canceled += canceled
			break
		}
		if err := store.MarkQueueRunning(sessionID, next.ID, drainOpts.now()); err != nil {
			return result, err
		}
		promptResult, err := runner.Prompt(ctx, next.Prompt)
		result.Processed++
		if err != nil {
			if markErr := store.MarkQueueFailed(sessionID, next.ID, err.Error(), drainOpts.now()); markErr != nil {
				return result, errors.Join(err, markErr)
			}
			result.Failed++
			result.Remaining = countPendingQueue(store, sessionID)
			return result, err
		}
		if result.ACPSessionID == "" {
			result.ACPSessionID = string(promptResult.SessionID)
		}
		if err := store.MarkQueueCompleted(sessionID, next.ID, promptResult.Text, string(promptResult.StopReason), drainOpts.now()); err != nil {
			return result, err
		}
		result.Completed++
	}
	result.Remaining = countPendingQueue(store, sessionID)
	return result, nil
}

func defaultDrainStartRunner(ctx context.Context, spec AgentSpec, opts RunOptions, existingID string) (DrainPromptRunner, func() error, error) {
	client, err := Start(ctx, spec, opts)
	if err != nil {
		return nil, nil, err
	}
	runner, err := client.StartSession(ctx, existingID)
	if err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	return runner, client.Close, nil
}

func (opts DrainOptions) now() time.Time {
	if opts.Now != nil {
		return opts.Now().UTC()
	}
	return time.Now().UTC()
}

func drainCancelRequested(store *Store, sessionID string, callback func(string) bool, acpSessionID string) bool {
	if callback != nil && callback(acpSessionID) {
		return true
	}
	if store == nil {
		return false
	}
	_, err := store.CancelRequest(sessionID)
	return err == nil
}

func countPendingQueue(store *Store, sessionID string) int {
	rec, err := store.Get(sessionID)
	if err != nil {
		return 0
	}
	count := 0
	for _, prompt := range rec.PromptQueue {
		if prompt.Status == QueuePromptStatusPending {
			count++
		}
	}
	return count
}
