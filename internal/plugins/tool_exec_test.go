package plugins

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExecTool_RoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script tools not supported on Windows")
	}
	dir := t.TempDir()

	// Write tool.json.
	def := ToolDef{
		Name:        "echo_tool",
		Description: "echoes its arguments",
		Protocol:    "exec",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"msg": map[string]any{"type": "string"},
			},
		},
	}
	defData, _ := json.Marshal(def)
	if err := os.WriteFile(filepath.Join(dir, "tool.json"), defData, 0644); err != nil {
		t.Fatal(err)
	}

	// Write a shell script that echoes stdin back as JSON.
	script := "#!/bin/sh\ncat\n"
	binPath := filepath.Join(dir, "echo_tool")
	if err := os.WriteFile(binPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	tool, err := LoadExecTool(dir)
	if err != nil {
		t.Fatalf("LoadExecTool: %v", err)
	}

	if tool.Name() != "echo_tool" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "echo_tool")
	}
	if tool.Description() != "echoes its arguments" {
		t.Errorf("Description() = %q", tool.Description())
	}

	args := map[string]any{"msg": "hello"}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// The binary echoes the envelope {"name":"echo_tool","arguments":{...}}.
	got, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if got["name"] != "echo_tool" {
		t.Errorf("result[name] = %v, want echo_tool", got["name"])
	}
	arguments, ok := got["arguments"].(map[string]any)
	if !ok {
		t.Fatalf("result[arguments] type = %T", got["arguments"])
	}
	if arguments["msg"] != "hello" {
		t.Errorf("arguments[msg] = %v, want hello", arguments["msg"])
	}
}

func TestExecTool_MissingBinary(t *testing.T) {
	dir := t.TempDir()

	def := ToolDef{Name: "no_bin", Protocol: "exec"}
	defData, _ := json.Marshal(def)
	_ = os.WriteFile(filepath.Join(dir, "tool.json"), defData, 0644)

	_, err := LoadExecTool(dir)
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestExecTool_MissingToolJSON(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadExecTool(dir)
	if err == nil {
		t.Fatal("expected error for missing tool.json")
	}
}

func TestExecTool_Definition(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script tools not supported on Windows")
	}
	dir := t.TempDir()

	params := map[string]any{"type": "object"}
	def := ToolDef{
		Name:        "test_tool",
		Description: "a test",
		Protocol:    "exec",
		Parameters:  params,
	}
	defData, _ := json.Marshal(def)
	_ = os.WriteFile(filepath.Join(dir, "tool.json"), defData, 0644)
	_ = os.WriteFile(filepath.Join(dir, "test_tool"), []byte("#!/bin/sh\necho '{}'"), 0755)

	tool, err := LoadExecTool(dir)
	if err != nil {
		t.Fatalf("LoadExecTool: %v", err)
	}

	td := tool.Definition()
	if td.Name != "test_tool" {
		t.Errorf("Definition().Name = %q", td.Name)
	}
	if td.Description != "a test" {
		t.Errorf("Definition().Description = %q", td.Description)
	}
}

func TestExecTool_ContextCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script tools not supported on Windows")
	}
	dir := t.TempDir()

	def := ToolDef{Name: "slow_tool", Protocol: "exec"}
	defData, _ := json.Marshal(def)
	_ = os.WriteFile(filepath.Join(dir, "tool.json"), defData, 0644)

	// Sleep forever script.
	_ = os.WriteFile(filepath.Join(dir, "slow_tool"), []byte("#!/bin/sh\nsleep 60\n"), 0755)

	tool, err := LoadExecTool(dir)
	if err != nil {
		t.Fatalf("LoadExecTool: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err = tool.Execute(ctx, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}
