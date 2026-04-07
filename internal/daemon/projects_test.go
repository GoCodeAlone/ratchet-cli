package daemon

import (
	"testing"
)

func TestProjectRegistry(t *testing.T) {
	pr := NewProjectRegistry()

	// Register a project.
	p, err := pr.Register("email-service", "/path/to/config.yaml")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if p.Name != "email-service" {
		t.Errorf("got name %q, want %q", p.Name, "email-service")
	}
	if p.Status != "active" {
		t.Errorf("got status %q, want %q", p.Status, "active")
	}

	// Duplicate name.
	if _, err := pr.Register("email-service", ""); err == nil {
		t.Error("expected error on duplicate project name")
	}

	// List.
	projects := pr.List()
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}

	// Get by ID.
	got, err := pr.Get(p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "email-service" {
		t.Errorf("got name %q, want %q", got.Name, "email-service")
	}

	// Get by name.
	got, err = pr.GetByName("email-service")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.ID != p.ID {
		t.Errorf("got ID %q, want %q", got.ID, p.ID)
	}

	// Pause.
	if err := pr.SetStatus(p.ID, "paused"); err != nil {
		t.Fatalf("SetStatus pause: %v", err)
	}
	got, _ = pr.Get(p.ID)
	if got.Status != "paused" {
		t.Errorf("got status %q, want %q", got.Status, "paused")
	}

	// AddTeam.
	pr.AddTeam(p.ID, "team-abc")
	got, _ = pr.Get(p.ID)
	if len(got.TeamIDs) != 1 || got.TeamIDs[0] != "team-abc" {
		t.Errorf("TeamIDs: got %v, want [team-abc]", got.TeamIDs)
	}

	// Kill.
	if err := pr.SetStatus(p.ID, "killed"); err != nil {
		t.Fatalf("SetStatus kill: %v", err)
	}
}
