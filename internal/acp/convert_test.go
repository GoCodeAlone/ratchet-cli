package acp

import (
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestChatEventToUpdates_Token(t *testing.T) {
	ev := &pb.ChatEvent{Event: &pb.ChatEvent_Token{Token: &pb.TokenDelta{Content: "hello"}}}
	updates := chatEventToUpdates(ev)
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	if updates[0].AgentMessageChunk == nil {
		t.Error("expected AgentMessageChunk update")
	}
}

func TestChatEventToUpdates_Thinking(t *testing.T) {
	ev := &pb.ChatEvent{Event: &pb.ChatEvent_Thinking{Thinking: &pb.ThinkingBlock{Content: "reasoning"}}}
	updates := chatEventToUpdates(ev)
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	if updates[0].AgentThoughtChunk == nil {
		t.Error("expected AgentThoughtChunk update")
	}
}

func TestChatEventToUpdates_ToolStart(t *testing.T) {
	ev := &pb.ChatEvent{Event: &pb.ChatEvent_ToolStart{ToolStart: &pb.ToolCallStart{
		ToolName:      "file_read",
		CallId:        "call-1",
		ArgumentsJson: `{"path":"/tmp/test"}`,
	}}}
	updates := chatEventToUpdates(ev)
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	if updates[0].ToolCall == nil {
		t.Error("expected ToolCall update")
	}
}

func TestChatEventToUpdates_ToolResult(t *testing.T) {
	ev := &pb.ChatEvent{Event: &pb.ChatEvent_ToolResult{ToolResult: &pb.ToolCallResult{
		CallId:     "call-1",
		ResultJson: `{"content":"file data"}`,
		Success:    true,
	}}}
	updates := chatEventToUpdates(ev)
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	if updates[0].ToolCallUpdate == nil {
		t.Error("expected ToolCallUpdate")
	}
}

func TestChatEventToUpdates_Error(t *testing.T) {
	ev := &pb.ChatEvent{Event: &pb.ChatEvent_Error{Error: &pb.ErrorEvent{Message: "timeout"}}}
	updates := chatEventToUpdates(ev)
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	if updates[0].AgentMessageChunk == nil {
		t.Error("expected AgentMessageChunk with error")
	}
}

func TestChatEventToUpdates_Permission(t *testing.T) {
	ev := &pb.ChatEvent{Event: &pb.ChatEvent_Permission{Permission: &pb.PermissionRequest{
		RequestId: "perm-1",
		ToolName:  "bash",
	}}}
	updates := chatEventToUpdates(ev)
	if updates != nil {
		t.Error("expected nil updates for permission event")
	}
}

func TestChatEventToUpdates_Complete(t *testing.T) {
	ev := &pb.ChatEvent{Event: &pb.ChatEvent_Complete{Complete: &pb.SessionComplete{Summary: "done"}}}
	updates := chatEventToUpdates(ev)
	if updates != nil {
		t.Error("expected nil updates for complete event")
	}
}

func TestMapPlanStepStatus(t *testing.T) {
	tests := []struct {
		input string
		want  acpsdk.PlanEntryStatus
	}{
		{"completed", acpsdk.PlanEntryStatusCompleted},
		{"in_progress", acpsdk.PlanEntryStatusInProgress},
		{"pending", acpsdk.PlanEntryStatusPending},
		{"failed", acpsdk.PlanEntryStatusCompleted},
		{"unknown", acpsdk.PlanEntryStatusPending},
	}
	for _, tt := range tests {
		got := mapPlanStepStatus(tt.input)
		if got != tt.want {
			t.Errorf("mapPlanStepStatus(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
