package mesh

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func testTrackerDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestTrackerCreateAndGet(t *testing.T) {
	db := testTrackerDB(t)
	tr, err := NewTracker(db)
	if err != nil {
		t.Fatal(err)
	}

	projID, err := tr.CreateProject("email-service", "")
	if err != nil {
		t.Fatal(err)
	}

	taskID, err := tr.CreateTask(projID, "Implement API", "Build REST endpoints", "dev", 1)
	if err != nil {
		t.Fatal(err)
	}
	if taskID == "" {
		t.Fatal("empty task ID")
	}

	task, err := tr.GetTask(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != "Implement API" {
		t.Errorf("got title %q, want %q", task.Title, "Implement API")
	}
	if task.Status != "pending" {
		t.Errorf("got status %q, want %q", task.Status, "pending")
	}
}

func TestTrackerClaim(t *testing.T) {
	db := testTrackerDB(t)
	tr, err := NewTracker(db)
	if err != nil {
		t.Fatal(err)
	}

	projID, _ := tr.CreateProject("test-proj", "")
	taskID, _ := tr.CreateTask(projID, "Task 1", "", "dev", 0)

	if err := tr.ClaimTask(taskID, "coder"); err != nil {
		t.Fatal(err)
	}

	task, _ := tr.GetTask(taskID)
	if task.ClaimedBy != "coder" {
		t.Errorf("claimed_by: got %q, want %q", task.ClaimedBy, "coder")
	}

	// Double claim should fail.
	if err := tr.ClaimTask(taskID, "other"); err == nil {
		t.Error("expected error on double claim")
	}
}

func TestTrackerUpdate(t *testing.T) {
	db := testTrackerDB(t)
	tr, err := NewTracker(db)
	if err != nil {
		t.Fatal(err)
	}

	projID, _ := tr.CreateProject("test-proj", "")
	taskID, _ := tr.CreateTask(projID, "Task 1", "", "dev", 0)

	if err := tr.UpdateTask(taskID, "in_progress", "started work"); err != nil {
		t.Fatal(err)
	}

	task, _ := tr.GetTask(taskID)
	if task.Status != "in_progress" {
		t.Errorf("status: got %q, want %q", task.Status, "in_progress")
	}
}

func TestTrackerList(t *testing.T) {
	db := testTrackerDB(t)
	tr, err := NewTracker(db)
	if err != nil {
		t.Fatal(err)
	}

	projID, _ := tr.CreateProject("test-proj", "")
	tr.CreateTask(projID, "Task 1", "", "dev", 1)
	tr.CreateTask(projID, "Task 2", "", "qa", 0)
	tr.CreateTask(projID, "Task 3", "", "dev", 2)

	// List all.
	tasks, err := tr.ListTasks("", "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Errorf("got %d tasks, want 3", len(tasks))
	}

	// Filter by team.
	tasks, err = tr.ListTasks("", "dev", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Errorf("got %d tasks for dev, want 2", len(tasks))
	}
}

func TestProjectStatus(t *testing.T) {
	db := testTrackerDB(t)
	tr, err := NewTracker(db)
	if err != nil {
		t.Fatal(err)
	}

	projID, _ := tr.CreateProject("test-proj", "")
	taskID1, _ := tr.CreateTask(projID, "Task 1", "", "dev", 0)
	tr.CreateTask(projID, "Task 2", "", "dev", 0)
	tr.UpdateTask(taskID1, "completed", "done")

	ps, err := tr.ProjectStatus(projID)
	if err != nil {
		t.Fatal(err)
	}
	if ps.Total != 2 {
		t.Errorf("total: got %d, want 2", ps.Total)
	}
	if ps.Completed != 1 {
		t.Errorf("completed: got %d, want 1", ps.Completed)
	}
}
