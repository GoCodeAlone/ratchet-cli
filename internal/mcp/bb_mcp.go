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
