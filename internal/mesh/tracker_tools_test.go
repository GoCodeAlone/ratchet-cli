package mesh

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func testTrackerWithDB(t *testing.T) (*Tracker, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tr, err := NewTracker(db)
	if err != nil {
		t.Fatal(err)
	}
	return tr, db
}

func TestTaskCreateTool(t *testing.T) {
	tr, _ := testTrackerWithDB(t)
	projID, _ := tr.CreateProject("my-proj", "")

	tool := &TaskCreateTool{tracker: tr}

	res, err := tool.Execute(context.Background(), map[string]any{
		"project_id":    projID,
		"title":         "Write tests",
		"description":   "TDD approach",
		"assigned_team": "dev",
		"priority":      float64(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	taskID, ok := res.(string)
	if !ok || taskID == "" {
		t.Fatalf("expected task ID string, got %v", res)
	}

	task, err := tr.GetTask(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != "Write tests" {
		t.Errorf("title: got %q", task.Title)
	}
}

func TestTaskClaimTool(t *testing.T) {
	tr, _ := testTrackerWithDB(t)
	projID, _ := tr.CreateProject("p", "")
	taskID, _ := tr.CreateTask(projID, "T", "", "dev", 0)

	tool := &TaskClaimTool{tracker: tr}
	_, err := tool.Execute(context.Background(), map[string]any{
		"task_id":    taskID,
		"agent_name": "coder",
	})
	if err != nil {
		t.Fatal(err)
	}

	task, _ := tr.GetTask(taskID)
	if task.ClaimedBy != "coder" {
		t.Errorf("claimed_by: got %q", task.ClaimedBy)
	}
}

func TestTaskUpdateTool(t *testing.T) {
	tr, _ := testTrackerWithDB(t)
	projID, _ := tr.CreateProject("p", "")
	taskID, _ := tr.CreateTask(projID, "T", "", "dev", 0)

	tool := &TaskUpdateTool{tracker: tr}
	_, err := tool.Execute(context.Background(), map[string]any{
		"task_id": taskID,
		"status":  "completed",
		"notes":   "done",
	})
	if err != nil {
		t.Fatal(err)
	}

	task, _ := tr.GetTask(taskID)
	if task.Status != "completed" {
		t.Errorf("status: got %q", task.Status)
	}
}

func TestTaskListTool(t *testing.T) {
	tr, _ := testTrackerWithDB(t)
	projID, _ := tr.CreateProject("p", "")
	tr.CreateTask(projID, "T1", "", "dev", 0)
	tr.CreateTask(projID, "T2", "", "qa", 0)

	tool := &TaskListTool{tracker: tr}
	res, err := tool.Execute(context.Background(), map[string]any{
		"project_id": projID,
	})
	if err != nil {
		t.Fatal(err)
	}
	tasks, ok := res.([]Task)
	if !ok {
		t.Fatalf("expected []Task, got %T", res)
	}
	if len(tasks) != 2 {
		t.Errorf("got %d tasks, want 2", len(tasks))
	}
}

func TestTaskGetTool(t *testing.T) {
	tr, _ := testTrackerWithDB(t)
	projID, _ := tr.CreateProject("p", "")
	taskID, _ := tr.CreateTask(projID, "Important", "", "dev", 0)

	tool := &TaskGetTool{tracker: tr}
	res, err := tool.Execute(context.Background(), map[string]any{
		"task_id": taskID,
	})
	if err != nil {
		t.Fatal(err)
	}
	task, ok := res.(*Task)
	if !ok {
		t.Fatalf("expected *Task, got %T", res)
	}
	if task.Title != "Important" {
		t.Errorf("title: got %q", task.Title)
	}
}

func TestProjectStatusTool(t *testing.T) {
	tr, _ := testTrackerWithDB(t)
	projID, _ := tr.CreateProject("p", "")
	taskID, _ := tr.CreateTask(projID, "T1", "", "dev", 0)
	tr.CreateTask(projID, "T2", "", "dev", 0)
	tr.UpdateTask(taskID, "completed", "done")

	tool := &ProjectStatusTool{tracker: tr}
	res, err := tool.Execute(context.Background(), map[string]any{
		"project_id": projID,
	})
	if err != nil {
		t.Fatal(err)
	}
	ps, ok := res.(*ProjectStatusSummary)
	if !ok {
		t.Fatalf("expected *ProjectStatusSummary, got %T", res)
	}
	if ps.Total != 2 {
		t.Errorf("total: got %d, want 2", ps.Total)
	}
	if ps.Completed != 1 {
		t.Errorf("completed: got %d, want 1", ps.Completed)
	}
}
