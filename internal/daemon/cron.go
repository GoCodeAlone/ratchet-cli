package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// CronJob represents a scheduled recurring command.
type CronJob struct {
	ID        string
	SessionID string
	Schedule  string // duration ("5m") or cron expr ("*/10 * * * *")
	Command   string
	Status    string // active, paused, stopped
	LastRun   string
	NextRun   string
	RunCount  int32
}

// cronEntry is the in-memory state for a running cron job.
type cronEntry struct {
	job    CronJob
	cancel context.CancelFunc
	mu     sync.Mutex
}

// CronScheduler manages cron jobs with SQLite persistence.
type CronScheduler struct {
	db         *sql.DB
	onTick     func(sessionID, command string)
	mu         sync.Mutex
	entries    map[string]*cronEntry
	parentCtx  context.Context // propagated to goroutines spawned by Resume
}

// NewCronScheduler creates a scheduler. onTick is called each time a job fires.
func NewCronScheduler(db *sql.DB, onTick func(sessionID, command string)) *CronScheduler {
	return &CronScheduler{
		db:        db,
		onTick:    onTick,
		entries:   make(map[string]*cronEntry),
		parentCtx: context.Background(), // overridden by Start
	}
}

// Start reloads persisted active jobs and begins running them.
// The context is stored so Resume can propagate it to restarted goroutines.
func (cs *CronScheduler) Start(ctx context.Context) error {
	cs.mu.Lock()
	cs.parentCtx = ctx
	cs.mu.Unlock()
	rows, err := cs.db.QueryContext(ctx,
		`SELECT id, session_id, schedule, command, status, COALESCE(last_run,''), COALESCE(next_run,''), run_count
		 FROM cron_jobs WHERE status = 'active'`)
	if err != nil {
		return fmt.Errorf("reload cron jobs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var j CronJob
		if err := rows.Scan(&j.ID, &j.SessionID, &j.Schedule, &j.Command, &j.Status, &j.LastRun, &j.NextRun, &j.RunCount); err != nil {
			log.Printf("cron: scan job: %v", err)
			continue
		}
		cs.startEntry(j)
	}
	return rows.Err()
}

// Create adds a new cron job and starts it immediately.
func (cs *CronScheduler) Create(ctx context.Context, sessionID, schedule, command string) (CronJob, error) {
	// Validate schedule
	if _, err := parseSchedule(schedule); err != nil {
		return CronJob{}, fmt.Errorf("invalid schedule %q: %w", schedule, err)
	}

	j := CronJob{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Schedule:  schedule,
		Command:   command,
		Status:    "active",
		RunCount:  0,
	}

	nextRun := time.Now().Add(mustParseScheduleDuration(schedule))
	j.NextRun = nextRun.UTC().Format(time.RFC3339)

	if _, err := cs.db.ExecContext(ctx,
		`INSERT INTO cron_jobs (id, session_id, schedule, command, status, next_run, run_count) VALUES (?,?,?,?,?,?,?)`,
		j.ID, j.SessionID, j.Schedule, j.Command, j.Status, j.NextRun, j.RunCount,
	); err != nil {
		return CronJob{}, fmt.Errorf("persist cron job: %w", err)
	}

	cs.startEntry(j)
	return j, nil
}

// List returns all cron jobs from the database.
func (cs *CronScheduler) List(ctx context.Context) ([]CronJob, error) {
	rows, err := cs.db.QueryContext(ctx,
		`SELECT id, session_id, schedule, command, status, COALESCE(last_run,''), COALESCE(next_run,''), run_count
		 FROM cron_jobs ORDER BY rowid`)
	if err != nil {
		return nil, fmt.Errorf("list cron jobs: %w", err)
	}
	defer rows.Close()

	var jobs []CronJob
	for rows.Next() {
		var j CronJob
		if err := rows.Scan(&j.ID, &j.SessionID, &j.Schedule, &j.Command, &j.Status, &j.LastRun, &j.NextRun, &j.RunCount); err != nil {
			return nil, fmt.Errorf("scan cron job: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// Pause suspends a job without removing it.
func (cs *CronScheduler) Pause(ctx context.Context, jobID string) error {
	cs.mu.Lock()
	entry, ok := cs.entries[jobID]
	cs.mu.Unlock()
	if !ok {
		return fmt.Errorf("cron job %s not found", jobID)
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.job.Status == "paused" {
		return nil
	}
	entry.cancel()
	entry.job.Status = "paused"

	_, err := cs.db.ExecContext(ctx, `UPDATE cron_jobs SET status='paused' WHERE id=?`, jobID)
	return err
}

// Resume restarts a paused job.
func (cs *CronScheduler) Resume(ctx context.Context, jobID string) error {
	cs.mu.Lock()
	entry, ok := cs.entries[jobID]
	cs.mu.Unlock()
	if !ok {
		return fmt.Errorf("cron job %s not found", jobID)
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.job.Status != "paused" {
		return nil
	}
	entry.job.Status = "active"
	if _, err := cs.db.ExecContext(ctx, `UPDATE cron_jobs SET status='active' WHERE id=?`, jobID); err != nil {
		return err
	}

	// Restart the goroutine using the daemon's parent context so it respects shutdown.
	cs.mu.Lock()
	parent := cs.parentCtx
	cs.mu.Unlock()
	newCtx, cancel := context.WithCancel(parent)
	entry.cancel = cancel
	go cs.run(newCtx, entry)
	return nil
}

// Stop permanently stops a job.
func (cs *CronScheduler) Stop(ctx context.Context, jobID string) error {
	cs.mu.Lock()
	entry, ok := cs.entries[jobID]
	if ok {
		delete(cs.entries, jobID)
	}
	cs.mu.Unlock()

	if !ok {
		return fmt.Errorf("cron job %s not found", jobID)
	}
	entry.cancel()
	_, err := cs.db.ExecContext(ctx, `UPDATE cron_jobs SET status='stopped' WHERE id=?`, jobID)
	return err
}

// startEntry launches the goroutine for a job using the daemon's parent context.
// Using parentCtx (not the RPC request context) ensures the goroutine survives
// after the CreateCron RPC returns.
func (cs *CronScheduler) startEntry(j CronJob) {
	cs.mu.Lock()
	parent := cs.parentCtx
	cs.mu.Unlock()

	runCtx, cancel := context.WithCancel(parent)
	entry := &cronEntry{job: j, cancel: cancel}

	cs.mu.Lock()
	cs.entries[j.ID] = entry
	cs.mu.Unlock()

	go cs.run(runCtx, entry)
}

// run is the per-job ticker goroutine.
func (cs *CronScheduler) run(ctx context.Context, e *cronEntry) {
	interval, err := parseSchedule(e.job.Schedule)
	if err != nil {
		log.Printf("cron: invalid schedule for job %s: %v", e.job.ID, err)
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			e.mu.Lock()
			if e.job.Status != "active" {
				e.mu.Unlock()
				return
			}
			e.job.LastRun = t.UTC().Format(time.RFC3339)
			e.job.RunCount++
			nextRun := t.Add(interval).UTC().Format(time.RFC3339)
			e.job.NextRun = nextRun
			runCount := e.job.RunCount
			lastRun := e.job.LastRun
			sessionID := e.job.SessionID
			command := e.job.Command
			jobID := e.job.ID
			e.mu.Unlock()

			// Persist updated state.
			if _, err := cs.db.Exec(
				`UPDATE cron_jobs SET last_run=?, next_run=?, run_count=? WHERE id=?`,
				lastRun, nextRun, runCount, jobID,
			); err != nil {
				log.Printf("cron: update job %s: %v", jobID, err)
			}

			if cs.onTick != nil {
				cs.onTick(sessionID, command)
			}
		}
	}
}

// parseSchedule converts a schedule string to a time.Duration.
// Supports Go duration strings ("5m", "1h30m") and simple cron expressions.
func parseSchedule(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)

	// Try Go duration first (e.g., "5m", "1h").
	if d, err := time.ParseDuration(s); err == nil {
		if d <= 0 {
			return 0, fmt.Errorf("duration must be positive")
		}
		return d, nil
	}

	// Try simple cron expression (5 fields: min hour dom mon dow).
	return parseCronExpr(s)
}

// mustParseScheduleDuration parses the schedule or returns 1 minute as fallback.
func mustParseScheduleDuration(s string) time.Duration {
	d, err := parseSchedule(s)
	if err != nil {
		return time.Minute
	}
	return d
}

// parseCronExpr handles a subset of cron: "*/N * * * *" (every N minutes).
func parseCronExpr(expr string) (time.Duration, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return 0, fmt.Errorf("expected 5 cron fields, got %d", len(fields))
	}

	minField := fields[0]

	// Support "*/N" in the minute field, everything else wildcarded.
	if step, ok := strings.CutPrefix(minField, "*/"); ok {
		n, err := strconv.Atoi(step)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid cron minute step %q", minField)
		}
		return time.Duration(n) * time.Minute, nil
	}

	// Support a fixed minute offset with all other fields wildcarded (e.g., "0 * * * *").
	if _, err := strconv.Atoi(minField); err == nil {
		// Fire every hour at that minute — approximate as 1h for simplicity.
		return time.Hour, nil
	}

	return 0, fmt.Errorf("unsupported cron expression %q", expr)
}
