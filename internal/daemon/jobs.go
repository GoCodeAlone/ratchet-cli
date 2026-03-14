package daemon

import (
	"context"
	"fmt"
	"strings"
	"time"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// JobProvider is implemented by each manager that owns trackable jobs.
type JobProvider interface {
	ActiveJobs() []*pb.Job
	PauseJob(id string) error
	KillJob(id string) error
}

// JobRegistry aggregates jobs from all registered providers.
type JobRegistry struct {
	providers map[string]JobProvider // keyed by job type prefix
}

// NewJobRegistry returns an empty registry. Register providers after creation.
func NewJobRegistry() *JobRegistry {
	return &JobRegistry{providers: make(map[string]JobProvider)}
}

// Register adds a provider under the given type key (e.g. "session", "cron").
func (jr *JobRegistry) Register(jobType string, p JobProvider) {
	jr.providers[jobType] = p
}

// ListJobs returns jobs aggregated from all providers.
func (jr *JobRegistry) ListJobs() []*pb.Job {
	var jobs []*pb.Job
	for _, p := range jr.providers {
		jobs = append(jobs, p.ActiveJobs()...)
	}
	return jobs
}

// PauseJob routes to the correct provider by job type prefix (e.g. "session:id").
func (jr *JobRegistry) PauseJob(id string) error {
	p, err := jr.providerFor(id)
	if err != nil {
		return err
	}
	return p.PauseJob(id)
}

// KillJob routes to the correct provider by job type prefix.
func (jr *JobRegistry) KillJob(id string) error {
	p, err := jr.providerFor(id)
	if err != nil {
		return err
	}
	return p.KillJob(id)
}

// ResumeJob is a best-effort resume — not all providers support pause/resume.
func (jr *JobRegistry) ResumeJob(id string) error {
	// Only CronProvider exposes Resume; others just re-use KillJob → re-spawn (not needed here).
	// For now delegate to the provider's KillJob as a no-op for non-pausable types.
	_, err := jr.providerFor(id)
	return err
}

func (jr *JobRegistry) providerFor(id string) (JobProvider, error) {
	for prefix, p := range jr.providers {
		if strings.HasPrefix(id, prefix+":") {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no provider for job %q", id)
}

// ---------------------------------------------------------------------------
// SessionJobProvider
// ---------------------------------------------------------------------------

// SessionJobProvider wraps SessionManager to expose session jobs.
type SessionJobProvider struct {
	sm *SessionManager
}

func NewSessionJobProvider(sm *SessionManager) *SessionJobProvider {
	return &SessionJobProvider{sm: sm}
}

func (p *SessionJobProvider) ActiveJobs() []*pb.Job {
	sessions, err := p.sm.List(context.Background())
	if err != nil {
		return nil
	}
	var jobs []*pb.Job
	for _, s := range sessions {
		if s.Status != "active" {
			continue
		}
		jobs = append(jobs, &pb.Job{
			Id:        "session:" + s.ID,
			Type:      "session",
			Name:      s.Name,
			Status:    s.Status,
			SessionId: s.ID,
			StartedAt: s.CreatedAt.Format(time.RFC3339),
			Elapsed:   time.Since(s.CreatedAt).Round(time.Second).String(),
			Metadata:  map[string]string{"working_dir": s.WorkingDir, "model": s.Model},
		})
	}
	return jobs
}

func (p *SessionJobProvider) PauseJob(id string) error {
	return fmt.Errorf("session jobs cannot be paused")
}

func (p *SessionJobProvider) KillJob(id string) error {
	sessionID := strings.TrimPrefix(id, "session:")
	return p.sm.Kill(context.Background(), sessionID)
}

// ---------------------------------------------------------------------------
// FleetJobProvider
// ---------------------------------------------------------------------------

// FleetJobProvider wraps FleetManager to expose fleet worker jobs.
type FleetJobProvider struct {
	fm *FleetManager
}

func NewFleetJobProvider(fm *FleetManager) *FleetJobProvider {
	return &FleetJobProvider{fm: fm}
}

func (p *FleetJobProvider) ActiveJobs() []*pb.Job {
	p.fm.mu.RLock()
	defer p.fm.mu.RUnlock()

	var jobs []*pb.Job
	for fleetID, fi := range p.fm.fleets {
		fi.mu.RLock()
		for _, w := range fi.status.Workers {
			if w.Status != "running" && w.Status != "pending" {
				continue
			}
			jobs = append(jobs, &pb.Job{
				Id:        "fleet_worker:" + w.Id,
				Type:      "fleet_worker",
				Name:      w.Name,
				Status:    w.Status,
				SessionId: fi.status.SessionId,
				Metadata:  map[string]string{"fleet_id": fleetID, "step_id": w.StepId, "model": w.Model},
			})
		}
		fi.mu.RUnlock()
	}
	return jobs
}

func (p *FleetJobProvider) PauseJob(id string) error {
	return fmt.Errorf("fleet worker jobs cannot be paused")
}

func (p *FleetJobProvider) KillJob(id string) error {
	workerID := strings.TrimPrefix(id, "fleet_worker:")
	// Find which fleet this worker belongs to.
	p.fm.mu.RLock()
	defer p.fm.mu.RUnlock()
	for fleetID, fi := range p.fm.fleets {
		fi.mu.RLock()
		_, ok := fi.cancelFns[workerID]
		fi.mu.RUnlock()
		if ok {
			return p.fm.KillWorker(fleetID, workerID)
		}
	}
	return fmt.Errorf("worker %s not found", workerID)
}

// ---------------------------------------------------------------------------
// TeamJobProvider
// ---------------------------------------------------------------------------

// TeamJobProvider wraps TeamManager to expose team agent jobs.
type TeamJobProvider struct {
	tm *TeamManager
}

func NewTeamJobProvider(tm *TeamManager) *TeamJobProvider {
	return &TeamJobProvider{tm: tm}
}

func (p *TeamJobProvider) ActiveJobs() []*pb.Job {
	p.tm.mu.RLock()
	defer p.tm.mu.RUnlock()

	var jobs []*pb.Job
	for _, ti := range p.tm.teams {
		ti.mu.RLock()
		for _, a := range ti.agents {
			a.mu.RLock()
			if a.status == "running" {
				jobs = append(jobs, &pb.Job{
					Id:      "team_agent:" + a.id,
					Type:    "team_agent",
					Name:    a.name,
					Status:  a.status,
					Metadata: map[string]string{"team_id": ti.id, "role": a.role, "model": a.model},
				})
			}
			a.mu.RUnlock()
		}
		ti.mu.RUnlock()
	}
	return jobs
}

func (p *TeamJobProvider) PauseJob(id string) error {
	return fmt.Errorf("team agent jobs cannot be paused")
}

func (p *TeamJobProvider) KillJob(id string) error {
	agentID := strings.TrimPrefix(id, "team_agent:")
	p.tm.mu.RLock()
	defer p.tm.mu.RUnlock()
	for _, ti := range p.tm.teams {
		ti.mu.RLock()
		_, ok := ti.agents[agentID]
		ti.mu.RUnlock()
		if ok {
			ti.mu.Lock()
			if a, exists := ti.agents[agentID]; exists {
				a.mu.Lock()
				a.status = "failed"
				a.mu.Unlock()
			}
			ti.mu.Unlock()
			return nil
		}
	}
	return fmt.Errorf("agent %s not found", agentID)
}

// ---------------------------------------------------------------------------
// CronJobProvider
// ---------------------------------------------------------------------------

// CronJobProvider wraps CronScheduler to expose cron jobs.
type CronJobProvider struct {
	cs *CronScheduler
}

func NewCronJobProvider(cs *CronScheduler) *CronJobProvider {
	return &CronJobProvider{cs: cs}
}

func (p *CronJobProvider) ActiveJobs() []*pb.Job {
	jobs, err := p.cs.List(context.Background())
	if err != nil {
		return nil
	}
	var pbJobs []*pb.Job
	for _, j := range jobs {
		if j.Status == "stopped" {
			continue
		}
		pbJobs = append(pbJobs, &pb.Job{
			Id:      "cron:" + j.ID,
			Type:    "cron",
			Name:    j.Command,
			Status:  j.Status,
			Metadata: map[string]string{
				"schedule":  j.Schedule,
				"next_run":  j.NextRun,
				"run_count": fmt.Sprintf("%d", j.RunCount),
			},
		})
	}
	return pbJobs
}

func (p *CronJobProvider) PauseJob(id string) error {
	return p.cs.Pause(context.Background(), strings.TrimPrefix(id, "cron:"))
}

func (p *CronJobProvider) KillJob(id string) error {
	return p.cs.Stop(context.Background(), strings.TrimPrefix(id, "cron:"))
}
