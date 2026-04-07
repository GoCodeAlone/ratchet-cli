package mesh

import (
	"database/sql"
	"encoding/hex"
	"crypto/rand"
	"fmt"
	"time"
)

// Task represents a unit of work tracked in the project.
type Task struct {
	ID           string
	ProjectID    string
	Title        string
	Description  string
	AssignedTeam string
	ClaimedBy    string
	Status       string
	Notes        string
	Priority     int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ProjectStatusSummary summarises task completion for a project.
type ProjectStatusSummary struct {
	ProjectID   string
	Name        string
	Total       int
	Completed   int
	InProgress  int
	Pending     int
}

// Tracker is an SQLite-backed task tracker for multi-team projects.
type Tracker struct {
	db *sql.DB
}

// NewTracker creates the schema (if needed) and returns a Tracker.
func NewTracker(db *sql.DB) (*Tracker, error) {
	schema := `
CREATE TABLE IF NOT EXISTS tracker_projects (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	config_path TEXT,
	created_at  DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS tracker_tasks (
	id            TEXT PRIMARY KEY,
	project_id    TEXT NOT NULL REFERENCES tracker_projects(id),
	title         TEXT NOT NULL,
	description   TEXT,
	assigned_team TEXT,
	claimed_by    TEXT,
	status        TEXT NOT NULL DEFAULT 'pending',
	notes         TEXT,
	priority      INTEGER NOT NULL DEFAULT 0,
	created_at    DATETIME NOT NULL,
	updated_at    DATETIME NOT NULL
);`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("tracker schema: %w", err)
	}
	return &Tracker{db: db}, nil
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b) //nolint:errcheck
	return hex.EncodeToString(b)
}

// CreateProject inserts a project row and returns its ID.
func (tr *Tracker) CreateProject(name, configPath string) (string, error) {
	id := newID()
	_, err := tr.db.Exec(
		`INSERT INTO tracker_projects (id, name, config_path, created_at) VALUES (?,?,?,?)`,
		id, name, configPath, time.Now().UTC(),
	)
	if err != nil {
		return "", fmt.Errorf("create project: %w", err)
	}
	return id, nil
}

// CreateTask inserts a task row and returns its ID.
func (tr *Tracker) CreateTask(projectID, title, description, assignedTeam string, priority int) (string, error) {
	id := newID()
	now := time.Now().UTC()
	_, err := tr.db.Exec(
		`INSERT INTO tracker_tasks (id, project_id, title, description, assigned_team, status, priority, created_at, updated_at)
		 VALUES (?,?,?,?,?,'pending',?,?,?)`,
		id, projectID, title, description, assignedTeam, priority, now, now,
	)
	if err != nil {
		return "", fmt.Errorf("create task: %w", err)
	}
	return id, nil
}

// GetTask fetches a single task by ID.
func (tr *Tracker) GetTask(taskID string) (*Task, error) {
	row := tr.db.QueryRow(
		`SELECT id, project_id, title, description, assigned_team, COALESCE(claimed_by,''),
		        status, COALESCE(notes,''), priority, created_at, updated_at
		 FROM tracker_tasks WHERE id = ?`, taskID)
	return scanTask(row)
}

// ClaimTask assigns a task to an agent (optimistic lock — fails if already claimed).
func (tr *Tracker) ClaimTask(taskID, agentName string) error {
	res, err := tr.db.Exec(
		`UPDATE tracker_tasks SET claimed_by=?, updated_at=?
		 WHERE id=? AND (claimed_by IS NULL OR claimed_by='')`,
		agentName, time.Now().UTC(), taskID,
	)
	if err != nil {
		return fmt.Errorf("claim task: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("task %s is already claimed or does not exist", taskID)
	}
	return nil
}

// UpdateTask sets the status and appends notes.
func (tr *Tracker) UpdateTask(taskID, status, notes string) error {
	_, err := tr.db.Exec(
		`UPDATE tracker_tasks SET status=?, notes=?, updated_at=? WHERE id=?`,
		status, notes, time.Now().UTC(), taskID,
	)
	return err
}

// ListTasks returns tasks filtered by optional projectID, team, and status.
func (tr *Tracker) ListTasks(projectID, team, status string, limit int) ([]Task, error) {
	query := `SELECT id, project_id, title, description, assigned_team, COALESCE(claimed_by,''),
	                 status, COALESCE(notes,''), priority, created_at, updated_at
	          FROM tracker_tasks WHERE 1=1`
	args := []any{}
	if projectID != "" {
		query += " AND project_id=?"
		args = append(args, projectID)
	}
	if team != "" {
		query += " AND assigned_team=?"
		args = append(args, team)
	}
	if status != "" {
		query += " AND status=?"
		args = append(args, status)
	}
	query += " ORDER BY priority DESC, created_at ASC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := tr.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, *t)
	}
	return tasks, rows.Err()
}

// ProjectStatus returns aggregate task counts for a project.
func (tr *Tracker) ProjectStatus(projectID string) (*ProjectStatusSummary, error) {
	var name string
	err := tr.db.QueryRow(`SELECT name FROM tracker_projects WHERE id=?`, projectID).Scan(&name)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	rows, err := tr.db.Query(
		`SELECT status, COUNT(*) FROM tracker_tasks WHERE project_id=? GROUP BY status`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ps := &ProjectStatusSummary{ProjectID: projectID, Name: name}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		ps.Total += n
		switch s {
		case "completed":
			ps.Completed += n
		case "in_progress":
			ps.InProgress += n
		default:
			ps.Pending += n
		}
	}
	return ps, rows.Err()
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanTask(s scanner) (*Task, error) {
	var t Task
	var createdAt, updatedAt string
	err := s.Scan(
		&t.ID, &t.ProjectID, &t.Title, &t.Description,
		&t.AssignedTeam, &t.ClaimedBy, &t.Status, &t.Notes,
		&t.Priority, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &t, nil
}
