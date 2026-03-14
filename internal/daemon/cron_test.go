package daemon

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestCronDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_journal_mode=WAL")
	if err != nil {
		t.Fatal(err)
	}
	if err := initDB(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCronScheduler_CreateInterval(t *testing.T) {
	db := newTestCronDB(t)
	ticks := make(chan string, 10)
	cs := NewCronScheduler(db, func(sessionID, command string) {
		ticks <- command
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job, err := cs.Create(ctx, "sess1", "100ms", "ping")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if job.Status != "active" {
		t.Errorf("expected status active, got %s", job.Status)
	}

	select {
	case cmd := <-ticks:
		if cmd != "ping" {
			t.Errorf("expected 'ping', got %q", cmd)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cron tick")
	}
}

func TestCronScheduler_Pause(t *testing.T) {
	db := newTestCronDB(t)
	ticks := make(chan string, 10)
	cs := NewCronScheduler(db, func(sessionID, command string) {
		ticks <- command
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job, err := cs.Create(ctx, "sess1", "50ms", "work")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Wait for at least one tick.
	select {
	case <-ticks:
	case <-time.After(time.Second):
		t.Fatal("no tick before pause")
	}

	if err := cs.Pause(ctx, job.ID); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	// Drain any buffered ticks.
	for len(ticks) > 0 {
		<-ticks
	}

	// Verify no more ticks arrive.
	select {
	case <-ticks:
		t.Error("received tick after pause")
	case <-time.After(300 * time.Millisecond):
		// OK: no tick while paused
	}
}

func TestCronScheduler_Resume(t *testing.T) {
	db := newTestCronDB(t)
	ticks := make(chan string, 10)
	cs := NewCronScheduler(db, func(sessionID, command string) {
		ticks <- command
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job, err := cs.Create(ctx, "sess1", "50ms", "work")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Wait for tick then pause.
	select {
	case <-ticks:
	case <-time.After(time.Second):
		t.Fatal("no initial tick")
	}
	if err := cs.Pause(ctx, job.ID); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	for len(ticks) > 0 {
		<-ticks
	}

	// Resume and verify ticks restart.
	if err := cs.Resume(ctx, job.ID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	select {
	case <-ticks:
		// OK: resumed
	case <-time.After(2 * time.Second):
		t.Fatal("no tick after resume")
	}
}

func TestCronScheduler_Stop(t *testing.T) {
	db := newTestCronDB(t)
	ticks := make(chan string, 10)
	cs := NewCronScheduler(db, func(sessionID, command string) {
		ticks <- command
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job, err := cs.Create(ctx, "sess1", "50ms", "work")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Wait for first tick.
	select {
	case <-ticks:
	case <-time.After(time.Second):
		t.Fatal("no initial tick")
	}

	if err := cs.Stop(ctx, job.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Verify entry removed from in-memory map.
	cs.mu.Lock()
	_, exists := cs.entries[job.ID]
	cs.mu.Unlock()
	if exists {
		t.Error("entry still exists after stop")
	}

	// Drain buffered ticks then verify silence.
	for len(ticks) > 0 {
		<-ticks
	}
	select {
	case <-ticks:
		t.Error("received tick after stop")
	case <-time.After(300 * time.Millisecond):
		// OK
	}
}

func TestCronScheduler_PersistReload(t *testing.T) {
	db := newTestCronDB(t)
	ticks := make(chan string, 10)

	cs1 := NewCronScheduler(db, func(_, cmd string) { ticks <- cmd })
	ctx := context.Background()

	job, err := cs1.Create(ctx, "sess1", "50ms", "reload-cmd")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Wait for tick so last_run/run_count are persisted.
	select {
	case <-ticks:
	case <-time.After(time.Second):
		t.Fatal("no tick from first scheduler")
	}

	// Simulate restart: stop old, create new scheduler with same DB.
	if err := cs1.Stop(ctx, job.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// Reset to active so reload picks it up.
	if _, err := db.Exec(`UPDATE cron_jobs SET status='active' WHERE id=?`, job.ID); err != nil {
		t.Fatal(err)
	}

	ticks2 := make(chan string, 10)
	cs2 := NewCronScheduler(db, func(_, cmd string) { ticks2 <- cmd })
	if err := cs2.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case cmd := <-ticks2:
		if cmd != "reload-cmd" {
			t.Errorf("expected 'reload-cmd', got %q", cmd)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no tick after reload")
	}

	// Cleanup.
	cs2.mu.Lock()
	for _, e := range cs2.entries {
		e.cancel()
	}
	cs2.mu.Unlock()
}

func TestParseSchedule(t *testing.T) {
	cases := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"5m", 5 * time.Minute, false},
		{"1h30m", 90 * time.Minute, false},
		{"100ms", 100 * time.Millisecond, false},
		{"*/10 * * * *", 10 * time.Minute, false},
		{"*/1 * * * *", 1 * time.Minute, false},
		{"0 * * * *", time.Hour, false},
		{"bad", 0, true},
		{"-1m", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseSchedule(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
