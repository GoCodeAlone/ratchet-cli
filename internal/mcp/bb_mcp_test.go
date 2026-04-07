package mcp

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/mesh"
)

func TestBBMCPServer_Initialize(t *testing.T) {
	bb := mesh.NewBlackboard()
	srv := NewBBMCPServer(bb)

	req := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}` + "\n"
	in := strings.NewReader(req)

	// Use io.Pipe so the scanner blocks until the server writes (not EOF immediately).
	pr, pw := io.Pipe()
	go func() {
		srv.Serve(bufio.NewReader(in), pw)
		pw.Close()
	}()

	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		line := scanner.Text()
		var resp map[string]any
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}
		if resp["id"] != nil {
			result, ok := resp["result"].(map[string]any)
			if !ok {
				t.Fatalf("expected result object, got %v", resp)
			}
			if result["protocolVersion"] == nil {
				t.Fatal("missing protocolVersion in initialize response")
			}
			return
		}
	}
	t.Fatal("no initialize response received")
}

func TestBBMCPServer_ToolsList(t *testing.T) {
	bb := mesh.NewBlackboard()
	srv := NewBBMCPServer(bb)

	result, err := srv.handleToolsList()
	if err != nil {
		t.Fatal(err)
	}
	tools, ok := result.(map[string]any)["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("expected tools array, got %T", result)
	}
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool["name"].(string)] = true
	}
	for _, expected := range []string{"bb_read", "bb_write", "bb_list"} {
		if !names[expected] {
			t.Errorf("missing tool %q", expected)
		}
	}
}

func TestBBMCPServer_ReadWrite(t *testing.T) {
	bb := mesh.NewBlackboard()
	srv := NewBBMCPServer(bb)

	// Write.
	_, err := srv.handleToolCall("bb_write", map[string]any{
		"section": "plan",
		"key":     "design",
		"value":   "test value",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Read.
	result, err := srv.handleToolCall("bb_read", map[string]any{
		"section": "plan",
		"key":     "design",
	})
	if err != nil {
		t.Fatal(err)
	}
	content, ok := result.(map[string]any)["content"].([]map[string]any)
	if !ok || len(content) == 0 {
		t.Fatalf("expected content array, got %v", result)
	}
	if !strings.Contains(content[0]["text"].(string), "test value") {
		t.Fatalf("expected 'test value' in response, got %s", content[0]["text"])
	}
}

func TestBBMCPServer_List(t *testing.T) {
	bb := mesh.NewBlackboard()
	bb.Write("plan", "design", "x", "test")
	bb.Write("code", "main.go", "y", "test")
	srv := NewBBMCPServer(bb)

	// List sections.
	result, err := srv.handleToolCall("bb_list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	text := result.(map[string]any)["content"].([]map[string]any)[0]["text"].(string)
	if !strings.Contains(text, "plan") || !strings.Contains(text, "code") {
		t.Fatalf("expected sections list, got %s", text)
	}

	// List keys in section.
	result, err = srv.handleToolCall("bb_list", map[string]any{"section": "plan"})
	if err != nil {
		t.Fatal(err)
	}
	text = result.(map[string]any)["content"].([]map[string]any)[0]["text"].(string)
	if !strings.Contains(text, "design") {
		t.Fatalf("expected 'design' key, got %s", text)
	}
}
