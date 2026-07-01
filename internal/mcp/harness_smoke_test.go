package mcp

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/mesh"
)

func TestHarnessSmokeJSONRPCInitializeToolsListAndCall(t *testing.T) {
	srv := NewBBMCPServer(mesh.NewBlackboard())
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"bb_write","arguments":{"section":"smoke","key":"status","value":"ok"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"bb_read","arguments":{"section":"smoke","key":"status"}}}`,
		"",
	}, "\n")

	pr, pw := io.Pipe()
	go func() {
		_ = srv.Serve(bufio.NewReader(strings.NewReader(input)), pw)
		_ = pw.Close()
	}()

	responses := readJSONRPCResponses(t, pr)
	if len(responses) != 4 {
		t.Fatalf("got %d responses, want 4: %#v", len(responses), responses)
	}
	if resultText(t, responses[3]) != "ok" {
		t.Fatalf("read response text = %q, want ok", resultText(t, responses[3]))
	}
}

func readJSONRPCResponses(t *testing.T, r io.Reader) []map[string]any {
	t.Helper()
	var responses []map[string]any
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		var resp map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response %q: %v", scanner.Text(), err)
		}
		responses = append(responses, resp)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan responses: %v", err)
	}
	return responses
}

func resultText(t *testing.T, resp map[string]any) string {
	t.Helper()
	if rpcErr, ok := resp["error"]; ok && rpcErr != nil {
		t.Fatalf("JSON-RPC error response: %#v", rpcErr)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result has type %T, want object: %#v", resp["result"], resp)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("content has type %T length %d, want non-empty array: %#v", result["content"], len(content), result)
	}
	first, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("first content item has type %T, want object: %#v", content[0], content[0])
	}
	text, ok := first["text"].(string)
	if !ok {
		t.Fatalf("text has type %T, want string: %#v", first["text"], first)
	}
	return text
}
