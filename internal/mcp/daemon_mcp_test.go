package mcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

type fakeDaemon struct {
	sessions []*pb.Session
	projects []*pb.ProjectStatus
	teams    []*pb.TeamStatus
	killed   string
	bb       map[string]map[string]*pb.BlackboardEntry
	msgErr   error
	msgTeam  string
	msgAgent string
	msgText  string
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

func (f *fakeDaemon) ReadBlackboard(section, key string) (*pb.BlackboardReadResp, error) {
	if f.bb == nil || f.bb[section] == nil || f.bb[section][key] == nil {
		return &pb.BlackboardReadResp{}, nil
	}
	return &pb.BlackboardReadResp{Found: true, Entry: f.bb[section][key]}, nil
}

func (f *fakeDaemon) WriteBlackboard(section, key, value string) (*pb.BlackboardEntry, error) {
	if f.bb == nil {
		f.bb = make(map[string]map[string]*pb.BlackboardEntry)
	}
	if f.bb[section] == nil {
		f.bb[section] = make(map[string]*pb.BlackboardEntry)
	}
	entry := &pb.BlackboardEntry{Section: section, Key: key, Value: value, Author: "mcp-client", Revision: 1}
	f.bb[section][key] = entry
	return entry, nil
}

func (f *fakeDaemon) ListBlackboard(section string) (*pb.BlackboardListResp, error) {
	if section == "" {
		resp := &pb.BlackboardListResp{}
		for sec := range f.bb {
			resp.Sections = append(resp.Sections, sec)
		}
		return resp, nil
	}
	resp := &pb.BlackboardListResp{}
	for _, entry := range f.bb[section] {
		resp.Entries = append(resp.Entries, entry)
	}
	return resp, nil
}

func (f *fakeDaemon) ListTeams() ([]*pb.TeamStatus, error) {
	return f.teams, nil
}

func (f *fakeDaemon) GetTeamStatus(teamID string) (*pb.TeamStatus, error) {
	for _, team := range f.teams {
		if team.TeamId == teamID {
			return team, nil
		}
	}
	return &pb.TeamStatus{TeamId: teamID, Status: "unknown"}, nil
}

func (f *fakeDaemon) DirectMessage(teamID, toAgent, content string) error {
	f.msgTeam = teamID
	f.msgAgent = toAgent
	f.msgText = content
	return f.msgErr
}

func TestDaemonMCPToolsListIncludesSessionAndProjectTools(t *testing.T) {
	srv := NewDaemonMCPServer(&fakeDaemon{})
	resp := runMCPSequence(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)

	tools := resp[0]["result"].(map[string]any)["tools"].([]any)
	names := make(map[string]bool, len(tools))
	var teamMessage map[string]any
	for _, tool := range tools {
		item := tool.(map[string]any)
		name := item["name"].(string)
		names[name] = true
		if name == "team_message" {
			teamMessage = item
		}
	}
	for _, want := range []string{"bb_read", "bb_write", "bb_list", "session_list", "session_kill", "project_list", "team_list", "team_status", "team_message"} {
		if !names[want] {
			t.Fatalf("tools/list missing %s: %#v", want, names)
		}
	}
	required := teamMessage["inputSchema"].(map[string]any)["required"].([]any)
	for _, field := range required {
		if field == "from_agent" {
			t.Fatal("team_message must not require unsupported from_agent field")
		}
	}
}

func TestDaemonMCPToolCallsUseDaemonClient(t *testing.T) {
	fake := &fakeDaemon{
		sessions: []*pb.Session{{Id: "s1", Status: "active", WorkingDir: "/tmp/project"}},
		projects: []*pb.ProjectStatus{{Id: "p1", Name: "demo", Status: "running"}},
		teams:    []*pb.TeamStatus{{TeamId: "t1", Status: "running", Agents: []*pb.Agent{{Id: "a1"}}}},
	}
	srv := NewDaemonMCPServer(fake)
	resp := runMCPSequence(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"session_list","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"session_kill","arguments":{"id":"s1"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"project_list","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"bb_write","arguments":{"section":"plan","key":"status","value":"ready"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"bb_read","arguments":{"section":"plan","key":"status"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"team_list","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"team_status","arguments":{"team_id":"t1"}}}`,
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
	if got := resultText(t, resp[4]); got != "ready" {
		t.Fatalf("bb_read text = %q, want ready", got)
	}
	if got := resultText(t, resp[5]); !strings.Contains(got, "t1") {
		t.Fatalf("team_list text = %q", got)
	}
	if got := resultText(t, resp[6]); !strings.Contains(got, "t1") || !strings.Contains(got, "running") {
		t.Fatalf("team_status text = %q", got)
	}
}

func TestDaemonMCPTeamMessageSurfacesDaemonError(t *testing.T) {
	srv := NewDaemonMCPServer(&fakeDaemon{msgErr: errors.New("DirectMessage not yet implemented")})
	resp := runMCPSequence(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"team_message","arguments":{"team_id":"t1","to_agent":"worker","content":"hello"}}}`,
	)

	errObj, ok := resp[0]["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error response, got %#v", resp[0])
	}
	if got := errObj["message"].(string); !strings.Contains(got, "DirectMessage not yet implemented") {
		t.Fatalf("error message = %q", got)
	}
}

func TestDaemonMCPTeamMessageSendsViaDaemon(t *testing.T) {
	fake := &fakeDaemon{}
	srv := NewDaemonMCPServer(fake)
	resp := runMCPSequence(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"team_message","arguments":{"team_id":"t1","to_agent":"worker","content":"hello"}}}`,
	)

	if got := resultText(t, resp[0]); got != "sent" {
		t.Fatalf("team_message text = %q, want sent", got)
	}
	if fake.msgTeam != "t1" || fake.msgAgent != "worker" || fake.msgText != "hello" {
		t.Fatalf("DirectMessage call = team=%q agent=%q text=%q", fake.msgTeam, fake.msgAgent, fake.msgText)
	}
}

func TestDaemonMCPTeamMessageValidatesRequiredArgs(t *testing.T) {
	cases := []string{
		`{"to_agent":"worker","content":"hello"}`,
		`{"team_id":"t1","content":"hello"}`,
		`{"team_id":"t1","to_agent":"worker"}`,
	}
	for _, args := range cases {
		srv := NewDaemonMCPServer(&fakeDaemon{})
		resp := runMCPSequence(t, srv,
			`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"team_message","arguments":`+args+`}}`,
		)
		errObj, ok := resp[0]["error"].(map[string]any)
		if !ok {
			t.Fatalf("expected error response for args %s, got %#v", args, resp[0])
		}
		if got := errObj["message"].(string); !strings.Contains(got, "team_id, to_agent, and content are required") {
			t.Fatalf("error message = %q", got)
		}
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
