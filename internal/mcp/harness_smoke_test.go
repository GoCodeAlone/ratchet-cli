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
	result := resp["result"].(map[string]any)
	content := result["content"].([]any)
	first := content[0].(map[string]any)
	return first["text"].(string)
}
