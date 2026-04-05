package daemon

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	"github.com/GoCodeAlone/ratchet-cli/internal/hooks"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/workflow-plugin-agent/executor"
	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

// fleetInstance tracks a running fleet.
type fleetInstance struct {
	mu          sync.RWMutex
	status      *pb.FleetStatus
	cancelFns   map[string]context.CancelFunc
	completedAt time.Time
}

// FleetManager manages fleet instances.
type FleetManager struct {
	mu      sync.RWMutex
	fleets  map[string]*fleetInstance
	routing config.ModelRouting
	engine  *EngineContext
	hooks   *hooks.HookConfig
	stop    chan struct{}
}

// NewFleetManager returns an initialized FleetManager with optional model routing config.
func NewFleetManager(routing config.ModelRouting, engine *EngineContext, hks *hooks.HookConfig) *FleetManager {
	fm := &FleetManager{
		fleets:  make(map[string]*fleetInstance),
		routing: routing,
		engine:  engine,
		hooks:   hks,
		stop:    make(chan struct{}),
	}
	go fm.cleanupLoop()
	return fm
}

// cleanupLoop periodically removes fleets that completed more than 10 minutes ago.
func (fm *FleetManager) cleanupLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("fleetManager cleanupLoop: panic: %v", r)
		}
	}()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-fm.stop:
			return
		case <-ticker.C:
			fm.purgeCompleted(10 * time.Minute)
		}
	}
}

// purgeCompleted removes fleets that completed more than ttl ago.
func (fm *FleetManager) purgeCompleted(ttl time.Duration) {
	now := time.Now()
	fm.mu.Lock()
	defer fm.mu.Unlock()
	for id, fi := range fm.fleets {
		fi.mu.RLock()
		completed := fi.completedAt
		fi.mu.RUnlock()
		if !completed.IsZero() && now.Sub(completed) > ttl {
			delete(fm.fleets, id)
		}
	}
}

// StartFleet spawns worker goroutines for each step in the plan (up to maxWorkers).
// It sends FleetStatus updates on the returned channel until all workers finish.
func (fm *FleetManager) StartFleet(ctx context.Context, req *pb.StartFleetReq, steps []string, eventCh chan<- *pb.FleetStatus) string {
	fleetID := uuid.New().String()

	if len(steps) == 0 {
		steps = []string{"step-1"} // default single step when no plan steps given
	}

	maxWorkers := int(req.MaxWorkers)
	if maxWorkers <= 0 || maxWorkers > len(steps) {
		maxWorkers = len(steps)
	}

	workers := make([]*pb.FleetWorker, len(steps))
	for i, stepID := range steps {
		workers[i] = &pb.FleetWorker{
			Id:     uuid.New().String(),
			Name:   fmt.Sprintf("worker-%d", i+1),
			StepId: stepID,
			Status: "pending",
			Model:  ModelForStep(stepID, fm.routing),
		}
	}

	fi := &fleetInstance{
		status: &pb.FleetStatus{
			FleetId:   fleetID,
			SessionId: req.SessionId,
			Workers:   workers,
			Status:    "running",
			Total:     int32(len(workers)),
		},
		cancelFns: make(map[string]context.CancelFunc),
	}

	fm.mu.Lock()
	fm.fleets[fleetID] = fi
	fm.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("fleet %s: panic: %v", fleetID, r)
			}
		}()
		if fm.hooks != nil {
			_ = fm.hooks.Run(hooks.PreFleet, map[string]string{"fleet_id": fleetID})
		}
		fm.runFleet(ctx, fi, maxWorkers, eventCh)
		if fm.hooks != nil {
			_ = fm.hooks.Run(hooks.PostFleet, map[string]string{"fleet_id": fleetID})
		}
	}()

	return fleetID
}

// runFleet executes workers with concurrency cap and sends status updates.
func (fm *FleetManager) runFleet(ctx context.Context, fi *fleetInstance, maxWorkers int, eventCh chan<- *pb.FleetStatus) {
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for _, w := range fi.status.Workers {
		w := w // capture loop var
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("fleet worker %s: panic: %v", w.Id, r)
				}
				<-sem
				wg.Done()
			}()

			workerCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			fi.mu.Lock()
			fi.cancelFns[w.Id] = cancel
			w.Status = "running"
			fi.mu.Unlock()

			sendFleetStatus(eventCh, fi)

			// Simulate work — in production this would delegate to an agent/session
			err := fm.executeWorker(workerCtx, w)

			fi.mu.Lock()
			delete(fi.cancelFns, w.Id)
			if err != nil {
				w.Status = "failed"
				w.Error = err.Error()
			} else {
				w.Status = "completed"
			}
			fi.status.Completed++
			fi.mu.Unlock()

			sendFleetStatus(eventCh, fi)
		}()
	}

	wg.Wait()

	fi.mu.Lock()
	fi.status.Status = "completed"
	fi.completedAt = time.Now()
	fi.mu.Unlock()

	sendFleetStatus(eventCh, fi)
	close(eventCh)
}

// secretGuardAdapter adapts *orchestrator.SecretGuard to executor.SecretRedactor.
// SecretGuard.CheckAndRedact returns bool but the interface requires no return.
type secretGuardAdapter struct {
	guard interface {
		Redact(string) string
		CheckAndRedact(msg *provider.Message) bool
	}
}

func (a *secretGuardAdapter) Redact(text string) string {
	return a.guard.Redact(text)
}

func (a *secretGuardAdapter) CheckAndRedact(msg *provider.Message) {
	a.guard.CheckAndRedact(msg)
}

// executeWorker runs a single fleet worker step using the real executor.
func (fm *FleetManager) executeWorker(ctx context.Context, w *pb.FleetWorker) error {
	if fm.engine == nil || fm.engine.ProviderRegistry == nil {
		return fmt.Errorf("fleet worker %s: no engine or provider registry configured", w.Id)
	}

	var prov provider.Provider
	var err error
	if w.Provider != "" {
		prov, err = fm.engine.ProviderRegistry.GetByAlias(ctx, w.Provider)
	} else {
		prov, err = fm.engine.ProviderRegistry.GetDefault(ctx)
	}
	if err != nil {
		return fmt.Errorf("fleet worker %s: resolve provider: %w", w.Id, err)
	}

	var redactor executor.SecretRedactor
	if fm.engine.SecretGuard != nil {
		redactor = &secretGuardAdapter{guard: fm.engine.SecretGuard}
	}

	cfg := executor.Config{
		Provider:       prov,
		MaxIterations:  25,
		SecretRedactor: redactor,
	}

	systemPrompt := fmt.Sprintf(
		"You are fleet worker %s. Execute the following task step thoroughly and report results.",
		w.Name,
	)

	result, err := executor.Execute(ctx, cfg, systemPrompt, w.StepId, w.Id)
	if err != nil {
		return fmt.Errorf("fleet worker %s: execute: %w", w.Id, err)
	}
	if result != nil && result.Status == "failed" {
		return fmt.Errorf("fleet worker %s: %s", w.Id, result.Error)
	}
	return nil
}

// GetStatus returns the current FleetStatus for the given fleet ID.
func (fm *FleetManager) GetStatus(fleetID string) (*pb.FleetStatus, error) {
	fm.mu.RLock()
	fi, ok := fm.fleets[fleetID]
	fm.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("fleet %s not found", fleetID)
	}

	fi.mu.RLock()
	defer fi.mu.RUnlock()
	return deepCopyFleetStatus(fi.status), nil
}

// KillWorker cancels a specific worker within a fleet.
func (fm *FleetManager) KillWorker(fleetID, workerID string) error {
	fm.mu.RLock()
	fi, ok := fm.fleets[fleetID]
	fm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("fleet %s not found", fleetID)
	}

	fi.mu.Lock()
	cancel, ok := fi.cancelFns[workerID]
	fi.mu.Unlock()
	if !ok {
		return fmt.Errorf("worker %s not found or already finished", workerID)
	}
	cancel()
	return nil
}

// deepCopyFleetStatus returns a new FleetStatus with deep-copied Workers so
// the returned value shares no pointers with the live fleet goroutines.
func deepCopyFleetStatus(src *pb.FleetStatus) *pb.FleetStatus {
	dst := &pb.FleetStatus{
		FleetId:   src.FleetId,
		SessionId: src.SessionId,
		Status:    src.Status,
		Completed: src.Completed,
		Total:     src.Total,
		Workers:   make([]*pb.FleetWorker, len(src.Workers)),
	}
	for i, w := range src.Workers {
		dst.Workers[i] = &pb.FleetWorker{
			Id:       w.Id,
			Name:     w.Name,
			StepId:   w.StepId,
			Status:   w.Status,
			Model:    w.Model,
			Provider: w.Provider,
			Error:    w.Error,
		}
	}
	return dst
}

func sendFleetStatus(ch chan<- *pb.FleetStatus, fi *fleetInstance) {
	if ch == nil {
		return
	}
	fi.mu.RLock()
	s := deepCopyFleetStatus(fi.status)
	fi.mu.RUnlock()
	select {
	case ch <- s:
	default:
	}
}
