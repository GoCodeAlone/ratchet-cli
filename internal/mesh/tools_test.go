package mesh

import (
	"context"
	"testing"

	"github.com/GoCodeAlone/workflow-plugin-agent/tools"
)

func TestBlackboardReadTool_ReadKey(t *testing.T) {
	bb := NewBlackboard()
	bb.Write("plan", "goal", "build mesh", "tester")

	tool := &BlackboardReadTool{bb: bb}
	result, err := tool.Execute(context.Background(), map[string]any{
		"section": "plan",
		"key":     "goal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != "build mesh" {
		t.Fatalf("got %v, want 'build mesh'", result)
	}
}

func TestBlackboardReadTool_ListKeys(t *testing.T) {
	bb := NewBlackboard()
	bb.Write("plan", "a", 1, "tester")
	bb.Write("plan", "b", 2, "tester")

	tool := &BlackboardReadTool{bb: bb}
	result, err := tool.Execute(context.Background(), map[string]any{
		"section": "plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	keys, ok := result.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", result)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestBlackboardReadTool_MissingSection(t *testing.T) {
	bb := NewBlackboard()
	tool := &BlackboardReadTool{bb: bb}
	result, err := tool.Execute(context.Background(), map[string]any{
		"section": "nope",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != "section not found" {
		t.Fatalf("expected 'section not found', got %v", result)
	}
}

func TestBlackboardReadTool_MissingKey(t *testing.T) {
	bb := NewBlackboard()
	bb.Write("plan", "a", 1, "tester")

	tool := &BlackboardReadTool{bb: bb}
	result, err := tool.Execute(context.Background(), map[string]any{
		"section": "plan",
		"key":     "missing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != "not found" {
		t.Fatalf("expected 'not found', got %v", result)
	}
}

func TestBlackboardReadTool_MissingSectionArg(t *testing.T) {
	bb := NewBlackboard()
	tool := &BlackboardReadTool{bb: bb}
	_, err := tool.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing section")
	}
}

func TestBlackboardWriteTool(t *testing.T) {
	bb := NewBlackboard()
	tool := &BlackboardWriteTool{bb: bb}

	ctx := tools.WithAgentID(context.Background(), "agent-1")
	result, err := tool.Execute(ctx, map[string]any{
		"section": "results",
		"key":     "answer",
		"value":   "42",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != "written (revision 1)" {
		t.Fatalf("unexpected result: %v", result)
	}

	// Verify the write
	e, ok := bb.Read("results", "answer")
	if !ok {
		t.Fatal("expected entry to exist")
	}
	if e.Value != "42" || e.Author != "agent-1" {
		t.Fatalf("unexpected entry: %+v", e)
	}
}

func TestBlackboardWriteTool_NoAuthor(t *testing.T) {
	bb := NewBlackboard()
	tool := &BlackboardWriteTool{bb: bb}

	_, err := tool.Execute(context.Background(), map[string]any{
		"section": "s",
		"key":     "k",
		"value":   "v",
	})
	if err != nil {
		t.Fatal(err)
	}

	e, _ := bb.Read("s", "k")
	if e.Author != "unknown" {
		t.Fatalf("expected author 'unknown', got %q", e.Author)
	}
}

func TestBlackboardWriteTool_MissingArgs(t *testing.T) {
	bb := NewBlackboard()
	tool := &BlackboardWriteTool{bb: bb}
	_, err := tool.Execute(context.Background(), map[string]any{
		"section": "s",
	})
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestBlackboardListTool_ListSections(t *testing.T) {
	bb := NewBlackboard()
	bb.Write("alpha", "k", 1, "w")
	bb.Write("beta", "k", 2, "w")

	tool := &BlackboardListTool{bb: bb}
	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	secs, ok := result.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", result)
	}
	if len(secs) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(secs))
	}
}

func TestBlackboardListTool_ListKeys(t *testing.T) {
	bb := NewBlackboard()
	bb.Write("sec", "a", 1, "w")
	bb.Write("sec", "b", 2, "w")

	tool := &BlackboardListTool{bb: bb}
	result, err := tool.Execute(context.Background(), map[string]any{
		"section": "sec",
	})
	if err != nil {
		t.Fatal(err)
	}
	keys, ok := result.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", result)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestBlackboardListTool_MissingSection(t *testing.T) {
	bb := NewBlackboard()
	tool := &BlackboardListTool{bb: bb}
	result, err := tool.Execute(context.Background(), map[string]any{
		"section": "nope",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != "section not found" {
		t.Fatalf("expected 'section not found', got %v", result)
	}
}

func TestSendMessageTool(t *testing.T) {
	outbox := make(chan Message, 10)
	tool := &SendMessageTool{outbox: outbox, from: "node-1"}

	result, err := tool.Execute(context.Background(), map[string]any{
		"to":      "node-2",
		"type":    "task",
		"content": "do something",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	select {
	case msg := <-outbox:
		if msg.From != "node-1" || msg.To != "node-2" || msg.Content != "do something" {
			t.Fatalf("unexpected message: %+v", msg)
		}
	default:
		t.Fatal("expected message in outbox")
	}
}

func TestSendMessageTool_MissingArgs(t *testing.T) {
	outbox := make(chan Message, 10)
	tool := &SendMessageTool{outbox: outbox, from: "node-1"}

	_, err := tool.Execute(context.Background(), map[string]any{
		"content": "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing 'to' and 'type'")
	}
}

func TestSendMessageTool_FullOutbox(t *testing.T) {
	outbox := make(chan Message) // unbuffered
	tool := &SendMessageTool{outbox: outbox, from: "node-1"}

	_, err := tool.Execute(context.Background(), map[string]any{
		"to":      "node-2",
		"type":    "task",
		"content": "overflow",
	})
	if err == nil {
		t.Fatal("expected error for full outbox")
	}
}
