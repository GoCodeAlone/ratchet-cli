package daemon

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func setupCronDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS cron_jobs (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		schedule TEXT NOT NULL,
		command TEXT NOT NULL,
		status TEXT DEFAULT 'active',
		last_run TEXT,
		next_run TEXT,
		run_count INTEGER DEFAULT 0
	)`)
	if err != nil {
		t.Fatalf("create cron_jobs table: %v", err)
	}
	return db
}

func TestCron_TickInjectsMessage(t *testing.T) {
	db := setupCronDB(t)

	ticked := make(chan string, 4)
	cs := NewCronScheduler(db, func(_ string, command string) {
		select {
		case ticked <- command:
		default:
		}
	})
	ctx := context.Background()
	if err := cs.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	job, err := cs.Create(ctx, "sess-cron", "100ms", "echo hello")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = cs.Stop(ctx, job.ID) }()

	select {
	case cmd := <-ticked:
		if cmd != "echo hello" {
			t.Errorf("unexpected command: %q", cmd)
		}
	case <-time.After(3 * time.Second):
		t.Error("timed out waiting for cron tick")
	}
}

func TestCron_PauseStopsTicks(t *testing.T) {
	db := setupCronDB(t)

	ticked := make(chan string, 8)
	cs := NewCronScheduler(db, func(_ string, command string) {
		select {
		case ticked <- command:
		default:
		}
	})
	ctx := context.Background()
	if err := cs.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	job, err := cs.Create(ctx, "sess-pause", "100ms", "ping")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = cs.Stop(ctx, job.ID) }()

	// Wait for at least one tick.
	select {
	case <-ticked:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for first tick before pause")
	}

	if err := cs.Pause(ctx, job.ID); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	// Drain any in-flight ticks.
	time.Sleep(50 * time.Millisecond)
	for len(ticked) > 0 {
		<-ticked
	}

	// After pause, no new ticks should arrive for 300ms.
	select {
	case cmd := <-ticked:
		t.Errorf("unexpected tick after pause: %q", cmd)
	case <-time.After(300 * time.Millisecond):
		// Expected: no ticks.
	}
}
