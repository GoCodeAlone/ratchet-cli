package acpclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

var ErrInvalidFlowDefinition = errors.New("invalid acp client flow definition")

const (
	FlowNodeTypeACP     = "acp"
	FlowNodeTypeCompute = "compute"

	FlowRunStatusRunning   = "running"
	FlowRunStatusCompleted = "completed"
	FlowRunStatusFailed    = "failed"

	FlowStepStatusCompleted = "completed"
	FlowStepStatusFailed    = "failed"
)

type FlowDefinition struct {
	FormatVersion int        `json:"format_version"`
	Name          string     `json:"name,omitempty"`
	StartAt       string     `json:"start_at"`
	Nodes         []FlowNode `json:"nodes"`
	Edges         []FlowEdge `json:"edges,omitempty"`
}

type FlowNode struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Prompt  string          `json:"prompt,omitempty"`
	Agent   string          `json:"agent,omitempty"`
	Command string          `json:"command,omitempty"`
	Args    []string        `json:"args,omitempty"`
	Session string          `json:"session,omitempty"`
	Value   json.RawMessage `json:"value,omitempty"`
	Select  string          `json:"select,omitempty"`
}

type FlowEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type FlowRunState struct {
	RunID  string           `json:"run_id"`
	Status string           `json:"status"`
	Steps  []FlowStepRecord `json:"steps"`
}

type FlowStepRecord struct {
	NodeID string          `json:"node_id"`
	Type   string          `json:"type,omitempty"`
	Status string          `json:"status"`
	Output json.RawMessage `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
}

func LoadFlowDefinition(path string) (FlowDefinition, error) {
	var def FlowDefinition
	b, err := os.ReadFile(path)
	if err != nil {
		return FlowDefinition{}, err
	}
	if err := json.Unmarshal(b, &def); err != nil {
		return FlowDefinition{}, fmt.Errorf("%w: %v", ErrInvalidFlowDefinition, err)
	}
	if err := def.Validate(); err != nil {
		return FlowDefinition{}, err
	}
	return def, nil
}

func (def FlowDefinition) Validate() error {
	if def.FormatVersion != 1 {
		return fmt.Errorf("%w: format_version must be 1", ErrInvalidFlowDefinition)
	}
	if def.StartAt == "" {
		return fmt.Errorf("%w: start_at is required", ErrInvalidFlowDefinition)
	}
	if len(def.Nodes) == 0 {
		return fmt.Errorf("%w: at least one node is required", ErrInvalidFlowDefinition)
	}
	nodes := make(map[string]FlowNode, len(def.Nodes))
	for _, node := range def.Nodes {
		if node.ID == "" {
			return fmt.Errorf("%w: node id is required", ErrInvalidFlowDefinition)
		}
		if _, exists := nodes[node.ID]; exists {
			return fmt.Errorf("%w: duplicate node %s", ErrInvalidFlowDefinition, node.ID)
		}
		switch node.Type {
		case FlowNodeTypeACP:
			if node.Prompt == "" {
				return fmt.Errorf("%w: acp node %s prompt is required", ErrInvalidFlowDefinition, node.ID)
			}
		case FlowNodeTypeCompute:
			if len(node.Value) == 0 && node.Select == "" {
				return fmt.Errorf("%w: compute node %s value or select is required", ErrInvalidFlowDefinition, node.ID)
			}
		default:
			return fmt.Errorf("%w: unknown node type %s", ErrInvalidFlowDefinition, node.Type)
		}
		nodes[node.ID] = node
	}
	if _, ok := nodes[def.StartAt]; !ok {
		return fmt.Errorf("%w: start_at node %s does not exist", ErrInvalidFlowDefinition, def.StartAt)
	}
	graph := make(map[string][]string, len(nodes))
	for _, edge := range def.Edges {
		if edge.From == "" || edge.To == "" {
			return fmt.Errorf("%w: edge endpoints are required", ErrInvalidFlowDefinition)
		}
		if _, ok := nodes[edge.From]; !ok {
			return fmt.Errorf("%w: edge from node %s does not exist", ErrInvalidFlowDefinition, edge.From)
		}
		if _, ok := nodes[edge.To]; !ok {
			return fmt.Errorf("%w: edge to node %s does not exist", ErrInvalidFlowDefinition, edge.To)
		}
		graph[edge.From] = append(graph[edge.From], edge.To)
	}
	if hasFlowCycle(def.StartAt, graph, map[string]bool{}, map[string]bool{}) {
		return fmt.Errorf("%w: graph contains cycle", ErrInvalidFlowDefinition)
	}
	return nil
}

func hasFlowCycle(node string, graph map[string][]string, visiting, visited map[string]bool) bool {
	if visiting[node] {
		return true
	}
	if visited[node] {
		return false
	}
	visiting[node] = true
	for _, next := range graph[node] {
		if hasFlowCycle(next, graph, visiting, visited) {
			return true
		}
	}
	visiting[node] = false
	visited[node] = true
	return false
}
