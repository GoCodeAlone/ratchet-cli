package mcp

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

type fakeDaemon struct {
	sessions []*pb.Session
	projects []*pb.ProjectStatus
	killed   string
}

func (f *fakeDaemon) ListSessions() ([]*pb.Session, error) {
	return f.sessions, nil
}

func (f *fakeDaemon) KillSession(id string) error {
	f.killed = id
	return nil
}

func (f *fakeDaemon) ListProjects() ([]*pb.ProjectStatus, error) {
	return f.projects, nil
}

func TestDaemonMCPToolsListIncludesSessionAndProjectTools(t *testing.T) {
	srv := NewDaemonMCPServer(&fakeDaemon{})
	resp := runMCPSequence(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)

	tools := resp[0]["result"].(map[string]any)["tools"].([]any)
	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		names[tool.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"session_list", "session_kill", "project_list"} {
		if !names[want] {
			t.Fatalf("tools/list missing %s: %#v", want, names)
		}
	}
}

func TestDaemonMCPToolCallsUseDaemonClient(t *testing.T) {
	fake := &fakeDaemon{
		sessions: []*pb.Session{{Id: "s1", Status: "active", WorkingDir: "/tmp/project"}},
		projects: []*pb.ProjectStatus{{Id: "p1", Name: "demo", Status: "running"}},
	}
	srv := NewDaemonMCPServer(fake)
	resp := runMCPSequence(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"session_list","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"session_kill","arguments":{"id":"s1"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"project_list","arguments":{}}}`,
	)

	if got := resultText(t, resp[0]); !strings.Contains(got, "s1") || !strings.Contains(got, "/tmp/project") {
		t.Fatalf("session_list text = %q", got)
	}
	if fake.killed != "s1" {
		t.Fatalf("killed session = %q, want s1", fake.killed)
	}
	if got := resultText(t, resp[2]); !strings.Contains(got, "p1") || !strings.Contains(got, "demo") {
		t.Fatalf("project_list text = %q", got)
	}
}

func runMCPSequence(t *testing.T, srv *BBMCPServer, requests ...string) []map[string]any {
	t.Helper()
	input := strings.Join(append(requests, ""), "\n")
	pr, pw := io.Pipe()
	go func() {
		_ = srv.Serve(bufio.NewReader(strings.NewReader(input)), pw)
		_ = pw.Close()
	}()

	var responses []map[string]any
	scanner := bufio.NewScanner(pr)
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
