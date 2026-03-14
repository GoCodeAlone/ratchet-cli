package daemon

import (
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// staticProvider is a test provider returning a fixed job list.
type staticProvider struct {
	jobs    []*pb.Job
	paused  string
	killed  string
	pauseErr error
	killErr  error
}

func (p *staticProvider) ActiveJobs() []*pb.Job { return p.jobs }
func (p *staticProvider) PauseJob(id string) error {
	p.paused = id
	return p.pauseErr
}
func (p *staticProvider) KillJob(id string) error {
	p.killed = id
	return p.killErr
}

func TestJobRegistry_Aggregate(t *testing.T) {
	jr := NewJobRegistry()
	jr.Register("session", &staticProvider{jobs: []*pb.Job{
		{Id: "session:s1", Type: "session", Name: "session-1"},
	}})
	jr.Register("cron", &staticProvider{jobs: []*pb.Job{
		{Id: "cron:c1", Type: "cron", Name: "cleanup"},
		{Id: "cron:c2", Type: "cron", Name: "report"},
	}})

	jobs := jr.ListJobs()
	if len(jobs) != 3 {
		t.Errorf("expected 3 jobs, got %d", len(jobs))
	}
}

func TestJobRegistry_KillSession(t *testing.T) {
	sp := &staticProvider{jobs: []*pb.Job{{Id: "session:abc"}}}
	jr := NewJobRegistry()
	jr.Register("session", sp)

	if err := jr.KillJob("session:abc"); err != nil {
		t.Fatalf("KillJob: %v", err)
	}
	if sp.killed != "session:abc" {
		t.Errorf("expected killed=session:abc, got %q", sp.killed)
	}
}

func TestJobRegistry_KillFleetWorker(t *testing.T) {
	fp := &staticProvider{jobs: []*pb.Job{{Id: "fleet_worker:w1"}}}
	jr := NewJobRegistry()
	jr.Register("fleet_worker", fp)

	if err := jr.KillJob("fleet_worker:w1"); err != nil {
		t.Fatalf("KillJob: %v", err)
	}
	if fp.killed != "fleet_worker:w1" {
		t.Errorf("expected killed=fleet_worker:w1, got %q", fp.killed)
	}
}

func TestJobRegistry_PauseCron(t *testing.T) {
	cp := &staticProvider{jobs: []*pb.Job{{Id: "cron:c1"}}}
	jr := NewJobRegistry()
	jr.Register("cron", cp)

	if err := jr.PauseJob("cron:c1"); err != nil {
		t.Fatalf("PauseJob: %v", err)
	}
	if cp.paused != "cron:c1" {
		t.Errorf("expected paused=cron:c1, got %q", cp.paused)
	}
}

func TestJobRegistry_UnknownJobType(t *testing.T) {
	jr := NewJobRegistry()
	jr.Register("session", &staticProvider{})

	if err := jr.KillJob("unknown:xyz"); err == nil {
		t.Error("expected error for unknown job type, got nil")
	}
}
