package mesh

import (
	"context"
	"fmt"
	"time"

	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
	"github.com/GoCodeAlone/workflow-plugin-agent/tools"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// BlackboardReadTool
// ---------------------------------------------------------------------------

// BlackboardReadTool reads an entry (or lists keys) from the blackboard.
type BlackboardReadTool struct {
	bb *Blackboard
}

func (t *BlackboardReadTool) Name() string { return "blackboard_read" }

func (t *BlackboardReadTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "blackboard_read",
		Description: "Read a value from the shared blackboard. If key is omitted, lists all keys in the section.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"section": map[string]any{
					"type":        "string",
					"description": "Blackboard section name",
				},
				"key": map[string]any{
					"type":        "string",
					"description": "Key to read (optional — omit to list all keys)",
				},
			},
			"required": []string{"section"},
		},
	}
}

func (t *BlackboardReadTool) Execute(_ context.Context, args map[string]any) (any, error) {
	section, _ := args["section"].(string)
	if section == "" {
		return nil, fmt.Errorf("section is required")
	}

	key, _ := args["key"].(string)
	if key == "" {
		// List all keys in the section
		entries := t.bb.List(section)
		if entries == nil {
			return "section not found", nil
		}
		keys := make([]string, 0, len(entries))
		for k := range entries {
			keys = append(keys, k)
		}
		return keys, nil
	}

	e, ok := t.bb.Read(section, key)
	if !ok {
		return "not found", nil
	}
	return e.Value, nil
}

// ---------------------------------------------------------------------------
// BlackboardWriteTool
// ---------------------------------------------------------------------------

// BlackboardWriteTool writes a value to the blackboard, stamping the author
// from the agent context.
type BlackboardWriteTool struct {
	bb *Blackboard
}

func (t *BlackboardWriteTool) Name() string { return "blackboard_write" }

func (t *BlackboardWriteTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "blackboard_write",
		Description: "Write a value to the shared blackboard.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"section": map[string]any{
					"type":        "string",
					"description": "Blackboard section name",
				},
				"key": map[string]any{
					"type":        "string",
					"description": "Key to write",
				},
				"value": map[string]any{
					"description": "Value to store",
				},
			},
			"required": []string{"section", "key", "value"},
		},
	}
}

func (t *BlackboardWriteTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	section, _ := args["section"].(string)
	key, _ := args["key"].(string)
	value := args["value"]

	if section == "" || key == "" {
		return nil, fmt.Errorf("section and key are required")
	}

	author := tools.AgentIDFromContext(ctx)
	if author == "" {
		author = "unknown"
	}

	e := t.bb.Write(section, key, value, author)
	return fmt.Sprintf("written (revision %d)", e.Revision), nil
}

// ---------------------------------------------------------------------------
// BlackboardListTool
// ---------------------------------------------------------------------------

// BlackboardListTool lists sections or keys within a section.
type BlackboardListTool struct {
	bb *Blackboard
}

func (t *BlackboardListTool) Name() string { return "blackboard_list" }

func (t *BlackboardListTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "blackboard_list",
		Description: "List blackboard sections, or keys within a section if section is provided.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"section": map[string]any{
					"type":        "string",
					"description": "Section name (optional — omit to list all sections)",
				},
			},
		},
	}
}

func (t *BlackboardListTool) Execute(_ context.Context, args map[string]any) (any, error) {
	section, _ := args["section"].(string)
	if section == "" {
		return t.bb.ListSections(), nil
	}

	entries := t.bb.List(section)
	if entries == nil {
		return "section not found", nil
	}
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	return keys, nil
}

// ---------------------------------------------------------------------------
// SendMessageTool
// ---------------------------------------------------------------------------

// SendMessageTool sends a mesh Message through the node's outbox channel.
type SendMessageTool struct {
	outbox chan<- Message
	from   string
}

func (t *SendMessageTool) Name() string { return "send_message" }

func (t *SendMessageTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "send_message",
		Description: "Send a message to another mesh node by name (e.g., 'coder', 'reviewer') or broadcast with to='*'.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"to": map[string]any{
					"type":        "string",
					"description": "Target agent name (e.g., 'coder'), node ID, or '*' for broadcast",
				},
				"message_type": map[string]any{
					"type":        "string",
					"description": "Message type: task, result, feedback, or request",
					"default":     "task",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Message body",
				},
			},
			"required": []string{"to", "content"},
		},
	}
}

func (t *SendMessageTool) Execute(_ context.Context, args map[string]any) (any, error) {
	to, _ := args["to"].(string)
	msgType, _ := args["message_type"].(string)
	content, _ := args["content"].(string)

	if to == "" {
		return nil, fmt.Errorf("'to' is required")
	}
	if msgType == "" {
		msgType = "task" // default when omitted
	}

	msg := Message{
		ID:        uuid.New().String(),
		From:      t.from,
		To:        to,
		Type:      msgType,
		Content:   content,
		Timestamp: time.Now(),
	}

	select {
	case t.outbox <- msg:
		return "sent", nil
	default:
		return nil, fmt.Errorf("outbox full, message dropped")
	}
}
