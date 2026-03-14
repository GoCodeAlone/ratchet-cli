package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckpoint_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	orig := CheckpointPath
	CheckpointPath = func() string { return filepath.Join(dir, "checkpoint.json") }
	defer func() { CheckpointPath = orig }()

	cp := &Checkpoint{
		Version: "v1.2.3",
		Sessions: []SessionCheckpoint{
			{ID: "s1", Name: "test-session", WorkingDir: "/tmp", Provider: "anthropic", Model: "claude-3-5-sonnet-20241022", Status: "active"},
		},
		CronJobs: []CronCheckpoint{
			{ID: "c1", SessionID: "s1", Schedule: "5m", Command: "git status", Status: "active"},
		},
		Providers: []ProviderCheckpoint{
			{Alias: "default", Type: "anthropic", Model: "claude-3-5-sonnet-20241022"},
		},
	}

	if err := SaveCheckpoint(cp); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	loaded, err := LoadCheckpoint()
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}

	if loaded.Version != cp.Version {
		t.Errorf("Version: got %q, want %q", loaded.Version, cp.Version)
	}
	if len(loaded.Sessions) != 1 || loaded.Sessions[0].ID != "s1" {
		t.Errorf("Sessions: %+v", loaded.Sessions)
	}
	if len(loaded.CronJobs) != 1 || loaded.CronJobs[0].Schedule != "5m" {
		t.Errorf("CronJobs: %+v", loaded.CronJobs)
	}
	if len(loaded.Providers) != 1 || loaded.Providers[0].Alias != "default" {
		t.Errorf("Providers: %+v", loaded.Providers)
	}
}

func TestCheckpoint_MissingFile(t *testing.T) {
	dir := t.TempDir()
	orig := CheckpointPath
	CheckpointPath = func() string { return filepath.Join(dir, "nonexistent.json") }
	defer func() { CheckpointPath = orig }()

	_, err := LoadCheckpoint()
	if err == nil {
		t.Error("expected error for missing checkpoint file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected IsNotExist error, got: %v", err)
	}
}

func TestCheckpoint_ExportImport(t *testing.T) {
	dir := t.TempDir()
	orig := CheckpointPath
	CheckpointPath = func() string { return filepath.Join(dir, "checkpoint.json") }
	defer func() { CheckpointPath = orig }()

	cp := &Checkpoint{
		Version: "v0.1.0",
		Sessions: []SessionCheckpoint{
			{ID: "abc123", Status: "active"},
			{ID: "def456", Status: "completed"},
		},
	}

	if err := SaveCheckpoint(cp); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := LoadCheckpoint()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got.Sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(got.Sessions))
	}
	if got.Version != "v0.1.0" {
		t.Errorf("Version: got %q, want v0.1.0", got.Version)
	}
}
