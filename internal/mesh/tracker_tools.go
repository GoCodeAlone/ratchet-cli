package mesh

import (
	"context"
	"fmt"

	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

// ---------------------------------------------------------------------------
// TaskCreateTool
// ---------------------------------------------------------------------------

// TaskCreateTool allows an agent to create a new task in the tracker.
type TaskCreateTool struct {
	tracker *Tracker
}

func (t *TaskCreateTool) Name() string { return "task_create" }

func (t *TaskCreateTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "task_create",
		Description: "Create a new task in the project tracker.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":    map[string]any{"type": "string", "description": "Project ID"},
				"title":         map[string]any{"type": "string", "description": "Task title"},
				"description":   map[string]any{"type": "string", "description": "Task description"},
				"assigned_team": map[string]any{"type": "string", "description": "Team responsible for this task"},
				"priority":      map[string]any{"type": "number", "description": "Priority (higher = more urgent)"},
			},
			"required": []string{"project_id", "title"},
		},
	}
}

func (t *TaskCreateTool) Execute(_ context.Context, args map[string]any) (any, error) {
	projID, _ := args["project_id"].(string)
	title, _ := args["title"].(string)
	if projID == "" || title == "" {
		return nil, fmt.Errorf("project_id and title are required")
	}
	desc, _ := args["description"].(string)
	team, _ := args["assigned_team"].(string)
	priority := 0
	if p, ok := args["priority"].(float64); ok {
		priority = int(p)
	}
	return t.tracker.CreateTask(projID, title, desc, team, priority)
}

// ---------------------------------------------------------------------------
// TaskClaimTool
// ---------------------------------------------------------------------------

// TaskClaimTool lets an agent claim a task (optimistic lock).
type TaskClaimTool struct {
	tracker *Tracker
}

func (t *TaskClaimTool) Name() string { return "task_claim" }

func (t *TaskClaimTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "task_claim",
		Description: "Claim a task so no other agent picks it up.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id":    map[string]any{"type": "string", "description": "Task ID"},
				"agent_name": map[string]any{"type": "string", "description": "Claiming agent name"},
			},
			"required": []string{"task_id", "agent_name"},
		},
	}
}

func (t *TaskClaimTool) Execute(_ context.Context, args map[string]any) (any, error) {
	taskID, _ := args["task_id"].(string)
	agent, _ := args["agent_name"].(string)
	if taskID == "" || agent == "" {
		return nil, fmt.Errorf("task_id and agent_name are required")
	}
	if err := t.tracker.ClaimTask(taskID, agent); err != nil {
		return nil, err
	}
	return "claimed", nil
}

// ---------------------------------------------------------------------------
// TaskUpdateTool
// ---------------------------------------------------------------------------

// TaskUpdateTool updates the status and notes of a task.
type TaskUpdateTool struct {
	tracker *Tracker
}

func (t *TaskUpdateTool) Name() string { return "task_update" }

func (t *TaskUpdateTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "task_update",
		Description: "Update task status and notes.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{"type": "string", "description": "Task ID"},
				"status":  map[string]any{"type": "string", "description": "New status (pending, in_progress, completed, blocked)"},
				"notes":   map[string]any{"type": "string", "description": "Progress notes"},
			},
			"required": []string{"task_id", "status"},
		},
	}
}

func (t *TaskUpdateTool) Execute(_ context.Context, args map[string]any) (any, error) {
	taskID, _ := args["task_id"].(string)
	status, _ := args["status"].(string)
	if taskID == "" || status == "" {
		return nil, fmt.Errorf("task_id and status are required")
	}
	notes, _ := args["notes"].(string)
	if err := t.tracker.UpdateTask(taskID, status, notes); err != nil {
		return nil, err
	}
	return "updated", nil
}

// ---------------------------------------------------------------------------
// TaskListTool
// ---------------------------------------------------------------------------

// TaskListTool lists tasks with optional filters.
type TaskListTool struct {
	tracker *Tracker
}

func (t *TaskListTool) Name() string { return "task_list" }

func (t *TaskListTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "task_list",
		Description: "List tasks, optionally filtered by project, team, or status.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Filter by project ID"},
				"team":       map[string]any{"type": "string", "description": "Filter by assigned team"},
				"status":     map[string]any{"type": "string", "description": "Filter by status"},
				"limit":      map[string]any{"type": "number", "description": "Max results (default 20)"},
			},
		},
	}
}

func (t *TaskListTool) Execute(_ context.Context, args map[string]any) (any, error) {
	projID, _ := args["project_id"].(string)
	team, _ := args["team"].(string)
	status, _ := args["status"].(string)
	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	return t.tracker.ListTasks(projID, team, status, limit)
}

// ---------------------------------------------------------------------------
// TaskGetTool
// ---------------------------------------------------------------------------

// TaskGetTool fetches a single task by ID.
type TaskGetTool struct {
	tracker *Tracker
}

func (t *TaskGetTool) Name() string { return "task_get" }

func (t *TaskGetTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "task_get",
		Description: "Get details of a specific task by ID.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{"type": "string", "description": "Task ID"},
			},
			"required": []string{"task_id"},
		},
	}
}

func (t *TaskGetTool) Execute(_ context.Context, args map[string]any) (any, error) {
	taskID, _ := args["task_id"].(string)
	if taskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	return t.tracker.GetTask(taskID)
}

// ---------------------------------------------------------------------------
// ProjectStatusTool
// ---------------------------------------------------------------------------

// ProjectStatusTool returns aggregate task counts for a project.
type ProjectStatusTool struct {
	tracker *Tracker
}

func (t *ProjectStatusTool) Name() string { return "project_status" }

func (t *ProjectStatusTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "project_status",
		Description: "Get task completion summary for a project.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project ID"},
			},
			"required": []string{"project_id"},
		},
	}
}

func (t *ProjectStatusTool) Execute(_ context.Context, args map[string]any) (any, error) {
	projID, _ := args["project_id"].(string)
	if projID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	return t.tracker.ProjectStatus(projID)
}
