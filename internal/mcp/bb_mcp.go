package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/GoCodeAlone/ratchet-cli/internal/mesh"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
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
	bb     *mesh.Blackboard
	daemon DaemonClient
}

// DaemonClient is the daemon surface exposed as MCP tools.
type DaemonClient interface {
	ListSessions() ([]*pb.Session, error)
	KillSession(id string) error
	ListProjects() ([]*pb.ProjectStatus, error)
	ReadBlackboard(section, key string) (*pb.BlackboardReadResp, error)
	WriteBlackboard(section, key, value string) (*pb.BlackboardEntry, error)
	ListBlackboard(section string) (*pb.BlackboardListResp, error)
	ListTeams() ([]*pb.TeamStatus, error)
	GetTeamStatus(teamID string) (*pb.TeamStatus, error)
	DirectMessage(teamID, toAgent, content string) error
}

// NewBBMCPServer creates an MCP server backed by the given Blackboard.
func NewBBMCPServer(bb *mesh.Blackboard) *BBMCPServer {
	return &BBMCPServer{bb: bb}
}

// NewDaemonMCPServer creates an MCP server backed by a running ratchet daemon.
func NewDaemonMCPServer(daemon DaemonClient) *BBMCPServer {
	return &BBMCPServer{daemon: daemon}
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
	var tools []map[string]any
	if s.bb != nil || s.daemon != nil {
		tools = append(tools,
			map[string]any{
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
			map[string]any{
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
			map[string]any{
				"name":        "bb_list",
				"description": "List Blackboard sections, or keys within a section.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"section": map[string]any{"type": "string", "description": "Section name (optional — omit to list all sections)"},
					},
				},
			},
		)
	}
	if s.daemon != nil {
		tools = append(tools,
			map[string]any{
				"name":        "session_list",
				"description": "List sessions from the ratchet daemon.",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
			},
			map[string]any{
				"name":        "session_kill",
				"description": "Mark a ratchet daemon session completed.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]any{"type": "string", "description": "Session ID"},
					},
					"required": []string{"id"},
				},
			},
			map[string]any{
				"name":        "project_list",
				"description": "List projects from the ratchet daemon.",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
			},
			map[string]any{
				"name":        "team_list",
				"description": "List teams from the ratchet daemon.",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
			},
			map[string]any{
				"name":        "team_status",
				"description": "Get a team status from the ratchet daemon.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"team_id": map[string]any{"type": "string", "description": "Team ID or name"},
					},
					"required": []string{"team_id"},
				},
			},
			map[string]any{
				"name":        "team_message",
				"description": "Send a direct message to a team agent when the daemon supports it. Sender identity is not part of the current daemon RPC.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"team_id":  map[string]any{"type": "string", "description": "Team ID or name"},
						"to_agent": map[string]any{"type": "string", "description": "Recipient agent name"},
						"content":  map[string]any{"type": "string", "description": "Message content"},
					},
					"required": []string{"team_id", "to_agent", "content"},
				},
			},
		)
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
	case "session_list":
		return s.toolSessionList()
	case "session_kill":
		return s.toolSessionKill(args)
	case "project_list":
		return s.toolProjectList()
	case "team_list":
		return s.toolTeamList()
	case "team_status":
		return s.toolTeamStatus(args)
	case "team_message":
		return s.toolTeamMessage(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func (s *BBMCPServer) toolRead(args map[string]any) (any, error) {
	if s.bb == nil && s.daemon == nil {
		return nil, fmt.Errorf("blackboard tools are not enabled")
	}
	section, _ := args["section"].(string)
	key, _ := args["key"].(string)
	if section == "" || key == "" {
		return nil, fmt.Errorf("section and key are required")
	}
	if s.daemon != nil {
		resp, err := s.daemon.ReadBlackboard(section, key)
		if err != nil {
			return nil, err
		}
		if !resp.Found {
			return mcpTextResult("not found"), nil
		}
		return mcpTextResult(resp.Entry.Value), nil
	}
	e, ok := s.bb.Read(section, key)
	if !ok {
		return mcpTextResult("not found"), nil
	}
	return mcpTextResult(fmt.Sprintf("%v", e.Value)), nil
}

func (s *BBMCPServer) toolWrite(args map[string]any) (any, error) {
	if s.bb == nil && s.daemon == nil {
		return nil, fmt.Errorf("blackboard tools are not enabled")
	}
	section, _ := args["section"].(string)
	key, _ := args["key"].(string)
	value, _ := args["value"].(string)
	if section == "" || key == "" {
		return nil, fmt.Errorf("section and key are required")
	}
	if s.daemon != nil {
		e, err := s.daemon.WriteBlackboard(section, key, value)
		if err != nil {
			return nil, err
		}
		return mcpTextResult(fmt.Sprintf("written (revision %d)", e.Revision)), nil
	}
	e := s.bb.Write(section, key, value, "mcp-client")
	return mcpTextResult(fmt.Sprintf("written (revision %d)", e.Revision)), nil
}

func (s *BBMCPServer) toolList(args map[string]any) (any, error) {
	if s.bb == nil && s.daemon == nil {
		return nil, fmt.Errorf("blackboard tools are not enabled")
	}
	section, _ := args["section"].(string)
	if s.daemon != nil {
		resp, err := s.daemon.ListBlackboard(section)
		if err != nil {
			return nil, err
		}
		if section == "" {
			sort.Strings(resp.Sections)
			return mcpTextResult(strings.Join(resp.Sections, ", ")), nil
		}
		keys := make([]string, 0, len(resp.Entries))
		for _, entry := range resp.Entries {
			keys = append(keys, entry.Key)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			return mcpTextResult("section not found"), nil
		}
		return mcpTextResult(strings.Join(keys, ", ")), nil
	}
	if section == "" {
		sections := s.bb.ListSections()
		sort.Strings(sections)
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
	sort.Strings(keys)
	return mcpTextResult(strings.Join(keys, ", ")), nil
}

func (s *BBMCPServer) toolSessionList() (any, error) {
	if s.daemon == nil {
		return nil, fmt.Errorf("daemon tools are not enabled")
	}
	sessions, err := s.daemon.ListSessions()
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return mcpTextResult("No sessions."), nil
	}
	var b strings.Builder
	for _, session := range sessions {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\n", session.Id, session.Status, session.Provider, session.WorkingDir)
	}
	return mcpTextResult(strings.TrimSpace(b.String())), nil
}

func (s *BBMCPServer) toolSessionKill(args map[string]any) (any, error) {
	if s.daemon == nil {
		return nil, fmt.Errorf("daemon tools are not enabled")
	}
	id, _ := args["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	if err := s.daemon.KillSession(id); err != nil {
		return nil, err
	}
	return mcpTextResult("killed " + id), nil
}

func (s *BBMCPServer) toolProjectList() (any, error) {
	if s.daemon == nil {
		return nil, fmt.Errorf("daemon tools are not enabled")
	}
	projects, err := s.daemon.ListProjects()
	if err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		return mcpTextResult("No projects."), nil
	}
	var b strings.Builder
	for _, project := range projects {
		fmt.Fprintf(&b, "%s\t%s\t%s\n", project.Id, project.Name, project.Status)
	}
	return mcpTextResult(strings.TrimSpace(b.String())), nil
}

func (s *BBMCPServer) toolTeamList() (any, error) {
	if s.daemon == nil {
		return nil, fmt.Errorf("daemon tools are not enabled")
	}
	teams, err := s.daemon.ListTeams()
	if err != nil {
		return nil, err
	}
	if len(teams) == 0 {
		return mcpTextResult("No teams."), nil
	}
	var b strings.Builder
	for _, team := range teams {
		fmt.Fprintf(&b, "%s\t%s\t%d\n", team.TeamId, team.Status, len(team.Agents))
	}
	return mcpTextResult(strings.TrimSpace(b.String())), nil
}

func (s *BBMCPServer) toolTeamStatus(args map[string]any) (any, error) {
	if s.daemon == nil {
		return nil, fmt.Errorf("daemon tools are not enabled")
	}
	teamID, _ := args["team_id"].(string)
	if teamID == "" {
		return nil, fmt.Errorf("team_id is required")
	}
	team, err := s.daemon.GetTeamStatus(teamID)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\t%s\t%d agents", team.TeamId, team.Status, len(team.Agents))
	return mcpTextResult(b.String()), nil
}

func (s *BBMCPServer) toolTeamMessage(args map[string]any) (any, error) {
	if s.daemon == nil {
		return nil, fmt.Errorf("daemon tools are not enabled")
	}
	teamID, _ := args["team_id"].(string)
	toAgent, _ := args["to_agent"].(string)
	content, _ := args["content"].(string)
	if teamID == "" || toAgent == "" || content == "" {
		return nil, fmt.Errorf("team_id, to_agent, and content are required")
	}
	if err := s.daemon.DirectMessage(teamID, toAgent, content); err != nil {
		return nil, err
	}
	return mcpTextResult("sent"), nil
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
