# Multi-Agent Orchestration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable a local LLM orchestrator to coordinate Claude Code and Copilot via Blackboard, with MCP integration for direct tool access and full transcript logging.

**Architecture:** 3-node mesh team (orchestrator/Ollama + claude_code/PTY + copilot/PTY). BB Bridge translates between mesh messages and PTY prompts. MCP server exposes Blackboard tools to both terminals. Transcript logger watches all BB writes and messages.

**Tech Stack:** Go, vt10x PTY, MCP (JSON-RPC stdio), Blackboard, mesh Router

---

## Task 1: Transcript Logger — Test

**File:** `internal/mesh/transcript_test.go`

Write a failing test that verifies the transcript logger captures BB writes and router messages with timestamps.

```go
package mesh

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestTranscriptLogger_BBWrite(t *testing.T) {
	var buf bytes.Buffer
	bb := NewBlackboard()
	router := NewRouter()
	logger := NewTranscriptLogger(&buf, bb, router, "test-team")
	defer logger.Stop()

	bb.Write("plan", "design", "build a URL shortener", "orchestrator")

	// Give watcher time to fire.
	time.Sleep(50 * time.Millisecond)

	out := buf.String()
	if !strings.Contains(out, "BB WRITE plan/design by orchestrator") {
		t.Fatalf("expected BB WRITE log entry, got:\n%s", out)
	}
}

func TestTranscriptLogger_Message(t *testing.T) {
	var buf bytes.Buffer
	bb := NewBlackboard()
	router := NewRouter()
	logger := NewTranscriptLogger(&buf, bb, router, "test-team")
	defer logger.Stop()

	logger.LogMessage(Message{
		From:    "orchestrator",
		To:      "claude_code",
		Type:    "task",
		Content: "Implement the design",
	})

	out := buf.String()
	if !strings.Contains(out, "MSG orchestrator → claude_code") {
		t.Fatalf("expected MSG log entry, got:\n%s", out)
	}
}

func TestTranscriptLogger_TeamLifecycle(t *testing.T) {
	var buf bytes.Buffer
	bb := NewBlackboard()
	router := NewRouter()
	logger := NewTranscriptLogger(&buf, bb, router, "test-team")

	logger.LogStart("Build email validator")
	bb.Write("plan", "design", "regex approach", "orchestrator")
	time.Sleep(50 * time.Millisecond)
	logger.LogComplete(3, 1)

	out := buf.String()
	if !strings.Contains(out, "TEAM test-team STARTED") {
		t.Fatalf("expected STARTED, got:\n%s", out)
	}
	if !strings.Contains(out, "TEAM test-team COMPLETED") {
		t.Fatalf("expected COMPLETED, got:\n%s", out)
	}
}
```

**Run:** `cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestTranscriptLogger -count=1`

**Commit:** `test: add transcript logger tests`

---

## Task 2: Transcript Logger — Implementation

**File:** `internal/mesh/transcript.go`

```go
package mesh

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// TranscriptLogger records BB writes and mesh messages to a writer.
type TranscriptLogger struct {
	w         io.Writer
	mu        sync.Mutex
	bb        *Blackboard
	watcherID WatcherID
	teamID    string
	start     time.Time
	writes    int
}

// NewTranscriptLogger creates a logger that watches bb writes and writes
// formatted transcript lines to w.
func NewTranscriptLogger(w io.Writer, bb *Blackboard, _ *Router, teamID string) *TranscriptLogger {
	tl := &TranscriptLogger{
		w:      w,
		bb:     bb,
		teamID: teamID,
		start:  time.Now(),
	}
	tl.watcherID = bb.Watch(func(key string, val Entry) {
		tl.mu.Lock()
		defer tl.mu.Unlock()
		tl.writes++
		elapsed := time.Since(tl.start)
		// Truncate value for log line.
		v := fmt.Sprintf("%v", val.Value)
		if len(v) > 200 {
			v = v[:200] + "..."
		}
		fmt.Fprintf(tl.w, "[%s] BB WRITE %s by %s rev=%d\n          | %s\n",
			formatElapsed(elapsed), key, val.Author, val.Revision, v)
	})
	return tl
}

// LogMessage records a mesh message to the transcript.
func (tl *TranscriptLogger) LogMessage(msg Message) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	elapsed := time.Since(tl.start)
	content := msg.Content
	if len(content) > 200 {
		content = content[:200] + "..."
	}
	fmt.Fprintf(tl.w, "[%s] MSG %s → %s (%s)\n          | %s\n",
		formatElapsed(elapsed), msg.From, msg.To, msg.Type, content)
}

// LogStart records the team start event.
func (tl *TranscriptLogger) LogStart(task string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.start = time.Now()
	fmt.Fprintf(tl.w, "[00:00.0] TEAM %s STARTED — task: %q\n", tl.teamID, task)
}

// LogComplete records the team completion event.
func (tl *TranscriptLogger) LogComplete(agentCount, _ int) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	elapsed := time.Since(tl.start)
	fmt.Fprintf(tl.w, "[%s] TEAM %s COMPLETED — %s, %d agents, %d BB writes\n",
		formatElapsed(elapsed), tl.teamID, elapsed.Round(100*time.Millisecond), agentCount, tl.writes)
}

// Stop removes the BB watcher.
func (tl *TranscriptLogger) Stop() {
	tl.bb.Unwatch(tl.watcherID)
}

// Writes returns the total number of BB writes observed.
func (tl *TranscriptLogger) Writes() int {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	return tl.writes
}

func formatElapsed(d time.Duration) string {
	totalSec := d.Seconds()
	min := int(totalSec) / 60
	sec := totalSec - float64(min*60)
	return fmt.Sprintf("%02d:%04.1f", min, sec)
}
```

**Run:** `cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestTranscriptLogger -count=1 -v`

**Commit:** `feat: add transcript logger for BB writes and mesh messages`

---

## Task 3: BB Bridge — Test

**File:** `internal/mesh/bb_bridge_test.go`

```go
package mesh

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestBBBridge_FormatPrompt(t *testing.T) {
	bb := NewBlackboard()
	bb.Write("plan", "design", "Build a URL shortener with Go", "orchestrator")

	bridge := &BBBridge{
		agentName: "claude_code",
		role:      "implementation",
		teamMembers: []string{"orchestrator", "copilot"},
		bb:        bb,
	}

	msg := Message{
		From:    "orchestrator",
		To:      "claude_code",
		Type:    "task",
		Content: "Implement the URL shortener based on the design.",
	}

	prompt := bridge.FormatPrompt(msg)

	if !strings.Contains(prompt, "[TEAM CONTEXT]") {
		t.Fatal("missing TEAM CONTEXT header")
	}
	if !strings.Contains(prompt, "claude_code") {
		t.Fatal("missing agent name")
	}
	if !strings.Contains(prompt, "[BLACKBOARD — plan]") {
		t.Fatal("missing blackboard section")
	}
	if !strings.Contains(prompt, "Build a URL shortener") {
		t.Fatal("missing BB content")
	}
	if !strings.Contains(prompt, "[TASK FROM orchestrator]") {
		t.Fatal("missing task header")
	}
	if !strings.Contains(prompt, "[RESULT:") {
		t.Fatal("missing RESULT instruction")
	}
}

func TestBBBridge_ParseResponse(t *testing.T) {
	bb := NewBlackboard()
	var buf bytes.Buffer
	router := NewRouter()
	logger := NewTranscriptLogger(&buf, bb, router, "test")
	defer logger.Stop()

	bridge := &BBBridge{
		agentName:  "claude_code",
		role:       "implementation",
		bb:         bb,
		transcript: logger,
	}

	response := "Here is the implementation...\n[RESULT: implemented URL shortener with tests]"
	bridge.ParseResponse(response)

	// Check artifact was written to BB.
	entries := bb.List("artifacts")
	if entries == nil {
		t.Fatal("expected artifacts section")
	}
	found := false
	for k := range entries {
		if strings.HasPrefix(k, "claude_code/") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected artifact key starting with claude_code/")
	}

	// Check status was written.
	e, ok := bb.Read("status", "claude_code")
	if !ok {
		t.Fatal("expected status entry")
	}
	if e.Value != "done" {
		t.Fatalf("expected status=done, got %v", e.Value)
	}
}

func TestBBBridge_RunLoop(t *testing.T) {
	bb := NewBlackboard()
	bb.Write("plan", "design", "test design", "orchestrator")

	var buf bytes.Buffer
	router := NewRouter()
	logger := NewTranscriptLogger(&buf, bb, router, "test")
	defer logger.Stop()

	inbox := make(chan Message, 1)
	outbox := make(chan Message, 10)

	// Mock PTY sender: just echoes back a result.
	mockSend := func(_ context.Context, prompt string) (string, error) {
		return "Done.\n[RESULT: completed the task]", nil
	}

	bridge := &BBBridge{
		agentName:   "claude_code",
		role:        "implementation",
		teamMembers: []string{"orchestrator"},
		bb:          bb,
		transcript:  logger,
		sendToPTY:   mockSend,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Send a task message.
	inbox <- Message{
		From:    "orchestrator",
		To:      "claude_code",
		Type:    "task",
		Content: "Do the thing",
	}
	close(inbox)

	err := bridge.Run(ctx, "initial task", bb, inbox, outbox)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify outbox got a result message back to orchestrator.
	select {
	case msg := <-outbox:
		if msg.To != "orchestrator" {
			t.Fatalf("expected message to orchestrator, got to=%s", msg.To)
		}
		if !strings.Contains(msg.Content, "completed the task") {
			t.Fatalf("expected result content, got: %s", msg.Content)
		}
	default:
		t.Fatal("expected outbox message")
	}
}
```

**Run:** `cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestBBBridge -count=1`

**Commit:** `test: add BB Bridge tests`

---

## Task 4: BB Bridge — Implementation

**File:** `internal/mesh/bb_bridge.go`

```go
package mesh

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// BBBridge translates mesh messages into PTY prompts and parses responses
// back into BB writes and outgoing messages.
type BBBridge struct {
	agentName   string
	role        string
	teamMembers []string
	bb          *Blackboard
	transcript  *TranscriptLogger
	sendToPTY   func(ctx context.Context, prompt string) (string, error)
}

// NewBBBridge creates a bridge for a PTY-backed agent.
func NewBBBridge(
	agentName, role string,
	teamMembers []string,
	bb *Blackboard,
	transcript *TranscriptLogger,
	sendToPTY func(ctx context.Context, prompt string) (string, error),
) *BBBridge {
	return &BBBridge{
		agentName:   agentName,
		role:        role,
		teamMembers: teamMembers,
		bb:          bb,
		transcript:  transcript,
		sendToPTY:   sendToPTY,
	}
}

// FormatPrompt builds a rich prompt from a mesh message, including BB state.
func (b *BBBridge) FormatPrompt(msg Message) string {
	var sb strings.Builder

	// Team context.
	sb.WriteString("[TEAM CONTEXT]\n")
	sb.WriteString(fmt.Sprintf("You are %q (%s role) in a multi-agent team.\n", b.agentName, b.role))
	sb.WriteString(fmt.Sprintf("The orchestrator is directing you. Other team members: %s\n\n",
		strings.Join(b.teamMembers, ", ")))

	// Inject relevant BB sections.
	for _, section := range b.bb.ListSections() {
		entries := b.bb.List(section)
		if len(entries) == 0 {
			continue
		}
		// Skip internal init entries.
		hasReal := false
		for k, e := range entries {
			if k != "_init" {
				if !hasReal {
					sb.WriteString(fmt.Sprintf("[BLACKBOARD — %s]\n", section))
					hasReal = true
				}
				v := fmt.Sprintf("%v", e.Value)
				if len(v) > 2000 {
					v = v[:2000] + "...(truncated)"
				}
				sb.WriteString(fmt.Sprintf("%s: %s\n", k, v))
			}
		}
		if hasReal {
			sb.WriteString("\n")
		}
	}

	// Task.
	sb.WriteString(fmt.Sprintf("[TASK FROM %s]\n", msg.From))
	sb.WriteString(msg.Content)
	sb.WriteString("\n\nWhen done, end your response with [RESULT: <one-line summary>].\n")

	return sb.String()
}

// ParseResponse extracts result markers and writes artifacts/status to BB.
func (b *BBBridge) ParseResponse(response string) string {
	// Write full response as artifact.
	artifactKey := fmt.Sprintf("%s/%s", b.agentName, uuid.NewString()[:8])
	b.bb.Write("artifacts", artifactKey, response, b.agentName)

	// Extract [RESULT: ...] marker.
	var resultSummary string
	if idx := strings.Index(response, "[RESULT:"); idx >= 0 {
		end := strings.Index(response[idx:], "]")
		if end > 0 {
			resultSummary = strings.TrimSpace(response[idx+8 : idx+end])
		}
		b.bb.Write("status", b.agentName, "done", b.agentName)
	}

	if resultSummary == "" {
		resultSummary = truncate(response, 200)
	}
	return resultSummary
}

// Run is the agent loop for PTY nodes. It processes inbox messages, sends
// prompts to the PTY, parses responses, and writes results to BB/outbox.
func (b *BBBridge) Run(ctx context.Context, _ string, _ *Blackboard, inbox <-chan Message, outbox chan<- Message) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-inbox:
			if !ok {
				return nil
			}
			if b.transcript != nil {
				b.transcript.LogMessage(msg)
			}

			prompt := b.FormatPrompt(msg)
			response, err := b.sendToPTY(ctx, prompt)
			if err != nil {
				return fmt.Errorf("PTY send for %s: %w", b.agentName, err)
			}

			resultSummary := b.ParseResponse(response)

			// Send result back to the sender.
			outMsg := Message{
				ID:        uuid.New().String(),
				From:      b.agentName,
				To:        msg.From,
				Type:      "result",
				Content:   resultSummary,
				Timestamp: time.Now(),
			}
			if b.transcript != nil {
				b.transcript.LogMessage(outMsg)
			}
			select {
			case outbox <- outMsg:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
```

**Run:** `cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestBBBridge -count=1 -v`

**Commit:** `feat: add BB Bridge for PTY agent message translation`

---

## Task 5: BB MCP Server — Test

**File:** `internal/mcp/bb_mcp_test.go`

```go
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/mesh"
)

func TestBBMCPServer_Initialize(t *testing.T) {
	bb := mesh.NewBlackboard()
	var out bytes.Buffer
	srv := NewBBMCPServer(bb)

	req := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}` + "\n"
	in := strings.NewReader(req)

	go srv.Serve(bufio.NewReader(in), &out)

	// Read until we get a response.
	scanner := bufio.NewScanner(&out)
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
```

**Run:** `cd /Users/jon/workspace/ratchet-cli && go test ./internal/mcp/ -run TestBBMCPServer -count=1`

**Commit:** `test: add BB MCP server tests`

---

## Task 6: BB MCP Server — Implementation

**File:** `internal/mcp/bb_mcp.go`

```go
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/mesh"
)

// jsonRPCRequest is a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id,omitempty"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

// BBMCPServer exposes Blackboard operations as MCP tools over stdio.
type BBMCPServer struct {
	bb *mesh.Blackboard
}

// NewBBMCPServer creates an MCP server backed by the given Blackboard.
func NewBBMCPServer(bb *mesh.Blackboard) *BBMCPServer {
	return &BBMCPServer{bb: bb}
}

// Serve reads JSON-RPC requests from r and writes responses to w.
// It blocks until r is exhausted or an error occurs.
func (s *BBMCPServer) Serve(r *bufio.Reader, w io.Writer) error {
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.writeResponse(w, nil, nil, map[string]any{
				"code": -32700, "message": "parse error",
			})
			continue
		}

		result, rpcErr := s.dispatch(req)
		if rpcErr != nil {
			s.writeResponse(w, req.ID, nil, rpcErr)
		} else {
			s.writeResponse(w, req.ID, result, nil)
		}
	}
}

func (s *BBMCPServer) dispatch(req jsonRPCRequest) (any, map[string]any) {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req.Params)
	case "notifications/initialized":
		return nil, nil // no-op notification
	case "tools/list":
		result, err := s.handleToolsList()
		if err != nil {
			return nil, map[string]any{"code": -32603, "message": err.Error()}
		}
		return result, nil
	case "tools/call":
		name, _ := req.Params["name"].(string)
		args, _ := req.Params["arguments"].(map[string]any)
		result, err := s.handleToolCall(name, args)
		if err != nil {
			return nil, map[string]any{"code": -32603, "message": err.Error()}
		}
		return result, nil
	default:
		return nil, map[string]any{"code": -32601, "message": "method not found: " + req.Method}
	}
}

func (s *BBMCPServer) handleInitialize(_ map[string]any) (any, map[string]any) {
	return map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "ratchet-blackboard",
			"version": "1.0.0",
		},
	}, nil
}

func (s *BBMCPServer) handleToolsList() (any, error) {
	tools := []map[string]any{
		{
			"name":        "bb_read",
			"description": "Read a value from the shared Blackboard.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"section": map[string]any{"type": "string", "description": "Blackboard section name"},
					"key":     map[string]any{"type": "string", "description": "Key to read"},
				},
				"required": []string{"section", "key"},
			},
		},
		{
			"name":        "bb_write",
			"description": "Write a value to the shared Blackboard.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"section": map[string]any{"type": "string", "description": "Blackboard section name"},
					"key":     map[string]any{"type": "string", "description": "Key to write"},
					"value":   map[string]any{"type": "string", "description": "Value to store"},
				},
				"required": []string{"section", "key", "value"},
			},
		},
		{
			"name":        "bb_list",
			"description": "List Blackboard sections, or keys within a section.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"section": map[string]any{"type": "string", "description": "Section name (optional — omit to list all sections)"},
				},
			},
		},
	}
	return map[string]any{"tools": tools}, nil
}

func (s *BBMCPServer) handleToolCall(name string, args map[string]any) (any, error) {
	switch name {
	case "bb_read":
		return s.toolRead(args)
	case "bb_write":
		return s.toolWrite(args)
	case "bb_list":
		return s.toolList(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func (s *BBMCPServer) toolRead(args map[string]any) (any, error) {
	section, _ := args["section"].(string)
	key, _ := args["key"].(string)
	if section == "" || key == "" {
		return nil, fmt.Errorf("section and key are required")
	}
	e, ok := s.bb.Read(section, key)
	if !ok {
		return mcpTextResult("not found"), nil
	}
	return mcpTextResult(fmt.Sprintf("%v", e.Value)), nil
}

func (s *BBMCPServer) toolWrite(args map[string]any) (any, error) {
	section, _ := args["section"].(string)
	key, _ := args["key"].(string)
	value, _ := args["value"].(string)
	if section == "" || key == "" {
		return nil, fmt.Errorf("section and key are required")
	}
	e := s.bb.Write(section, key, value, "mcp-client")
	return mcpTextResult(fmt.Sprintf("written (revision %d)", e.Revision)), nil
}

func (s *BBMCPServer) toolList(args map[string]any) (any, error) {
	section, _ := args["section"].(string)
	if section == "" {
		sections := s.bb.ListSections()
		return mcpTextResult(strings.Join(sections, ", ")), nil
	}
	entries := s.bb.List(section)
	if entries == nil {
		return mcpTextResult("section not found"), nil
	}
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	return mcpTextResult(strings.Join(keys, ", ")), nil
}

func mcpTextResult(text string) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
	}
}

func (s *BBMCPServer) writeResponse(w io.Writer, id any, result any, rpcErr any) {
	resp := jsonRPCResponse{JSONRPC: "2.0", ID: id}
	if rpcErr != nil {
		resp.Error = rpcErr
	} else {
		resp.Result = result
	}
	data, _ := json.Marshal(resp)
	fmt.Fprintf(w, "%s\n", data)
}
```

**Run:** `cd /Users/jon/workspace/ratchet-cli && go test ./internal/mcp/ -run TestBBMCPServer -count=1 -v`

**Commit:** `feat: add BB MCP server exposing bb_read/bb_write/bb_list over stdio`

---

## Task 7: BB MCP Command — `ratchet mcp blackboard`

**File:** `cmd/ratchet/cmd_mcp.go` (new)

```go
package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/GoCodeAlone/ratchet-cli/internal/mcp"
	"github.com/GoCodeAlone/ratchet-cli/internal/mesh"
)

func handleMCP(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet mcp <blackboard> [flags]")
		return
	}

	switch args[0] {
	case "blackboard":
		handleMCPBlackboard(args[1:])
	default:
		fmt.Printf("unknown mcp command: %s\n", args[0])
	}
}

func handleMCPBlackboard(_ []string) {
	// For now, create a standalone Blackboard instance.
	// TODO: connect to daemon's shared Blackboard via Unix socket when
	// team-id flag is implemented.
	bb := mesh.NewBlackboard()

	srv := mcp.NewBBMCPServer(bb)
	if err := srv.Serve(bufio.NewReader(os.Stdin), os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "mcp server error: %v\n", err)
		os.Exit(1)
	}
}
```

**File:** `cmd/ratchet/main.go` — Add `mcp` case to command switch.

Add this case after the `"acp"` case in the switch statement:

```go
	case "mcp":
		handleMCP(filteredArgs[1:])
```

**Run:** `cd /Users/jon/workspace/ratchet-cli && go build ./cmd/ratchet/`

**Commit:** `feat: add ratchet mcp blackboard command`

---

## Task 8: MCP Config Management — Test

**File:** `internal/mcp/config_test.go`

```go
package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteClaudeCodeMCPConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".claude", "mcp.json")

	err := WriteMCPConfig(configPath, "ratchet-blackboard", MCPServerEntry{
		Command: "ratchet",
		Args:    []string{"mcp", "blackboard", "--team-id", "team-1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config ClaudeCodeMCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	entry, ok := config.MCPServers["ratchet-blackboard"]
	if !ok {
		t.Fatal("missing ratchet-blackboard entry")
	}
	if entry.Command != "ratchet" {
		t.Fatalf("expected command=ratchet, got %s", entry.Command)
	}
}

func TestWriteClaudeCodeMCPConfig_MergeExisting(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".claude", "mcp.json")

	// Write an existing config.
	os.MkdirAll(filepath.Dir(configPath), 0o755)
	existing := `{"mcpServers":{"existing-server":{"command":"existing","args":[]}}}`
	os.WriteFile(configPath, []byte(existing), 0o644)

	err := WriteMCPConfig(configPath, "ratchet-blackboard", MCPServerEntry{
		Command: "ratchet",
		Args:    []string{"mcp", "blackboard"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(configPath)
	var config ClaudeCodeMCPConfig
	json.Unmarshal(data, &config)

	if _, ok := config.MCPServers["existing-server"]; !ok {
		t.Fatal("existing server was clobbered")
	}
	if _, ok := config.MCPServers["ratchet-blackboard"]; !ok {
		t.Fatal("ratchet-blackboard not added")
	}
}

func TestRemoveMCPConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".claude", "mcp.json")

	// Write config with two entries.
	os.MkdirAll(filepath.Dir(configPath), 0o755)
	config := ClaudeCodeMCPConfig{
		MCPServers: map[string]MCPServerEntry{
			"ratchet-blackboard": {Command: "ratchet"},
			"other-server":      {Command: "other"},
		},
	}
	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(configPath, data, 0o644)

	err := RemoveMCPConfig(configPath, "ratchet-blackboard")
	if err != nil {
		t.Fatal(err)
	}

	data, _ = os.ReadFile(configPath)
	var updated ClaudeCodeMCPConfig
	json.Unmarshal(data, &updated)

	if _, ok := updated.MCPServers["ratchet-blackboard"]; ok {
		t.Fatal("ratchet-blackboard should have been removed")
	}
	if _, ok := updated.MCPServers["other-server"]; !ok {
		t.Fatal("other-server should still exist")
	}
}

func TestBackupRestore(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp.json")
	os.WriteFile(configPath, []byte(`{"original":true}`), 0o644)

	backupPath, err := BackupConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}

	// Overwrite original.
	os.WriteFile(configPath, []byte(`{"modified":true}`), 0o644)

	err = RestoreConfig(configPath, backupPath)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(configPath)
	if string(data) != `{"original":true}` {
		t.Fatalf("expected original content, got: %s", data)
	}
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatal("backup should have been removed after restore")
	}
}
```

**Run:** `cd /Users/jon/workspace/ratchet-cli && go test ./internal/mcp/ -run "TestWrite|TestRemove|TestBackup" -count=1`

**Commit:** `test: add MCP config management tests`

---

## Task 9: MCP Config Management — Implementation

**File:** `internal/mcp/config.go`

```go
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MCPServerEntry describes a single MCP server in a config file.
type MCPServerEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// ClaudeCodeMCPConfig is the structure of .claude/mcp.json.
type ClaudeCodeMCPConfig struct {
	MCPServers map[string]MCPServerEntry `json:"mcpServers"`
}

// CopilotMCPConfig is the structure of ~/.copilot/mcp-config.json.
type CopilotMCPConfig struct {
	Servers map[string]MCPServerEntry `json:"servers"`
}

// WriteMCPConfig merges a server entry into a Claude Code-format MCP config file.
// Creates the file and parent directories if they don't exist.
func WriteMCPConfig(path, serverName string, entry MCPServerEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	var config ClaudeCodeMCPConfig
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &config)
	}
	if config.MCPServers == nil {
		config.MCPServers = make(map[string]MCPServerEntry)
	}
	config.MCPServers[serverName] = entry

	return writeJSON(path, config)
}

// WriteCopilotMCPConfig merges a server entry into a Copilot-format MCP config file.
func WriteCopilotMCPConfig(path, serverName string, entry MCPServerEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	var config CopilotMCPConfig
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &config)
	}
	if config.Servers == nil {
		config.Servers = make(map[string]MCPServerEntry)
	}
	config.Servers[serverName] = entry

	return writeJSON(path, config)
}

// RemoveMCPConfig removes a server entry from a Claude Code-format MCP config.
func RemoveMCPConfig(path, serverName string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var config ClaudeCodeMCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	delete(config.MCPServers, serverName)
	return writeJSON(path, config)
}

// RemoveCopilotMCPConfig removes a server entry from a Copilot-format MCP config.
func RemoveCopilotMCPConfig(path, serverName string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var config CopilotMCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	delete(config.Servers, serverName)
	return writeJSON(path, config)
}

// BackupConfig copies the file at path to path.bak and returns the backup path.
// Returns ("", nil) if the file does not exist.
func BackupConfig(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	backupPath := path + ".ratchet-bak"
	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		return "", err
	}
	return backupPath, nil
}

// RestoreConfig restores the backup to the original path and removes the backup.
func RestoreConfig(path, backupPath string) error {
	if backupPath == "" {
		// No backup means the file didn't exist before — remove it.
		os.Remove(path)
		return nil
	}
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	return os.Remove(backupPath)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
```

**Run:** `cd /Users/jon/workspace/ratchet-cli && go test ./internal/mcp/ -run "TestWrite|TestRemove|TestBackup" -count=1 -v`

**Commit:** `feat: add MCP config write/backup/restore for Claude Code and Copilot`

---

## Task 10: LocalNode PTY Detection — Test

**File:** `internal/mesh/local_node_pty_test.go`

```go
package mesh

import (
	"testing"
)

func TestIsPTYProvider(t *testing.T) {
	tests := []struct {
		provider string
		want     bool
	}{
		{"claude_code", true},
		{"copilot_cli", true},
		{"ollama", false},
		{"anthropic", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsPTYProvider(tt.provider); got != tt.want {
			t.Errorf("IsPTYProvider(%q) = %v, want %v", tt.provider, got, tt.want)
		}
	}
}
```

**Run:** `cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestIsPTYProvider -count=1`

**Commit:** `test: add PTY provider detection test`

---

## Task 11: LocalNode PTY Detection — Implementation

**File:** `internal/mesh/pty_detect.go` (new)

```go
package mesh

// ptyProviders is the set of provider names that use PTY sessions
// and should be routed through BBBridge instead of executor.Execute.
var ptyProviders = map[string]bool{
	"claude_code": true,
	"copilot_cli": true,
	"codex_cli":   true,
	"gemini_cli":  true,
	"cursor_cli":  true,
}

// IsPTYProvider returns true if the named provider requires a PTY session.
func IsPTYProvider(provider string) bool {
	return ptyProviders[provider]
}
```

**Run:** `cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestIsPTYProvider -count=1 -v`

**Commit:** `feat: add PTY provider detection for mesh nodes`

---

## Task 12: Orchestrate Team Config

**File:** `internal/mesh/teams/orchestrate.yaml` (new)

```yaml
name: orchestrate
timeout: 15m
max_review_rounds: 2
agents:
  - name: orchestrator
    role: orchestrator
    provider: ollama
    model: qwen3:8b
    max_iterations: 30
    tools: [blackboard_read, blackboard_write, blackboard_list, send_message]
    system_prompt: |
      You are the orchestrator of a 3-agent team. Your team members are:
      - "claude_code": An AI coding assistant. Strong at implementation.
      - "copilot": An AI coding assistant. Strong at code review.

      Your workflow:
      1. Analyze the task and design the approach
      2. Write your design to blackboard section "plan" key "design"
      3. Send implementation task to "claude_code" via send_message
      4. Wait — read blackboard "artifacts" to check for claude_code's output
      5. Send review task to "copilot" via send_message with the code
      6. Read blackboard "reviews" for copilot's feedback
      7. If changes needed, send feedback to "claude_code"
      8. When satisfied, write summary to blackboard "artifacts" key "final"
      9. Write "done" to blackboard "status" key "orchestrator"

      Rules:
      - Always specify "to" in send_message. Never broadcast.
      - Read blackboard before sending new tasks (check agent status).
      - Keep messages focused and specific.

  - name: claude_code
    role: implementation
    provider: claude_code
    max_iterations: 5
    tools: [blackboard_read, blackboard_write]

  - name: copilot
    role: review
    provider: copilot_cli
    max_iterations: 5
    tools: [blackboard_read, blackboard_write]
```

**File:** `internal/mesh/config.go` — Register orchestrate as a builtin.

Add the embed directive and update `BuiltinTeamConfigs()`:

```go
//go:embed teams/orchestrate.yaml
var defaultOrchestrateTeam []byte

// DefaultOrchestrateTeamConfig returns the built-in orchestrate team configuration.
func DefaultOrchestrateTeamConfig() (*TeamConfig, error) {
	var tc TeamConfig
	if err := yaml.Unmarshal(defaultOrchestrateTeam, &tc); err != nil {
		return nil, err
	}
	return &tc, nil
}
```

Update `BuiltinTeamConfigs()` to include orchestrate:

```go
func BuiltinTeamConfigs() (map[string]*TeamConfig, error) {
	tc, err := DefaultCodeGenTeamConfig()
	if err != nil {
		return nil, err
	}
	ot, err := DefaultOrchestrateTeamConfig()
	if err != nil {
		return nil, err
	}
	return map[string]*TeamConfig{
		"code-gen":    tc,
		"orchestrate": ot,
	}, nil
}
```

Also update `knownTools` in config.go's `ValidateTeamConfig` to be a no-op for PTY providers (they get tools via MCP, not mesh tool registry). Actually, the existing validation will fail because PTY nodes still declare `blackboard_read`/`blackboard_write` which ARE known tools, so no change needed there.

**Run:** `cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestValidateTeamConfig -count=1 && go build ./cmd/ratchet/`

**Commit:** `feat: add orchestrate team config with PTY agents`

---

## Task 13: Integration Test — Mock Providers

**File:** `internal/mesh/orchestration_test.go`

```go
package mesh

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// mockPTYSender returns a canned response for testing.
func mockPTYSender(name string) func(context.Context, string) (string, error) {
	return func(_ context.Context, prompt string) (string, error) {
		switch name {
		case "claude_code":
			return "package main\n\nfunc validate(email string) bool { return true }\n\n[RESULT: implemented email validator]", nil
		case "copilot":
			return "Code looks good, no issues found.\n\n[RESULT: approved with no changes]", nil
		default:
			return "[RESULT: done]", nil
		}
	}
}

func TestOrchestration_EndToEnd(t *testing.T) {
	bb := NewBlackboard()
	// Seed BB sections like SpawnTeam does.
	for _, section := range []string{"plan", "code", "reviews", "status", "artifacts"} {
		bb.Write(section, "_init", "empty", "mesh")
	}

	router := NewRouter()
	var transcript bytes.Buffer
	logger := NewTranscriptLogger(&transcript, bb, router, "test-orchestration")
	defer logger.Stop()
	logger.LogStart("Build email validator")

	// Simulate orchestrator writing plan.
	bb.Write("plan", "design", "Go function using regex to validate emails", "orchestrator")

	// Create BB Bridges for both PTY agents.
	claudeBridge := NewBBBridge(
		"claude_code", "implementation",
		[]string{"orchestrator", "copilot"},
		bb, logger,
		mockPTYSender("claude_code"),
	)
	copilotBridge := NewBBBridge(
		"copilot", "review",
		[]string{"orchestrator", "claude_code"},
		bb, logger,
		mockPTYSender("copilot"),
	)

	// Wire up inboxes/outboxes.
	claudeInbox := make(chan Message, 10)
	claudeOutbox := make(chan Message, 10)
	copilotInbox := make(chan Message, 10)
	copilotOutbox := make(chan Message, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start bridges in goroutines.
	errCh := make(chan error, 2)
	go func() { errCh <- claudeBridge.Run(ctx, "", bb, claudeInbox, claudeOutbox) }()
	go func() { errCh <- copilotBridge.Run(ctx, "", bb, copilotInbox, copilotOutbox) }()

	// Simulate orchestrator sending task to claude_code.
	claudeInbox <- Message{
		From:    "orchestrator",
		To:      "claude_code",
		Type:    "task",
		Content: "Implement the email validator based on the design in BB plan/design.",
	}

	// Wait for claude_code's response.
	select {
	case msg := <-claudeOutbox:
		if !strings.Contains(msg.Content, "implemented email validator") {
			t.Fatalf("unexpected claude_code result: %s", msg.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for claude_code response")
	}

	// Verify artifact written.
	artifacts := bb.List("artifacts")
	foundClaudeArtifact := false
	for k := range artifacts {
		if strings.HasPrefix(k, "claude_code/") {
			foundClaudeArtifact = true
		}
	}
	if !foundClaudeArtifact {
		t.Fatal("claude_code artifact not found in BB")
	}

	// Simulate orchestrator sending review task to copilot.
	copilotInbox <- Message{
		From:    "orchestrator",
		To:      "copilot",
		Type:    "task",
		Content: "Review the code in BB artifacts for correctness.",
	}

	select {
	case msg := <-copilotOutbox:
		if !strings.Contains(msg.Content, "approved") {
			t.Fatalf("unexpected copilot result: %s", msg.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for copilot response")
	}

	// Verify status entries.
	if e, ok := bb.Read("status", "claude_code"); !ok || e.Value != "done" {
		t.Fatal("claude_code status not set to done")
	}
	if e, ok := bb.Read("status", "copilot"); !ok || e.Value != "done" {
		t.Fatal("copilot status not set to done")
	}

	// Close inboxes to end bridges.
	close(claudeInbox)
	close(copilotInbox)

	for range 2 {
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("bridge error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for bridges to exit")
		}
	}

	logger.LogComplete(3, logger.Writes())

	// Verify transcript has key events.
	out := transcript.String()
	if !strings.Contains(out, "TEAM test-orchestration STARTED") {
		t.Error("transcript missing STARTED")
	}
	if !strings.Contains(out, "BB WRITE plan/design") {
		t.Error("transcript missing BB WRITE for plan")
	}
	if !strings.Contains(out, "MSG orchestrator → claude_code") {
		t.Error("transcript missing MSG to claude_code")
	}
	if !strings.Contains(out, "TEAM test-orchestration COMPLETED") {
		t.Error("transcript missing COMPLETED")
	}
}
```

**Run:** `cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestOrchestration_EndToEnd -count=1 -v`

**Commit:** `test: add end-to-end orchestration integration test`

---

## Task 14: Manual Test Instructions

**File:** Update this plan with manual test instructions (no code change).

### Manual Testing

**Prerequisites:**
- Ollama running with `qwen3:8b` pulled: `ollama pull qwen3:8b`
- Claude Code installed and authenticated
- Copilot CLI installed and authenticated (optional — can test with Claude Code only)
- `ratchet` built: `cd /Users/jon/workspace/ratchet-cli && go build -o ratchet ./cmd/ratchet/`

**Step 1: Verify MCP server starts**
```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./ratchet mcp blackboard
```
Expected: JSON response with `protocolVersion` and `serverInfo`.

**Step 2: Verify team config loads**
```bash
./ratchet team list
```
Expected: Shows both `code-gen` and `orchestrate` configs.

**Step 3: Run the orchestration team (when PTY wiring is complete)**
```bash
./ratchet team start orchestrate --task "Build a Go function that validates email addresses with regex and unit tests"
```

**Step 4: Check transcript**
```bash
cat ~/.ratchet/transcripts/<team-id>.log
```

**Step 5: Verify MCP config was written/cleaned up**
```bash
cat .claude/mcp.json  # Should have ratchet-blackboard during run
# After team completes, ratchet-blackboard entry should be removed
```

**Commit:** `docs: add manual test instructions to orchestration plan`

---

## Summary of Files

| # | File | Action | Description |
|---|------|--------|-------------|
| 1 | `internal/mesh/transcript_test.go` | New | Transcript logger tests |
| 2 | `internal/mesh/transcript.go` | New | Transcript logger implementation |
| 3 | `internal/mesh/bb_bridge_test.go` | New | BB Bridge tests |
| 4 | `internal/mesh/bb_bridge.go` | New | BB Bridge implementation |
| 5 | `internal/mcp/bb_mcp_test.go` | New | BB MCP server tests |
| 6 | `internal/mcp/bb_mcp.go` | New | BB MCP server (stdio JSON-RPC) |
| 7 | `cmd/ratchet/cmd_mcp.go` | New | `ratchet mcp blackboard` command |
| 7 | `cmd/ratchet/main.go` | Modify | Add `mcp` case to switch |
| 8 | `internal/mcp/config_test.go` | New | MCP config management tests |
| 9 | `internal/mcp/config.go` | New | MCP config write/backup/restore |
| 10 | `internal/mesh/local_node_pty_test.go` | New | PTY detection test |
| 11 | `internal/mesh/pty_detect.go` | New | PTY provider detection |
| 12 | `internal/mesh/teams/orchestrate.yaml` | New | Orchestrate team config |
| 12 | `internal/mesh/config.go` | Modify | Register orchestrate builtin |
| 13 | `internal/mesh/orchestration_test.go` | New | End-to-end integration test |
